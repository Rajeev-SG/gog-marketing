package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var docsSlidesCreateScopeDriveFile = []string{"https://www.googleapis.com/auth/drive.file"}

// TestSignedNativeDocsSlidesCreateBlankOnlyTitleWithoutResourceGrant proves
// that docs_create_blank and slides_create_blank each issue exactly one
// POST https://www.googleapis.com/drive/v3/files with a name + fixed MIME
// body under the drive.file scope, deny missing tool/write/scope grants,
// documents/presentations-only scopes, unsupported parent/template/file/
// pageless arguments, malformed titles and foreign tenants before Google,
// and never turn the created file into an automatic read grant.
func TestSignedNativeDocsSlidesCreateBlankOnlyTitleWithoutResourceGrant(t *testing.T) {
	const (
		createdDocID  = "new-blank-doc"
		createdDeckID = "new-blank-deck"
		docTitle      = "Quarterly memo"
		deckTitle     = "Board deck"
		docMime       = "application/vnd.google-apps.document"
		deckMime      = "application/vnd.google-apps.presentation"
	)

	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices = app.Services{Drive: func(ctx context.Context, account string) (*drive.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted execution identity was lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodPost || r.URL.Hostname() != "www.googleapis.com" || r.URL.Path != "/drive/v3/files" {
					t.Errorf("unexpected native Drive create request: %s %s", r.Method, r.URL)
				}

				if r.URL.Query().Get("access_token") != "" {
					t.Errorf("create request leaked access token in query: %s", r.URL.RawQuery)
				}

				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}

				if len(body) != 2 {
					t.Errorf("create body carried fields beyond name and mimeType: %v", body)
				}

				if _, ok := body["parents"]; ok {
					t.Error("create body must not carry parents")
				}

				var name, mimeType string
				if err := json.Unmarshal(body["name"], &name); err != nil {
					t.Error(err)
				}

				if err := json.Unmarshal(body["mimeType"], &mimeType); err != nil {
					t.Error(err)
				}

				var id, webViewLink string

				switch mimeType {
				case docMime:
					id, webViewLink = createdDocID, "https://docs.google.com/document/d/"+createdDocID+"/edit"
				case deckMime:
					id, webViewLink = createdDeckID, "https://docs.google.com/presentation/d/"+createdDeckID+"/edit"
				default:
					t.Fatalf("unexpected create mimeType: %q", mimeType)
				}

				response := fmt.Sprintf(`{"id":%q,"name":%q,"mimeType":%q,"webViewLink":%q}`, id, name, mimeType, webViewLink)

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
			})

			return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}}
	})

	call := func(service, tool string, args map[string]any, scopes []string, allowedTools []string, allowWrite bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{
				Name:      tool,
				Arguments: args,
				Authorization: ToolAuthorization{
					Service:      service,
					AllowedTools: allowedTools,
					Scopes:       scopes,
					AllowWrite:   allowWrite,
				},
			}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	docs := call("docs", "docs_create_blank", map[string]any{"title": docTitle}, docsSlidesCreateScopeDriveFile, []string{"docs_create_blank"}, true)
	if !docs.OK || calls.Load() != 1 {
		t.Fatalf("native docs blank create failed: %s", docs.Body)
	}

	for _, want := range []string{createdDocID, docTitle, docMime, "https://docs.google.com/document/d/" + createdDocID + "/edit"} {
		if !strings.Contains(string(docs.Result), want) {
			t.Fatalf("docs create result missing %q: %s", want, docs.Result)
		}
	}

	deck := call("slides", "slides_create_blank", map[string]any{"title": deckTitle}, docsSlidesCreateScopeDriveFile, []string{"slides_create_blank"}, true)
	if !deck.OK || calls.Load() != 2 {
		t.Fatalf("native slides blank create failed: %s", deck.Body)
	}

	for _, want := range []string{createdDeckID, deckTitle, deckMime, "https://docs.google.com/presentation/d/" + createdDeckID + "/edit"} {
		if !strings.Contains(string(deck.Result), want) {
			t.Fatalf("slides create result missing %q: %s", want, deck.Result)
		}
	}

	before := calls.Load()

	denials := []struct {
		label   string
		service string
		tool    string
		args    map[string]any
		scopes  []string
		tools   []string
		write   bool
	}{
		{label: "missing tool grant", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"drive_get"}, write: true},
		{label: "missing write permission", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: false},
		{label: "missing write scope", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle}, scopes: []string{"https://www.googleapis.com/auth/drive.readonly"}, tools: []string{"docs_create_blank"}, write: true},
		{label: "documents-only scope", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle}, scopes: []string{"https://www.googleapis.com/auth/documents"}, tools: []string{"docs_create_blank"}, write: true},
		{label: "presentations-only scope", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle}, scopes: []string{"https://www.googleapis.com/auth/presentations"}, tools: []string{"docs_create_blank"}, write: true},
		{label: "parent argument", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle, "parent": "folder-1"}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "file argument", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle, "file": "notes.md"}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "pageless argument", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": docTitle, "pageless": true}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "template argument", service: "slides", tool: "slides_create_blank", args: map[string]any{"title": deckTitle, "template": "template-1"}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"slides_create_blank"}, write: true},
		{label: "parent argument (slides)", service: "slides", tool: "slides_create_blank", args: map[string]any{"title": deckTitle, "parent": "folder-1"}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"slides_create_blank"}, write: true},
		{label: "unicode control title", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": "Quarterly\u0085plan"}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "whitespace title", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": "   "}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "empty title", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": ""}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "oversized title", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": strings.Repeat("a", 513)}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
		{label: "control character title", service: "docs", tool: "docs_create_blank", args: map[string]any{"title": "Quarterly\x07plan"}, scopes: docsSlidesCreateScopeDriveFile, tools: []string{"docs_create_blank"}, write: true},
	}
	for _, denial := range denials {
		result := call(denial.service, denial.tool, denial.args, denial.scopes, denial.tools, denial.write)
		if result.OK || calls.Load() != before {
			t.Fatalf("%s was not denied before Google: %s", denial.label, result.Body)
		}
	}

	for _, tenantCase := range []struct{ service, tool, title string }{
		{"docs", "docs_create_blank", docTitle},
		{"slides", "slides_create_blank", deckTitle},
	} {
		foreignBody := nativeToolBody(t, func(req *Request) {
			req.TenantID = "tenant-b"
			req.Tool = &ToolInvocation{
				Name:      tenantCase.tool,
				Arguments: map[string]any{"title": tenantCase.title},
				Authorization: ToolAuthorization{
					Service:      tenantCase.service,
					AllowedTools: []string{tenantCase.tool},
					Scopes:       docsSlidesCreateScopeDriveFile,
					AllowWrite:   true,
				},
			}
		})

		foreign := postExecute(t, harness.server, signInvocation(t, foreignBody, func(claims map[string]any) { claims["operation"] = OperationToolCall }), foreignBody)
		if foreign.OK || calls.Load() != before {
			t.Fatalf("foreign tenant %s invocation was not denied before Google: %s", tenantCase.tool, foreign.Body)
		}
	}

	// The returned id is not authority: following drive_get on the created
	// file without a resource grant must be denied before Google.
	for _, follow := range []struct{ id, label string }{
		{createdDocID, "created document"},
		{createdDeckID, "created presentation"},
	} {
		readBody := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{
				Name:      "drive_get",
				Arguments: map[string]any{"file_id": follow.id},
				Authorization: ToolAuthorization{
					Service:      "drive",
					AllowedTools: []string{"drive_get"},
					Scopes:       docsSlidesCreateScopeDriveFile,
				},
			}
		})

		read := postExecute(t, harness.server, signInvocation(t, readBody, func(claims map[string]any) { claims["operation"] = OperationToolCall }), readBody)
		if read.OK || calls.Load() != before {
			t.Fatalf("%s became read authority without a grant: %s", follow.label, read.Body)
		}
	}
}

