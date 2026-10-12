package runner

import (
	"net/http"
	"strings"

	"github.com/openclaw/gogcli/internal/hosted/native"
)

const (
	calendarOperationUpdate = "update"
	calendarOperationDelete = "delete"
)

func nativeCalendarEventBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 4)

	for _, operation := range []string{"get", "create", calendarOperationUpdate, calendarOperationDelete} {
		scopes := []string{"calendar"}
		if operation == "get" {
			scopes = append(scopes, "calendar.readonly")
		}
		result = append(result, NativeToolBinding{
			Name: "calendar_event_" + operation, Service: "calendar", ResourceType: "calendar", ResourceArgument: "calendar_id", Write: operation != "get", destructive: operation == calendarOperationDelete, hosts: []string{"www.googleapis.com"}, scopes: scopes, paths: exactPaths("", ""), calendarEventOperation: operation,
			argumentPaths: func(args map[string]any) ([]string, error) {
				id, _ := args["calendar_id"].(string)
				if !calendarResourceID.MatchString(id) {
					return nil, invalidf("invalid calendar ID")
				}

				if operation != "create" {
					event, _ := args["event_id"].(string)
					if !fileResourceID.MatchString(event) {
						return nil, invalidf("invalid event ID")
					}
				}

				return []string{}, nil
			},
		})
	}

	return result
}

func admitCalendarEventRequest(invocation *ToolInvocation, request *http.Request) error {
	operation := strings.TrimPrefix(invocation.Name, "calendar_event_")
	id := invocation.Authorization.Resource.ResourceID
	root := "/calendar/v3/calendars/" + id

	path := request.URL.Path
	if path == root && request.Method == http.MethodGet {
		return nil
	}

	if path == root+"/events" && request.Method == http.MethodPost && (operation == "create" || operation == calendarOperationUpdate) {
		return nil
	}

	if strings.HasPrefix(path, root+"/events/") {
		segments := strings.Split(strings.TrimPrefix(path, root+"/events/"), "/")
		if len(segments) == 1 && fileResourceID.MatchString(segments[0]) {
			if request.Method == http.MethodGet && (operation == "get" || operation == calendarOperationUpdate || operation == calendarOperationDelete) {
				return nil
			}

			if request.Method == http.MethodPatch && (operation == calendarOperationUpdate || operation == calendarOperationDelete) {
				return nil
			}

			if request.Method == http.MethodDelete && operation == calendarOperationDelete {
				return nil
			}
		}

		if len(segments) == 2 && fileResourceID.MatchString(segments[0]) && segments[1] == "instances" && request.Method == http.MethodGet && (operation == calendarOperationUpdate || operation == calendarOperationDelete) {
			return nil
		}
	}

	return native.ErrDenied
}
