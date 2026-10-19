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

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	slides "google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const slidesReadFixture = `{
  "presentationId":"selected-deck",
  "title":"Launch deck",
  "locale":"en-US",
  "revisionId":"rev-1",
  "slides":[
    {
      "objectId":"p:0",
      "slideProperties":{"notesPage":{"pageElements":[{
        "objectId":"notes:body",
        "shape":{"placeholder":{"type":"BODY"},"text":{"textElements":[
          {"startIndex":0,"endIndex":5,"textRun":{"content":"BODY\n"}}
        ]}}
      }]}},
      "pageElements":[{
        "objectId":"shape:one",
        "size":{"width":{"magnitude":100,"unit":"PT"},"height":{"magnitude":40,"unit":"PT"}},
        "transform":{"scaleX":1,"scaleY":1,"translateX":10,"translateY":20,"unit":"PT"},
        "shape":{"shapeType":"TEXT_BOX","text":{"textElements":[
          {"startIndex":1,"endIndex":11,"textRun":{"content":"Launch 🚀 ","style":{"bold":true}}},
          {"startIndex":11,"endIndex":17,"textRun":{"content":"launch"}}
        ]}}
      }]
    },
    {"objectId":"p:1","pageElements":[{"objectId":"second:text","shape":{"text":{"textElements":[
      {"startIndex":1,"endIndex":7,"textRun":{"content":"Launch"}}
    ]}}}]}
  ]
}`

func TestNativeSlidesReadSignedExecution(t *testing.T) {
	var driveCalls, slidesCalls atomic.Int64
	var fixtureMode atomic.Value
	fixtureMode.Store("normal")

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Errorf("mutating Slides/Drive request: %s %s", r.Method, r.URL)
			}

			query := r.URL.Query()
			if query.Get("alt") != "json" || query.Get("prettyPrint") != "false" {
				t.Errorf("native defaults missing: %s", r.URL.RawQuery)
			}

			var typed any

			switch r.URL.Hostname() {
			case "www.googleapis.com":
				if r.URL.Path != "/drive/v3/files/selected-deck" {
					t.Errorf("foreign Drive request: %s", r.URL)
				}

				if got := query.Get("fields"); got == "" {
					t.Errorf("Drive fields query was empty")
				}

				driveCalls.Add(1)

				var file drive.File
				payload := `{"id":"selected-deck","name":"Launch deck","mimeType":"application/vnd.google-apps.presentation"}`

				switch fixtureMode.Load().(string) {
				case "wrong_mime":
					payload = `{"id":"selected-deck","name":"Folder","mimeType":"application/vnd.google-apps.folder"}`
				case "foreign_id":
					payload = `{"id":"foreign-deck","name":"Foreign deck","mimeType":"application/vnd.google-apps.presentation"}`
				}

				if err := json.Unmarshal([]byte(payload), &file); err != nil {
					t.Fatalf("decode Drive fixture into drive.File: %v", err)
				}

				if file.Id != "selected-deck" && fixtureMode.Load().(string) == "normal" {
					t.Fatalf("Drive fixture id = %q", file.Id)
				}
				typed = &file
			case "slides.googleapis.com":
				if r.URL.Path != "/v1/presentations/selected-deck" {
					t.Errorf("foreign Slides request: %s", r.URL)
				}

				if len(query) != 2 {
					t.Errorf("unexpected Slides query: %s", r.URL.RawQuery)
				}

				slidesCalls.Add(1)

				var deck slides.Presentation
				if err := json.Unmarshal([]byte(slidesReadFixture), &deck); err != nil {
					t.Fatalf("decode fixture into slides.Presentation: %v", err)
				}

				if deck.PresentationId != "selected-deck" || len(deck.Slides) != 2 {
					t.Fatalf("decoded fixture deck = %#v", deck)
				}
				shape := deck.Slides[0].PageElements[0].Shape

				runText := shape.Text.TextElements[0].TextRun.Content + shape.Text.TextElements[1].TextRun.Content
				if runText != "Launch 🚀 launch" || shape.Text.TextElements[0].TextRun.Content != "Launch 🚀 " {
					t.Fatalf("decoded split-run text = %#v", shape.Text)
				}
				typed = &deck
			default:
				t.Errorf("foreign Google host: %s", r.URL)
			}

			payload, _ := json.Marshal(typed)

			return &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(string(payload))), Request: r,
			}, nil
		})

		engine.NativeServices = app.Services{
			Drive: func(ctx context.Context, account string) (*drive.Service, error) {
				if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
					t.Error("trusted Drive execution identity was lost")
				}

				return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
			},
			Slides: func(ctx context.Context, account string) (*slides.Service, error) {
				if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
					t.Error("trusted Slides execution identity was lost")
				}

				return slides.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
			},
		}
	})

	resource := &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}
	call := func(tool string, args map[string]any, scope string, allowedTool string) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{
				Name: tool, Arguments: args,
				Authorization: ToolAuthorization{
					Service: "slides", AllowedTools: []string{allowedTool},
					Scopes:   []string{"https://www.googleapis.com/auth/" + scope},
					Resource: resource,
				},
			}
		})

		var probe Request
		if err := json.Unmarshal(body, &probe); err == nil {
			t.Logf("policy %s: %v", tool, validateToolInvocation(probe.Tool))
		}

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
			claims["operation"] = OperationToolCall
		}), body)
	}
	requireNonEmpty := func(response capturedResponse) map[string]any {
		t.Helper()

		if !response.OK {
			t.Fatalf("native call failed: %s", response.Body)
		}

		var result map[string]any
		if err := json.Unmarshal(response.Result, &result); err != nil || len(result) == 0 {
			t.Fatalf("empty or invalid result %q: %v", response.Result, err)
		}

		return result
	}

	infoArgs := map[string]any{"presentation_id": "selected-deck"}
	readArgs := map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}
	locateArgs := map[string]any{"presentation_id": "selected-deck", "text": "launch"}

	read := call("slides_read_slide", readArgs, "presentations.readonly", "slides_read_slide")

	readResult := requireNonEmpty(read)
	if readResult["slideObjectId"] != "p:0" || readResult["notes"] != "BODY" {
		t.Fatalf("wrong slide/notes output: %#v", readResult)
	}
	infoDriveBefore, infoSlidesBefore := driveCalls.Load(), slidesCalls.Load()
	info := call("slides_info", infoArgs, "drive.file", "slides_info")

	infoResult := requireNonEmpty(info)
	if infoResult["file"] == nil || infoResult["presentation"] == nil {
		t.Fatalf("info output lacked Drive and Slides objects: %#v", infoResult)
	}

	if driveCalls.Load()-infoDriveBefore != 1 || slidesCalls.Load()-infoSlidesBefore != 1 {
		t.Fatalf("info calls = Drive %d/Slides %d, want 1/1", driveCalls.Load(), slidesCalls.Load())
	}

	for _, scope := range []string{"presentations.readonly", "presentations", "drive.file"} {
		read := call("slides_read_slide", readArgs, scope, "slides_read_slide")
		requireNonEmpty(read)
		located := call("slides_locate", locateArgs, scope, "slides_locate")
		requireNonEmpty(located)
	}

	detail := call("slides_read_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "detail": true}, "presentations", "slides_read_slide")

	detailResult := requireNonEmpty(detail)
	if detailResult["elements"] == nil {
		t.Fatalf("detail omitted elements: %#v", detailResult)
	}

	all := call("slides_locate", map[string]any{"presentation_id": "selected-deck", "text": "launch", "all": true}, "presentations.readonly", "slides_locate")

	allResult := requireNonEmpty(all)
	if matches, _ := allResult["matches"].([]any); len(matches) != 3 {
		t.Fatalf("whole-presentation all matches = %#v, want 3", allResult["matches"])
	}
	occurrence := call("slides_locate", map[string]any{"presentation_id": "selected-deck", "text": "launch", "occurrence": float64(2)}, "presentations.readonly", "slides_locate")

	occurrenceResult := requireNonEmpty(occurrence)
	if matches, _ := occurrenceResult["matches"].([]any); len(matches) != 1 {
		t.Fatalf("occurrence output = %#v, want one match", occurrenceResult)
	}

	second := occurrenceResult["matches"].([]any)[0].(map[string]any)
	if second["startIndex"] != float64(11) || second["endIndex"] != float64(17) || second["slideObjectId"] != "p:0" {
		t.Fatalf("wrong second occurrence: %#v", second)
	}
	matchCase := call("slides_locate", map[string]any{"presentation_id": "selected-deck", "text": "launch", "match_case": true}, "drive.file", "slides_locate")

	caseResult := requireNonEmpty(matchCase)
	if matches, _ := caseResult["matches"].([]any); len(matches) != 1 {
		t.Fatalf("case-sensitive matches: %#v", caseResult)
	}
	page := call("slides_locate", map[string]any{"presentation_id": "selected-deck", "text": "launch", "page_id": "p:0", "all": true}, "drive.file", "slides_locate")

	pageResult := requireNonEmpty(page)
	if matches, _ := pageResult["matches"].([]any); len(matches) != 2 {
		t.Fatalf("selected-page matches: %#v", pageResult)
	}
	utf16 := call("slides_locate", map[string]any{"presentation_id": "selected-deck", "text": "🚀 launch"}, "drive.file", "slides_locate")

	utf16Result := requireNonEmpty(utf16)
	if matches, _ := utf16Result["matches"].([]any); len(matches) != 1 {
		t.Fatalf("UTF-16 emoji output = %#v", utf16Result)
	}

	span := utf16Result["matches"].([]any)[0].(map[string]any)
	if span["startIndex"] != float64(8) || span["endIndex"] != float64(17) {
		t.Fatalf("wrong UTF16 split-run span: %#v", span)
	}

	beforeDrive, beforeSlides := driveCalls.Load(), slidesCalls.Load()

	denials := []struct {
		name        string
		tool        string
		args        map[string]any
		scope       string
		allowed     string
		resource    *ResourceGrantInput
		fixtureMode string
		tenant      string
	}{
		{name: "presentations scope cannot drive", tool: "slides_info", args: infoArgs, scope: "presentations.readonly", allowed: "slides_info", resource: resource},
		{name: "wrong actual tool", tool: "slides_info", args: infoArgs, scope: "drive.file", allowed: "slides_list_slides", resource: resource},
		{name: "wrong allowed tool", tool: "slides_list_slides", args: infoArgs, scope: "drive.file", allowed: "slides_info", resource: resource},
		{name: "missing resource", tool: "slides_read_slide", args: readArgs, scope: "presentations.readonly", allowed: "slides_read_slide", resource: nil},
		{name: "disabled resource", tool: "slides_read_slide", args: readArgs, scope: "presentations.readonly", allowed: "slides_read_slide", resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck"}},
		{name: "foreign resource", tool: "slides_read_slide", args: map[string]any{"presentation_id": "foreign-deck", "slide_id": "p:0"}, scope: "presentations.readonly", allowed: "slides_read_slide", resource: resource},
		{name: "wrong scope", tool: "slides_read_slide", args: readArgs, scope: "documents.readonly", allowed: "slides_read_slide", resource: resource},
		{name: "unsupported field", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "unsupported": true}, scope: "presentations.readonly", allowed: "slides_read_slide", resource: resource},
		{name: "invalid detail", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "detail": "yes"}, scope: "presentations.readonly", allowed: "slides_read_slide", resource: resource},
		{name: "all and occurrence", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "all": true, "occurrence": float64(2)}, scope: "presentations.readonly", allowed: "slides_locate", resource: resource},
		{name: "invalid occurrence", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "occurrence": float64(0)}, scope: "presentations.readonly", allowed: "slides_locate", resource: resource},
		{name: "traversal selector", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "page_id": "../slide"}, scope: "presentations.readonly", allowed: "slides_locate", resource: resource},
	}
	for _, denial := range denials {
		t.Run("denied/"+denial.name, func(t *testing.T) {
			if denial.fixtureMode != "" {
				fixtureMode.Store(denial.fixtureMode)
				t.Cleanup(func() { fixtureMode.Store("normal") })
			}
			body := nativeToolBody(t, func(req *Request) {
				if denial.tenant != "" {
					req.TenantID = denial.tenant
				}
				req.Tool = &ToolInvocation{
					Name: denial.tool, Arguments: denial.args,
					Authorization: ToolAuthorization{
						Service: "slides", AllowedTools: []string{denial.allowed},
						Scopes:   []string{"https://www.googleapis.com/auth/" + denial.scope},
						Resource: denial.resource,
					},
				}
			})

			denied := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) {
				claims["operation"] = OperationToolCall
			}), body)
			if denied.OK {
				t.Fatalf("denial succeeded: %s", denied.Body)
			}
		})
	}

	if driveCalls.Load() != beforeDrive || slidesCalls.Load() != beforeSlides {
		t.Fatalf("denials reached Google: Drive %d->%d Slides %d->%d", beforeDrive, driveCalls.Load(), beforeSlides, slidesCalls.Load())
	}

	fixtureMode.Store("wrong_mime")

	wrongMime := call("slides_info", infoArgs, "drive.file", "slides_info")
	if wrongMime.OK || driveCalls.Load() != beforeDrive+1 || slidesCalls.Load() != beforeSlides {
		t.Fatalf("wrong MIME reached Slides: %s", wrongMime.Body)
	}
	beforeDrive = driveCalls.Load()

	fixtureMode.Store("foreign_id")

	foreign := call("slides_info", infoArgs, "drive.file", "slides_info")
	if foreign.OK || driveCalls.Load() != beforeDrive+1 || slidesCalls.Load() != beforeSlides {
		t.Fatalf("foreign returned ID reached Slides: %s", foreign.Body)
	}

	foreignTenantBody := nativeToolBody(t, func(req *Request) {
		req.TenantID = "tenant-b"
		req.Tool = &ToolInvocation{
			Name: "slides_read_slide", Arguments: readArgs,
			Authorization: ToolAuthorization{
				Service: "slides", AllowedTools: []string{"slides_read_slide"},
				Scopes:   []string{"https://www.googleapis.com/auth/presentations.readonly"},
				Resource: resource,
			},
		}
	})

	foreignTenant := postExecute(t, harness.server, signInvocation(t, foreignTenantBody, func(claims map[string]any) {
		claims["operation"] = OperationToolCall
	}), foreignTenantBody)
	if foreignTenant.OK || foreignTenant.Status != http.StatusForbidden {
		t.Fatalf("foreign tenant reached execution: %s", foreignTenant.Body)
	}
}

func TestNativeSlidesReadGuardPinsHostPathPairs(t *testing.T) {
	info := &ToolInvocation{
		Name: "slides_info", Arguments: map[string]any{"presentation_id": "selected-deck"},
		Authorization: ToolAuthorization{Service: "slides", AllowedTools: []string{"slides_info"}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}},
	}
	read := &ToolInvocation{
		Name: "slides_read_slide", Arguments: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"},
		Authorization: ToolAuthorization{Service: "slides", AllowedTools: []string{"slides_read_slide"}, Scopes: []string{"https://www.googleapis.com/auth/presentations.readonly"}, Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}},
	}

	tests := []struct {
		invocation *ToolInvocation
		method     string
		rawURL     string
		allowed    bool
	}{
		{invocation: info, method: http.MethodGet, rawURL: "https://www.googleapis.com/drive/v3/files/selected-deck", allowed: true},
		{invocation: info, method: http.MethodGet, rawURL: "https://slides.googleapis.com/v1/presentations/selected-deck", allowed: true},
		{invocation: info, method: http.MethodGet, rawURL: "https://slides.googleapis.com/drive/v3/files/selected-deck"},
		{invocation: info, method: http.MethodGet, rawURL: "https://www.googleapis.com/v1/presentations/selected-deck"},
		{invocation: info, method: http.MethodPost, rawURL: "https://www.googleapis.com/drive/v3/files/selected-deck"},
		{invocation: read, method: http.MethodGet, rawURL: "https://slides.googleapis.com/v1/presentations/selected-deck", allowed: true},
		{invocation: read, method: http.MethodGet, rawURL: "https://www.googleapis.com/drive/v3/files/selected-deck"},
		{invocation: read, method: http.MethodGet, rawURL: "https://slides.googleapis.com/v1/presentations/foreign-deck"},
		{invocation: read, method: http.MethodPost, rawURL: "https://slides.googleapis.com/v1/presentations/selected-deck:batchUpdate"},
	}
	for _, test := range tests {
		guard := nativeRequestGuard(test.invocation)

		parsed, err := url.Parse(test.rawURL)
		if err != nil {
			t.Fatal(err)
		}

		request := &http.Request{Method: test.method, URL: parsed, Header: http.Header{}}
		if (guard(request) == nil) != test.allowed {
			t.Fatalf("%s %s allowed mismatch", test.method, test.rawURL)
		}
	}
}
