package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tagmanager/v2"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeGTMPublishEnforcesContainerScopeAndDestructiveGrant(t *testing.T) {
	var calls atomic.Int64
	var compilerFailure atomic.Bool
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.TagManager = func(ctx context.Context, _ string) (*tagmanager.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodPost || r.URL.Host != "tagmanager.googleapis.com" || r.URL.Path != "/tagmanager/v2/accounts/111/containers/222/versions/7:publish" || r.URL.Query().Get("fingerprint") != "expected-fingerprint" {
					t.Errorf("unexpected publish request: %s %s", r.Method, r.URL)
				}

				body := `{"containerVersion":{"containerVersionId":"7","path":"accounts/111/containers/222/versions/7"}}`
				if compilerFailure.Load() {
					body = `{"compilerError":true}`
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return tagmanager.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(mutate func(*ToolInvocation)) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: "tagmanager_versions_publish", Arguments: map[string]any{"container_path": "accounts/111/containers/222", "version": "7", "fingerprint": "expected-fingerprint"}, Authorization: ToolAuthorization{
				Service: "tagmanager", AllowedTools: []string{"tagmanager_versions_publish"}, Scopes: []string{"https://www.googleapis.com/auth/tagmanager.publish"},
				Resource: &ResourceGrantInput{Service: "tagmanager", ResourceType: "container", ResourceID: "accounts/111/containers/222", Enabled: true}, AllowWrite: true, AllowDestructive: true,
			}}
			if mutate != nil {
				mutate(req.Tool)
			}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, mutate := range []func(*ToolInvocation){
		func(tool *ToolInvocation) { tool.Authorization.AllowWrite = false },
		func(tool *ToolInvocation) { tool.Authorization.AllowDestructive = false },
		func(tool *ToolInvocation) {
			tool.Authorization.Scopes = []string{"https://www.googleapis.com/auth/tagmanager.edit.containerversions"}
		},
		func(tool *ToolInvocation) { tool.Arguments["container_path"] = "accounts/111/containers/333" },
		func(tool *ToolInvocation) { tool.Arguments["version"] = "7/../../foreign" },
		func(tool *ToolInvocation) { tool.Authorization.Resource.Enabled = false },
	} {
		response := call(mutate)
		if response.Status != 400 || calls.Load() != 0 {
			t.Fatalf("denied publication reached Google: %s", response.Body)
		}
	}

	published := call(nil)
	if !published.OK || calls.Load() != 1 || !strings.Contains(string(published.Result), `"containerVersionId":"7"`) {
		t.Fatalf("native publish failed: %s", published.Body)
	}

	compilerFailure.Store(true)

	failed := call(nil)
	if failed.OK || failed.Status != 502 || calls.Load() != 2 {
		t.Fatalf("compiler failure was reported as successful publication: %s", failed.Body)
	}
}