// TestDocsSlidesBlankCreateGuardRejectsOtherEndpointsAndMethods pins the
// request guard for both blank-creation tools to exactly POST
// https://www.googleapis.com/drive/v3/files.
func TestDocsSlidesBlankCreateGuardRejectsOtherEndpointsAndMethods(t *testing.T) {
	for _, tool := range []struct{ name, service string }{
		{"docs_create_blank", "docs"},
		{"slides_create_blank", "slides"},
	} {
		guard := nativeRequestGuard(&ToolInvocation{
			Name: tool.name, Arguments: map[string]any{"title": "Blank"},
			Authorization: ToolAuthorization{Service: tool.service, AllowedTools: []string{tool.name}, Scopes: docsSlidesCreateScopeDriveFile, AllowWrite: true},
		})

		for _, entry := range []struct {
			method  string
			url     string
			allowed bool
		}{
			{http.MethodPost, "https://www.googleapis.com/drive/v3/files", true},
			{http.MethodGet, "https://www.googleapis.com/drive/v3/files", false},
			{http.MethodPatch, "https://www.googleapis.com/drive/v3/files", false},
			{http.MethodPost, "http://www.googleapis.com/drive/v3/files", false},
			{http.MethodPost, "https://www.googleapis.com/drive/v3/files/new-blank-doc", false},
			{http.MethodPost, "https://www.googleapis.com/upload/drive/v3/files", false},
			{http.MethodPost, "https://www.googleapis.com/drive/v3/files?access_token=foreign", false},
		} {
			parsed, err := url.Parse(entry.url)
			if err != nil {
				t.Fatal(err)
			}

			allowed := guard(&http.Request{Method: entry.method, URL: parsed, Header: http.Header{}}) == nil
			if allowed != entry.allowed {
				t.Errorf("%s %s %s: allowed=%v, want %v", tool.name, entry.method, entry.url, allowed, entry.allowed)
			}
		}
	}
}
