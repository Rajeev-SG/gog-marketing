package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// hostedSheetsLayoutTestRequest builds a plain MCP call-tool request over the
// given typed arguments; the specs under test always come from
// hostedSheetsLayoutTools, never from test-only fakes.
func hostedSheetsLayoutTestRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

func requiredHas(required []string, field string) bool {
	for _, name := range required {
		if name == field {
			return true
		}
	}

	return false
}

func TestHostedSheetsLayoutToolSpecs(t *testing.T) {
	specs := hostedSheetsLayoutTools()
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want 2", len(specs))
	}
	if specs[0].Name != "sheets_freeze" || specs[1].Name != "sheets_reorder_tab" {
		t.Fatalf("spec names = %q, %q; want sheets_freeze, sheets_reorder_tab", specs[0].Name, specs[1].Name)
	}

	wantFields := map[string][]string{
		"sheets_freeze":      {"spreadsheet_id", "sheet", "rows", "cols"},
		"sheets_reorder_tab": {"spreadsheet_id", "tab", "to"},
	}
	wantRequired := map[string]map[string]bool{
		"sheets_freeze":      {"spreadsheet_id": true, "sheet": true},
		"sheets_reorder_tab": {"spreadsheet_id": true, "tab": true, "to": true},
	}

	for _, spec := range specs {
		if spec.Service != "sheets" {
			t.Errorf("%s: Service = %q, want sheets", spec.Name, spec.Service)
		}
		if spec.Risk != mcpRiskWrite {
			t.Errorf("%s: Risk = %v, want %v", spec.Name, spec.Risk, mcpRiskWrite)
		}
		if spec.BuildArgs == nil {
			t.Errorf("%s: BuildArgs is nil", spec.Name)
		}

		tool := mcp.NewTool(spec.Name, spec.Options...)
		gotFields := make(map[string]bool, len(tool.InputSchema.Properties))
		for field := range tool.InputSchema.Properties {
			gotFields[field] = true
		}
		for _, field := range wantFields[spec.Name] {
			if !gotFields[field] {
				t.Errorf("%s: schema missing field %q", spec.Name, field)
			}
		}
		if len(gotFields) != len(wantFields[spec.Name]) {
			t.Errorf("%s: schema fields %v, want exactly %v", spec.Name, gotFields, wantFields[spec.Name])
		}
		for _, field := range tool.InputSchema.Required {
			if !wantRequired[spec.Name][field] {
				t.Errorf("%s: unexpected required field %q", spec.Name, field)
			}
		}
		for field := range wantRequired[spec.Name] {
			if !requiredHas(tool.InputSchema.Required, field) {
				t.Errorf("%s: field %q must be required", spec.Name, field)
			}
		}
	}

	for _, optional := range []string{"rows", "cols"} {
		if requiredHas(toolRequired(specs[0]), optional) {
			t.Errorf("sheets_freeze: optional field %q must not be required", optional)
		}
	}
}

func toolRequired(spec mcpToolSpec) []string {
	tool := mcp.NewTool(spec.Name, spec.Options...)

	return tool.InputSchema.Required
}

