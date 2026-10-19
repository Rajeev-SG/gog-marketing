package runner

import (
	"net/http"
	"net/url"

	"github.com/openclaw/gogcli/internal/hosted/native"
)

const nativeTasksServiceName = "tasks"

func nativeTasksLifecycleBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 4)

	for _, op := range []struct {
		name, method string
		destructive  bool
	}{
		{scriptProjectGet, http.MethodGet, false}, {"rename", http.MethodPatch, false}, {"delete", http.MethodDelete, true},
	} {
		scopes := []string{nativeTasksServiceName}
		if op.method == http.MethodGet {
			scopes = append(scopes, "tasks.readonly")
		}
		result = append(result, NativeToolBinding{
			Name: "tasks_lists_" + op.name, Service: nativeTasksServiceName, Write: op.method != http.MethodGet, destructive: op.destructive, hosts: []string{"tasks.googleapis.com"}, scopes: scopes, methods: []string{op.method}, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				id, _ := args["tasklist_id"].(string)
				if !validLifecycleTaskListID(id) || op.destructive && id == "@default" {
					return nil, invalidf("use an explicit opaque non-default list for deletion")
				}

				return []string{"/tasks/v1/users/@me/lists/" + id}, nil
			},
		})
	}

	result = append(result, NativeToolBinding{Name: "tasks_move", Service: nativeTasksServiceName, Write: true, hosts: []string{"tasks.googleapis.com"}, scopes: []string{nativeTasksServiceName}, methods: []string{http.MethodPost}, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
		list, _ := args["tasklist_id"].(string)

		task, _ := args["task_id"].(string)
		if !validLifecycleTaskListID(list) || !taskOpaqueID.MatchString(task) {
			return nil, invalidf("invalid move source selectors")
		}

		for _, key := range []string{"parent", "previous", "destination_tasklist_id"} {
			if raw, present := args[key]; present {
				value, ok := raw.(string)
				if !ok {
					return nil, invalidf("move selectors must be strings")
				}

				if value == "" {
					continue
				}

				if key == "destination_tasklist_id" {
					if !validLifecycleTaskListID(value) {
						return nil, invalidf("invalid destination list")
					}
				} else if !taskOpaqueID.MatchString(value) {
					return nil, invalidf("invalid related task ID")
				}
			}
		}

		return []string{"/tasks/v1/lists/" + list + "/tasks/" + task + "/move"}, nil
	}})

	return result
}

func validLifecycleTaskListID(id string) bool {
	return id == "@default" || len(id) >= 16 && taskOpaqueID.MatchString(id)
}

func admitTasksMoveQuery(invocation *ToolInvocation, request *http.Request) error {
	expected := map[string]string{"parent": "parent", "previous": "previous", "destinationTasklist": "destination_tasklist_id"}

	query, queryErr := url.ParseQuery(request.URL.RawQuery)
	if queryErr != nil {
		return native.ErrDenied
	}
	for key, values := range query {
		if len(values) != 1 {
			return native.ErrDenied
		}

		if key == "alt" {
			if values[0] != "json" {
				return native.ErrDenied
			}

			continue
		}

		if key == "prettyPrint" {
			if values[0] != "false" {
				return native.ErrDenied
			}

			continue
		}

		argument, known := expected[key]
		if !known {
			return native.ErrDenied
		}

		value, _ := invocation.Arguments[argument].(string)
		if value == "" || value != values[0] {
			return native.ErrDenied
		}
	}

	for key, argument := range expected {
		value, _ := invocation.Arguments[argument].(string)
		if value != "" && query.Get(key) != value {
			return native.ErrDenied
		}
	}

	return nil
}

func admitScopedToolQuery(binding NativeToolBinding, invocation *ToolInvocation, request *http.Request) error {
	if binding.Service == "youtube" {
		return admitYouTubePlaylistQuery(invocation, request)
	}

	if invocation.Name == "tasks_move" {
		return admitTasksMoveQuery(invocation, request)
	}

	return nil
}
