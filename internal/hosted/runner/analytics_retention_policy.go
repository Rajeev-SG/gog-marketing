package runner

import "net/http"

func nativeAnalyticsRetentionBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 2)

	for _, op := range []struct {
		name, method string
		write        bool
	}{
		{"get", http.MethodGet, false}, {"update", http.MethodPatch, true},
	} {
		scopes := []string{"analytics.edit"}
		if !op.write {
			scopes = append(scopes, "analytics.readonly")
		}
		bindings = append(bindings, NativeToolBinding{
			Name: "analytics_retention_" + op.name, Service: ServiceAnalytics, ResourceType: "property", ResourceArgument: "property", Write: op.write, destructive: op.write,
			hosts: []string{"analyticsadmin.googleapis.com"}, scopes: scopes, methods: []string{op.method}, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				property, _ := args["property"].(string)
				if !canonicalPropertyID.MatchString(property) {
					return nil, invalidf("invalid property selector")
				}

				return []string{"/v1beta/" + property + "/dataRetentionSettings"}, nil
			},
		})
	}

	return bindings
}
