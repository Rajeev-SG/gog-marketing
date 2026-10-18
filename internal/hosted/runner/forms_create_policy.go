package runner

import (
	"net/http"
	"strings"
	"unicode"
)

func nativeFormsCreateBindings() []NativeToolBinding {
	return []NativeToolBinding{{
		Name: "forms_create_blank", Service: "forms", Write: true,
		hosts: []string{"forms.googleapis.com"}, methods: []string{http.MethodPost},
		scopes: []string{"forms.body", driveFileScope}, paths: exactPaths("/v1/forms", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			if len(args) != 1 {
				return nil, invalidf("blank form creation accepts only title")
			}

			title, ok := args["title"].(string)
			if !ok || strings.TrimSpace(title) == "" || len(title) > 512 || strings.ContainsFunc(title, unicode.IsControl) {
				return nil, invalidf("invalid form title")
			}

			return []string{"/v1/forms"}, nil
		},
	}}
}
