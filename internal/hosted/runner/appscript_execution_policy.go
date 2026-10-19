package runner

import "net/http"

func nativeAppScriptExecutionBinding() NativeToolBinding {
	return NativeToolBinding{
		Name: "appscript_run", Service: "appscript", Write: true, destructive: true,
		hosts: []string{"script.googleapis.com"}, methods: []string{http.MethodPost},
		scopes: []string{"userinfo.email", "documents", "spreadsheets"}, paths: exactPaths("", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			id, _ := args["deployment_id"].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid executable deployment ID")
			}

			return []string{"/v1/scripts/" + id + ":run"}, nil
		},
	}
}

// validateAppScriptExecution rejects appscript_run invocations whose trusted,
// exact-body-signed authorization carries BigQuery scope: arbitrary scripts
// cannot enforce BigQuery max_bytes_billed safeguards, so execution must use a
// connection without BigQuery authority. Runs after policy.ValidateScopes and
// before any Google request; inspects signed authorization metadata only,
// never model arguments.
func validateAppScriptExecution(invocation *ToolInvocation) error {
	if invocation.Name != "appscript_run" {
		return nil
	}

	for _, scope := range invocation.Authorization.Scopes {
		if scope == "https://www.googleapis.com/auth/bigquery" {
			return invalidf("appscript_run invocation authorized with %q: arbitrary scripts cannot enforce BigQuery max_bytes_billed; use a connection without BigQuery authority", scope)
		}
	}

	return nil
}
