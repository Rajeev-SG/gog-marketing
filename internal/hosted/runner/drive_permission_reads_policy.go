package runner

import "net/http"

func nativeDrivePermissionReadBindings() []NativeToolBinding {
	names := []string{"drive_permissions", "drive_audit_sharing", "drive_audit_user"}

	bindings := make([]NativeToolBinding, 0, len(names))
	for _, name := range names {
		isPermissions := name == "drive_permissions"
		bindings = append(bindings, NativeToolBinding{
			Name: name, Service: "drive", ResourceType: resourceTypeFile, ResourceArgument: "file_id",
			hosts: []string{nativeDocsSlidesDriveHost}, scopes: []string{driveFileScope}, methods: []string{http.MethodGet}, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				file, ok := args["file_id"].(string)
				if !ok || !fileResourceID.MatchString(file) {
					return nil, invalidf("invalid file_id")
				}
				base := "/drive/v3/files/" + file

				permissions := base + "/permissions"
				if isPermissions {
					return []string{permissions}, nil
				}

				return []string{base, permissions}, nil
			},
		})
	}

	return bindings
}
