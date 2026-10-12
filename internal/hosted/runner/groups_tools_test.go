package runner

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/cloudidentity/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeGroupsListingKeepsConnectedAccountAndReadScope(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.CloudIdentity = func(ctx context.Context, account string) (*cloudidentity.Service, error) {
			if account != "owner@example.com" {
				t.Errorf("wrong connected account: %s", account)
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodGet || r.URL.Host != "cloudidentity.googleapis.com" || r.URL.Path != "/v1/groups/-/memberships:searchTransitiveGroups" || !strings.Contains(r.URL.Query().Get("query"), "owner@example.com") {
					t.Errorf("unexpected group request: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"memberships":[{"group":"groups/beta","groupKey":{"id":"beta@example.test"},"displayName":"Beta group"}]} `)), Request: r}, nil
			})

			return cloudidentity.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(scope string) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: "groups_list", Arguments: map[string]any{}, Authorization: ToolAuthorization{Service: "groups", AllowedTools: []string{"groups_list"}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	denied := call("contacts.readonly")
	if denied.OK || calls.Load() != 0 {
		t.Fatal("unrelated scope admitted group lookup")
	}

	listed := call("cloud-identity.groups.readonly")
	if !listed.OK || calls.Load() != 1 || !strings.Contains(string(listed.Result), "beta@example.test") {
		t.Fatalf("native group list failed: %s", listed.Body)
	}
}

func TestNativeGroupMembersBindsLookupAndRejectsMalformedResolvedNames(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(strconv.FormatBool(malformed), func(t *testing.T) {
			var calls atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.CloudIdentity = func(ctx context.Context, _ string) (*cloudidentity.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)
						body := `{"name":"groups/group-7"}`

						switch r.URL.Path {
						case "/v1/groups:lookup":
							if r.URL.Query().Get("groupKey.id") != "beta@example.test" {
								t.Error("lookup group identity changed")
							}

							if malformed {
								body = `{"name":"groups/../foreign"}`
							}
						case "/v1/groups/group-7/memberships":
							body = `{"memberships":[{"preferredMemberKey":{"id":"member@example.test"},"roles":[{"name":"MEMBER"}],"type":"USER"}]}`
						default:
							t.Errorf("foreign membership path: %s", r.URL)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})

					return cloudidentity.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: "groups_members", Arguments: map[string]any{"group_email": "beta@example.test"}, Authorization: ToolAuthorization{Service: "groups", AllowedTools: []string{"groups_members"}, Scopes: []string{"https://www.googleapis.com/auth/cloud-identity.groups.readonly"}}}
			})

			result := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			if malformed {
				if result.OK || calls.Load() != 1 {
					t.Fatal("malformed provider-resolved group escaped admission")
				}

				return
			}

			if !result.OK || calls.Load() != 2 || !strings.Contains(string(result.Result), "member@example.test") {
				t.Fatalf("native membership listing failed: %s", result.Body)
			}
		})
	}
}
