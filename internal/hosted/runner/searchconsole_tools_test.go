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

	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeSearchConsoleSelectedSiteReadWriteAndInspection(t *testing.T) {
	for _, site := range []string{"https://example.test/", "sc-domain:example.test"} {
		t.Run(site, func(t *testing.T) {
			feed := "https://example.test/sitemap.xml"
			var calls atomic.Int64
			submitted := false
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.SearchConsole = func(ctx context.Context, _ string) (*searchconsole.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)
						root := "/webmasters/v3/sites/" + site
						body := `{}`
						status := 200

						switch {
						case r.Method == http.MethodGet && r.URL.Path == "/webmasters/v3/sites":
							data, _ := json.Marshal(map[string]any{"siteEntry": []any{map[string]any{"siteUrl": site, "permissionLevel": "siteOwner"}}})
							body = string(data)
						case r.Method == http.MethodGet && r.URL.Path == root:
							data, _ := json.Marshal(map[string]any{"siteUrl": site, "permissionLevel": "siteOwner"})
							body = string(data)
						case r.Method == http.MethodPut && r.URL.Path == root+"/sitemaps/"+feed:
							submitted = true
							status = 204
						case r.Method == http.MethodDelete && r.URL.Path == root+"/sitemaps/"+feed:
							submitted = false
							status = 204
						case r.Method == http.MethodGet && r.URL.Path == root+"/sitemaps":
							body = `{"sitemap":[]}`
							if submitted {
								body = `{"sitemap":[{"path":"` + feed + `","type":"SITEMAP"}]}`
							}
						case r.Method == http.MethodGet && r.URL.Path == root+"/sitemaps/"+feed:
							body = `{"path":"` + feed + `","type":"SITEMAP"}`
						case r.Method == http.MethodPost && r.URL.Path == root+"/searchAnalytics/query":
							body = `{"rows":[{"keys":["query"],"clicks":1}]}`
						case r.Method == http.MethodPost && r.URL.Path == "/v1/urlInspection/index:inspect":
							var input searchconsole.InspectUrlIndexRequest
							if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
								t.Error(err)
							}

							if input.SiteUrl != site || input.InspectionUrl != "https://example.test/page" {
								t.Errorf("foreign inspection body: %+v", input)
							}
							body = `{"inspectionResult":{"indexStatusResult":{"verdict":"PASS"}}}`
						default:
							t.Errorf("unexpected request: %s %s", r.Method, r.URL)
							status = 500
						}

						return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})

					return searchconsole.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})

			call := func(name, scope string, destructive bool, args map[string]any) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					binding, _ := nativeBinding(name)

					auth := ToolAuthorization{Service: "searchconsole", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive}
					if binding.ResourceType != "" {
						auth.Resource = &ResourceGrantInput{Service: "searchconsole", ResourceType: "site", ResourceID: site, Enabled: true}
					}
					req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}
			for _, name := range []string{"searchconsole_sites_list", "searchconsole_sites_get", "searchconsole_sitemaps_submit", "searchconsole_sitemaps_list", "searchconsole_sitemaps_get", "searchconsole_query", "searchconsole_inspect", "searchconsole_sitemaps_delete"} {
				args := map[string]any{}
				if name != "searchconsole_sites_list" {
					args["site_url"] = site
				}

				if name == "searchconsole_sitemaps_submit" || name == "searchconsole_sitemaps_delete" || name == "searchconsole_sitemaps_get" {
					args["feed_path"] = feed
				}

				if name == "searchconsole_inspect" {
					args["url"] = "https://example.test/page"
				}

				if name == "searchconsole_query" {
					args["from"] = "2026-10-01"
					args["to"] = "2026-10-10"
				}

				binding, _ := nativeBinding(name)
				if binding.Write {
					before := calls.Load()

					denied := call(name, "webmasters.readonly", true, args)
					if denied.OK || calls.Load() != before {
						t.Fatal("read scope authorized sitemap mutation")
					}
				}

				if binding.destructive {
					before := calls.Load()

					denied := call(name, "webmasters", false, args)
					if denied.OK || calls.Load() != before {
						t.Fatal("delete bypassed destructive permission")
					}
				}

				response := call(name, "webmasters", true, args)
				if !response.OK {
					t.Fatalf("native %s failed: %s", name, response.Body)
				}

				if name == "searchconsole_sitemaps_list" && !strings.Contains(string(response.Result), feed) {
					t.Fatal("submitted sitemap did not persist in fixture")
				}
			}

			if submitted {
				t.Fatal("sitemap deletion did not persist")
			}
			before := calls.Load()

			for _, name := range []string{"searchconsole_inspect", "searchconsole_sitemaps_submit"} {
				denied := call(name, "webmasters", true, map[string]any{"site_url": site, "feed_path": "https://foreign.test/map.xml", "url": "https://evil-example.test/page"})
				if denied.OK || calls.Load() != before {
					t.Fatal("foreign URL reached Google")
				}
			}
			binding, _ := nativeBinding("searchconsole_sitemaps_submit")
			invocation := &ToolInvocation{Name: binding.Name, Arguments: map[string]any{"site_url": site, "feed_path": feed}}
			// A path with decoded URL separators is not the SDK's escaped segment.
			unescaped := url.URL{Scheme: "https", Host: "searchconsole.googleapis.com", Path: "/webmasters/v3/sites/" + site + "/sitemaps/" + feed}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, unescaped.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			if nativeRequestGuard(invocation)(req) == nil {
				t.Fatal("alternate URL segment encoding admitted")
			}
		})
	}
}
