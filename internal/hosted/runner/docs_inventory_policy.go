package runner

import (
	"net/http"
)

// Native Docs inventory reads use exact-tool and per-file grants and fixed GET
// paths. The separately defined named-range edits appended below retain their
// explicit write/destructive authority and GET/POST method policy.
func nativeDocsInventoryBindings() []NativeToolBinding {
	names := []string{
		"docs_info", "docs_list_tabs", "docs_structure", "docs_headings_list",
		"docs_paragraphs_list", "docs_tables_list", "docs_images_list",
		"docs_named_range_list", "docs_find_range", "docs_header_list", "docs_footer_list", "docs_suggestions_list",
	}

	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, nativeDocsInventoryBinding(name))
	}

	return append(bindings, nativeDocsNamedEditBindings()...)
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
