package runner

import "net/http"

// Native Sheets inventory is read-only. The five inventory tools read one
// selected spreadsheet through an account-level exact tool grant and a
// per-file resource grant. Native handlers use only Spreadsheets.Get on the
// selected id with fixed metadata fields. The spreadsheet id is bound by the
// shared native resource validation and request guard, so only the fixed
// GET /v4/spreadsheets/<id> path is ever permitted and no custom argument
// paths are needed.
func nativeSheetsInventoryBindings() []NativeToolBinding {
	names := []string{
		"sheets_chart_list",
		"sheets_named_ranges_list",
		"sheets_table_list",
		"sheets_banding_list",
		"sheets_conditional_format_list",
	}

	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, NativeToolBinding{
			Name:             name,
			Service:          "sheets",
			ResourceType:     resourceTypeFile,
			ResourceArgument: nativeSheetsResourceArgument,
			Write:            false,
			hosts:            []string{nativeSheetsHost},
			scopes:           []string{"spreadsheets.readonly", "spreadsheets", driveFileScope},
			methods:          []string{http.MethodGet},
			paths:            exactPaths("/v4/spreadsheets/", ""),
		})
	}

	return bindings
}
