package runner

import "net/http"

func nativeDocsTabBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 3)
	for _, name := range []string{"docs_add_tab", "docs_rename_tab", "docs_delete_tab"} {
		bindings = append(bindings, NativeToolBinding{Name: name, Service: "docs", ResourceType: resourceTypeFile, ResourceArgument: "document_id", Write: true, destructive: name == "docs_delete_tab", hosts: []string{"docs.googleapis.com"}, scopes: []string{"documents", driveFileScope}, methods: []string{http.MethodGet, http.MethodPost}, paths: func(id string) []string {
			return []string{"/v1/documents/" + id, "/v1/documents/" + id + ":batchUpdate"}
		}})
	}

	return bindings
}
