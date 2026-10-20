package runner

import (
	"context"
	"encoding/json"
	"errors"
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

var errSlidesRemainingFixture = errors.New("unexpected Slides table fixture request")

func TestNativeSlidesTableRemainingRunsSelectedFileMutations(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]any
		requestKey    string
		requests      int
		destructive   bool
		expectedSlide string
	}{
		{name: "slides_table_border_style", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(0), "col": float64(1),
			"row_span": float64(1), "col_span": float64(1), "position": "OUTER", "border_color": "#EA4335", "weight": float64(2), "dash": "SOLID",
		}, requestKey: "updateTableBorderProperties", requests: 1},
		{name: "slides_table_cell_style", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1), "col": float64(2),
			"fill_color": "#3367D6", "content_align": "MIDDLE", "bold": true, "text_color": "#FFFFFF", "size": float64(18), "font": "Cambria",
		}, requestKey: "updateTableCellProperties", requests: 2},
		{name: "slides_table_column_delete", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "col": float64(1),
		}, requestKey: "deleteTableColumn", requests: 1, destructive: true},
		{name: "slides_table_column_insert", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "col": float64(1), "count": float64(2), "right": true,
		}, requestKey: "insertTableColumns", requests: 1},
		{name: "slides_table_column_size", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "col": float64(1), "width": float64(120),
		}, requestKey: "updateTableColumnProperties", requests: 1},
		{name: "slides_table_create", args: map[string]any{
			"presentation_id": "selected-deck", "slide_id": "slide:one", "object_id": "table:new", "rows": float64(3), "cols": float64(4),
		}, requestKey: "createTable", requests: 1},
		{name: "slides_table_merge", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(0), "col": float64(1), "row_span": float64(1), "col_span": float64(2),
		}, requestKey: "mergeTableCells", requests: 1, destructive: true},
		{name: "slides_table_row_delete", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1),
		}, requestKey: "deleteTableRow", requests: 1, destructive: true},
		{name: "slides_table_row_insert", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1), "count": float64(2), "below": true,
		}, requestKey: "insertTableRows", requests: 1},
		{name: "slides_table_row_size", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1), "height": float64(48),
		}, requestKey: "updateTableRowProperties", requests: 1},
		{name: "slides_table_unmerge", args: map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(0), "col": float64(1), "row_span": float64(1), "col_span": float64(2),
		}, requestKey: "unmergeTableCells", requests: 1},
	}

	for _, scope := range []string{"presentations", "drive.file"} {
		t.Run(scope, func(t *testing.T) {
			batches := make([]*slides.BatchUpdatePresentationRequest, 0, len(tests))
			failWrites := false
			h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Slides = func(ctx context.Context, account string) (*slides.Service, error) {
					if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
						t.Error("trusted table mutation identity lost")
					}
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						var response any

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/v1/presentations/selected-deck":
							response = map[string]any{
								"presentationId": "selected-deck", "revisionId": "rev-1",
								"slides": []any{map[string]any{
									"objectId": "slide:one",
									"pageElements": []any{map[string]any{
										"objectId": "table:one",
										"table":    map[string]any{"rows": 3, "columns": 4},
									}},
								}},
							}
						case r.Method == http.MethodPost && r.URL.Path == "/v1/presentations/selected-deck:batchUpdate":
							body, err := io.ReadAll(r.Body)
							if err != nil {
								return nil, fmt.Errorf("read batch: %w", err)
							}

							batch := &slides.BatchUpdatePresentationRequest{}
							if err = json.Unmarshal(body, batch); err != nil {
								return nil, fmt.Errorf("decode batch: %w", err)
							}
							batches = append(batches, batch)

							if failWrites {
								return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":503,"message":"uncertain write"}}`)), Request: r}, nil
							}
							response = map[string]any{"presentationId": "selected-deck", "replies": []any{}}
						default:
							return nil, fmt.Errorf("%w: %s %s", errSlidesRemainingFixture, r.Method, r.URL)
						}

						encoded, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("encode response: %w", err)
						}

						return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r}, nil
					})
					retry := googleapi.NewRetryTransport(transport)
					retry.MaxRetries5xx = 1

					return slides.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, retry)}))
				}
			})
			call := func(name string, args map[string]any) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{
						Service: "slides", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope},
						AllowWrite: true, AllowDestructive: true,
						Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true},
					}}
				})

				return postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					result := call(test.name, test.args)
					if !result.OK {
						t.Fatalf("%s: %s", test.name, result.Body)
					}

					batch := batches[len(batches)-1]
					if len(batch.Requests) != test.requests {
						t.Fatalf("%s requests = %d, want %d", test.name, len(batch.Requests), test.requests)
					}

					if batch.WriteControl == nil || batch.WriteControl.RequiredRevisionId != "rev-1" {
						t.Fatalf("%s write control = %+v", test.name, batch.WriteControl)
					}

					if !hasSlideRequestKey(batch, test.requestKey) {
						t.Fatalf("%s missing request key %s: %+v", test.name, test.requestKey, batch.Requests)
					}
					before := len(batches)
					failWrites = true
					failed := call(test.name, test.args)
					failWrites = false

					if failed.OK || len(batches) != before+1 {
						t.Fatalf("uncertain table mutation replayed: %d -> %d %s", before, len(batches), failed.Body)
					}
				})
			}
		})
	}
}

func hasSlideRequestKey(batch *slides.BatchUpdatePresentationRequest, key string) bool {
	for _, request := range batch.Requests {
		if request == nil {
			continue
		}

		encoded, err := json.Marshal(request)
		if err != nil {
			continue
		}

		if strings.Contains(string(encoded), `"`+key+`"`) {
			return true
		}
	}

	return false
}

