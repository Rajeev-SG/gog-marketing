package cmd

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsFormatLayoutArguments(t *testing.T) {
	tools := hostedDocsFormatLayoutTools()
	docID := "selected-doc"
	build := func(t *testing.T, name string, args map[string]any) []string {
		t.Helper()
		for _, spec := range tools {
			if spec.Name != name {
				continue
			}
			req := mcp.CallToolRequest{}
			req.Params.Arguments = args
			got, err := spec.BuildArgs(req)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", name, err)
			}
			return got
		}
		t.Fatalf("no spec named %q", name)
		return nil
	}
	want := func(got []string, flag string) {
		if !slices.Contains(got, flag) {
			t.Errorf("args %v: missing %s", got, flag)
		}
	}
	notWant := func(got []string, flag string) {
		for _, arg := range got {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				t.Errorf("args %v: unexpected %s", got, flag)
			}
		}
	}
	wantErr := func(name string, args map[string]any) {
		t.Helper()
		for _, spec := range tools {
			if spec.Name != name {
				continue
			}
			req := mcp.CallToolRequest{}
			req.Params.Arguments = args
			if _, err := spec.BuildArgs(req); err == nil {
				t.Errorf("%s args %v: expected error", name, args)
			}
			return
		}
		t.Fatalf("no spec named %q", name)
	}

	got := build(t, "docs_format", map[string]any{"document_id": docID, "font_size": float64(12), "bold": false, "keep_with_next": false, "space_above": float64(0), "link": ""})
	want(got, "--no-bold")
	want(got, "--keep-with-next=false")
	want(got, "--space-above=0")
	want(got, "--no-link")

	got = build(t, "docs_page_layout", map[string]any{"document_id": docID, "margin_left": "0"})
	want(got, "--margin-left=0")
	notWant(got, "--layout")

	got = build(t, "docs_page_layout", map[string]any{"document_id": docID, "margin_left": "0", "layout": "pages"})
	want(got, "--layout=pages")

	got = build(t, "docs_section_columns", map[string]any{"document_id": docID, "count": float64(2), "index": float64(1)})
	want(got, "--count=2")
	want(got, "--index=1")

	wantErr("docs_format", map[string]any{"document_id": docID})
	wantErr("docs_format", map[string]any{"document_id": docID, "font_size": float64(0)})
	wantErr("docs_format", map[string]any{"document_id": docID, "font_size": math.NaN()})
	wantErr("docs_format", map[string]any{"document_id": docID, "bold": "true"})
	wantErr("docs_format", map[string]any{"document_id": docID, "match": " "})
	wantErr("docs_page_layout", map[string]any{"document_id": docID, "margin_left": nil})
	wantErr("docs_page_layout", map[string]any{"document_id": docID, "layout": nil})
	wantErr("docs_page_layout", map[string]any{"document_id": docID, "layout": "invalid"})
	wantErr("docs_section_columns", map[string]any{"document_id": docID})
	wantErr("docs_section_columns", map[string]any{"document_id": docID, "count": float64(0)})
	wantErr("docs_section_columns", map[string]any{"document_id": docID, "count": float64(4)})
	wantErr("docs_section_columns", map[string]any{"document_id": docID, "count": float64(2), "separator": "bad"})
	wantErr("docs_section_columns", map[string]any{"document_id": docID, "count": float64(2), "index": float64(1), "at_end": true})
}
