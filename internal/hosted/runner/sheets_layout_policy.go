package runner

import "net/http"

// nativeSheetsLayoutBindings returns the two fixed native sheets layout write
// tools: row and column freezing and tab reordering. Both are reversible
// layout changes on one explicitly selected spreadsheet, never destructive
// data changes. The shared argument-path validator admits exactly the
// metadata lookup and the selected spreadsheet's :batchUpdate endpoint; no
// new authorization logic, defaults or scopes are introduced here.
func nativeSheetsLayoutBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{
			Name:             "sheets_freeze",
			Service:          nativeSheetsService,
			ResourceType:     resourceTypeFile,
			ResourceArgument: nativeSheetsResourceArgument,
			Write:            true,
			hosts:            []string{nativeSheetsHost},
			methods:          []string{http.MethodGet, http.MethodPost},
			scopes:           sheetsWriteScopes,
			paths:            exactPaths("", ""),
			argumentPaths:    validateSheetsSpreadsheetPaths("sheet"),
		},
		{
			Name:             "sheets_reorder_tab",
			Service:          nativeSheetsService,
			ResourceType:     resourceTypeFile,
			ResourceArgument: nativeSheetsResourceArgument,
			Write:            true,
			hosts:            []string{nativeSheetsHost},
			methods:          []string{http.MethodGet, http.MethodPost},
			scopes:           sheetsWriteScopes,
			paths:            exactPaths("", ""),
			argumentPaths:    validateSheetsSpreadsheetPaths("tab"),
		},
	}
}
