package runner

import "net/http"

func nativeAppScriptManagementBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 3)
	for _, op := range []struct {
		name, path, scope, method string
		destructive               bool
	}{
		{"push", "content", "script.projects", http.MethodPut, true},
		{"version_create", "versions", "script.projects", http.MethodPost, false},
		{"deployment_create", "deployments", "script.deployments", http.MethodPost, true},
	} {
		result = append(result, NativeToolBinding{
			Name: "appscript_" + op.name, Service: "appscript", Write: true, destructive: op.destructive,
			hosts: []string{"script.googleapis.com"}, methods: []string{op.method}, scopes: []string{op.scope}, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				id, _ := args["script_id"].(string)
				if !fileResourceID.MatchString(id) {
					return nil, invalidf("invalid script project ID")
				}

				return []string{"/v1/projects/" + id + "/" + op.path}, nil
			},
		})
	}

	result = append(result, NativeToolBinding{
		Name: "appscript_deployment_delete", Service: "appscript", Write: true, destructive: true,
		hosts: []string{"script.googleapis.com"}, methods: []string{http.MethodDelete}, scopes: []string{"script.deployments"}, paths: exactPaths("", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			scriptID, _ := args["script_id"].(string)
			if !fileResourceID.MatchString(scriptID) {
				return nil, invalidf("invalid script project ID")
			}
			deploymentID, _ := args["deployment_id"].(string)
			if !fileResourceID.MatchString(deploymentID) {
				return nil, invalidf("invalid deployment ID")
			}

			return []string{"/v1/projects/" + scriptID + "/deployments/" + deploymentID}, nil
		},
	})

	return append(result, NativeToolBinding{
		Name: "appscript_bound_create", Service: "appscript", ResourceType: resourceTypeFile, ResourceArgument: "parent_id", Write: true,
		hosts: []string{"script.googleapis.com"}, methods: []string{http.MethodPost}, scopes: []string{"script.projects"}, paths: exactPaths("", ""), argumentPaths: func(map[string]any) ([]string, error) { return []string{"/v1/projects"}, nil },
	})
}
