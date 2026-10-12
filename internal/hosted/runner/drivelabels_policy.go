package runner

import (
	"net/http"
	"regexp"
)

var labelSchemaName = regexp.MustCompile(`^labels/[A-Za-z0-9_-]{1,256}(?:@[A-Za-z0-9_-]{1,64})?$`)

func nativeDriveLabelsBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "drivelabels_list", Service: "drivelabels", hosts: []string{"drivelabels.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"drive.labels.readonly"}, paths: exactPaths("/v2/labels", "")},
		{Name: "drivelabels_get", Service: "drivelabels", hosts: []string{"drivelabels.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"drive.labels.readonly"}, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			name, _ := args["label_name"].(string)
			if !labelSchemaName.MatchString(name) {
				return nil, invalidf("invalid label schema name")
			}

			return []string{"/v2/" + name}, nil
		}},
	}
}
