package runner

import "net/http"

const (
	slidesLifecycleService           = "slides"
	slidesLifecyclePresentationArg   = "presentation_id"
	slidesLifecyclePresentationsScop = "presentations"
)

func nativeSlidesLifecycleBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "slides_new_slide", Service: slidesLifecycleService, ResourceType: resourceTypeFile, ResourceArgument: slidesLifecyclePresentationArg, Write: true, hosts: []string{nativeSlidesHost}, scopes: []string{slidesLifecyclePresentationsScop, driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/presentations/", ":batchUpdate")},
		{Name: "slides_duplicate_slide", Service: slidesLifecycleService, ResourceType: resourceTypeFile, ResourceArgument: slidesLifecyclePresentationArg, Write: true, hosts: []string{nativeSlidesHost}, scopes: []string{slidesLifecyclePresentationsScop, driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/presentations/", ":batchUpdate")},
		{Name: "slides_move_slide", Service: slidesLifecycleService, ResourceType: resourceTypeFile, ResourceArgument: slidesLifecyclePresentationArg, Write: true, hosts: []string{nativeSlidesHost}, scopes: []string{slidesLifecyclePresentationsScop, driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/presentations/", ":batchUpdate")},
		{Name: "slides_delete_slide", Service: slidesLifecycleService, ResourceType: resourceTypeFile, ResourceArgument: slidesLifecyclePresentationArg, Write: true, hosts: []string{nativeSlidesHost}, scopes: []string{slidesLifecyclePresentationsScop, driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/presentations/", ":batchUpdate")},
	}
}
