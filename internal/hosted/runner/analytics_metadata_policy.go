package runner

import (
	"net/http"
	"regexp"
	"strings"
)

var positiveAnalyticsPropertyID = regexp.MustCompile(`^[1-9][0-9]*$`)

func nativeAnalyticsMetadataBindings() []NativeToolBinding {
	return []NativeToolBinding{{
		Name: "analytics_metadata_get", Service: ServiceAnalytics, ResourceType: ResourceTypeProperty, ResourceArgument: ResourceTypeProperty,
		hosts: []string{"analyticsdata.googleapis.com"}, scopes: []string{"analytics.readonly"},
		methods: []string{http.MethodGet}, paths: exactPaths("/v1beta/", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			property, _ := args["property"].(string)
			if !canonicalPropertyID.MatchString(property) || !positiveAnalyticsPropertyID.MatchString(strings.TrimPrefix(property, "properties/")) {
				return nil, invalidf("invalid property selector")
			}

			return []string{"/v1beta/" + property + "/metadata"}, nil
		},
	}}
}
