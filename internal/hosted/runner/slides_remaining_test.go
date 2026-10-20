package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

const remainingSlidesFixture = `{"presentationId":"selected-deck","title":"Deck","revisionId":"rev-1","slides":[{"objectId":"slide-1","slideProperties":{"notesPage":{"notesProperties":{"speakerNotesObjectId":"notes-1"},"pageElements":[{"objectId":"notes-1","shape":{"text":{"textElements":[{"textRun":{"content":"notes\n"}}]}}}]}},"pageElements":[{"objectId":"shape-1","shape":{"text":{"textElements":[{"startIndex":0,"endIndex":4,"textRun":{"content":"abc\n"}}]}}},{"objectId":"shape-2","shape":{"text":{"textElements":[{"textRun":{"content":"xyz\n"}}]}}},{"objectId":"image-1","image":{"contentUrl":"https://never-follow.example/original.png"}},{"objectId":"group-1","elementGroup":{"children":[{"objectId":"shape-3","shape":{}},{"objectId":"shape-4","shape":{}}]}}]}],"unknownZero":0}`

func TestNativeSlidesRemaining(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       map[string]any
		requestKey string
	}{
		{"slides_bullets", map[string]any{"object_id": "shape-1", "range": "0:3", "on": true}, "createParagraphBullets"},
		{"slides_copy", map[string]any{"title": "Copy"}, ""},
		{"slides_element_alt_text", map[string]any{"object_id": "shape-1", "title": "", "description": "Accessible"}, "updatePageElementAltText"},
		{"slides_element_create_line", map[string]any{"slide_id": "slide-1", "object_id": "new-line"}, "createLine"},
		{"slides_element_create_shape", map[string]any{"slide_id": "slide-1", "object_id": "new-shape"}, "createShape"},
		{"slides_element_delete", map[string]any{"object_ids": []any{"shape-1"}}, "deleteObject"},
		{"slides_element_group", map[string]any{"object_ids": []any{"shape-1", "shape-2"}, "group_id": "new-group"}, "groupObjects"},
		{"slides_element_style", map[string]any{"object_id": "shape-1", "fill_color": "#112233"}, "updateShapeProperties"},
		{"slides_element_transform", map[string]any{"object_id": "shape-1", "translate_x": 0, "translate_y": 12}, "updatePageElementTransform"},
		{"slides_element_ungroup", map[string]any{"group_ids": []any{"group-1"}}, "ungroupObjects"},
		{"slides_element_z_order", map[string]any{"object_ids": []any{"shape-1"}, "operation": "BRING_TO_FRONT"}, "updatePageElementsZOrder"},
		{"slides_insert_text", map[string]any{"object_id": "shape-1", "text": "literal --file=/tmp/not-read"}, "insertText"},
		{"slides_link", map[string]any{"object_id": "shape-1", "range": "0:3", "url": "https://never-follow.example/link"}, "updateTextStyle"},
		{"slides_paragraph_style", map[string]any{"object_id": "shape-1", "align": "CENTER", "space_above": 0}, "updateParagraphStyle"},
		{"slides_raw", map[string]any{}, ""},
		{"slides_replace_slide", map[string]any{"slide_id": "slide-1", "url": "https://never-follow.example/replacement.png", "notes": ""}, "replaceImage"},
		{"slides_style_text", map[string]any{"object_id": "shape-1", "range": "0:3", "no_bold": true, "size": 14}, "updateTextStyle"},
		{"slides_thumbnail", map[string]any{"slide_id": "slide-1", "size": "small", "format": "png"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes, calls := 0, 0
			failWrite := false
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				client := func(ctx context.Context) *http.Client {
					base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++

						body := remainingSlidesFixture
						if r.URL.Hostname() == nativeDocsSlidesDriveHost {
							body = `{"id":"selected-deck","mimeType":"application/vnd.google-apps.presentation","webViewLink":"https://never-follow.example/deck"}`
						}

						if strings.HasSuffix(r.URL.Path, "/thumbnail") {
							body = `{"contentUrl":"https://never-follow.example/thumb.png","width":160,"height":90}`
						}
						status := 200

						if r.Method == http.MethodPost {
							writes++

							var payload map[string]any
							if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
								t.Fatal(err)
							}

							if tc.name == "slides_copy" {
								if payload["name"] != "Copy" {
									t.Fatalf("bad copy %v", payload)
								}
							} else {
								requests, _ := payload["requests"].([]any)
								if len(requests) == 0 {
									t.Fatal("missing typed requests")
								}

								if _, ok := requests[0].(map[string]any)[tc.requestKey]; !ok {
									t.Fatalf("expected %s in %v", tc.requestKey, requests)
								}
							}
							body = `{"presentationId":"selected-deck","id":"created-deck","replies":[{}]}`

							if failWrite {
								status = 503
								body = `{"error":{"code":503,"message":"uncertain"}}`
							}
						}

						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})
					retry := googleapi.NewRetryTransport(base)
					retry.MaxRetries5xx = 1

					return &http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}
				}
				engine.NativeServices.Slides = func(ctx context.Context, _ string) (*slides.Service, error) {
					return slides.NewService(ctx, option.WithHTTPClient(client(ctx)))
				}
				engine.NativeServices.SlidesHTTP = func(ctx context.Context, _ string) (*http.Client, error) { return client(ctx), nil }
				engine.NativeServices.Drive = func(ctx context.Context, _ string) (*drive.Service, error) {
					return drive.NewService(ctx, option.WithHTTPClient(client(ctx)))
				}
			})

			binding, ok := nativeBinding(tc.name)
			if !ok {
				t.Fatal("missing binding")
			}
			tc.args["presentation_id"] = "selected-deck"
			call := func(change func(*ToolAuthorization)) capturedResponse {
				auth := ToolAuthorization{Service: "slides", AllowedTools: []string{tc.name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, AllowWrite: true, AllowDestructive: true, Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true}}
				if change != nil {
					change(&auth)
				}
				data := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: tc.name, Arguments: tc.args, Authorization: auth} })

				return postExecute(t, harness.server, signInvocation(t, data, func(claims map[string]any) { claims["operation"] = OperationToolCall }), data)
			}

			result := call(nil)
			if !result.OK || calls == 0 || binding.Write && writes != 1 {
				t.Fatalf("%s writes=%d calls=%d", result.Body, writes, calls)
			}

			if tc.name == "slides_raw" && !strings.Contains(string(result.Result), `"unknownZero":0`) {
				t.Fatalf("raw structure lost %s", result.Result)
			}
			before := calls

			denials := []func(*ToolAuthorization){func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign" }, func(a *ToolAuthorization) { a.Resource.Enabled = false }, func(a *ToolAuthorization) { a.AllowedTools = []string{"other_tool"} }, func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/tasks"} }}
			if binding.Write {
				denials = append(denials, func(a *ToolAuthorization) { a.AllowWrite = false })
			}

			if binding.destructive {
				denials = append(denials, func(a *ToolAuthorization) { a.AllowDestructive = false })
			}

			for _, deny := range denials {
				if denied := call(deny); denied.OK {
					t.Fatal("unauthorized call admitted")
				}
			}

			if calls != before {
				t.Fatal("denied call reached Google")
			}

			if binding.Write {
				beforeWrites := writes
				failWrite = true

				if failed := call(nil); failed.OK || writes != beforeWrites+1 {
					t.Fatalf("uncertain write replayed: %d -> %d %s", beforeWrites, writes, failed.Body)
				}
			}
		})
	}
}

