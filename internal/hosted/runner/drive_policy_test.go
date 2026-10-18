package runner

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNativeDriveGuardsUseOnlySelectedPathsAndMethods(t *testing.T) {
	for _, entry := range []struct {
		name, method, suffix string
		allowed              bool
	}{
		{"drive_rename", http.MethodPatch, "/selected-file", true},
		{"drive_rename", http.MethodPost, "/selected-file", false},
		{"drive_rename", http.MethodPatch, "/foreign-file", false},
		{"drive_copy", http.MethodGet, "/selected-file", true},
		{"drive_copy", http.MethodPost, "/selected-file/copy", true},
		{"drive_copy", http.MethodGet, "/selected-file/copy", false},
		{"drive_copy", http.MethodPost, "/selected-file", false},
		{"drive_trash", http.MethodGet, "/selected-file", true},
		{"drive_trash", http.MethodPatch, "/selected-file", true},
		{"drive_trash", http.MethodDelete, "/selected-file", false},
		{"drive_trash", http.MethodPatch, "/foreign-file", false},
	} {
		invocation := &ToolInvocation{Name: entry.name, Arguments: map[string]any{"file_id": "selected-file", "new_name": "Literal"}, Authorization: ToolAuthorization{Service: "drive", AllowedTools: []string{entry.name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, Resource: &ResourceGrantInput{Service: "drive", ResourceType: "file", ResourceID: "selected-file", Enabled: true}, AllowWrite: true, AllowDestructive: true}}

		parsed, err := url.Parse("https://www.googleapis.com/drive/v3/files" + entry.suffix)
		if err != nil {
			t.Fatal(err)
		}

		allowed := nativeRequestGuard(invocation)(&http.Request{Method: entry.method, URL: parsed, Header: http.Header{}}) == nil
		if allowed != entry.allowed {
			t.Errorf("%s %s %s: allowed=%v, want %v", entry.name, entry.method, entry.suffix, allowed, entry.allowed)
		}
	}
}
