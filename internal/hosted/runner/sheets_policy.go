package runner

import (
	"net/http"
	"regexp"
)

// sheetsRangeSelector admits A1 ranges, anchored cell references and named
// ranges, including quoted tab names. It rejects every URL-significant or
// traversal character so the constructed values path stays an exact suffix of
// one selected spreadsheet.
var sheetsRangeSelector = regexp.MustCompile("^[A-Za-z0-9_$'!:. -]{1,512}$")

func nativeSheetsBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "sheets_metadata", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"spreadsheets.readonly", "spreadsheets", driveFileScope}, paths: exactPaths("/v4/spreadsheets/", "")},
		{Name: "sheets_append_range", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodPost}, scopes: []string{"spreadsheets", driveFileScope}, paths: exactPaths("", ""), argumentPaths: validateSheetsRangePaths(":append")},
		{Name: "sheets_clear_range", Service: "sheets", ResourceType: resourceTypeFile, ResourceArgument: "spreadsheet_id", Write: true, destructive: true, hosts: []string{"sheets.googleapis.com"}, methods: []string{http.MethodPost}, scopes: []string{"spreadsheets", driveFileScope}, paths: exactPaths("", ""), argumentPaths: validateSheetsRangePaths(":clear")},
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
