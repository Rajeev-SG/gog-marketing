package runner

import (
	"net/http"
	"regexp"
)

var positiveAnalyticsPropertyID = regexp.MustCompile(`^[1-9][0-9]*$`)

func nativeAnalyticsMetadataBindings() []NativeToolBinding {
	return []NativeToolBinding{{
		Name: "analytics_metadata_get", Service: ServiceAnalytics, ResourceType: "property", ResourceArgument: "property_id",
		hosts: []string{"analyticsdata.googleapis.com"}, scopes: []string{"analytics.readonly", "analytics"},
		methods: []string{http.MethodGet}, paths: exactPaths("/v1beta/", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			property, _ := args["property_id"].(string)
			if !positiveAnalyticsPropertyID.MatchString(property) {
				return nil, invalidf("invalid property ID")
			}
			return []string{"/v1beta/properties/" + property + "/metadata"}, nil
		},
	}}
}
