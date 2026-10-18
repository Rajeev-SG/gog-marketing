package runner

import (
	"net/http"
	"strings"
	"unicode"
)

const nativeDocsSlidesDriveHost = "www.googleapis.com"

// nativeDocsSlidesCreateBindings registers the two fixed blank-creation tools.
// Both create through the Drive API, so the only admitted hosted OAuth scope is
// drive.file: the documents and presentations scopes alone do not authorize a
// Drive Files.Create. The request is always a single POST to
// https://www.googleapis.com/drive/v3/files with a name + fixed mimeType body;
// no caller chooses the MIME type, host, method or root command.
func nativeDocsSlidesCreateBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{
			Name: "docs_create_blank", Service: "docs", Write: true,
			hosts: []string{nativeDocsSlidesDriveHost}, methods: []string{http.MethodPost},
			scopes: []string{driveFileScope}, paths: exactPaths("/drive/v3/files", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				if err := validateNativeBlankCreateTitle(args, "document"); err != nil {
					return nil, err
				}

				return []string{"/drive/v3/files"}, nil
			},
		},
		{
			Name: "slides_create_blank", Service: "slides", Write: true,
			hosts: []string{nativeDocsSlidesDriveHost}, methods: []string{http.MethodPost},
			scopes: []string{driveFileScope}, paths: exactPaths("/drive/v3/files", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				if err := validateNativeBlankCreateTitle(args, "presentation"); err != nil {
					return nil, err
				}

				return []string{"/drive/v3/files"}, nil
			},
		},
	}
}

// validateNativeBlankCreateTitle enforces the literal-title contract shared by
// docs_create_blank and slides_create_blank: exactly one required string
// title, at most 512 UTF-8 bytes, no Unicode control characters and no blank
// value. No parent, template, source file, pageless or markdown argument is
// accepted.
func validateNativeBlankCreateTitle(args map[string]any, kind string) error {
	if len(args) != 1 {
		return invalidf("blank %s creation accepts only title", kind)
	}

	title, ok := args["title"].(string)
	if !ok || strings.TrimSpace(title) == "" || len(title) > 512 || strings.ContainsFunc(title, unicode.IsControl) {
		return invalidf("invalid %s title", kind)
	}

	return nil
}
