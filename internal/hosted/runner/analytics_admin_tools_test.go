package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeAdminCRUDUsesSelectedPropertyAndInlinePayloads(t *testing.T) {
	for _, family := range []struct{ command, api, field, mask, value string }{{"datastreams", "dataStreams", "displayName", "display_name", "Updated stream"}, {"keyevents", "keyEvents", "countingMethod", "counting_method", "ONCE_PER_SESSION"}} {
		t.Run(family.command, func(t *testing.T) {
			var requests atomic.Int64
			var stored map[string]any
			root := "/v1beta/properties/123/" + family.api
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.AnalyticsAdmin = func(ctx context.Context, _ string) (*analyticsadmin.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						requests.Add(1)
						var response any = stored

						switch {
						case r.Method == http.MethodPost && r.URL.Path == root:
							if err := json.NewDecoder(r.Body).Decode(&stored); err != nil {
								t.Error(err)
							}
							stored["name"] = "properties/123/" + family.api + "/7"
							response = stored
						case r.Method == http.MethodPatch && r.URL.Path == root+"/7":
							var patch map[string]any
							if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
								t.Error(err)
							}

							if r.URL.Query().Get("updateMask") != family.mask {
								t.Error("update mask was lost")
							}

							for key, value := range patch {
								stored[key] = value
							}
							response = stored
						case r.Method == http.MethodGet && r.URL.Path == root:
							items := []any{}
							if stored != nil {
								items = append(items, stored)
							}
							response = map[string]any{family.api: items}
						case r.Method == http.MethodGet && r.URL.Path == root+"/7":
						case r.Method == http.MethodDelete && r.URL.Path == root+"/7":
							stored = nil
							response = map[string]any{}
						default:
							t.Errorf("foreign or unexpected request: %s %s", r.Method, r.URL)
						}

						data, err := json.Marshal(response)
						if err != nil {
							return nil, fmt.Errorf("encode fixture: %w", err)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
					})

					return analyticsadmin.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(operation, scope string, destructive bool, args map[string]any) capturedResponse {
				name := "analytics_" + family.command + "_" + operation
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{
						Service: "analytics", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive,
						Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true},
					}}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}
			for _, operation := range []string{"create", "list", "get", "update", "get", "delete"} {
				args := map[string]any{"property": "properties/123"}
				if operation == "get" || operation == "update" || operation == "delete" {
					args["resource_id"] = "7"
				}

				if operation == "create" {
					if family.command == "datastreams" {
						args["body_json"] = `{"displayName":"Test stream","type":"WEB_DATA_STREAM","webStreamData":{"defaultUri":"https://example.test"}}`
					} else {
						args["body_json"] = `{"eventName":"hosted_test","countingMethod":"ONCE_PER_EVENT"}`
					}
				}

				if operation == "update" {
					args["body_json"] = `{"` + family.field + `":"` + family.value + `"}`
					args["update_mask"] = family.mask
				}

				if operation == "create" || operation == "update" || operation == "delete" {
					before := requests.Load()

					denied := call(operation, "analytics.readonly", true, args)
					if denied.OK || requests.Load() != before {
						t.Fatal("read scope authorized Admin mutation")
					}
				}

				if operation == "delete" {
					before := requests.Load()

					denied := call(operation, "analytics.edit", false, args)
					if denied.OK || requests.Load() != before {
						t.Fatal("destructive denial reached Google")
					}
				}

				response := call(operation, "analytics.edit", true, args)
				if !response.OK {
					t.Fatalf("native %s failed: %s", operation, response.Body)
				}

				if operation == "update" && stored[family.field] != family.value {
					t.Fatal("fixture update did not persist")
				}
			}

			if stored != nil {
				t.Fatal("fixture delete did not persist")
			}
			before := requests.Load()

			wildcard := call("update", "analytics.edit", false, map[string]any{"property": "properties/123", "resource_id": "7", "body_json": "{}", "update_mask": "*"})
			if wildcard.OK || requests.Load() != before {
				t.Fatal("full replacement reached Google without destructive permission")
			}

			for _, raw := range []string{"@/etc/passwd", `[1]`, `{"name":"properties/999/` + family.api + `/7"}`} {
				denied := call("update", "analytics.edit", true, map[string]any{"property": "properties/123", "resource_id": "7", "body_json": raw, "update_mask": family.mask})
				if denied.OK || requests.Load() != before {
					t.Fatal("file/foreign inline payload reached Google")
				}
			}

			denied := call("get", "analytics.edit", false, map[string]any{"property": "properties/999", "resource_id": "7"})
			if denied.OK || requests.Load() != before {
				t.Fatal("foreign property reached Google")
			}
		})
	}
}
