package runner

import (
	"net/http"
	"regexp"
)

// sheetsRangeSelector admits A1 ranges, anchored cell references and named
// ranges, including quoted tab names with punctuation, spaces and Unicode
// characters (e.g. 'Support Sales (UK)', 'Sales & Marketing', 'Français',
// '売上'). The installed google.golang.org/api expands {range} with the value
// verbatim in the decoded path (RawPath percent-encodes everything outside
// [A-Za-z0-9-._~]), so the guard's exact decoded-path match holds. Characters
// that could alter URL or range structure — / \ ? # % @ " [ ] { } * and
// control bytes — stay rejected, and the request guard still denies traversal
// (/../, /./, backslash) before any exact admission.
var sheetsRangeSelector = regexp.MustCompile(`^[\p{L}\p{N}\p{M}~ _.$:!'()&+,;=-]{1,512}$`)

// sheetsLabelSelector admits ordinary tab titles while rejecting control
// characters and oversized labels. Unlike range selectors, tab titles are not
// embedded in request paths, so punctuation is not admitted for URL purposes.
var sheetsLabelSelector = regexp.MustCompile(`^[^\p{C}]{1,512}$`)

var sheetsWriteScopes = []string{"spreadsheets", driveFileScope}

func nativeSheetsBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "sheets_metadata", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"spreadsheets.readonly", "spreadsheets", driveFileScope}, paths: exactPaths("/v4/spreadsheets/", "")},
		{Name: "sheets_append_range", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodPost}, scopes: []string{"spreadsheets", driveFileScope}, paths: exactPaths("", ""), argumentPaths: validateSheetsRangePaths(":append")},
		{Name: "sheets_clear_range", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, destructive: true, hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodPost}, scopes: []string{"spreadsheets", driveFileScope}, paths: exactPaths("", ""), argumentPaths: validateSheetsRangePaths(":clear")},
		{Name: "sheets_add_tab", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodPost}, scopes: sheetsWriteScopes, paths: exactPaths("", ":batchUpdate"), argumentPaths: validateSheetsSpreadsheetPaths("tab_name")},
		{Name: "sheets_rename_tab", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, hosts: []string{"sheets.googleapis.com"}, scopes: sheetsWriteScopes, paths: exactPaths("/v4/spreadsheets/", ""), argumentPaths: validateSheetsSpreadsheetPaths("old_name", "new_name")},
		{Name: "sheets_delete_tab", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, destructive: true, hosts: []string{"sheets.googleapis.com"}, scopes: sheetsWriteScopes, paths: exactPaths("/v4/spreadsheets/", ""), argumentPaths: validateSheetsSpreadsheetPaths("tab_name")},
		{Name: "sheets_number_format_range", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, hosts: []string{"sheets.googleapis.com"}, scopes: sheetsWriteScopes, paths: exactPaths("/v4/spreadsheets/", ""), argumentPaths: validateSheetsFormattingPaths},
	}
}

// validateSheetsRangePaths admits exactly one native values action per selected
// spreadsheet and range; arbitrary batch or read endpoints are never built.
func validateSheetsRangePaths(suffix string) func(map[string]any) ([]string, error) {
	validator := func(args map[string]any) ([]string, error) {
		spreadsheetID, _ := args["spreadsheet_id"].(string)
		rangeSpec, _ := args["range"].(string)

		if !fileResourceID.MatchString(spreadsheetID) || !sheetsRangeSelector.MatchString(rangeSpec) {
			return nil, invalidf("invalid spreadsheet or range selector")
		}

		return []string{"/v4/spreadsheets/" + spreadsheetID + "/values/" + rangeSpec + suffix}, nil
	}

	return validator
}

func validSheetsLabel(value string) bool {
	return sheetsLabelSelector.MatchString(value)
}

// validateSheetsSpreadsheetPaths validates labels in the typed arguments while
// admitting only the metadata lookup and exact selected-spreadsheet batch
// endpoint. The generic batch endpoint is still narrowed to one native request
// kind by the write-method admission and schema-controlled request body.
func validateSheetsSpreadsheetPaths(labels ...string) func(map[string]any) ([]string, error) {
	return func(args map[string]any) ([]string, error) {
		spreadsheetID, _ := args["spreadsheet_id"].(string)
		if !fileResourceID.MatchString(spreadsheetID) {
			return nil, invalidf("invalid spreadsheet selector")
		}
		for _, label := range labels {
			value, _ := args[label].(string)
			if !validSheetsLabel(value) {
				return nil, invalidf("invalid sheet label")
			}
		}

		return []string{"/v4/spreadsheets/" + spreadsheetID, "/v4/spreadsheets/" + spreadsheetID + ":batchUpdate"}, nil
	}
}

func validateSheetsFormattingPaths(args map[string]any) ([]string, error) {
	spreadsheetID, _ := args["spreadsheet_id"].(string)
	rangeSpec, _ := args["range"].(string)
	if !fileResourceID.MatchString(spreadsheetID) || !sheetsRangeSelector.MatchString(rangeSpec) {
		return nil, invalidf("invalid spreadsheet or range selector")
	}

	return []string{"/v4/spreadsheets/" + spreadsheetID, "/v4/spreadsheets/" + spreadsheetID + ":batchUpdate"}, nil
}
