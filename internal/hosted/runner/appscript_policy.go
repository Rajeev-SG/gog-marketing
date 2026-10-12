package runner

import "net/http"

const (
	scriptProjectCreate = "create"
	scriptProjectGet    = "get"
)

func nativeAppScriptProjectBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 3)

	for _, operation := range []string{scriptProjectGet, "content", scriptProjectCreate} {
		scopes := []string{"script.projects"}

		method := http.MethodPost
		if operation != scriptProjectCreate {
			method = http.MethodGet

			scopes = append(scopes, "script.projects.readonly")
		}
		result = append(result, NativeToolBinding{Name: "appscript_" + operation, Service: "appscript", Write: operation == scriptProjectCreate, hosts: []string{"script.googleapis.com"}, methods: []string{method}, scopes: scopes, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			if operation == scriptProjectCreate {
				return []string{"/v1/projects"}, nil
			}

			id, _ := args["script_id"].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid script project ID")
			}

			path := "/v1/projects/" + id
			if operation == "content" {
				path += "/content"
			}

			return []string{path}, nil
		}})
	}

	return result
}
