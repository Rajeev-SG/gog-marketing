package runner

import (
	"net/http"
	"strings"

	"github.com/openclaw/gogcli/internal/hosted/native"
	"github.com/openclaw/gogcli/internal/sheetsa1"
)

// These adapters emit only native typed requests on the selected spreadsheet.
func nativeSheetsRemainingBindings() []NativeToolBinding {
	names := []string{"sheets_table_append", "sheets_table_clear", "sheets_copy", "sheets_banding_clear", "sheets_banding_set", "sheets_batch_update", "sheets_chart_create", "sheets_chart_delete", "sheets_chart_update", "sheets_conditional_format_add", "sheets_conditional_format_clear", "sheets_copy_paste", "sheets_delete_dimension", "sheets_duplicate_tab", "sheets_filter_set", "sheets_find_replace", "sheets_format", "sheets_insert", "sheets_links_set", "sheets_raw", "sheets_table_create", "sheets_table_delete", "sheets_validation_clear", "sheets_validation_set"}
	bindings := make([]NativeToolBinding, 0, len(names))

	for _, name := range names {
		name := name
		destructive := false

		switch name {
		case "sheets_table_clear", "sheets_banding_clear", "sheets_chart_delete", "sheets_conditional_format_clear", "sheets_copy_paste", "sheets_delete_dimension", "sheets_filter_set", "sheets_find_replace", "sheets_table_delete", "sheets_validation_clear":
			destructive = true
		}
		methods := []string{http.MethodGet, http.MethodPost}
		scopes := sheetsWriteScopes

		if name == "sheets_raw" {
			methods = []string{http.MethodGet}

			scopes = append([]string{"spreadsheets.readonly"}, sheetsWriteScopes...)
		}

		hosts := []string{nativeSheetsHost}
		if name == "sheets_copy" {
			hosts = []string{nativeDocsSlidesDriveHost}
			scopes = []string{driveFileScope}
		}
		bindings = append(bindings, NativeToolBinding{Name: name, Service: nativeSheetsService, ResourceType: resourceTypeFile, ResourceArgument: nativeSheetsResourceArgument, Write: name != "sheets_raw", destructive: destructive, hosts: hosts, scopes: scopes, methods: methods, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			id, ok := args[nativeSheetsResourceArgument].(string)
			if !ok || !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid spreadsheet id")
			}

			if name == "sheets_copy" {
				root := "/drive/v3/files/" + id
				return []string{root, root + "/copy"}, nil
			}

			root := "/v4/spreadsheets/" + id
			if name == "sheets_raw" || name == "sheets_table_append" || name == "sheets_table_clear" {
				return []string{root}, nil
			}

			if name == "sheets_batch_update" {
				return []string{root + "/values:batchUpdate"}, nil
			}

			return []string{root, root + ":batchUpdate"}, nil
		}})
	}

	return bindings
}

// Called by the central write-method dispatcher for remaining Sheets tools.
// The binding separately pins the exact selected-resource paths and host.
func admitSheetsRemainingRequestMethod(name, path, method string) error {
	switch name {
	case "sheets_table_append", "sheets_table_clear", "sheets_copy", "sheets_banding_clear", "sheets_banding_set", "sheets_batch_update", "sheets_chart_create", "sheets_chart_delete", "sheets_chart_update", "sheets_conditional_format_add", "sheets_conditional_format_clear", "sheets_copy_paste", "sheets_delete_dimension", "sheets_duplicate_tab", "sheets_filter_set", "sheets_find_replace", "sheets_format", "sheets_insert", "sheets_links_set", "sheets_raw", "sheets_table_create", "sheets_table_delete", "sheets_validation_clear", "sheets_validation_set":
	default:
		return nil
	}

	if name == "sheets_table_append" || name == "sheets_table_clear" {
		suffix := ":append"
		if name == "sheets_table_clear" {
			suffix = ":clear"
		}

		if strings.Contains(path, "/values/") {
			if method == http.MethodPost && strings.HasSuffix(path, suffix) {
				return nil
			}

			return native.ErrDenied
		}

		if method == http.MethodGet {
			return nil
		}

		return native.ErrDenied
	}

	if name == "sheets_copy" {
		if strings.HasSuffix(path, "/copy") {
			if method == http.MethodPost {
				return nil
			}
		} else if method == http.MethodGet {
			return nil
		}

		return native.ErrDenied
	}

	if name == "sheets_batch_update" {
		if strings.HasSuffix(path, "/values:batchUpdate") && method == http.MethodPost {
			return nil
		}

		return native.ErrDenied
	}

	if name == "sheets_raw" {
		if method == http.MethodGet {
			return nil
		}

		return native.ErrDenied
	}

	if strings.HasSuffix(path, ":batchUpdate") {
		if method == http.MethodPost {
			return nil
		}
	} else if method == http.MethodGet {
		return nil
	}

	return native.ErrDenied
}

// admitSheetsRemainingDynamicPath admits only typed A1 actions in the exact
// signed spreadsheet. The existing grant covers the whole file, not a range.
func admitSheetsRemainingDynamicPath(invocation *ToolInvocation, request *http.Request) (handled, allowed bool) {
	if invocation == nil || (invocation.Name != "sheets_table_append" && invocation.Name != "sheets_table_clear") {
		return false, false
	}

	if request == nil || request.URL == nil {
		return true, false
	}

	id, ok := invocation.Arguments[nativeSheetsResourceArgument].(string)
	if !ok || !fileResourceID.MatchString(id) || invocation.Authorization.Resource == nil || invocation.Authorization.Resource.ResourceID != id {
		return true, false
	}

	if request.URL.Scheme != "https" || request.URL.Host != nativeSheetsHost || request.URL.User != nil {
		return true, false
	}

	root := "/v4/spreadsheets/" + id
	if request.URL.Path == root {
		return true, request.Method == http.MethodGet
	}

	suffix := ":append"
	if invocation.Name == "sheets_table_clear" {
		suffix = ":clear"
	}
	prefix := root + "/values/"

	path := request.URL.Path
	if request.Method != http.MethodPost || !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return true, false
	}

	a1 := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if !sheetsRangeSelector.MatchString(a1) {
		return true, false
	}
	parsed, err := sheetsa1.Parse(a1)

	return true, err == nil && parsed.SheetName != "" && parsed.StartRow > 0 && parsed.EndRow > 0 && parsed.StartCol > 0 && parsed.EndCol > 0
}
