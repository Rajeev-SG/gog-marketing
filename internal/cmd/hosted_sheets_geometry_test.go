package cmd

import (
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedSheetsGeometryArguments(t *testing.T) {
	tools := hostedSheetsGeometryTools()
	if len(tools) != 4 {
		t.Fatalf("geometry tools=%d", len(tools))
	}
	specs := map[string]mcpToolSpec{}
	for _, spec := range tools {
		if spec.Service != "sheets" || spec.Risk != mcpRiskWrite {
			t.Fatalf("unexpected geometry contract %s", spec.Name)
		}
		if _, exists := specs[spec.Name]; exists {
			t.Fatalf("duplicate%s", spec.Name)
		}
		specs[spec.Name] = spec
	}
	build := func(name string, args map[string]any) ([]string, error) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = args
		return specs[name].BuildArgs(req)
	}
	var cases []struct {
		Name string
		Args map[string]any
		Want []string
	}
	if err := json.Unmarshal([]byte(`[{"name":"sheets_merge","args":{"range":"First!A1:B2","spreadsheet_id":"selected-sheet"},"want":["sheets","merge","--type=MERGE_ALL","--","selected-sheet","First!A1:B2"]},{"name":"sheets_merge","args":{"range":"First!A1:B2","type":"MERGE_ROWS","spreadsheet_id":"selected-sheet"},"want":["sheets","merge","--type=MERGE_ROWS","--","selected-sheet","First!A1:B2"]},{"name":"sheets_merge","args":{"range":"First!A1:B2","type":"MERGE_COLUMNS","spreadsheet_id":"selected-sheet"},"want":["sheets","merge","--type=MERGE_COLUMNS","--","selected-sheet","First!A1:B2"]},{"name":"sheets_unmerge","args":{"range":"First!A1:B2","spreadsheet_id":"selected-sheet"},"want":["sheets","unmerge","--","selected-sheet","First!A1:B2"]},{"name":"sheets_resize_columns","args":{"columns":"A:C","width":120,"spreadsheet_id":"selected-sheet"},"want":["sheets","resize-columns","--width=120","--","selected-sheet","A:C"]},{"name":"sheets_resize_columns","args":{"columns":"'Revenue Tab'!C:B","auto":true,"spreadsheet_id":"selected-sheet"},"want":["sheets","resize-columns","--auto","--","selected-sheet","'Revenue Tab'!C:B"]},{"name":"sheets_resize_rows","args":{"rows":"'Revenue Tab'!2:1","height":24,"auto":false,"spreadsheet_id":"selected-sheet"},"want":["sheets","resize-rows","--height=24","--","selected-sheet","'Revenue Tab'!2:1"]},{"name":"sheets_resize_rows","args":{"rows":"1:2","auto":true,"spreadsheet_id":"selected-sheet"},"want":["sheets","resize-rows","--auto","--","selected-sheet","1:2"]},{"name":"sheets_resize_columns","args":{"columns":"'--account=else'!A:C","width":80,"spreadsheet_id":"selected-sheet"},"want":["sheets","resize-columns","--width=80","--","selected-sheet","'--account=else'!A:C"]}]`), &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		got, err := build(test.Name, test.Args)
		if err != nil || !slices.Equal(got, test.Want) {
			t.Fatalf("%s argv=%q want%q err=%v", test.Name, got, test.Want, err)
		}
	}
	valid := map[string]map[string]any{
		"sheets_merge": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, "sheets_unmerge": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2"}, "sheets_resize_columns": {"spreadsheet_id": "selected-sheet", "columns": "A:C", "width": 120}, "sheets_resize_rows": {"spreadsheet_id": "selected-sheet", "rows": "1:2", "height": 24},
	}
	for name, args := range valid {
		rangeArg := "range"
		if name == "sheets_resize_columns" {
			rangeArg = "columns"
		}
		if name == "sheets_resize_rows" {
			rangeArg = "rows"
		}
		for _, bad := range []any{nil, 1, "", " spaced ", "a/b", "https://example.test/sheet"} {
			candidate := maps.Clone(args)
			candidate["spreadsheet_id"] = bad
			if _, err := build(name, candidate); err == nil {
				t.Fatalf("%s accepted invalid id%v", name, bad)
			}
		}
		missing := maps.Clone(args)
		delete(missing, "spreadsheet_id")
		if _, err := build(name, missing); err == nil {
			t.Fatalf("%s accepted missing id", name)
		}
		for _, bad := range []any{nil, 1, "", " ", "First!A1\n", strings.Repeat("x", 4097)} {
			candidate := maps.Clone(args)
			candidate[rangeArg] = bad
			if _, err := build(name, candidate); err == nil {
				t.Fatalf("%s accepted invalid range%v", name, bad)
			}
		}
		missing = maps.Clone(args)
		delete(missing, rangeArg)
		if _, err := build(name, missing); err == nil {
			t.Fatalf("%s accepted missing range", name)
		}
	}
	for _, name := range []string{"sheets_merge", "sheets_unmerge"} {
		args := maps.Clone(valid[name])
		args["range"] = "A1:B2"
		if _, err := build(name, args); err == nil {
			t.Fatalf("%s accepted unqualified cell range", name)
		}
	}
	for _, bad := range []any{nil, 1, "INVALID"} {
		args := maps.Clone(valid["sheets_merge"])
		args["type"] = bad
		if _, err := build("sheets_merge", args); err == nil {
			t.Fatalf("accepted invalid merge type%v", bad)
		}
	}
	for _, test := range []struct{ name, size, span, invalid string }{{"sheets_resize_columns", "width", "columns", "First!1:3"}, {"sheets_resize_rows", "height", "rows", "First!A:C"}} {
		for _, bad := range []any{nil, "120", 0, -1, 1.5, math.NaN(), math.Inf(1), int64(math.MaxInt32) + 1} {
			args := maps.Clone(valid[test.name])
			args[test.size] = bad
			if _, err := build(test.name, args); err == nil {
				t.Fatalf("%s accepted invalid size%v", test.name, bad)
			}
		}
		for _, bad := range []any{nil, "true", 1, true} {
			args := maps.Clone(valid[test.name])
			args["auto"] = bad
			if _, err := build(test.name, args); err == nil {
				t.Fatalf("%s accepted invalid/conflicting auto%v", test.name, bad)
			}
		}
		args := maps.Clone(valid[test.name])
		delete(args, test.size)
		if _, err := build(test.name, args); err == nil {
			t.Fatalf("%s accepted missing size/mode", test.name)
		}
		args["auto"] = false
		if _, err := build(test.name, args); err == nil {
			t.Fatalf("%s accepted false auto without size", test.name)
		}
		args = maps.Clone(valid[test.name])
		args[test.span] = test.invalid
		if _, err := build(test.name, args); err == nil {
			t.Fatalf("%s accepted wrong dimension range", test.name)
		}
		for _, size := range []any{int64(math.MaxInt32), float64(math.MaxInt32)} {
			args := maps.Clone(valid[test.name])
			args[test.size] = size
			got, err := build(test.name, args)
			if err != nil || !slices.Contains(got, "--"+test.size+"=2147483647") {
				t.Fatalf("%s rejected int32 boundary argv=%q err=%v", test.name, got, err)
			}
		}
	}
}
