package runner

import (
	"net/http"
)

// Native comments/watch reads are read-only. drive_comments_list,
// drive_comments_get and forms_watch_list read against one selected resource
// through an account-level exact tool grant and a per-file resource grant.
// Every resource id is validated with the existing fileResourceID pattern and
// only the fixed GET paths below are ever permitted; no generic proxy path,
// extra hosts or broad scopes are exposed.
func nativeCommentsWatchReadBindings() []NativeToolBinding {
	idArg := func(args map[string]any, name string) (string, error) {
		id, _ := args[name].(string)
		if !fileResourceID.MatchString(id) {
			return "", invalidf("invalid %s", name)
		}

		return id, nil
	}

	return []NativeToolBinding{
		{
			Name:             "drive_comments_list",
			Service:          "drive",
			ResourceType:     resourceTypeFile,
			ResourceArgument: "file_id",
			Write:            false,
			hosts:            []string{nativeDocsSlidesDriveHost},
			scopes:           []string{driveFileScope},
			methods:          []string{http.MethodGet},
			paths:            exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				fileID, err := idArg(args, "file_id")
				if err != nil {
					return nil, err
				}

				return []string{"/drive/v3/files/" + fileID + "/comments"}, nil
			},
		},
		{
			Name:             "drive_comments_get",
			Service:          "drive",
			ResourceType:     resourceTypeFile,
			ResourceArgument: "file_id",
			Write:            false,
			hosts:            []string{nativeDocsSlidesDriveHost},
			scopes:           []string{driveFileScope},
			methods:          []string{http.MethodGet},
			paths:            exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				fileID, err := idArg(args, "file_id")
				if err != nil {
					return nil, err
				}

				commentID, err := idArg(args, "comment_id")
				if err != nil {
					return nil, err
				}

				return []string{"/drive/v3/files/" + fileID + "/comments/" + commentID}, nil
			},
		},
		{
			Name:             "forms_watch_list",
			Service:          "forms",
			ResourceType:     resourceTypeFile,
			ResourceArgument: "form_id",
			Write:            false,
			hosts:            []string{"forms.googleapis.com"},
			scopes: []string{
				driveFileScope,
				"forms.body.readonly",
				"forms.body",
				"forms.responses.readonly",
			},
			methods: []string{http.MethodGet},
			paths:   exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				formID, err := idArg(args, "form_id")
				if err != nil {
					return nil, err
				}

				return []string{"/v1/forms/" + formID + "/watches"}, nil
			},
		},
	}
}