func TestSlidesRemainingHTTPGuard(t *testing.T) {
	for _, binding := range nativeSlidesRemainingBindings() {
		invocation := &ToolInvocation{Name: binding.Name, Arguments: map[string]any{"presentation_id": "selected-deck", "slide_id": "slide-1"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-deck"}}}
		guard := nativeRequestGuard(invocation)
		base := "https://slides.googleapis.com/v1/presentations/selected-deck"

		action := base + ":batchUpdate"
		if binding.Name == "slides_copy" {
			base = "https://www.googleapis.com/drive/v3/files/selected-deck"
			action = base + "/copy"
		}

		if binding.Name == "slides_thumbnail" {
			base += "/pages/slide-1/thumbnail"
		}

		for _, tc := range []struct {
			method, url string
			allow       bool
		}{{"GET", base, true}, {"POST", action, binding.Write}, {"POST", base, false}, {"GET", action, false}, {"DELETE", base, false}, {"GET", strings.ReplaceAll(base, "selected-deck", "foreign"), false}, {"GET", strings.ReplaceAll(base, "googleapis.com", "evil.test"), false}} {
			request, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err = guard(request); (err == nil) != tc.allow {
				t.Errorf("%s %s %s %v", binding.Name, tc.method, tc.url, err)
			}
		}
	}
}
