package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var (
	formsCreateScopeFormsBody = []string{"https://www.googleapis.com/auth/forms.body"}
	formsCreateScopeDriveFile = []string{"https://www.googleapis.com/auth/drive.file"}
)

// TestSignedNativeFormsCreateBlankOnlyTitleWithoutResourceGrant proves the
// fixed forms_create_blank tool issues exactly one POST /v1/forms with an
// info.title-only body under either write scope, denies missing tool/write/
// scope grants, unsupported parent/description/publish/template arguments,
// malformed titles and foreign tenants before Google, and never turns the
// created form into an automatic read grant.
func TestSignedNativeFormsCreateBlankOnlyTitleWithoutResourceGrant(t *testing.T) {
	const (
		createdFormID = "new-blank-form"
		createdTitle  = "Quarterly plan"
	)

	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Forms = func(ctx context.Context, account string) (*forms.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted execution identity was lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodPost || r.URL.Hostname() != "forms.googleapis.com" || r.URL.Path != "/v1/forms" {
					t.Errorf("unexpected native Forms create request: %s %s", r.Method, r.URL)
				}

				var formBody struct {
					Info *struct {
						Title string `json:"title"`
					} `json:"info"`
				}
				decoder := json.NewDecoder(r.Body)
				decoder.DisallowUnknownFields()

				if err := decoder.Decode(&formBody); err != nil {
					t.Errorf("unexpected Forms create body: %v", err)
				}

				if formBody.Info == nil || formBody.Info.Title != createdTitle {
					t.Errorf("Forms title mismatch: %+v", formBody.Info)
				}

				response := `{"formId":"` + createdFormID + `","info":{"title":"` + createdTitle + `"},"responderUri":"https://docs.google.com/forms/d/e/` + createdFormID + `/viewform"}`

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
			})

			return forms.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(args map[string]any, scopes []string, allowedTools []string, allowWrite bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{
				Name:      "forms_create_blank",
				Arguments: args,
				Authorization: ToolAuthorization{
					Service:      "forms",
					AllowedTools: allowedTools,
					Scopes:       scopes,
					AllowWrite:   allowWrite,
				},
			}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	created := call(map[string]any{"title": createdTitle}, formsCreateScopeFormsBody, []string{"forms_create_blank"}, true)
	if !created.OK || calls.Load() != 1 {
		t.Fatalf("native blank create failed: %s", created.Body)
	}

	if !strings.Contains(string(created.Result), createdFormID) {
		t.Fatalf("create result missing form id: %s", created.Result)
	}

	driveFile := call(map[string]any{"title": createdTitle}, formsCreateScopeDriveFile, []string{"forms_create_blank"}, true)
	if !driveFile.OK || calls.Load() != 2 {
		t.Fatalf("native blank create under drive.file failed: %s", driveFile.Body)
	}

	before := calls.Load()

	denials := []struct {
		label  string
		args   map[string]any
		scopes []string
		tools  []string
		write  bool
	}{
		{label: "missing tool grant", args: map[string]any{"title": createdTitle}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_get"}, write: true},
		{label: "missing write permission", args: map[string]any{"title": createdTitle}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: false},
		{label: "missing write scope", args: map[string]any{"title": createdTitle}, scopes: []string{"https://www.googleapis.com/auth/forms.body.readonly"}, tools: []string{"forms_create_blank"}, write: true},
		{label: "unknown parent argument", args: map[string]any{"title": createdTitle, "parent": "folder-1"}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "description argument", args: map[string]any{"title": createdTitle, "description": "extra text"}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "publish argument", args: map[string]any{"title": createdTitle, "publish": true}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "template argument", args: map[string]any{"title": createdTitle, "template": "template-1"}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "unicode control title", args: map[string]any{"title": "Quarterly\u0085plan"}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "whitespace title", args: map[string]any{"title": "   "}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "empty title", args: map[string]any{"title": ""}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "oversized title", args: map[string]any{"title": strings.Repeat("a", 513)}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
		{label: "control character title", args: map[string]any{"title": "Quarterly\x07plan"}, scopes: formsCreateScopeFormsBody, tools: []string{"forms_create_blank"}, write: true},
	}
	for _, denial := range denials {
		result := call(denial.args, denial.scopes, denial.tools, denial.write)
		if result.OK || calls.Load() != before {
			t.Fatalf("%s was not denied before Google: %s", denial.label, result.Body)
		}
	}

	foreignBody := nativeToolBody(t, func(req *Request) {
		req.TenantID = "tenant-b"
		req.Tool = &ToolInvocation{
			Name:      "forms_create_blank",
			Arguments: map[string]any{"title": createdTitle},
			Authorization: ToolAuthorization{
				Service:      "forms",
				AllowedTools: []string{"forms_create_blank"},
				Scopes:       formsCreateScopeFormsBody,
				AllowWrite:   true,
			},
		}
	})

	foreign := postExecute(t, harness.server, signInvocation(t, foreignBody, func(claims map[string]any) { claims["operation"] = OperationToolCall }), foreignBody)
	if foreign.OK || calls.Load() != before {
		t.Fatalf("foreign tenant invocation was not denied before Google: %s", foreign.Body)
	}

	readBody := nativeToolBody(t, func(req *Request) {
		req.Tool = &ToolInvocation{
			Name:      "forms_get",
			Arguments: map[string]any{"form_id": createdFormID},
			Authorization: ToolAuthorization{
				Service:      "forms",
				AllowedTools: []string{"forms_get"},
				Scopes:       []string{"https://www.googleapis.com/auth/drive.file"},
			},
		}
	})

	read := postExecute(t, harness.server, signInvocation(t, readBody, func(claims map[string]any) { claims["operation"] = OperationToolCall }), readBody)
	if read.OK || calls.Load() != before {
		t.Fatalf("created form became read authority without a grant: %s", read.Body)
	}
}

func TestFormsBlankCreateGuardRejectsOtherEndpointsAndMethods(t *testing.T) {
	guard := nativeRequestGuard(&ToolInvocation{
		Name: "forms_create_blank", Arguments: map[string]any{"title": "Blank"},
		Authorization: ToolAuthorization{Service: "forms", AllowedTools: []string{"forms_create_blank"}, Scopes: formsCreateScopeFormsBody, AllowWrite: true},
	})

	for _, entry := range []struct {
		method  string
		url     string
		allowed bool
	}{
		{http.MethodPost, "https://forms.googleapis.com/v1/forms", true},
		{http.MethodGet, "https://forms.googleapis.com/v1/forms", false},
		{http.MethodPatch, "https://forms.googleapis.com/v1/forms", false},
		{http.MethodPost, "http://forms.googleapis.com/v1/forms", false},
		{http.MethodPost, "https://forms.googleapis.com/v1/forms/foreign", false},
		{http.MethodPost, "https://forms.googleapis.com/v1/forms/foreign:batchUpdate", false},
		{http.MethodPost, "https://forms.googleapis.com/v1/forms/foreign:setPublishSettings", false},
		{http.MethodPost, "https://forms.googleapis.com/v1/forms?access_token=foreign", false},
	} {
		parsed, err := url.Parse(entry.url)
		if err != nil {
			t.Fatal(err)
		}

		allowed := guard(&http.Request{Method: entry.method, URL: parsed, Header: http.Header{}}) == nil
		if allowed != entry.allowed {
			t.Errorf("%s %s: allowed=%v, want %v", entry.method, entry.url, allowed, entry.allowed)
		}
	}
}
