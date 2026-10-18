package cmd

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedBlankCreateRequest(t *testing.T, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"params": map[string]any{"name": "docs_create_blank", "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	var req mcp.CallToolRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	return req
}

// TestHostedDocsSlidesCreateToolSpecs verifies the two fixed blank-creation
// tool registrations: exact names (never case labels), their services and the
// write risk class.
func TestHostedDocsSlidesCreateToolSpecs(t *testing.T) {
	specs := hostedDocsSlidesCreateTools()
	if len(specs) != 2 {
		t.Fatalf("expected exactly two blank-create tools, got %d", len(specs))
	}

	byName := map[string]mcpToolSpec{}
	for _, spec := range specs {
		byName[spec.Name] = spec
	}

	for _, want := range []struct{ name, service string }{
		{"docs_create_blank", "docs"},
		{"slides_create_blank", "slides"},
	} {
		spec, ok := byName[want.name]
		if !ok {
			t.Fatalf("tool %s was not registered", want.name)
		}
		if spec.Service != want.service {
			t.Errorf("%s: service=%q, want %q", want.name, spec.Service, want.service)
		}
		if spec.Risk != mcpRiskWrite {
			t.Errorf("%s: risk=%v, want write", want.name, spec.Risk)
		}
		if spec.BuildArgs == nil {
			t.Errorf("%s: BuildArgs is nil", want.name)
		}
	}
}

// TestHostedDocsSlidesCreateBuildArgs verifies that both tools build the docs
// and slides create commands with the literal title as the only argument after
// the "--" separator, and reject every invalid title before reaching the CLI.
func TestHostedDocsSlidesCreateBuildArgs(t *testing.T) {
	for _, tool := range []struct{ name, root string }{
		{"docs_create_blank", "docs"},
		{"slides_create_blank", "slides"},
	} {
		spec := toolByName(t, hostedDocsSlidesCreateTools(), tool.name)

		cases := []struct {
			label   string
			args    map[string]any
			want    []string
			wantErr bool
		}{
			{
				label: "positive title",
				args:  map[string]any{"title": "Quarterly plan"},
				want:  []string{tool.root, "create", "--", "Quarterly plan"},
			},
			{
				label: "flag-like title stays positional after separator",
				args:  map[string]any{"title": "--parent=foreign"},
				want:  []string{tool.root, "create", "--", "--parent=foreign"},
			},
			{
				label: "512-byte multibyte title accepted",
				args:  map[string]any{"title": strings.Repeat("é", 256)},
				want:  []string{tool.root, "create", "--", strings.Repeat("é", 256)},
			},
			{label: "missing title", args: map[string]any{}, wantErr: true},
			{label: "empty title", args: map[string]any{"title": ""}, wantErr: true},
			{label: "whitespace title", args: map[string]any{"title": "   "}, wantErr: true},
			{label: "unicode control title", args: map[string]any{"title": "Quarterly\u0085plan"}, wantErr: true},
			{label: "control character title", args: map[string]any{"title": "Quarterly\x07plan"}, wantErr: true},
			{label: "oversized title", args: map[string]any{"title": strings.Repeat("a", 513)}, wantErr: true},
			{label: "non-string title", args: map[string]any{"title": 42}, wantErr: true},
		}

		for _, tc := range cases {
			args, err := spec.BuildArgs(hostedBlankCreateRequest(t, tc.args))
			if tc.wantErr {
				if err == nil {
					t.Errorf("%s %s: expected error, got args %v", tool.name, tc.label, args)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s %s: unexpected error: %v", tool.name, tc.label, err)
				continue
			}
			if !slices.Equal(args, tc.want) {
				t.Errorf("%s %s: args=%v, want %v", tool.name, tc.label, args, tc.want)
			}
		}
	}
}

func toolByName(t *testing.T, specs []mcpToolSpec, name string) mcpToolSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("tool %s was not registered", name)
	return mcpToolSpec{}
}
