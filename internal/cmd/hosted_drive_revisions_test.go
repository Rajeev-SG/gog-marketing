package cmd

import (
	"math"
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDriveRevisionArguments(t *testing.T) {
	specs := map[string]mcpToolSpec{}
	for _, spec := range hostedDriveRevisionTools() {
		specs[spec.Name] = spec
	}
	for _, test := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"drive_revisions_list", map[string]any{"file_id": "file-1"}, []string{"drive", "revisions", "list", "--max=200", "--", "file-1"}},
		{"drive_revisions_list", map[string]any{"file_id": "file-1", "max_results": float64(1000), "page_token": "--account=else&cursor=1"}, []string{"drive", "revisions", "list", "--max=1000", "--page=--account=else&cursor=1", "--", "file-1"}},
		{"drive_revisions_get", map[string]any{"file_id": "file-1", "revision_id": "Rev_2-x"}, []string{"drive", "revisions", "get", "--", "file-1", "Rev_2-x"}},
	} {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = test.args
		got, err := specs[test.name].BuildArgs(request)
		if err != nil || !slices.Equal(got, test.want) {
			t.Fatalf("%s got=%q err=%v", test.name, got, err)
		}
	}
	for _, spec := range specs {
		for _, bad := range []any{nil, 1, "", " file-1", "file-1 ", "../file", "a/b", "https://drive.google.com/file/1", "a\nb"} {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"file_id": bad, "revision_id": "r1"}
			if _, err := spec.BuildArgs(request); err == nil {
				t.Errorf("%s accepted file_id=%v", spec.Name, bad)
			}
		}
	}
	for _, bad := range []any{nil, "1", 0, -1, 1001, 1.5, math.NaN(), math.Inf(1)} {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{"file_id": "file-1", "max_results": bad}
		if _, err := specs["drive_revisions_list"].BuildArgs(request); err == nil {
			t.Errorf("accepted max_results=%v", bad)
		}
	}
	for _, bad := range []any{nil, 1, "", " r1", "r1 ", "../r1", "r1/other", "r1?alt=media", "r1\n"} {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{"file_id": "file-1", "revision_id": bad}
		if _, err := specs["drive_revisions_get"].BuildArgs(request); err == nil {
			t.Errorf("accepted revision_id=%v", bad)
		}
	}
	for _, bad := range []any{nil, 1, "bad\ncursor"} {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{"file_id": "file-1", "page_token": bad}
		if _, err := specs["drive_revisions_list"].BuildArgs(request); err == nil {
			t.Errorf("accepted page_token=%v", bad)
		}
	}
}