func TestHostedSheetsFreezeBuildArgs(t *testing.T) {
	freeze := hostedSheetsLayoutTools()[0].BuildArgs

	tests := []struct {
		name string
		args map[string]any
		want []string
	}{
		{
			name: "rows int and cols zero unfreeze",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "sheet": "Sheet One", "rows": int(2), "cols": float64(0)},
			want: []string{"sheets", "freeze", "--sheet=Sheet One", "--rows=2", "--cols=0", "--", "abc123XYZ_-"},
		},
		{
			name: "rows only zero unfreeze",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "sheet": "Data", "rows": float64(0)},
			want: []string{"sheets", "freeze", "--sheet=Data", "--rows=0", "--", "abc123XYZ_-"},
		},
		{
			name: "cols only, omitted rows stays unchanged",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "sheet": "Data", "cols": int64(10000)},
			want: []string{"sheets", "freeze", "--sheet=Data", "--cols=10000", "--", "abc123XYZ_-"},
		},
		{
			name: "unicode and ampersand sheet label",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "sheet": "売上 & Plan", "rows": float64(3)},
			want: []string{"sheets", "freeze", "--sheet=売上 & Plan", "--rows=3", "--", "abc123XYZ_-"},
		},
		{
			name: "literal flag-like sheet label stays one argv element",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "sheet": "--account=x", "rows": float64(1)},
			want: []string{"sheets", "freeze", "--sheet=--account=x", "--rows=1", "--", "abc123XYZ_-"},
		},
		{
			name: "sheet label exactly 512 utf-8 bytes",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "sheet": strings.Repeat("é", 256), "rows": float64(1)},
			want: []string{"sheets", "freeze", "--sheet=" + strings.Repeat("é", 256), "--rows=1", "--", "abc123XYZ_-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := freeze(hostedSheetsLayoutTestRequest(tt.args))
			if err != nil {
				t.Fatalf("freeze(...) error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("freeze(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostedSheetsReorderTabBuildArgs(t *testing.T) {
	reorder := hostedSheetsLayoutTools()[1].BuildArgs

	tests := []struct {
		name string
		args map[string]any
		want []string
	}{
		{
			name: "tab by name with ampersand",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "tab": "Q3 & Q4", "to": float64(3)},
			want: []string{"sheets", "reorder-tab", "--tab=Q3 & Q4", "--to=3", "--", "abc123XYZ_-"},
		},
		{
			name: "tab by numeric id and to zero",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "tab": "12345", "to": int64(0)},
			want: []string{"sheets", "reorder-tab", "--tab=12345", "--to=0", "--", "abc123XYZ_-"},
		},
		{
			name: "to max boundary",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "tab": "Summary", "to": float64(10000)},
			want: []string{"sheets", "reorder-tab", "--tab=Summary", "--to=10000", "--", "abc123XYZ_-"},
		},
		{
			name: "unicode tab label",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "tab": "Résumé & Notes", "to": float64(7)},
			want: []string{"sheets", "reorder-tab", "--tab=Résumé & Notes", "--to=7", "--", "abc123XYZ_-"},
		},
		{
			name: "literal flag-like tab label stays one argv element",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "tab": "--account=x", "to": float64(2)},
			want: []string{"sheets", "reorder-tab", "--tab=--account=x", "--to=2", "--", "abc123XYZ_-"},
		},
		{
			name: "tab label exactly 512 utf-8 bytes",
			args: map[string]any{"spreadsheet_id": "abc123XYZ_-", "tab": strings.Repeat("t", 512), "to": float64(4)},
			want: []string{"sheets", "reorder-tab", "--tab=" + strings.Repeat("t", 512), "--to=4", "--", "abc123XYZ_-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reorder(hostedSheetsLayoutTestRequest(tt.args))
			if err != nil {
				t.Fatalf("reorder-tab(...) error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reorder-tab(...) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostedSheetsLayoutBuildArgsRejectsInvalidInput(t *testing.T) {
	freeze := hostedSheetsLayoutTools()[0].BuildArgs
	reorder := hostedSheetsLayoutTools()[1].BuildArgs
	id := "abc123XYZ_-"

	freezeRejections := []struct {
		name string
		args map[string]any
	}{
		{"missing rows and cols", map[string]any{"spreadsheet_id": id, "sheet": "Data"}},
		{"negative rows", map[string]any{"spreadsheet_id": id, "sheet": "Data", "rows": int(-1)}},
		{"fractional rows", map[string]any{"spreadsheet_id": id, "sheet": "Data", "rows": float64(2.5)}},
		{"string rows", map[string]any{"spreadsheet_id": id, "sheet": "Data", "rows": "2"}},
		{"bool rows", map[string]any{"spreadsheet_id": id, "sheet": "Data", "rows": true}},
		{"too big rows", map[string]any{"spreadsheet_id": id, "sheet": "Data", "rows": float64(10001)}},
		{"sheet leading whitespace", map[string]any{"spreadsheet_id": id, "sheet": " Data", "rows": float64(1)}},
		{"sheet trailing whitespace", map[string]any{"spreadsheet_id": id, "sheet": "Data ", "rows": float64(1)}},
		{"sheet newline", map[string]any{"spreadsheet_id": id, "sheet": "Da\nta", "rows": float64(1)}},
		{"sheet control character", map[string]any{"spreadsheet_id": id, "sheet": "Da\tta", "rows": float64(1)}},
		{"sheet empty", map[string]any{"spreadsheet_id": id, "sheet": "", "rows": float64(1)}},
		{"sheet too long", map[string]any{"spreadsheet_id": id, "sheet": strings.Repeat("x", 513), "rows": float64(1)}},
		{"spreadsheet id whitespace", map[string]any{"spreadsheet_id": " " + id, "sheet": "Data", "rows": float64(1)}},
		{"spreadsheet id empty", map[string]any{"spreadsheet_id": "", "sheet": "Data", "rows": float64(1)}},
		{"spreadsheet id path characters", map[string]any{"spreadsheet_id": "abc/def", "sheet": "Data", "rows": float64(1)}},
	}

	for _, tt := range freezeRejections {
		t.Run("freeze/"+tt.name, func(t *testing.T) {
			_, err := freeze(hostedSheetsLayoutTestRequest(tt.args))
			if err == nil {
				t.Fatalf("freeze(%v) error = %v, want input rejection", tt.args, err)
			}
		})
	}

	reorderRejections := []struct {
		name string
		args map[string]any
	}{
		{"missing to", map[string]any{"spreadsheet_id": id, "tab": "Data"}},
		{"string to", map[string]any{"spreadsheet_id": id, "tab": "Data", "to": "3"}},
		{"fractional to", map[string]any{"spreadsheet_id": id, "tab": "Data", "to": float64(3.5)}},
		{"negative to", map[string]any{"spreadsheet_id": id, "tab": "Data", "to": int64(-1)}},
		{"too big to", map[string]any{"spreadsheet_id": id, "tab": "Data", "to": float64(10001)}},
		{"tab leading whitespace", map[string]any{"spreadsheet_id": id, "tab": " Data", "to": float64(1)}},
		{"tab trailing whitespace", map[string]any{"spreadsheet_id": id, "tab": "Data ", "to": float64(1)}},
		{"tab newline", map[string]any{"spreadsheet_id": id, "tab": "Da\nta", "to": float64(1)}},
		{"tab control character", map[string]any{"spreadsheet_id": id, "tab": "Da\tta", "to": float64(1)}},
		{"tab empty", map[string]any{"spreadsheet_id": id, "tab": "", "to": float64(1)}},
		{"tab too long", map[string]any{"spreadsheet_id": id, "tab": strings.Repeat("t", 513), "to": float64(1)}},
	}

	for _, tt := range reorderRejections {
		t.Run("reorder/"+tt.name, func(t *testing.T) {
			_, err := reorder(hostedSheetsLayoutTestRequest(tt.args))
			if err == nil {
				t.Fatalf("reorder-tab(%v) error = %v, want input rejection", tt.args, err)
			}
		})
	}

	t.Run("freeze/missing sheet", func(t *testing.T) {
		_, err := freeze(hostedSheetsLayoutTestRequest(map[string]any{"spreadsheet_id": id, "rows": float64(1)}))
		if err == nil {
			t.Fatal("freeze(...) error = nil, want non-nil")
		}
	})

	t.Run("freeze/missing spreadsheet id", func(t *testing.T) {
		_, err := freeze(hostedSheetsLayoutTestRequest(map[string]any{"sheet": "Data", "rows": float64(1)}))
		if err == nil {
			t.Fatal("freeze(...) error = nil, want non-nil")
		}
	})

	t.Run("reorder/missing spreadsheet id", func(t *testing.T) {
		_, err := reorder(hostedSheetsLayoutTestRequest(map[string]any{"tab": "Data", "to": float64(1)}))
		if err == nil {
			t.Fatal("reorder-tab(...) error = nil, want non-nil")
		}
	})
}
