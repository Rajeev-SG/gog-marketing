package runner

import "net/http"

func nativeSlidesTableRemainingBindings() []NativeToolBinding {
	names := []string{
		"slides_table_border_style",
		"slides_table_cell_style",
		"slides_table_column_delete",
		"slides_table_column_insert",
		"slides_table_column_size",
		"slides_table_create",
		"slides_table_merge",
		"slides_table_row_delete",
		"slides_table_row_insert",
		"slides_table_row_size",
		"slides_table_unmerge",
	}

	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, NativeToolBinding{
			Name: name, Service: slidesLifecycleService, ResourceType: resourceTypeFile, ResourceArgument: slidesLifecyclePresentationArg,
			Write: true, hosts: []string{nativeSlidesHost},
			scopes:  []string{"presentations", driveFileScope},
			methods: []string{http.MethodGet, http.MethodPost},
			paths: func(id string) []string {
				return []string{"/v1/presentations/" + id, "/v1/presentations/" + id + ":batchUpdate"}
			},
			destructive: name == "slides_table_column_delete" || name == "slides_table_row_delete" || name == "slides_table_merge",
		})
	}

	return bindings
}
