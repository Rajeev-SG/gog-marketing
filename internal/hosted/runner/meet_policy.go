package runner

import (
	"net/http"
	"regexp"
)

var meetingSpaceName = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]{1,256}$`)

const meetReadOperation = "get"

func nativeMeetSpaceBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 4)

	for _, operation := range []string{"create", meetReadOperation, "update", "end"} {
		scopes := []string{"meetings.space.created"}
		methods := make([]string, 1, 2)
		methods[0] = http.MethodGet

		switch operation {
		case meetReadOperation:
			scopes = append(scopes, "meetings.space.readonly", "meetings.space.settings")
		case "create":
			methods[0] = http.MethodPost
		case "update":
			scopes = append(scopes, "meetings.space.settings")
			methods = append(methods, http.MethodPatch)
		case "end":
			methods = append(methods, http.MethodPost)
		}
		result = append(result, NativeToolBinding{Name: "meet_space_" + operation, Service: "meet", Write: operation != meetReadOperation, destructive: operation == "end", hosts: []string{"meet.googleapis.com"}, scopes: scopes, methods: methods, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			if operation == "create" {
				return []string{"/v2/spaces"}, nil
			}

			name, _ := args["space_name"].(string)
			if !meetingSpaceName.MatchString(name) {
				return nil, invalidf("invalid canonical meeting space")
			}

			paths := []string{"/v2/" + name}
			if operation == "end" {
				paths = append(paths, "/v2/"+name+":endActiveConference")
			}

			return paths, nil
		}})
	}

	return result
}
