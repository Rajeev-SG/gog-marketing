package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeChatSpaceAndSendDoNotUseRestrictedMessageReads(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Chat = func(ctx context.Context, _ string) (*chat.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				body := `{"name":"spaces/beta","displayName":"Beta space"}`

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/spaces":
					body = `{"spaces":[{"name":"spaces/beta","displayName":"Beta space"}]}`
				case r.Method == http.MethodPost && r.URL.Path == "/v1/spaces:setup":
				case r.Method == http.MethodPost && r.URL.Path == "/v1/spaces/beta/messages":
					var input chat.Message
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}

					if input.Text != "Beta message" {
						t.Errorf("native send text lost: %+v", input)
					}
					body = `{"name":"spaces/beta/messages/message1","text":"Beta message"}`
				default:
					t.Errorf("restricted/undeclared Chat endpoint: %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return chat.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name, scope string, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "chat", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, test := range []struct {
		name, scope string
		args        map[string]any
	}{
		{"chat_spaces_list", "chat.spaces.readonly", map[string]any{}},
		{"chat_spaces_create", "chat.spaces", map[string]any{"display_name": "Beta space"}},
		{"chat_messages_send", "chat.messages.create", map[string]any{"space_name": "spaces/beta", "text": "Beta message"}},
	} {
		result := call(test.name, test.scope, test.args)
		if !result.OK {
			t.Fatalf("native %s failed: %s", test.name, result.Body)
		}
	}
	before := calls.Load()

	for _, test := range []struct {
		name, scope string
		args        map[string]any
	}{
		{"chat_messages_send", "chat.spaces.readonly", map[string]any{"space_name": "spaces/beta", "text": "Beta message"}},
		{"chat_messages_send", "chat.messages", map[string]any{"space_name": "spaces/beta", "text": "Beta message"}},
		{"chat_messages_send", "chat.messages.create", map[string]any{"space_name": "spaces/../foreign", "text": "Beta message"}},
		{"chat_messages_list", "chat.messages.create", map[string]any{}},
	} {
		denied := call(test.name, test.scope, test.args)
		if denied.OK || calls.Load() != before {
			t.Fatal("restricted/invalid operation reached Google")
		}
	}
}
