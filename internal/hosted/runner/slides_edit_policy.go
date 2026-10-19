package runner

import "net/http"

func nativeSlidesEditBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{
			Name: "slides_replace_text", Service: "slides", ResourceType: resourceTypeFile,
			ResourceArgument: "presentation_id", Write: true,
			hosts: []string{nativeSlidesHost}, scopes: []string{"presentations", driveFileScope},
			methods: []string{http.MethodGet, http.MethodPost}, paths: func(id string) []string {
				return []string{"/v1/presentations/" + id, "/v1/presentations/" + id + ":batchUpdate"}
			},
		},
		{Name: "slides_skip_slide", Service: "slides", ResourceType: resourceTypeFile, ResourceArgument: "presentation_id", Write: true, hosts: []string{nativeSlidesHost}, scopes: []string{"presentations", driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/presentations/", ":batchUpdate")},
		{Name: "slides_unskip_slide", Service: "slides", ResourceType: resourceTypeFile, ResourceArgument: "presentation_id", Write: true, hosts: []string{nativeSlidesHost}, scopes: []string{"presentations", driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/presentations/", ":batchUpdate")},
	}
}
