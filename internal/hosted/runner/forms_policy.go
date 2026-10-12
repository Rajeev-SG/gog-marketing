package runner

import "net/http"

const (
	formsOperationUpdate = "update"
	driveFileScope       = "drive.file"
)

func nativeFormsBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 5)

	for _, operation := range []string{"get", formsOperationUpdate, "publish", "responses_list", "responses_get"} {
		scopes := []string{driveFileScope}
		methods := []string{http.MethodGet}
		write := false

		switch operation {
		case "get":
			scopes = append(scopes, "forms.body.readonly", "forms.body")
		case formsOperationUpdate:
			write = true

			scopes = append(scopes, "forms.body")
			methods = []string{http.MethodPost}
		case "publish":
			write = true

			scopes = append(scopes, "forms.body")
			methods = []string{http.MethodGet, http.MethodPost}
		default:
			scopes = append(scopes, "forms.responses.readonly")
		}
		result = append(result, NativeToolBinding{Name: "forms_" + operation, Service: "forms", ResourceType: resourceTypeFile, ResourceArgument: "form_id", Write: write, hosts: []string{"forms.googleapis.com"}, scopes: scopes, methods: methods, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			id, _ := args["form_id"].(string)
			if !fileResourceID.MatchString(id) {
				return nil, invalidf("invalid form ID")
			}
			root := "/v1/forms/" + id

			switch operation {
			case formsOperationUpdate:
				return []string{root + ":batchUpdate"}, nil
			case "publish":
				return []string{root, root + ":setPublishSettings"}, nil
			case "responses_list":
				return []string{root + "/responses"}, nil
			case "responses_get":
				response, _ := args["response_id"].(string)
				if !fileResourceID.MatchString(response) {
					return nil, invalidf("invalid response ID")
				}

				return []string{root + "/responses/" + response}, nil
			default:
				return []string{root}, nil
			}
		}})
	}

	return result
}
