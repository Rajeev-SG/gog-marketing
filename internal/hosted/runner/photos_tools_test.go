package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativePhotosAppCreatedReadsRejectLegacyGeneralLibraryScopes(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Photos = func(ctx context.Context, _ string) (*googleapi.PhotosClient, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.URL.Host != "photoslibrary.googleapis.com" {
					t.Errorf("foreign photos host: %s", r.URL)
				}
				body := `{"id":"media-1","filename":"beta.jpg","mimeType":"image/jpeg"}`

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/mediaItems":
					body = `{"mediaItems":[{"id":"media-1","filename":"beta.jpg"}],"nextPageToken":"next"}`
				case r.Method == http.MethodPost && r.URL.Path == "/v1/mediaItems:search":
					body = `{"mediaItems":[{"id":"media-1","filename":"beta.jpg"}]}`
				case r.Method == http.MethodGet && r.URL.Path == "/v1/mediaItems/media-1":
				default:
					t.Errorf("undeclared photos endpoint: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return googleapi.NewPhotosClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}), nil
		}
	})
	call := func(name, scope string, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "photos", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}

	denied := call("photos_list", "photoslibrary.readonly", map[string]any{})
	if denied.OK || calls.Load() != 0 {
		t.Fatal("legacy general-library scope was admitted")
	}

	for _, name := range []string{"photos_list", "photos_get", "photos_search"} {
		args := map[string]any{}
		if name == "photos_get" {
			args["media_id"] = "media-1"
		}

		result := call(name, "photoslibrary.readonly.appcreateddata", args)
		if !result.OK || !strings.Contains(string(result.Result), "beta.jpg") {
			t.Fatalf("native %s failed: %s", name, result.Body)
		}
	}
	before := calls.Load()

	invalid := call("photos_get", "photoslibrary.readonly.appcreateddata", map[string]any{"media_id": "../foreign"})
	if invalid.OK || calls.Load() != before {
		t.Fatal("invalid media selector reached Google")
	}
}
