package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsSegmentReadTools(t *testing.T) {
	tools := hostedDocsSegmentReadTools()
	if len(tools) != 3 {
		t.Fatalf("hostedDocsSegmentReadTools() returned %d tools, want 3", len(tools))
	}

	wantSub := map[string][]string{
		"docs_header_list":      {"docs", "header", "list"},
		"docs_footer_list":      {"docs", "footer", "list"},
		"docs_suggestions_list": {"docs", "suggestions", "list"},
	}

	type argvCase struct {
		name    string
		args    map[string]any
		tab     string
		docID   string
		wantErr bool
	}
	cases := []argvCase{
		{
			name:  "defaults",
			args:  map[string]any{"document_id": "selected-doc"},
			docID: "selected-doc",
		},
		{
			name:  "tab value",
			args:  map[string]any{"document_id": "selected-doc", "tab": "Tab 👩‍💻"},
			tab:   "--tab=Tab 👩‍💻",
			docID: "selected-doc",
		},
		{
			name:  "tab flaglike preserved",
			args:  map[string]any{"document_id": "selected-doc", "tab": "--account=else"},
			tab:   "--tab=--account=else",
			docID: "selected-doc",
		},
		{
			name:  "tab empty accepted",
			args:  map[string]any{"document_id": "selected-doc", "tab": ""},
			tab:   "--tab=",
			docID: "selected-doc",
		},
		{
			name:    "document_id missing",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "document_id nil",
			args:    map[string]any{"document_id": nil},
			wantErr: true,
		},
		{
			name:    "document_id int",
			args:    map[string]any{"document_id": 42},
			wantErr: true,
		},
		{
			name:    "document_id empty",
			args:    map[string]any{"document_id": ""},
			wantErr: true,
		},
		{
			name:    "document_id leading trailing whitespace",
			args:    map[string]any{"document_id": " selected-doc "},
			wantErr: true,
		},
		{
			name:    "document_id dotdot",
			args:    map[string]any{"document_id": ".."},
			wantErr: true,
		},
		{
			name:    "document_id traversal",
			args:    map[string]any{"document_id": "../other"},
			wantErr: true,
		},
		{
			name:    "document_id other type",
			args:    map[string]any{"document_id": true},
			wantErr: true,
		},
		{
			name:    "document_id URL",
			args:    map[string]any{"document_id": "https://docs.google.com/document/d/abc123"},
			wantErr: true,
		},
		{
			name:    "tab nil",
			args:    map[string]any{"document_id": "selected-doc", "tab": nil},
			wantErr: true,
		},
		{
			name:    "tab int",
			args:    map[string]any{"document_id": "selected-doc", "tab": 42},
			wantErr: true,
		},
		{
			name:    "tab newline",
			args:    map[string]any{"document_id": "selected-doc", "tab": "a\nb"},
			wantErr: true,
		},
		{
			name:    "tab too long",
			args:    map[string]any{"document_id": "selected-doc", "tab": strings.Repeat("x", 513)},
			wantErr: true,
		},
	}

	seen := make(map[string]int, len(tools))
	for _, spec := range tools {
		seen[spec.Name]++
		sub, ok := wantSub[spec.Name]
		if !ok {
			t.Errorf("unexpected tool %q", spec.Name)
			continue
		}
		if seen[spec.Name] > 1 {
			t.Errorf("tool %q registered %d times", spec.Name, seen[spec.Name])
		}
		if spec.Service != "docs" {
			t.Errorf("%s: Service = %q, want %q", spec.Name, spec.Service, "docs")
		}
		if spec.Risk != mcpRiskRead {
			t.Errorf("%s: Risk = %q, want %q", spec.Name, spec.Risk, mcpRiskRead)
		}
		for _, tc := range cases {
			t.Run(spec.Name+"/"+tc.name, func(t *testing.T) {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = tc.args
				argv, err := spec.BuildArgs(req)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("%s: argv = %q, want error", tc.name, argv)
					}
					return
				}
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", tc.name, err)
				}
				want := append([]string{}, sub...)
				if tc.tab != "" {
					want = append(want, tc.tab)
				}
				if tc.docID != "" {
					want = append(want, "--", tc.docID)
				}
				if !slices.Equal(argv, want) {
					t.Errorf("%s: argv = %q, want %q", tc.name, argv, want)
				}
			})
		}
	}
	for name := range wantSub {
		if seen[name] == 0 {
			t.Errorf("missing tool %q", name)
		}
	}
}
