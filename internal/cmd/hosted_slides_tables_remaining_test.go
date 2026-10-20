package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedSlidesTableRemainingToolsBuildTypedArgs(t *testing.T) {
	specs := hostedSlidesTableRemainingTools()
	if len(specs) != 11 {
		t.Fatalf("tools = %d, want 11", len(specs))
	}
	byName := make(map[string]mcpToolSpec, len(specs))
	for _, spec := range specs {
		if spec.Service != "slides" || spec.Risk != mcpRiskWrite {
			t.Fatalf("unexpected risk/service for %s", spec.Name)
		}
		byName[spec.Name] = spec
	}

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"slides_table_border_style", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(0), "col": float64(1),
			"row_span": float64(1), "col_span": float64(1), "position": "OUTER", "border_color": "#EA4335",
			"weight": float64(2), "dash": "SOLID",
		}, "slides table border style --row=0 --col=1 --row-span=1 --col-span=1 --position=OUTER --border-color=#EA4335 --weight=2 --dash=SOLID -- selected-deck table:one"},
		{"slides_table_cell_style", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1), "col": float64(2),
			"fill_color": "#3367D6", "content_align": "MIDDLE", "bold": true, "text_color": "#FFFFFF", "size": float64(18), "font": "Cambria",
		}, "slides table cell style --row=1 --col=2 --fill-color=#3367D6 --text-color=#FFFFFF --font=Cambria --content-align=MIDDLE --size=18 --bold -- selected-deck table:one"},
		{"slides_table_column_delete", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "col": float64(1),
		}, "slides table column delete --col=1 -- selected-deck table:one"},
		{"slides_table_column_insert", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "col": float64(1), "count": float64(2), "right": true,
		}, "slides table column insert --col=1 --right --count=2 -- selected-deck table:one"},
		{"slides_table_column_size", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "col": float64(1), "width": float64(120),
		}, "slides table column size --col=1 --width=120 -- selected-deck table:one"},
		{"slides_table_create", map[string]any{
			"presentation_id": "selected-deck", "slide_id": "slide:one", "object_id": "table:new", "rows": float64(3), "cols": float64(4),
		}, "slides table create --object-id=table:new --rows=3 --cols=4 -- selected-deck slide:one"},
		{"slides_table_merge", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(0), "col": float64(1), "row_span": float64(1), "col_span": float64(2),
		}, "slides table merge --row=0 --col=1 --row-span=1 --col-span=2 -- selected-deck table:one"},
		{"slides_table_row_delete", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1),
		}, "slides table row delete --row=1 -- selected-deck table:one"},
		{"slides_table_row_insert", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1), "count": float64(2), "below": true,
		}, "slides table row insert --row=1 --below --count=2 -- selected-deck table:one"},
		{"slides_table_row_size", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(1), "height": float64(48),
		}, "slides table row size --row=1 --height=48 -- selected-deck table:one"},
		{"slides_table_unmerge", map[string]any{
			"presentation_id": "selected-deck", "table_object_id": "table:one", "row": float64(0), "col": float64(1), "row_span": float64(1), "col_span": float64(2),
		}, "slides table unmerge --row=0 --col=1 --row-span=1 --col-span=2 -- selected-deck table:one"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, ok := byName[test.name]
			if !ok {
				t.Fatalf("missing tool %s", test.name)
			}
			args, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: test.args}})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(args, " ") != test.want {
				t.Fatalf("args = %q, want %q", strings.Join(args, " "), test.want)
			}
		})
	}
}

func TestHostedSlidesTableRemainingToolsRejectUntypedArguments(t *testing.T) {
	byName := map[string]mcpToolSpec{}
	for _, spec := range hostedSlidesTableRemainingTools() {
		byName[spec.Name] = spec
	}

	for _, test := range []struct {
		tool, key string
		bad       any
	}{
		{"slides_table_row_insert", "presentation_id", "../foreign"},
		{"slides_table_row_insert", "table_object_id", 42},
		{"slides_table_row_insert", "row", "1"},
		{"slides_table_column_insert", "col", "0"},
	} {
		args := map[string]any{"presentation_id": "selected-deck", "table_object_id": "table:one"}
		if test.tool == "slides_table_column_insert" {
			args["col"] = 0
		} else {
			args["row"] = 0
		}
		args[test.key] = test.bad
		if _, err := byName[test.tool].BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}); err == nil {
			t.Fatalf("accepted invalid %s=%v", test.key, test.bad)
		}
	}
}

func TestHostedSlidesTableNativeDefaults(t *testing.T) {
	for _, test := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"slides_table_row_insert", map[string]any{"row": 0}, "--count=1"},
		{"slides_table_column_insert", map[string]any{"col": 0}, "--count=1"},
		{"slides_table_unmerge", map[string]any{"row": 0, "col": 0}, "--row-span=1 --col-span=1"},
		{"slides_table_border_style", map[string]any{"row": 0, "col": 0, "border_color": "#112233"}, "--position=ALL"},
		{"slides_table_border_style", map[string]any{"row": 0, "col": 0, "transparent": true}, "--transparent"},
	} {
		test.args["presentation_id"] = "selected-deck"
		test.args["table_object_id"] = "table:one"
		got, err := hostedSlidesTableRemainingArgs(test.name, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: test.args}})
		if err != nil || !strings.Contains(strings.Join(got, " "), test.want) {
			t.Fatalf("%s defaults lost %v %v", test.name, got, err)
		}
	}
}
