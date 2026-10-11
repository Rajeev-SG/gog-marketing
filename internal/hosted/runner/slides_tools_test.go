package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeSlidesHandlersAndDestructiveAdmission(t *testing.T) {
	reads, writes := 0, 0
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Slides = func(ctx context.Context, _ string) (*slides.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"presentationId":"selected-deck","title":"Fixture","slides":[{"objectId":"slide-a","slideProperties":{"notesPage":{"pageElements":[{"objectId":"notes-a","shape":{"placeholder":{"type":"BODY"}}}]}}}]}`

				if r.Method == http.MethodPost {
					writes++

					data, err := io.ReadAll(r.Body)
					if err != nil || !strings.Contains(string(data), `"text":"verified notes"`) || r.URL.Path != "/v1/presentations/selected-deck:batchUpdate" {
						t.Errorf("unexpected notes mutation: %s %v", data, err)
					}
					body = `{"presentationId":"selected-deck","replies":[{}]}`
				} else {
					reads++

					if r.URL.Path != "/v1/presentations/selected-deck" {
						t.Errorf("foreign path: %s", r.URL.Path)
					}
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return slides.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name string, destructive bool) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool.Name = name

			req.Tool.Arguments = map[string]any{"presentation_id": "selected-deck"}
			if name == "slides_update_notes" {
				req.Tool.Arguments["slide_id"] = "slide-a"
				req.Tool.Arguments["notes"] = "verified notes"
			}
			req.Tool.Authorization = ToolAuthorization{Service: "slides", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}, AllowWrite: true, AllowDestructive: destructive}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	denied := call("slides_update_notes", false)
	if denied.Status != 400 || reads != 0 || writes != 0 {
		t.Fatalf("destructive denial reached Google: %+v", denied)
	}

	listed := call("slides_list_slides", false)
	if !listed.OK || !strings.Contains(string(listed.Result), "slide-a") {
		t.Fatalf("native Slides list failed: %s", listed.Body)
	}

	updated := call("slides_update_notes", true)
	if !updated.OK || reads != 2 || writes != 1 {
		t.Fatalf("native notes update failed: %s, reads=%d writes=%d", updated.Body, reads, writes)
	}
}
