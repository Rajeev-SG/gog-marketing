package runner

import (
	"net/http"
	"regexp"
)

var taskOpaqueID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}={0,2}$`)

// Account-level Tasks permission covers accessible lists, not Drive files.
// Selectors must be explicit IDs: name resolution cannot change the admitted path.
func nativeTasksItemBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 9)

	for _, operation := range []struct {
		name, method      string
		item, destructive bool
	}{
		{"list", http.MethodGet, false, false},
		{"get", http.MethodGet, true, false},
		{"raw", http.MethodGet, true, false},
		{"add", http.MethodPost, false, false},
		{"update", http.MethodPatch, true, false},
		{"done", http.MethodPatch, true, false},
		{"undo", http.MethodPatch, true, false},
		{"delete", http.MethodDelete, true, true},
		{"clear", http.MethodPost, false, true},
	} {
		scopes := []string{"tasks"}
		if operation.method == http.MethodGet {
			scopes = append(scopes, "tasks.readonly")
		}
		result = append(result, NativeToolBinding{
			Name: "tasks_" + operation.name, Service: "tasks", Write: operation.method != http.MethodGet,
			hosts: []string{"tasks.googleapis.com"}, scopes: scopes, methods: []string{operation.method}, destructive: operation.destructive,
			paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				list, ok := args["tasklist_id"].(string)
				if !ok || (list != "@default" && (len(list) < 16 || !taskOpaqueID.MatchString(list))) {
					return nil, invalidf("use an opaque task-list ID or @default")
				}

				path := "/tasks/v1/lists/" + list + "/tasks"
				if operation.name == "clear" {
					path = "/tasks/v1/lists/" + list + "/clear"
				}

				if operation.item {
					item, ok := args["task_id"].(string)
					if !ok || !taskOpaqueID.MatchString(item) {
						return nil, invalidf("invalid task ID")
					}
					path += "/" + item
				}

				return []string{path}, nil
			},
		})
	}

	return result
}
