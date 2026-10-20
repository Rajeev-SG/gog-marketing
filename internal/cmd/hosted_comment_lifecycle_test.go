package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedCommentLifecycleArgs(t *testing.T) {
	for _, spec := range hostedCommentLifecycleTools() {
		args := map[string]any{"content": "literal --file=/tmp/no-read", "document_id": "selected-file", "file_id": "selected-file", "comment_id": "comment-1"}
		req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
		argv, err := spec.BuildArgs(req)
		if err != nil {
			t.Fatalf("%s %v", spec.Name, err)
		}
		if !strings.Contains(strings.Join(argv, "\x00"), "\x00--\x00selected-file") {
			t.Errorf("%s positional boundary missing %v", spec.Name, argv)
		}
		args["document_id"] = "../secret"
		args["file_id"] = "../secret"
		if _, err = spec.BuildArgs(req); err == nil {
			t.Errorf("%s path admitted", spec.Name)
		}
	}
}
