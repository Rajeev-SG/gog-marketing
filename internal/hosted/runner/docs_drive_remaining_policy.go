package runner

import (
	"net/http"
	"strings"

	"github.com/openclaw/gogcli/internal/hosted/native"
)

const (
	remainingDocsService    = "docs"
	remainingDriveService   = "drive"
	remainingFileArgument   = "file_id"
	remainingDocumentsScope = "documents"
)

func nativeDocsDriveRemainingBindings() []NativeToolBinding {
	names := []string{"docs_cell_style", "docs_cell_update", "docs_table_column_width", "docs_table_row_style", "docs_update", "docs_sed", "docs_copy", "docs_raw", "drive_raw", "drive_url", "drive_mkdir", "drive_share", "drive_unshare", "drive_bulk_remove_public", "drive_bulk_update_role"}

	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		service, key := remainingDocsService, "document_id"
		if strings.HasPrefix(name, "drive_") {
			service, key = remainingDriveService, remainingFileArgument
		}

		if name == "drive_mkdir" {
			key = "parent_id"
		}
		write := name != "docs_raw" && name != "drive_raw" && name != "drive_url"

		binding := NativeToolBinding{Name: name, Service: service, ResourceType: resourceTypeFile, ResourceArgument: key, Write: write, hosts: []string{nativeDocsSlidesDriveHost}, scopes: []string{driveFileScope}, methods: []string{http.MethodGet}, paths: exactPaths("", "")}
		if service == remainingDocsService && name != "docs_copy" {
			binding.hosts = []string{"docs.googleapis.com"}

			binding.scopes = []string{remainingDocumentsScope, driveFileScope}
			if !write {
				binding.scopes = append(binding.scopes, "documents.readonly")
			}
		}

		if write {
			binding.methods = []string{http.MethodGet, http.MethodPost}
		}

		switch name {
		case "drive_mkdir":
			binding.methods = []string{http.MethodPost}
		case "drive_unshare":
			binding.methods = []string{http.MethodDelete}
		case "drive_bulk_remove_public":
			binding.methods = []string{http.MethodGet, http.MethodDelete}
		case "drive_bulk_update_role":
			binding.methods = []string{http.MethodGet, http.MethodPatch}
		}
		binding.destructive = name == "docs_sed" || name == "drive_share" || name == "drive_unshare" || strings.HasPrefix(name, "drive_bulk_")
		binding.argumentPaths = func(args map[string]any) ([]string, error) {
			id, _ := args[key].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid selected resource")
			}
			base := "/drive/v3/files/" + id

			switch name {
			case "docs_copy":
				return []string{base, base + "/copy"}, nil
			case "drive_mkdir":
				return []string{"/drive/v3/files"}, nil
			case "drive_share":
				return []string{base, base + "/permissions"}, nil
			case "drive_unshare":
				permission, _ := args["permission_id"].(string)
				if !fileResourceID.MatchString(permission) {
					return nil, invalidf("invalid permission")
				}

				return []string{base + "/permissions/" + permission}, nil
			case "drive_bulk_remove_public", "drive_bulk_update_role":
				return []string{base, base + "/permissions"}, nil
			case "drive_raw", "drive_url":
				return []string{base}, nil
			default:
				base = "/v1/documents/" + id
				if write {
					return []string{base, base + ":batchUpdate"}, nil
				}

				return []string{base}, nil
			}
		}
		bindings = append(bindings, binding)
	}

	return bindings
}

// Only these new tools use the selected-file permission-member suffix. Exact
// operation verbs are enforced here, not by a permissive host/path cross product.
func admitDocsDriveRemainingRequest(invocation *ToolInvocation, request *http.Request) (bool, bool) {
	found := false

	for _, binding := range nativeDocsDriveRemainingBindings() {
		if binding.Name == invocation.Name {
			found = true
			break
		}
	}

	if !found {
		return false, false
	}

	query := request.URL.Query()
	if alt, present := query["alt"]; present && (len(alt) != 1 || alt[0] != "json") {
		return true, false
	}
	path, method := request.URL.Path, request.Method

	key := remainingFileArgument
	if strings.HasPrefix(invocation.Name, "docs_") {
		key = "document_id"
	}

	if invocation.Name == "drive_mkdir" {
		key = "parent_id"
	}
	id, _ := invocation.Arguments[key].(string)
	base := "/drive/v3/files/" + id

	switch invocation.Name {
	case "docs_copy":
		return true, (path == base && method == http.MethodGet) || (path == base+"/copy" && method == http.MethodPost)
	case "drive_mkdir":
		return true, path == "/drive/v3/files" && method == http.MethodPost
	case "drive_share":
		return true, (path == base && method == http.MethodGet) || (path == base+"/permissions" && method == http.MethodPost)
	case "drive_unshare":
		permission, _ := invocation.Arguments["permission_id"].(string)
		return true, path == base+"/permissions/"+permission && method == http.MethodDelete
	case "drive_bulk_remove_public", "drive_bulk_update_role":
		if method == http.MethodGet {
			return true, path == base || path == base+"/permissions"
		}

		prefix := base + "/permissions/"
		if !strings.HasPrefix(path, prefix) || !fileResourceID.MatchString(strings.TrimPrefix(path, prefix)) {
			return true, false
		}

		if invocation.Name == "drive_bulk_remove_public" {
			return true, method == http.MethodDelete
		}

		return true, method == http.MethodPatch
	case "drive_raw", "drive_url":
		return true, path == base && method == http.MethodGet
	default:
		base = "/v1/documents/" + id
		return true, (path == base && method == http.MethodGet) || (invocation.Name != "docs_raw" && path == base+":batchUpdate" && method == http.MethodPost)
	}
}

func validateRemainingDocsClear(invocation *ToolInvocation) error {
	if invocation.Name == "docs_cell_update" {
		text, _ := invocation.Arguments["content"].(string)

		appendOnly, _ := invocation.Arguments["append"].(bool)
		if text == "" && !appendOnly && !invocation.Authorization.AllowDestructive {
			return native.ErrDenied
		}
	}

	return nil
}