func TestNativeSlidesTableRemainingBindingContract(t *testing.T) {
	bindings := nativeSlidesTableRemainingBindings()
	if len(bindings) != 11 {
		t.Fatalf("bindings = %d, want 11", len(bindings))
	}
	destructive := map[string]bool{
		"slides_table_column_delete": true,
		"slides_table_row_delete":    true,
		"slides_table_merge":         true,
	}

	for _, binding := range bindings {
		if binding.Service != "slides" || binding.ResourceType != resourceTypeFile || binding.ResourceArgument != "presentation_id" || !binding.Write {
			t.Fatalf("%s binding contract: %+v", binding.Name, binding)
		}

		if len(binding.scopes) != 2 || binding.scopes[0] != "presentations" || binding.scopes[1] != driveFileScope {
			t.Fatalf("%s scopes = %v", binding.Name, binding.scopes)
		}

		if len(binding.methods) != 2 || binding.methods[0] != http.MethodGet || binding.methods[1] != http.MethodPost {
			t.Fatalf("%s methods = %v", binding.Name, binding.methods)
		}

		if binding.destructive != destructive[binding.Name] {
			t.Fatalf("%s destructive = %v", binding.Name, binding.destructive)
		}
	}
}

func TestNativeSlidesTableRemainingDenialsDoNotReachGoogle(t *testing.T) {
	calls := 0
	h := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Slides = func(context.Context, string) (*slides.Service, error) {
			calls++

			t.Error("denied table mutation reached service creation")

			return nil, errSlidesRemainingFixture
		}
	})

	for _, binding := range nativeSlidesTableRemainingBindings() {
		baseArgs := map[string]any{"presentation_id": "selected-deck", "table_object_id": "table:one"}

		switch binding.Name {
		case "slides_table_create":
			baseArgs = map[string]any{"presentation_id": "selected-deck", "slide_id": "slide:one", "rows": 2, "cols": 2}
		case "slides_table_border_style":
			baseArgs["row"], baseArgs["col"], baseArgs["border_color"] = 0, 0, "#112233"
		case "slides_table_cell_style":
			baseArgs["row"], baseArgs["col"], baseArgs["fill_color"] = 0, 0, "#112233"
		case "slides_table_column_size":
			baseArgs["col"], baseArgs["width"] = 0, 40
		case "slides_table_row_size":
			baseArgs["row"], baseArgs["height"] = 0, 20
		case "slides_table_column_insert", "slides_table_column_delete":
			baseArgs["col"] = 0
		case "slides_table_row_insert", "slides_table_row_delete":
			baseArgs["row"] = 0
		default:
			baseArgs["row"], baseArgs["col"], baseArgs["row_span"], baseArgs["col_span"] = 0, 0, 1, 2
		}

		changes := []struct {
			name   string
			change func(*ToolAuthorization)
		}{
			{"tool", func(a *ToolAuthorization) { a.AllowedTools = []string{"slides_list_slides"} }},
			{"scope", func(a *ToolAuthorization) {
				a.Scopes = []string{"https://www.googleapis.com/auth/presentations.readonly"}
			}},
			{"write", func(a *ToolAuthorization) { a.AllowWrite = false }},
			{"resource", func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-deck" }},
		}
		if binding.destructive {
			changes = append(changes, struct {
				name   string
				change func(*ToolAuthorization)
			}{"destructive", func(a *ToolAuthorization) { a.AllowDestructive = false }})
		}

		for _, change := range changes {
			t.Run(binding.Name+"/"+change.name, func(t *testing.T) {
				auth := ToolAuthorization{
					Service: "slides", AllowedTools: []string{binding.Name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"},
					AllowWrite: true, AllowDestructive: true,
					Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true},
				}
				change.change(&auth)
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: binding.Name, Arguments: baseArgs, Authorization: auth}
				})

				response := postExecute(t, h.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
				if response.OK || calls != 0 {
					t.Fatalf("denied mutation admitted or reached Google: %s", response.Body)
				}
			})
		}
	}
}

func TestNativeSlidesTableRemainingPinsSelectedFileAndMethods(t *testing.T) {
	for _, binding := range nativeSlidesTableRemainingBindings() {
		t.Run(binding.Name, func(t *testing.T) {
			invocation := &ToolInvocation{
				Name: binding.Name, Arguments: map[string]any{"presentation_id": "selected-deck"},
				Authorization: ToolAuthorization{
					Service: "slides", AllowedTools: []string{binding.Name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"},
					AllowWrite: true, AllowDestructive: true,
					Resource: &ResourceGrantInput{Service: "slides", ResourceType: "file", ResourceID: "selected-deck", Enabled: true},
				},
			}
			guard := nativeRequestGuard(invocation)

			root := "https://slides.googleapis.com/v1/presentations/selected-deck"
			for _, test := range []struct {
				method, url string
				allow       bool
			}{
				{http.MethodGet, root, true},
				{http.MethodPost, root + ":batchUpdate", true},
				{http.MethodGet, root + ":batchUpdate", false},
				{http.MethodPost, root, false},
				{http.MethodDelete, root, false},
				{http.MethodPost, "https://slides.googleapis.com/v1/presentations/foreign-deck:batchUpdate", false},
				{http.MethodPost, "https://example.test/v1/presentations/selected-deck:batchUpdate", false},
			} {
				request, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
				if err != nil {
					t.Fatal(err)
				}

				if err = guard(request); (err == nil) != test.allow {
					t.Fatalf("%s %s allow=%t error=%v", test.method, test.url, test.allow, err)
				}
			}
		})
	}
}
