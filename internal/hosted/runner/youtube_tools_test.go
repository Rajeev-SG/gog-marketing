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

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeYouTubePlaylistWritesRequireYouTubeNotReadScope(t *testing.T) {
	var calls atomic.Int64
	var playlist youtube.Playlist
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		factory := func(ctx context.Context, _ string) (*youtube.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				var body any = &playlist
				status := 200

				switch r.Method {
				case http.MethodPost:
					if err := json.NewDecoder(r.Body).Decode(&playlist); err != nil {
						t.Error(err)
					}

					playlist.Id = "PLbeta123"
					if playlist.Status == nil || playlist.Status.PrivacyStatus != "private" {
						t.Error("native private privacy default lost")
					}
				case http.MethodGet:
					if r.URL.Query().Get("mine") != "true" {
						t.Error("playlist listing lost connected-account selector")
					}

					items := []*youtube.Playlist{}
					if playlist.Id != "" {
						items = append(items, &playlist)
					}
					body = map[string]any{"items": items}
				case http.MethodDelete:
					if r.URL.Query().Get("id") != "PLbeta123" {
						t.Error("delete playlist ID changed")
					}
					playlist = youtube.Playlist{}
					status = 204
				default:
					t.Errorf("undeclared playlist method: %s", r.Method)
				}

				if r.URL.Host != "youtube.googleapis.com" || r.URL.Path != "/youtube/v3/playlists" {
					t.Errorf("undeclared playlist endpoint: %s", r.URL)
				}

				data, err := json.Marshal(body)
				if err != nil {
					return nil, fmt.Errorf("encode fixture: %w", err)
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})

			return youtube.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
		engine.NativeServices.YouTubeAccount = factory
		engine.NativeServices.YouTubeWrite = factory
	})

	call := func(name, scope string, destructive bool, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "youtube", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"youtube_playlists_create", map[string]any{"title": "Beta playlist"}},
		{"youtube_playlists_list", map[string]any{}},
		{"youtube_playlists_delete", map[string]any{"playlist_id": "PLbeta123"}},
	} {
		binding, _ := nativeBinding(test.name)
		if binding.Write {
			before := calls.Load()

			denied := call(test.name, "youtube.readonly", true, test.args)
			if denied.OK || calls.Load() != before {
				t.Fatal("read-only YouTube scope authorized mutation")
			}
		}

		if binding.destructive {
			before := calls.Load()

			denied := call(test.name, "youtube", false, test.args)
			if denied.OK || calls.Load() != before {
				t.Fatal("playlist deletion bypassed destructive permission")
			}
		}

		result := call(test.name, "youtube", true, test.args)
		if !result.OK {
			t.Fatalf("native %s failed: %s", test.name, result.Body)
		}

		if test.name == "youtube_playlists_list" && !strings.Contains(string(result.Result), "Beta playlist") {
			t.Fatal("playlist creation didn't persist in fixture list")
		}
	}

	if playlist.Id != "" {
		t.Fatal("playlist deletion didn't persist")
	}
	before := calls.Load()

	bad := call("youtube_playlists_delete", "youtube", true, map[string]any{"playlist_id": "../foreign"})
	if bad.OK || calls.Load() != before {
		t.Fatal("invalid playlist selector reached Google")
	}
}
