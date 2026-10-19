package runner

import "net/http"

func nativeDriveRevisionBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 2)
	for _, name := range []string{"drive_revisions_list", "drive_revisions_get"} {
		bindings = append(bindings, NativeToolBinding{
			Name: name, Service: "drive", ResourceType: resourceTypeFile, ResourceArgument: "file_id",
			hosts: []string{nativeDocsSlidesDriveHost}, scopes: []string{driveFileScope}, methods: []string{http.MethodGet}, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				file, ok := args["file_id"].(string)
				if !ok || !fileResourceID.MatchString(file) {
					return nil, invalidf("invalid file_id")
				}
				path := "/drive/v3/files/" + file + "/revisions"

				if name == "drive_revisions_get" {
					revision, valid := args["revision_id"].(string)
					if !valid || !fileResourceID.MatchString(revision) {
						return nil, invalidf("invalid revision_id")
					}
					path += "/" + revision
				}

				return []string{path}, nil
			},
		})
	}

	return bindings
}
