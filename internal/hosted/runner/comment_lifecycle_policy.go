package runner

import (
	"net/http"
	"strings"
)

const commentOperationList = "list"

func nativeCommentLifecycleBindings() []NativeToolBinding {
	bindings := make([]NativeToolBinding, 0, 14)

	for _, service := range []string{"docs", "drive"} {
		verbs := []string{scriptProjectCreate, calendarOperationUpdate, calendarOperationDelete, "reply", "resolve", "reopen"}
		if service == "docs" {
			verbs = []string{"add", calendarOperationDelete, scriptProjectGet, commentOperationList, "locate", "reply", "resolve", "reopen"}
		}

		for _, verb := range verbs {
			key := "file_id"
			if service == "docs" {
				key = "document_id"
			}
			binding := NativeToolBinding{Name: service + "_comments_" + verb, Service: service, ResourceType: resourceTypeFile, ResourceArgument: key, Write: verb != scriptProjectGet && verb != commentOperationList && verb != "locate", destructive: verb == calendarOperationDelete, hosts: []string{nativeDocsSlidesDriveHost}, scopes: []string{driveFileScope}, paths: exactPaths("", "")}
			method := http.MethodPost

			switch verb {
			case scriptProjectGet, commentOperationList, "locate":
				method = http.MethodGet
			case calendarOperationUpdate:
				method = http.MethodPatch
			case calendarOperationDelete:
				method = http.MethodDelete
			}

			binding.methods = []string{method}
			if verb == "locate" {
				binding.hosts = append(binding.hosts, "docs.googleapis.com")
			}
			binding.argumentPaths = func(args map[string]any) ([]string, error) {
				id, _ := args[key].(string)
				if !fileResourceID.MatchString(id) {
					return nil, invalidf("invalid selected file")
				}
				path := "/drive/v3/files/" + id + "/comments"

				if verb != scriptProjectCreate && verb != "add" && verb != commentOperationList {
					comment, _ := args["comment_id"].(string)
					if !fileResourceID.MatchString(comment) {
						return nil, invalidf("invalid comment")
					}
					path += "/" + comment
				}

				if verb == "reply" || verb == "resolve" || verb == "reopen" {
					path += "/replies"
				}

				paths := []string{path}
				if verb == "locate" {
					paths = append(paths, "/v1/documents/"+id)
				}

				return paths, nil
			}
			bindings = append(bindings, binding)
		}
	}

	return bindings
}

// A locate binding spans two APIs, but never permits their path/host cross product.
func admitCommentLocateHost(name, host, path string) bool {
	if name != "docs_comments_locate" {
		return true
	}

	if strings.HasPrefix(path, "/v1/documents/") {
		return host == "docs.googleapis.com"
	}

	return host == nativeDocsSlidesDriveHost
}
