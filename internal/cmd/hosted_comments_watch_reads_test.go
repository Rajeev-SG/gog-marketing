package cmd

import (
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func commentsWatchSpecByName(t *testing.T, name string) mcpToolSpec {
	t.Helper()
	for _, spec := range hostedCommentsWatchReadTools() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("tool %q not found", name)
	return mcpToolSpec{}
}

func commentsWatchRequestWithArgs(t *testing.T, spec mcpToolSpec, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = spec.Name
	req.Params.Arguments = args
	return req
}

func TestDriveCommentsListDefaults(t *testing.T) {
	spec := commentsWatchSpecByName(t, "drive_comments_list")
	argv, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{"file_id": "abc123"}))
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	want := []string{"drive", "comments", "list", "--max=50", "--", "abc123"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
}

func TestDriveCommentsListOptions(t *testing.T) {
	spec := commentsWatchSpecByName(t, "drive_comments_list")
	argv, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{
		"file_id":        "abc123",
		"max":            float64(100),
		"page":           "tok",
		"include_quoted": true,
	}))
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	want := []string{"drive", "comments", "list", "--max=100", "--page=tok", "--include-quoted", "--", "abc123"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
}

func TestDriveCommentsListMaxLimits(t *testing.T) {
	spec := commentsWatchSpecByName(t, "drive_comments_list")
	for _, maxResults := range []float64{0, 101, -1, 1.5} {
		if _, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{"file_id": "abc123", "max": maxResults})); err == nil {
			t.Fatalf("max=%v: expected error", maxResults)
		}
	}
	argv, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{"file_id": "abc123", "max": float64(1)}))
	if err != nil {
		t.Fatalf("max=1: %v", err)
	}
	if argv[3] != "--max=1" {
		t.Fatalf("argv[3] = %q", argv[3])
	}
}

func TestDriveCommentsListMalformedFileID(t *testing.T) {
	spec := commentsWatchSpecByName(t, "drive_comments_list")
	for _, id := range []string{"", "../etc", "a/b", "a b", "a\nb"} {
		if _, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{"file_id": id})); err == nil {
			t.Fatalf("file_id=%q: expected error", id)
		}
	}
	if _, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{})); err == nil {
		t.Fatal("missing file_id: expected error")
	}
}

func TestDriveCommentsGetArgvAndValidation(t *testing.T) {
	spec := commentsWatchSpecByName(t, "drive_comments_get")
	argv, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{
		"file_id": "abc123", "comment_id": "Cmt_9-x",
	}))
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	want := []string{"drive", "comments", "get", "--", "abc123", "Cmt_9-x"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for _, args := range []map[string]any{
		{"file_id": "bad/id", "comment_id": "ok"},
		{"file_id": "ok", "comment_id": ""},
		{"file_id": "ok"},
	} {
		if _, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, args)); err == nil {
			t.Fatalf("args=%v: expected error", args)
		}
	}
}

func TestFormsWatchListArgvAndValidation(t *testing.T) {
	spec := commentsWatchSpecByName(t, "forms_watch_list")
	argv, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, map[string]any{"form_id": "form_1A"}))
	if err != nil {
		t.Fatalf("BuildArgs: %v", err)
	}
	want := []string{"forms", "watch", "list", "--", "form_1A"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	for _, args := range []map[string]any{
		{"form_id": "../x"},
		{"form_id": ""},
		{},
	} {
		if _, err := spec.BuildArgs(commentsWatchRequestWithArgs(t, spec, args)); err == nil {
			t.Fatalf("args=%v: expected error", args)
		}
	}
}

func TestCommentsWatchReadToolSchemas(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"drive_comments_list", []string{"file_id", "max", "page", "include_quoted"}},
		{"drive_comments_get", []string{"file_id", "comment_id"}},
		{"forms_watch_list", []string{"form_id"}},
	}
	for _, tc := range cases {
		spec := commentsWatchSpecByName(t, tc.name)
		tool := mcp.NewTool(spec.Name, spec.Options...)
		var got []string
		for key := range tool.InputSchema.Properties {
			got = append(got, key)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s properties = %v, want %v", tc.name, got, tc.want)
		}
		set := map[string]bool{}
		for _, k := range got {
			set[k] = true
		}
		for _, k := range tc.want {
			if !set[k] {
				t.Fatalf("%s missing property %q (got %v)", tc.name, k, got)
			}
		}
	}
}
