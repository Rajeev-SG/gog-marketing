package runner

import (
	"context"
	"net/http"
	"testing"
)

func TestDriveRevisionMetadataOnly(t *testing.T) {
	for _, name := range []string{"drive_revisions_list", "drive_revisions_get"} {
		t.Run(name, func(t *testing.T) {
			invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"file_id": "selected-file", "revision_id": "revision-1"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-file"}}}

			path := "https://www.googleapis.com/drive/v3/files/selected-file/revisions"
			if name == "drive_revisions_get" {
				path += "/revision-1"
			}
			guard := nativeRequestGuard(invocation)

			for _, test := range []struct {
				query   string
				allowed bool
			}{
				{"", true},
				{"?alt=json", true},
				{"?alt=media", false},
				{"?alt=", false},
				{"?alt=json&alt=media", false},
				{"?alt=media&alt=json", false},
			} {
				request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, path+test.query, nil)
				if err != nil {
					t.Fatal(err)
				}

				if err := guard(request); (err == nil) != test.allowed {
					t.Errorf("query %q: allowed=%t err=%v", test.query, test.allowed, err)
				}
			}
		})
	}
}
