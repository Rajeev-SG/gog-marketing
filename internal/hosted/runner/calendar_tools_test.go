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

func TestNativeCalendarEventCRUDAndScopeDenials(t *testing.T) {
	id := "owner@example.test"
	var calls atomic.Int64
	var stored calendar.Event
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Calendar = func(ctx context.Context, _ string) (*calendar.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				root := "/calendar/v3/calendars/" + id
				var body any = &stored
				status := 200

				switch {
				case r.Method == http.MethodGet && r.URL.Path == root:
					body = map[string]any{"timeZone": "UTC"}
				case r.Method == http.MethodPost && r.URL.Path == root+"/events":
					if err := json.NewDecoder(r.Body).Decode(&stored); err != nil {
						t.Error(err)
					}
					stored.Id = "event-1"
					stored.Etag = "synthetic-etag"
				case r.Method == http.MethodGet && r.URL.Path == root+"/events/event-1":
				case r.Method == http.MethodPatch && r.URL.Path == root+"/events/event-1":
					var patch map[string]any
					if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
						t.Error(err)
					}

					if value, ok := patch["summary"].(string); ok {
						stored.Summary = value
					}

					if value, ok := patch["description"].(string); ok {
						stored.Description = value
					}
				case r.Method == http.MethodDelete && r.URL.Path == root+"/events/event-1":
					stored = calendar.Event{}
					status = 204
				default:
					t.Errorf("foreign or unsupported request: %s %s", r.Method, r.URL)
					status = 500
				}

				encoded, err := json.Marshal(body)
				if err != nil {
					return nil, fmt.Errorf("encode fixture: %w", err)
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r}, nil
			})

			return calendar.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(operation, scope string, destructive bool, args map[string]any) capturedResponse {
		name := "calendar_event_" + operation
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "calendar", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "calendar", ResourceType: "calendar", ResourceID: id, Enabled: true}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, operation := range []string{"create", "get", "update", "get", "delete"} {
		args := map[string]any{"calendar_id": id}
		if operation != "create" {
			args["event_id"] = "event-1"
		}

		if operation == "create" {
			args["summary"] = "Before"
			args["from"] = "2026-10-12T10:00:00Z"
			args["to"] = "2026-10-12T11:00:00Z"
			args["description"] = "Before note"
		}

		if operation == "update" {
			args["summary"] = "After"
			args["description"] = ""
		}

		if operation != "get" {
			before := calls.Load()

			denied := call(operation, "calendar.readonly", true, args)
			if denied.OK || calls.Load() != before {
				t.Fatal("read scope authorized Calendar mutation")
			}
		}

		if operation == "delete" {
			before := calls.Load()

			denied := call(operation, "calendar", false, args)
			if denied.OK || calls.Load() != before {
				t.Fatal("Calendar delete bypassed destructive permission")
			}
		}

		result := call(operation, "calendar", true, args)
		if !result.OK {
			t.Fatalf("native %s failed: %s", operation, result.Body)
		}

		if operation == "update" && (stored.Summary != "After" || stored.Description != "") {
			t.Fatal("Calendar update/clear did not persist")
		}
	}

	if stored.Id != "" {
		t.Fatal("Calendar deletion did not persist")
	}
	before := calls.Load()

	denied := call("get", "calendar", false, map[string]any{"calendar_id": "foreign@example.test", "event_id": "event-1"})
	if denied.OK || calls.Load() != before {
		t.Fatal("foreign calendar reached Google")
	}
}
