package runner

import "net/http"

func nativeSheetsRangeNoteBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 4)
	for _, name := range []string{"sheets_named_ranges_add", "sheets_named_ranges_update", "sheets_named_ranges_delete", "sheets_update_note"} {
		bindings = append(bindings, NativeToolBinding{
			Name: name, Service: nativeSheetsService, ResourceType: resourceTypeFile, ResourceArgument: nativeSheetsResourceArgument, Write: true, destructive: name == "sheets_named_ranges_delete", hosts: []string{nativeSheetsHost}, scopes: sheetsWriteScopes, methods: []string{http.MethodGet, http.MethodPost}, paths: exactPaths("", ""),
			argumentPaths: validateSheetsSpreadsheetPaths(),
		})
	}

	return bindings
}
