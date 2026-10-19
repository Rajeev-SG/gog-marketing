package runner

import "net/http"

// Native Admin operations are bound to the selected property and opaque child ID.
// Payloads are decoded by the existing typed command handlers, never proxied.
func nativeAnalyticsAdminBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 25)

	for _, family := range []struct{ command, api string }{{"datastreams", "dataStreams"}, {"keyevents", "keyEvents"}, {"custom-dimensions", "customDimensions"}, {"custom-metrics", "customMetrics"}, {"googleads-links", "googleAdsLinks"}} {
		operations := []struct {
			name, method string
			item         bool
		}{
			{"list", http.MethodGet, false}, {scriptProjectGet, http.MethodGet, true}, {"create", http.MethodPost, false}, {"update", http.MethodPatch, true}, {"delete", http.MethodDelete, true},
		}
		if family.api == "customDimensions" || family.api == "customMetrics" {
			operations[4].name = "archive"
			operations[4].method = http.MethodPost
		}

		for _, operation := range operations {
			scopes := []string{"analytics.edit"}
			if operation.method == http.MethodGet {
				scopes = append(scopes, "analytics.readonly")
			}
			result = append(result, NativeToolBinding{
				Name: "analytics_" + family.command + "_" + operation.name, Service: "analytics", ResourceType: "property", ResourceArgument: "property", Write: operation.method != http.MethodGet,
				hosts: []string{"analyticsadmin.googleapis.com"}, scopes: scopes, methods: []string{operation.method}, destructive: operation.name == "delete" || operation.name == "archive", paths: exactPaths("", ""),
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

						if family.api != "googleAdsLinks" || operation.name != scriptProjectGet {
							path += "/" + id
						}
					}

					if operation.name == "archive" {
						path += ":archive"
					}

					return []string{path}, nil
				},
			})
		}
	}

	return append(result, nativeAnalyticsRetentionBindings()...)
}
