package cmd

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedSheetsRangeNoteArguments(t *testing.T) {
	tools := hostedSheetsRangeNoteTools()
	if len(tools) != 4 {
		t.Fatalf("range/note tools=%d", len(tools))
	}
	specs := map[string]mcpToolSpec{}
	for _, spec := range tools {
		if spec.Service != "sheets" || spec.Risk != mcpRiskWrite {
			t.Fatalf("unexpected contract%s", spec.Name)
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
	if err := json.Unmarshal([]byte(`[{"name":"sheets_named_ranges_add","args":{"name":"NewRange","range":"First!A1:B2","spreadsheet_id":"selected-sheet"},"want":["sheets","named-ranges","add","--","selected-sheet","NewRange","First!A1:B2"]},{"name":"sheets_named_ranges_update","args":{"name_or_id":"range-1","name":"Renamed","spreadsheet_id":"selected-sheet"},"want":["sheets","named-ranges","update","--name=Renamed","--","selected-sheet","range-1"]},{"name":"sheets_named_ranges_update","args":{"name_or_id":"range-1","range":"'Revenue Tab'!B2:C3","spreadsheet_id":"selected-sheet"},"want":["sheets","named-ranges","update","--range='Revenue Tab'!B2:C3","--","selected-sheet","range-1"]},{"name":"sheets_named_ranges_update","args":{"name_or_id":"range-1","name":"Renamed","range":"'Revenue Tab'!B2:C3","spreadsheet_id":"selected-sheet"},"want":["sheets","named-ranges","update","--name=Renamed","--range='Revenue Tab'!B2:C3","--","selected-sheet","range-1"]},{"name":"sheets_named_ranges_update","args":{"name_or_id":"newrange","name":"Updated","spreadsheet_id":"selected-sheet"},"want":["sheets","named-ranges","update","--name=Updated","--","selected-sheet","newrange"]},{"name":"sheets_named_ranges_delete","args":{"name_or_id":"range-1","spreadsheet_id":"selected-sheet"},"want":["sheets","named-ranges","delete","--","selected-sheet","range-1"]},{"name":"sheets_update_note","args":{"range":"First!A1:B2","note":"Line 1\nLine 2","spreadsheet_id":"selected-sheet"},"want":["sheets","update-note","--note=Line 1\nLine 2","--","selected-sheet","First!A1:B2"]},{"name":"sheets_update_note","args":{"range":"First!A1:B2","note":"","spreadsheet_id":"selected-sheet"},"want":["sheets","update-note","--note=","--","selected-sheet","First!A1:B2"]},{"name":"sheets_update_note","args":{"range":"First!A1:B2","note":"@/tmp/not-a-file","spreadsheet_id":"selected-sheet"},"want":["sheets","update-note","--note=@/tmp/not-a-file","--","selected-sheet","First!A1:B2"]},{"name":"sheets_named_ranges_update","args":{"spreadsheet_id":"selected-sheet","name_or_id":"range-1","name":"","range":"First!A1:B2"},"want":["sheets","named-ranges","update","--name=","--range=First!A1:B2","--","selected-sheet","range-1"]},{"name":"sheets_named_ranges_update","args":{"spreadsheet_id":"selected-sheet","name_or_id":"range-1","name":"Renamed","range":""},"want":["sheets","named-ranges","update","--name=Renamed","--range=","--","selected-sheet","range-1"]}]`), &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		got, err := build(test.Name, test.Args)
		if err != nil || !slices.Equal(got, test.Want) {
			t.Fatalf("%s argv=%q want%q err=%v", test.Name, got, test.Want, err)
		}
	}
	valid := map[string]map[string]any{"sheets_named_ranges_add": {"spreadsheet_id": "selected-sheet", "name": "Created", "range": "First!A1:B2"}, "sheets_named_ranges_update": {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1", "name": "Renamed"}, "sheets_named_ranges_delete": {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1"}, "sheets_update_note": {"spreadsheet_id": "selected-sheet", "range": "First!A1:B2", "note": "Inline"}}
	for name, args := range valid {
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
	}
	for _, name := range []string{"sheets_named_ranges_add", "sheets_named_ranges_update", "sheets_update_note"} {
		for _, bad := range []any{nil, 1, "A1:B2", "First!A1\n", strings.Repeat("x", 4097)} {
			candidate := maps.Clone(valid[name])
			candidate["range"] = bad
			if _, err := build(name, candidate); err == nil {
				t.Fatalf("%s accepted invalid range%v", name, bad)
			}
		}
	}
	for _, name := range []string{"sheets_named_ranges_add", "sheets_named_ranges_update", "sheets_named_ranges_delete"} {
		field := "name_or_id"
		if name == "sheets_named_ranges_add" {
			field = "name"
		}
		for _, bad := range []any{nil, 1, "", " ", "bad\nlabel", strings.Repeat("x", 4097)} {
			candidate := maps.Clone(valid[name])
			candidate[field] = bad
			if _, err := build(name, candidate); err == nil {
				t.Fatalf("%s accepted invalid selector/name%v", name, bad)
			}
		}
	}
	for _, args := range []map[string]any{{"spreadsheet_id": "selected-sheet", "name_or_id": "range-1"}, {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1", "name": "", "range": " "}, {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1", "name": nil, "range": "First!A1:B2"}, {"spreadsheet_id": "selected-sheet", "name_or_id": "range-1", "name": "Renamed", "range": nil}} {
		if _, err := build("sheets_named_ranges_update", args); err == nil {
			t.Fatalf("accepted invalid partial update%v", args)
		}
	}
	for _, bad := range []any{nil, 1, strings.Repeat("n", 262145)} {
		candidate := maps.Clone(valid["sheets_update_note"])
		candidate["note"] = bad
		if _, err := build("sheets_update_note", candidate); err == nil {
			t.Fatal("accepted invalid note")
		}
	}
	missing := maps.Clone(valid["sheets_update_note"])
	delete(missing, "note")
	if _, err := build("sheets_update_note", missing); err == nil {
		t.Fatal("accepted missing note as clear")
	}
	for _, note := range []string{"", " ", "Line 1\n你好 🌍", "\x00", "--note-file=/tmp/not-a-file", "=IMPORTRANGE(\"foreign\",\"A1\")", strings.Repeat("n", 262144)} {
		candidate := maps.Clone(valid["sheets_update_note"])
		candidate["note"] = note
		got, err := build("sheets_update_note", candidate)
		if err != nil || !slices.Equal(got, []string{"sheets", "update-note", "--note=" + note, "--", "selected-sheet", "First!A1:B2"}) {
			t.Fatalf("note literal/boundary not preserved err=%v", err)
		}
	}
}
