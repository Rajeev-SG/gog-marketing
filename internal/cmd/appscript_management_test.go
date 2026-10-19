package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppScriptManagementRequests(t *testing.T) {
	files := `[{"name":"appsscript","type":"JSON","source":"{}"},{"name":"Code","type":"SERVER_JS","source":"function hello(){return 1;}"}]`
	for _, tc := range []struct {
		name, method, path, response string
		args                         []string
		want                         map[string]any
	}{
		{"push", http.MethodPut, "/v1/projects/script123/content", `{"scriptId":"script123"}`, []string{"push", "script123", "--files-json=" + files, "--force"}, map[string]any{"files": []any{map[string]any{"name": "appsscript", "type": "JSON", "source": "{}"}, map[string]any{"name": "Code", "type": "SERVER_JS", "source": "function hello(){return 1;}"}}}},
		{"bound", http.MethodPost, "/v1/projects", `{"scriptId":"bound123","parentId":"parent123"}`, []string{"bound-create", "--title=bound", "--parent-id=parent123"}, map[string]any{"title": "bound", "parentId": "parent123"}},
		{"version", http.MethodPost, "/v1/projects/script123/versions", `{"versionNumber":7}`, []string{"version-create", "script123", "--description=release"}, map[string]any{"description": "release"}},
		{"deployment", http.MethodPost, "/v1/projects/script123/deployments", `{"deploymentId":"deployment123"}`, []string{"deployment-create", "script123", "--version-number=7", "--description=release"}, map[string]any{"versionNumber": float64(7), "description": "release"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var got map[string]any
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Fatal(err)
				}
				actual, _ := json.Marshal(got)
				want, _ := json.Marshal(tc.want)
				if string(actual) != string(want) {
					t.Errorf("body %s, want %s", actual, want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			args := append([]string{"--account=a@b.com", "--json", "appscript"}, tc.args...)
			result := executeWithAppScriptTestService(t, args, newAppScriptTestService(t, server))
			if result.err != nil || calls != 1 {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
		})
	}
}

func TestAppScriptManagementRejectsBeforeService(t *testing.T) {
	for _, args := range [][]string{
		{"push", "script123", "--files-json=[]", "--force"},
		{"push", "script123", "--files-json=[{\"name\":\"appsscript\",\"type\":\"JSON\",\"source\":\"{}\"}]"},
		{"deployment-create", "script123", "--version-number=0"},
		{"deployment-create", "script123", "--version-number=2147483648"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := executeWithAppScriptTestServiceFactory(t, append([]string{"--account=a@b.com", "--no-input", "appscript"}, args...), unexpectedAppScriptTestService(t, "invalid or unconfirmed write reached service"))
			if result.err == nil {
				t.Fatal("invalid/unconfirmed request succeeded")
			}
		})
	}
}
