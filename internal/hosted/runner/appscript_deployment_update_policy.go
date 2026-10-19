package runner

import "net/http"

func nativeAppScriptDeploymentUpdateBinding() NativeToolBinding {
	return NativeToolBinding{
		Name: "appscript_deployment_update", Service: "appscript", Write: true, destructive: true,
		hosts:   []string{"script.googleapis.com"},
		methods: []string{http.MethodGet, http.MethodPut},
		scopes:  []string{"script.deployments"},
		paths:   exactPaths("", ""),
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
	}
}
