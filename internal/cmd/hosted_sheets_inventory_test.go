package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedSheetsInventoryToolsShape(t *testing.T) {
	wantCount := 5

	specs := hostedSheetsInventoryTools()
	if len(specs) != wantCount {
		t.Fatalf("got %d sheets inventory tools, want %d", len(specs), wantCount)
	}

	seen := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if spec.Service != "sheets" {
			t.Errorf("%s: service = %q, want %q", spec.Name, spec.Service, "sheets")
		}
		if spec.Risk != mcpRiskRead {
			t.Errorf("%s: risk = %q, want %q", spec.Name, spec.Risk, mcpRiskRead)
		}
		if spec.Description == "" {
			t.Errorf("%s: empty description", spec.Name)
		}
		if len(spec.Options) != 1 {
			t.Errorf("%s: got %d tool options, want 1", spec.Name, len(spec.Options))
		}

		seen[spec.Name] = true
	}

	wantNames := []string{
		"sheets_chart_list",
		"sheets_named_ranges_list",
		"sheets_table_list",
		"sheets_banding_list",
		"sheets_conditional_format_list",
	}
	for _, name := range wantNames {
		if !seen[name] {
			t.Errorf("missing sheets inventory tool %q", name)
		}
	}
}

func TestHostedSheetsInventoryBuildArgsExactArgv(t *testing.T) {
	wantArgv := map[string][]string{
		"sheets_chart_list":              {"sheets", "chart", "list", "--", "id-A_1"},
		"sheets_named_ranges_list":       {"sheets", "named-ranges", "list", "--", "id-A_1"},
		"sheets_table_list":              {"sheets", "table", "list", "--", "id-A_1"},
		"sheets_banding_list":            {"sheets", "banding", "list", "--", "id-A_1"},
		"sheets_conditional_format_list": {"sheets", "conditional-format", "list", "--", "id-A_1"},
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"spreadsheet_id": "id-A_1"}

	for _, spec := range hostedSheetsInventoryTools() {
		want, ok := wantArgv[spec.Name]
		if !ok {
			t.Fatalf("unexpected sheets inventory tool %q", spec.Name)
		}

		argv, err := spec.BuildArgs(req)
		if err != nil {
			t.Fatalf("%s: BuildArgs returned error: %v", spec.Name, err)
		}
		if len(argv) != len(want) {
			t.Fatalf("%s: got argv %v, want %v", spec.Name, argv, want)
		}
		for i, part := range want {
			if argv[i] != part {
				t.Errorf("%s: argv[%d] = %q, want %q", spec.Name, i, argv[i], part)
			}
		}
	}
}

func TestHostedSheetsInventorySchemaOnlySpreadsheetID(t *testing.T) {
	for _, spec := range hostedSheetsInventoryTools() {
		tool := mcp.NewTool(spec.Name, spec.Options...)

		if len(tool.InputSchema.Properties) != 1 {
			t.Errorf("%s: got %d schema properties, want 1", spec.Name, len(tool.InputSchema.Properties))
		}
		if _, ok := tool.InputSchema.Properties["spreadsheet_id"]; !ok {
			t.Errorf("%s: schema must expose only spreadsheet_id", spec.Name)
		}
		if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "spreadsheet_id" {
			t.Errorf("%s: required = %v, want [spreadsheet_id]", spec.Name, tool.InputSchema.Required)
		}
	}
}

func TestHostedSheetsInventoryRejectsMissingAndMalformedIDs(t *testing.T) {
	malformed := []string{
		"",
		"spreadsheet id",
		"../../etc/passwd",
		"a/b",
		"a.b",
		"a;b",
		"a:b",
		"id\n",
		strings.Repeat("a", 257),
	}

	for _, spec := range hostedSheetsInventoryTools() {
		empty := mcp.CallToolRequest{}
		if _, err := spec.BuildArgs(empty); err == nil {
			t.Errorf("%s: expected missing spreadsheet_id to be rejected", spec.Name)
		}

		for _, id := range malformed {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"spreadsheet_id": id}

			_, err := spec.BuildArgs(req)
			if err == nil {
				t.Errorf("%s: malformed id %q: got err %v, want input rejection", spec.Name, id, err)
			}
		}
	}
}

func TestHostedSheetsInventoryAcceptsValidIDs(t *testing.T) {
	valid := []string{
		"abc",
		"1-Sheet_a",
		strings.Repeat("b", 256),
	}

	for _, spec := range hostedSheetsInventoryTools() {
		for _, id := range valid {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"spreadsheet_id": id}

			argv, err := spec.BuildArgs(req)
			if err != nil {
				t.Fatalf("%s: valid id %q rejected: %v", spec.Name, id, err)
			}
			if len(argv) < 2 || argv[len(argv)-2] != "--" || argv[len(argv)-1] != id {
				t.Errorf("%s: argv %v must end with -- %q", spec.Name, argv, id)
			}
		}
	}
}
