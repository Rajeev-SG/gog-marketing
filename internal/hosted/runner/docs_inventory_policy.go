package runner

import (
	"net/http"
)

// Native Docs inventory is read-only. Each tool reads one selected document
// through an account-level exact tool grant and a per-file resource grant.
// The document id is validated against the same file resource id pattern used
// for Drive resources; only the fixed GET /v1/documents/<id> path is ever
// permitted.
func nativeDocsInventoryBindings() []NativeToolBinding {
	names := []string{
		"docs_info", "docs_list_tabs", "docs_structure", "docs_headings_list",
		"docs_paragraphs_list", "docs_tables_list", "docs_images_list",
		"docs_named_range_list",
	}
	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, nativeDocsInventoryBinding(name))
	}
	return bindings
}

func nativeDocsInventoryBinding(name string) NativeToolBinding {
	return NativeToolBinding{
		Name:             name,
		Service:          "docs",
		ResourceType:     resourceTypeFile,
		ResourceArgument: "document_id",
		Write:            false,
		hosts:            []string{"docs.googleapis.com"},
		scopes:           []string{"documents.readonly", "documents", driveFileScope},
		methods:          []string{http.MethodGet},
		paths:            exactPaths("", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			id, _ := args["document_id"].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid document id")
			}

			return []string{"/v1/documents/" + id}, nil
		},
	}
}
