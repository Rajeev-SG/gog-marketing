package runner

import "net/http"

func nativeFormsQuestionBindings() []NativeToolBinding {
	return []NativeToolBinding{
		{Name: "forms_add_question", Service: "forms", ResourceType: resourceTypeFile, ResourceArgument: "form_id", Write: true, hosts: []string{"forms.googleapis.com"}, scopes: []string{"forms.body", driveFileScope}, methods: []string{http.MethodGet, http.MethodPost}, paths: formsQuestionPaths},
		{Name: "forms_move_question", Service: "forms", ResourceType: resourceTypeFile, ResourceArgument: "form_id", Write: true, hosts: []string{"forms.googleapis.com"}, scopes: []string{"forms.body", driveFileScope}, methods: []string{http.MethodPost}, paths: exactPaths("/v1/forms/", ":batchUpdate")},
		{Name: "forms_delete_question", Service: "forms", ResourceType: resourceTypeFile, ResourceArgument: "form_id", Write: true, destructive: true, hosts: []string{"forms.googleapis.com"}, scopes: []string{"forms.body", driveFileScope}, methods: []string{http.MethodGet, http.MethodPost}, paths: formsQuestionPaths},
	}
}

func formsQuestionPaths(id string) []string {
	return []string{"/v1/forms/" + id, "/v1/forms/" + id + ":batchUpdate"}
}
