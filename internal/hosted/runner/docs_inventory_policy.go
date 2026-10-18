package runner

import (
	"net/http"
)

// Native Docs inventory is read-only. docs_info and docs_list_tabs read one
// selected document through an account-level exact tool grant and a
// per-file resource grant. The document id is validated against the same
// file resource id pattern used for Drive resources; only the fixed GET
// /v1/documents/<id> path is ever permitted.
func nativeDocsInventoryBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{
			Name:             "docs_info",
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
		},
		{
			Name:             "docs_list_tabs",
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
		},
	}
}
