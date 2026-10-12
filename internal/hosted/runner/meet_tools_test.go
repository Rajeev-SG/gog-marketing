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

	"google.golang.org/api/meet/v2"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeMeetSpaceReadWriteAndEndScope(t *testing.T) {
	var calls atomic.Int64
	var space meet.Space
	ended := false
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Meet = func(ctx context.Context, _ string) (*meet.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				var response any = &space

				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v2/spaces":
					if err := json.NewDecoder(r.Body).Decode(&space); err != nil {
						t.Error(err)
					}
					space.Name = "spaces/beta7"
					space.MeetingCode = "abc-defg-hij"
					space.MeetingUri = "https://meet.google.com/abc-defg-hij"
				case r.Method == http.MethodGet && r.URL.Path == "/v2/spaces/beta7":
				case r.Method == http.MethodPatch && r.URL.Path == "/v2/spaces/beta7":
					var patch meet.Space
					if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
						t.Error(err)
					}

					if r.URL.Query().Get("updateMask") == "" {
						t.Error("native configuration mask was lost")
					}
					space.Config = patch.Config
				case r.Method == http.MethodPost && r.URL.Path == "/v2/spaces/beta7:endActiveConference":
					ended = true
					response = map[string]any{}
				default:
					t.Errorf("unexpected meeting request: %s %s", r.Method, r.URL)
				}

				data, err := json.Marshal(response)
				if err != nil {
					return nil, fmt.Errorf("encode fixture: %w", err)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})

			return meet.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(operation, scope string, destructive bool, args map[string]any) capturedResponse {
		name := "meet_space_" + operation
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "meet", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, operation := range []string{"create", "get", "update", "get", "end"} {
		args := map[string]any{}
		if operation != "create" {
			args["space_name"] = "spaces/beta7"
		}

		if operation == "update" {
			args["access"] = "restricted"
		}

		if operation != "get" {
			before := calls.Load()

			denied := call(operation, "meetings.space.readonly", true, args)
			if denied.OK || calls.Load() != before {
				t.Fatal("read scope authorized meeting mutation")
			}
		}

		if operation == "end" {
			before := calls.Load()
			for _, denied := range []capturedResponse{call(operation, "meetings.space.created", false, args), call(operation, "meetings.space.settings", true, args)} {
				if denied.OK || calls.Load() != before {
					t.Fatal("conference end bypassed exact scope/destructive admission")
				}
			}
		}

		result := call(operation, "meetings.space.created", true, args)
		if !result.OK {
			t.Fatalf("native %s failed: %s", operation, result.Body)
		}

		if operation == "update" && (space.Config == nil || space.Config.AccessType != "RESTRICTED") {
			t.Fatal("updated space configuration did not persist")
		}
	}

	if !ended {
		t.Fatal("native conference end was not executed")
	}
	before := calls.Load()

	denied := call("get", "meetings.space.readonly", false, map[string]any{"space_name": "spaces/../foreign"})
	if denied.OK || calls.Load() != before {
		t.Fatal("invalid meeting selector reached Google")
	}
}
