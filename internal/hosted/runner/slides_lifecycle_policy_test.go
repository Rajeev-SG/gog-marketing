package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func lifecycleLifecycleAuth(name string, scope string, destructive bool) ToolAuthorization {
	return ToolAuthorization{
		Service: "slides", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope},
		AllowWrite: true, AllowDestructive: destructive,
		Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true},
	}
}

func TestNativeSlidesLifecycleRunsSelectedFileMutations(t *testing.T) {
	for _, scope := range []string{"presentations", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			var batches []*slides.BatchUpdatePresentationRequest
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Slides = func(ctx context.Context, account string) (*slides.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted mutation identity lost")
					}
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						if r.Method != http.MethodPost || r.URL.Hostname() != nativeSlidesHost || r.URL.Path != "/v1/presentations/selected-deck:batchUpdate" {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL)
							return nil, errSlidesEditFixture
						}
						batch := &slides.BatchUpdatePresentationRequest{}

						body, err := io.ReadAll(r.Body)
						if err != nil {
							return nil, fmt.Errorf("read batch: %w", err)
						}

						if unmarshalErr := json.Unmarshal(body, batch); unmarshalErr != nil {
							return nil, fmt.Errorf("unmarshal batch: %w", unmarshalErr)
						}
						batches = append(batches, batch)

						response, err := json.Marshal(map[string]any{"presentationId": "selected-deck", "replies": []any{map[string]any{}}})
						if err != nil {
							return nil, fmt.Errorf("marshal batch: %w", err)
						}

						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(response))), Request: r}, nil
					})

					return slides.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(name string, args map[string]any, destructive bool) {
				t.Helper()
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: lifecycleLifecycleAuth(name, scope, destructive)}
				})

				response := postExecute(t, h.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
				if !response.OK {
					t.Fatalf("%s: %s", name, response.Body)
				}
			}

			call("slides_new_slide", map[string]any{"presentation_id": "selected-deck"}, false)

			if len(batches[len(batches)-1].Requests) != 1 || batches[len(batches)-1].Requests[0].CreateSlide == nil || batches[len(batches)-1].Requests[0].CreateSlide.SlideLayoutReference != nil {
				t.Fatalf("new default = %+v", batches[len(batches)-1])
			}

			call("slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout": "TITLE_AND_BODY", "index": int64(0)}, false)

			batch := batches[len(batches)-1]
			if len(batch.Requests) != 1 || batch.Requests[0].CreateSlide == nil || batch.Requests[0].CreateSlide.SlideLayoutReference == nil || batch.Requests[0].CreateSlide.SlideLayoutReference.PredefinedLayout != "TITLE_AND_BODY" || batch.Requests[0].CreateSlide.InsertionIndex != 0 {
				t.Fatalf("new explicit = %+v", batch)
			}

			call("slides_new_slide", map[string]any{"presentation_id": "selected-deck", "layout_id": "layout:one"}, false)

			batch = batches[len(batches)-1]
			if len(batch.Requests) != 1 || batch.Requests[0].CreateSlide == nil || batch.Requests[0].CreateSlide.SlideLayoutReference == nil || batch.Requests[0].CreateSlide.SlideLayoutReference.LayoutId != "layout:one" {
				t.Fatalf("new layout id = %+v", batch)
			}

			call("slides_duplicate_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "to_index": int64(0)}, false)

			batch = batches[len(batches)-1]
			if len(batch.Requests) != 2 || batch.Requests[0].DuplicateObject == nil || batch.Requests[0].DuplicateObject.ObjectId != "p:0" || batch.Requests[0].DuplicateObject.ObjectIds["p:0"] == "" || batch.Requests[1].UpdateSlidesPosition == nil || batch.Requests[1].UpdateSlidesPosition.SlideObjectIds[0] != batch.Requests[0].DuplicateObject.ObjectIds["p:0"] || batch.Requests[1].UpdateSlidesPosition.InsertionIndex != 0 {
				t.Fatalf("duplicate = %+v", batch)
			}

			call("slides_move_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "to_index": int64(0)}, false)

			batch = batches[len(batches)-1]
			if len(batch.Requests) != 1 || batch.Requests[0].UpdateSlidesPosition == nil || batch.Requests[0].UpdateSlidesPosition.SlideObjectIds[0] != "p:0" || batch.Requests[0].UpdateSlidesPosition.InsertionIndex != 0 {
				t.Fatalf("move = %+v", batch)
			}

			call("slides_delete_slide", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, true)

			batch = batches[len(batches)-1]
			if len(batch.Requests) != 1 || batch.Requests[0].DeleteObject == nil || batch.Requests[0].DeleteObject.ObjectId != "p:0" {
				t.Fatalf("delete = %+v", batch)
			}
		})
	}
}

