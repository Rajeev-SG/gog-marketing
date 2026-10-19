package runner

import "net/http"

func nativeSheetsGeometryBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 4)
	for _, name := range []string{"sheets_merge", "sheets_unmerge", "sheets_resize_columns", "sheets_resize_rows"} {
		bindings = append(bindings, NativeToolBinding{
			Name: name, Service: nativeSheetsService, ResourceType: resourceTypeFile, ResourceArgument: nativeSheetsResourceArgument,
			Write: true, destructive: name == "sheets_merge", hosts: []string{nativeSheetsHost}, scopes: sheetsWriteScopes, methods: []string{http.MethodGet, http.MethodPost}, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				id, ok := args[nativeSheetsResourceArgument].(string)
				if !ok || !fileResourceID.MatchString(id) {
					return nil, invalidf("invalid spreadsheet id")
				}
				root := "/v4/spreadsheets/" + id

				return []string{root, root + ":batchUpdate"}, nil
			},
		})
	}

	return bindings
}
