package runner

import (
	"net/http"
	"regexp"
)

var driveFileID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func nativeDriveRenameBinding() NativeToolBinding {
	return NativeToolBinding{
		Name:             "drive_rename",
		Service:          "drive",
		ResourceType:     resourceTypeFile,
		ResourceArgument: "file_id",
		Write:            true,
		hosts:            []string{"www.googleapis.com"},
		methods:          []string{http.MethodPatch},
		scopes:           []string{driveFileScope},
		paths:            exactPaths("/drive/v3/files/", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			fileID, _ := args["file_id"].(string)
			if !driveFileID.MatchString(fileID) {
				return nil, invalidf("invalid Drive file ID")
			}

			return []string{"/drive/v3/files/" + fileID}, nil
		},
	}
}

func nativeDriveCopyBinding() NativeToolBinding {
	return NativeToolBinding{
		Name:             "drive_copy",
		Service:          "drive",
		ResourceType:     resourceTypeFile,
		ResourceArgument: "file_id",
		Write:            true,
		hosts:            []string{"www.googleapis.com"},
		methods:          []string{http.MethodGet, http.MethodPost},
		scopes:           []string{driveFileScope},
		paths:            exactPaths("/drive/v3/files/", "/copy"),
		argumentPaths: func(args map[string]any) ([]string, error) {
			fileID, _ := args["file_id"].(string)
			if !driveFileID.MatchString(fileID) {
				return nil, invalidf("invalid Drive file ID")
			}

			return []string{"/drive/v3/files/" + fileID, "/drive/v3/files/" + fileID + "/copy"}, nil
		},
	}
}

func nativeDriveTrashBinding() NativeToolBinding {
	return NativeToolBinding{
		Name:             "drive_trash",
		Service:          "drive",
		ResourceType:     resourceTypeFile,
		ResourceArgument: "file_id",
		Write:            true,
		destructive:      true,
		hosts:            []string{"www.googleapis.com"},
		methods:          []string{http.MethodGet, http.MethodPatch},
		scopes:           []string{driveFileScope},
		paths:            exactPaths("/drive/v3/files/", ""),
		argumentPaths: func(args map[string]any) ([]string, error) {
			fileID, _ := args["file_id"].(string)
			if !driveFileID.MatchString(fileID) {
				return nil, invalidf("invalid Drive file ID")
			}

			return []string{"/drive/v3/files/" + fileID}, nil
		},
	}
}
