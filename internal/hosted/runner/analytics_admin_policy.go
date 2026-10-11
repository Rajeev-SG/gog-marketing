package runner

import "net/http"

// Native Admin operations are bound to the selected property and opaque child ID.
// Payloads are decoded by the existing typed command handlers, never proxied.
func nativeAnalyticsAdminBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 10)

	for _, family := range []struct{ command, api string }{{"datastreams", "dataStreams"}, {"keyevents", "keyEvents"}} {
		for _, operation := range []struct {
			name, method string
			item         bool
		}{
			{"list", http.MethodGet, false}, {"get", http.MethodGet, true}, {"create", http.MethodPost, false}, {"update", http.MethodPatch, true}, {"delete", http.MethodDelete, true},
		} {
			scopes := []string{"analytics.edit"}
			if operation.method == http.MethodGet {
				scopes = append(scopes, "analytics.readonly")
			}
			result = append(result, NativeToolBinding{
				Name: "analytics_" + family.command + "_" + operation.name, Service: "analytics", ResourceType: "property", ResourceArgument: "property", Write: operation.method != http.MethodGet,
				hosts: []string{"analyticsadmin.googleapis.com"}, scopes: scopes, methods: []string{operation.method}, destructive: operation.name == "delete", paths: exactPaths("", ""),
				argumentPaths: func(args map[string]any) ([]string, error) {
					property, _ := args["property"].(string)
					if !canonicalPropertyID.MatchString(property) {
						return nil, invalidf("invalid property selector")
					}
					path := "/v1beta/" + property + "/" + family.api

					if operation.item {
						id, _ := args["resource_id"].(string)
						if !fileResourceID.MatchString(id) {
							return nil, invalidf("invalid Admin child selector")
						}
						path += "/" + id
					}

					return []string{path}, nil
				},
			})
		}
	}

	return result
}
