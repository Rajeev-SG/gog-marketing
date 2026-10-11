package runner

import (
	"context"
	"testing"

	"google.golang.org/api/calendar/v3"
)

// Known engineering blocker: native Events currently requires local alias
// configuration. Preserve the hosted no-ambient-config boundary until its
// explicit-ID execution path is fixed; this is not Calendar acceptance proof.
func TestCalendarEventsFailsClosedBeforeAmbientAliasConfigForDiscoveredIDs(t *testing.T) {
	for _, id := range []string{"owner+beta@example.test", "en.uk#holiday@group.v.calendar.google.com"} {
		t.Run(id, func(t *testing.T) {
			factoryCalls := 0
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices.Calendar = func(context.Context, string) (*calendar.Service, error) {
					factoryCalls++
					return nil, errUnexpectedNativeFactory
				}
			})
			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{Name: "calendar_events", Arguments: map[string]any{"calendar_id": id, "from": "2026-10-12T00:00:00Z", "to": "2026-10-13T00:00:00Z"}, Authorization: ToolAuthorization{Service: "calendar", AllowedTools: []string{"calendar_events"}, Scopes: []string{"https://www.googleapis.com/auth/calendar.readonly"}, Resource: &ResourceGrantInput{Service: "calendar", ResourceType: "calendar", ResourceID: id, Enabled: true}}}
			})

			result := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			if result.OK || result.Status != 403 || factoryCalls != 0 {
				t.Fatalf("ambient-config boundary did not fail closed before Google: %s, factory calls=%d", result.Body, factoryCalls)
			}
		})
	}
}
