package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestCalendarEventsUsesOnlyExplicitCalendarWithoutAmbientConfig(t *testing.T) {
	for _, id := range []string{"owner+beta@example.test", "en.uk#holiday@group.v.calendar.google.com"} {
		t.Run(id, func(t *testing.T) {
			var calls atomic.Int64
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Calendar = func(ctx context.Context, _ string) (*calendar.Service, error) {
					transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls.Add(1)
						body := `{"timeZone":"Pacific/Auckland"}`

						switch r.URL.Path {
						case "/calendar/v3/calendars/" + id:
						case "/calendar/v3/calendars/" + id + "/events":
							if !strings.HasPrefix(r.URL.Query().Get("timeMin"), "2026-10-12T00:00:00+13:00") {
								t.Errorf("range did not use selected calendar timezone: %s", r.URL)
							}
							body = `{"timeZone":"Pacific/Auckland","items":[{"id":"event-1","summary":"Fixture event","start":{"dateTime":"2026-10-12T10:00:00+13:00"},"end":{"dateTime":"2026-10-12T11:00:00+13:00"}}]}`
						default:
							t.Errorf("unselected calendar/settings path reached provider: %s", r.URL)
						}

						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})

					return calendar.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
				}
			})
			call := func(selected string, enabled bool) capturedResponse {
				body := nativeToolBody(t, func(req *Request) {
					req.Tool = &ToolInvocation{Name: "calendar_events", Arguments: map[string]any{"calendar_id": selected, "from": "2026-10-12", "to": "2026-10-13"}, Authorization: ToolAuthorization{Service: "calendar", AllowedTools: []string{"calendar_events"}, Scopes: []string{"https://www.googleapis.com/auth/calendar.readonly"}, Resource: &ResourceGrantInput{Service: "calendar", ResourceType: "calendar", ResourceID: id, Enabled: enabled}}}
				})

				return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			}

			result := call(id, true)
			if !result.OK || !strings.Contains(string(result.Result), "event-1") || calls.Load() == 0 {
				t.Fatalf("explicit native calendar execution failed: %s", result.Body)
			}

			before := calls.Load()
			for _, denied := range []capturedResponse{call("foreign@example.test", true), call(id, false)} {
				if denied.OK || calls.Load() != before {
					t.Fatal("foreign/disabled calendar reached Google")
				}
			}
		})
	}
}
