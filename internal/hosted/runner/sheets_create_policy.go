package runner

import (
	"net/http"
	"strings"
	"unicode"
)

func nativeSheetsCreateBindings() []NativeToolBinding {
	return []NativeToolBinding{{
		Name: "sheets_create_blank", Service: "sheets", Write: true,
		hosts: []string{nativeSheetsHost}, methods: []string{http.MethodPost},
		scopes: []string{"spreadsheets", driveFileScope}, paths: exactPaths("/v4/spreadsheets", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			if len(args) != 1 {
				return nil, invalidf("blank creation accepts only title")
			}

			title, ok := args["title"].(string)
			if !ok || strings.TrimSpace(title) == "" || len(title) > 512 || strings.ContainsFunc(title, unicode.IsControl) {
				return nil, invalidf("invalid spreadsheet title")
			}

			return []string{"/v4/spreadsheets"}, nil
		},
	}}
}
