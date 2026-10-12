package runner

import (
	"net/http"
	"regexp"
)

var contactResourceID = regexp.MustCompile(`^people/[A-Za-z0-9_-]{1,256}$`)

func nativeContactsBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 6)

	for _, operation := range []struct{ name, method string }{
		{"list", http.MethodGet}, {"search", http.MethodGet}, {"get", http.MethodGet}, {"create", http.MethodPost}, {"update", http.MethodPatch}, {"delete", http.MethodDelete},
	} {
		write := operation.method != http.MethodGet

		scopes := []string{"contacts"}
		if !write {
			scopes = append(scopes, "contacts.readonly")
		}

		result = append(result, NativeToolBinding{
			Name: "contacts_" + operation.name, Service: "contacts", Write: write, destructive: operation.name == "delete", hosts: []string{"people.googleapis.com"}, scopes: scopes, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				switch operation.name {
				case "list":
					return []string{"/v1/people/me/connections"}, nil
				case "search":
					return []string{"/v1/people:searchContacts"}, nil
				case "create":
					return []string{"/v1/people:createContact"}, nil
				default:
					id, _ := args["resource_name"].(string)
					if !contactResourceID.MatchString(id) {
						return nil, invalidf("invalid contact resource")
					}

					path := "/v1/" + id
					if operation.name == "update" {
						return []string{path, path + ":updateContact"}, nil
					}

					if operation.name == "delete" {
						path += ":deleteContact"
					}

					return []string{path}, nil
				}
			},
		})
		if operation.name == "update" {
			result[len(result)-1].methods = []string{http.MethodGet, http.MethodPatch}
		} else {
			result[len(result)-1].methods = []string{operation.method}
		}
	}

	return result
}
