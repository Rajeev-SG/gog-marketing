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

	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeClassroomCourseCRUDAndScopeDenials(t *testing.T) {
	var calls atomic.Int64
	var stored classroom.Course
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Classroom = func(ctx context.Context, _ string) (*classroom.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				var response any = &stored
				status := 200

				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/courses":
					if err := json.NewDecoder(r.Body).Decode(&stored); err != nil {
						t.Error(err)
					}
					stored.CourseState = "ACTIVE"
					stored.Id = "123"
					stored.OwnerId = "owner"
				case r.Method == http.MethodGet && r.URL.Path == "/v1/courses":
					response = map[string]any{"courses": []*classroom.Course{&stored}}
				case r.Method == http.MethodGet && r.URL.Path == "/v1/courses/123":
				case r.Method == http.MethodPatch && r.URL.Path == "/v1/courses/123":
					var patch classroom.Course
					if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
						t.Error(err)
					}

					if !strings.Contains(r.URL.Query().Get("updateMask"), "name") {
						t.Error("native update mask lost")
					}

					stored.Name = patch.Name
					if patch.CourseState != "" {
						stored.CourseState = patch.CourseState
					}
				case r.Method == http.MethodDelete && r.URL.Path == "/v1/courses/123":
					stored = classroom.Course{}
					status = 204
				default:
					t.Errorf("undeclared course endpoint: %s %s", r.Method, r.URL)
				}

				data, err := json.Marshal(response)
				if err != nil {
					return nil, fmt.Errorf("encode fixture: %w", err)
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})

			return classroom.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(operation, scope string, destructive bool, args map[string]any) capturedResponse {
		name := "classroom_courses_" + operation
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "classroom", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, operation := range []string{"create", "list", "get", "update", "get", "delete"} {
		args := map[string]any{}
		if operation == "create" {
			args["name"] = "Before"
		}

		if operation == "get" || operation == "update" || operation == "delete" {
			args["course_id"] = "123"
		}

		if operation == "update" {
			before := calls.Load()

			activeDelete := call("delete", "classroom.courses", true, map[string]any{"course_id": "123"})
			if activeDelete.OK || calls.Load() != before+1 || stored.Id == "" {
				t.Fatal("native active-course archival safeguard was bypassed")
			}
		}

		if operation == "update" {
			args["name"] = "After"
			args["state"] = "ARCHIVED"
		}

		if operation == "create" || operation == "update" || operation == "delete" {
			before := calls.Load()

			denied := call(operation, "classroom.courses.readonly", true, args)
			if denied.OK || calls.Load() != before {
				t.Fatal("read course scope authorized mutation")
			}
		}

		if operation == "delete" {
			before := calls.Load()

			denied := call(operation, "classroom.courses", false, args)
			if denied.OK || calls.Load() != before {
				t.Fatal("course deletion bypassed destructive permission")
			}
		}

		result := call(operation, "classroom.courses", true, args)
		if !result.OK {
			t.Fatalf("native %s failed: %s", operation, result.Body)
		}

		if operation == "update" && stored.Name != "After" {
			t.Fatal("course update didn't persist")
		}
	}

	if stored.Id != "" {
		t.Fatal("course delete didn't persist")
	}
	before := calls.Load()

	invalid := call("get", "classroom.courses.readonly", false, map[string]any{"course_id": "../foreign"})
	if invalid.OK || calls.Load() != before {
		t.Fatal("invalid course selector reached Google")
	}
}