func TestNativeSlidesLifecycleDenialsDoNotReachGoogle(t *testing.T) {
	var calls int
	h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Slides = func(context.Context, string) (*slides.Service, error) {
			calls++

			t.Error("denied mutation reached service creation")

			return nil, errSlidesEditFixture
		}
	})

	tests := []struct {
		name   string
		args   map[string]any
		change func(*ToolAuthorization)
	}{
		{"write", map[string]any{"presentation_id": "selected-deck"}, func(a *ToolAuthorization) { a.AllowWrite = false }},
		{"scope", map[string]any{"presentation_id": "selected-deck"}, func(a *ToolAuthorization) {
			a.Scopes = []string{"https://www.googleapis.com/auth/presentations.readonly"}
		}},
		{"no resource", map[string]any{"presentation_id": "selected-deck"}, func(a *ToolAuthorization) { a.Resource = nil }},
		{"disabled resource", map[string]any{"presentation_id": "selected-deck"}, func(a *ToolAuthorization) { a.Resource.Enabled = false }},
		{"tool", map[string]any{"presentation_id": "selected-deck"}, func(a *ToolAuthorization) { a.AllowedTools = []string{"slides_list_slides"} }},
		{"foreign file", map[string]any{"presentation_id": "selected-deck"}, func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-deck" }},
		{"destructive", map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			auth := lifecycleLifecycleAuth("slides_new_slide", "presentations", false)
			if test.change != nil {
				test.change(&auth)
			}

			if test.name == "destructive" {
				auth.AllowedTools = []string{"slides_delete_slide"}
				auth.AllowDestructive = false
			}

			invocationName := auth.AllowedTools[0]
			if test.name == "tool" {
				invocationName = "slides_new_slide"
			}
			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: invocationName, Arguments: test.args, Authorization: auth}
			})

			response := postExecute(t, h.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
			if response.OK {
				t.Fatalf("denied mutation succeeded: %s", response.Body)
			}

			if calls != 0 {
				t.Fatalf("service calls = %d, want 0", calls)
			}
		})
	}
}

func TestNativeSlidesLifecyclePinsSelectedFileAndMethods(t *testing.T) {
	for _, name := range []string{"slides_new_slide", "slides_duplicate_slide", "slides_move_slide", "slides_delete_slide"} {
		t.Run(name, func(t *testing.T) {
			inv := &ToolInvocation{Name: name, Arguments: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, Authorization: lifecycleLifecycleAuth(name, "drive.file", true)}
			guard := nativeRequestGuard(inv)

			root := "https://slides.googleapis.com/v1/presentations/selected-deck"
			for _, test := range []struct {
				method, url string
				allow       bool
			}{
				{http.MethodPost, root + ":batchUpdate", true},
				{http.MethodGet, root + ":batchUpdate", false},
				{http.MethodPost, root, false},
				{http.MethodPost, "https://slides.googleapis.com/v1/presentations/foreign-deck:batchUpdate", false},
				{http.MethodPost, "https://example.test/v1/presentations/selected-deck:batchUpdate", false},
			} {
				req, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
				if err != nil {
					t.Fatal(err)
				}

				if err := guard(req); (err == nil) != test.allow {
					t.Errorf("%s %s allow=%t error=%v", test.method, test.url, test.allow, err)
				}
			}
		})
	}
}
