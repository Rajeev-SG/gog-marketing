package cmd

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

const sheetsRemainingCasesJSON = `{
 "sheets_copy":{"title":"Copy"},
 "sheets_banding_clear":{"id":12},
 "sheets_banding_set":{"range":"Revenue Tab!A1:B3","row_properties_json":"{\"firstBandColor\":{\"red\":1}}"},
 "sheets_batch_update":{"data_json":"[{\"range\":\"'Revenue Tab'!A1\",\"values\":[[\"hello\"]]}]"},
 "sheets_chart_create":{"sheet":"Revenue Tab","spec_json":"{\"title\":\"Revenue\",\"basicChart\":{\"chartType\":\"COLUMN\"}}"},
 "sheets_chart_delete":{"chart_id":73},
 "sheets_chart_update":{"chart_id":73,"spec_json":"{\"title\":\"Updated\"}"},
 "sheets_conditional_format_add":{"range":"Revenue Tab!A1:B3","type":"blank","format_json":"{\"textFormat\":{\"bold\":true}}"},
 "sheets_conditional_format_clear":{"sheet":"Revenue Tab","index":0},
 "sheets_copy_paste":{"source":"Revenue Tab!A1:B2","dest":"Revenue Tab!C1:D2","type":"VALUES"},
 "sheets_delete_dimension":{"target":"Revenue Tab!2:2","dimension":"ROWS"},
 "sheets_duplicate_tab":{"source_tab":"Revenue Tab","new_name":"Copy"},
 "sheets_filter_set":{"range":"Revenue Tab!A1:B3"},
 "sheets_find_replace":{"sheet":"Revenue Tab","find":"old","replace":"new"},
 "sheets_format":{"range":"Revenue Tab!A1:B3","format_json":"{\"textFormat\":{\"bold\":false}}"},
 "sheets_insert":{"sheet":"Revenue Tab","dimension":"rows","start":2},
 "sheets_links_set":{"cell":"Revenue Tab!A1","url":"https://example.test/","text":"Example"},
 "sheets_raw":{},
 "sheets_table_create":{"range":"Revenue Tab!A1:B3","name":"Created","columns_json":"[{\"columnName\":\"A\",\"columnType\":\"TEXT\"},{\"columnName\":\"B\",\"columnType\":\"TEXT\"}]"},
 "sheets_table_append":{"table_id":"table-1","values_json":"[[\"appended\",42]]"},
 "sheets_table_clear":{"table_id":"table-1"},
 "sheets_table_delete":{"table_id":"table-1"},
 "sheets_validation_clear":{"range":"Revenue Tab!D1:D3"},
 "sheets_validation_set":{"range":"Revenue Tab!D1:D3","type":"BOOLEAN"}
}`

func TestHostedSheetsRemainingArguments(t *testing.T) {
	var cases map[string]map[string]any
	if err := json.Unmarshal([]byte(sheetsRemainingCasesJSON), &cases); err != nil {
		t.Fatal(err)
	}
	tools := hostedSheetsRemainingTools()
	if len(tools) != len(cases) {
		t.Fatalf("tools=%d cases=%d", len(tools), len(cases))
	}
	for _, spec := range tools {
		t.Run(spec.Name, func(t *testing.T) {
			args := maps.Clone(cases[spec.Name])
			args["spreadsheet_id"] = "selected-sheet"
			build := func(args map[string]any) ([]string, error) {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = args
				return spec.BuildArgs(req)
			}
			argv, err := build(args)
			if err != nil {
				t.Fatal(err)
			}
			sep := slices.Index(argv, "--")
			if sep < 1 || argv[sep+1] != "selected-sheet" || argv[0] != "sheets" {
				t.Fatalf("unsafe argv %q", argv)
			}
			for _, bad := range []any{nil, "", true, "../other", "https://example.test", " selected-sheet"} {
				c := maps.Clone(args)
				c["spreadsheet_id"] = bad
				if _, err := build(c); err == nil {
					t.Fatalf("accepted id %v", bad)
				}
			}
			for _, key := range []string{"requests_json", "parent", "account", "force", "path", "method"} {
				c := maps.Clone(args)
				c[key] = "unsafe"
				if _, err := build(c); err == nil {
					t.Fatalf("accepted %s", key)
				}
			}
			for key, value := range args {
				if key == "spreadsheet_id" {
					continue
				}
				c := maps.Clone(args)
				c[key] = nil
				if _, err := build(c); err == nil {
					t.Fatalf("accepted null %s", key)
				}
				if strings.HasSuffix(key, "_json") {
					for _, bad := range []string{"@/tmp/secret", "@-", "https://example.test", "{\"requests\":[{\"deleteSheet\":{}}]}"} {
						c[key] = bad
						if _, err := build(c); err == nil {
							t.Fatalf("accepted %s=%s original=%v", key, bad, value)
						}
					}
				}
			}
		})
	}
}

func TestHostedSheetsRemainingTypedJSON(t *testing.T) {
	for _, test := range []struct{ kind, value string }{
		{"format", `{"requests":[]}`},
		{"format", `{"textFormat":{"unknown":true}}`},
		{"chart", `{"title":"x","dataSourceChartProperties":{"dataSourceId":"paid"}}`},
		{"chart", `{"title":"x","basicChart":{"domains":[{"domain":{"dataSourceColumnReference":{"name":"x"}}}]}}`},
		{"banding", `{"deleteSheet":{}}`},
		{"columns", `[{"columnName":"x","requests":[]}]`},
		{"values", `[{"range":"A1","values":[[{"deleteSheet":{}}]]}]`},
	} {
		if _, err := hostedSheetsRemainingValue(test.value, test.kind); err == nil {
			t.Errorf("accepted %s %s", test.kind, test.value)
		}
	}
}

func TestHostedSheetsRemainingCrossFileFormulaDenials(t *testing.T) {
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"sheets_batch_update", map[string]any{"data_json": `[{"range":"Sheet1!A1","values":[["\u003d\u0049MPORTRANGE(\"foreign\",\"A1\")"]]}]`}},
		{"sheets_find_replace", map[string]any{"sheet": "Sheet1", "find": "old", "replace": " =importrange (\"foreign\",\"A1\")"}},
		{"sheets_conditional_format_add", map[string]any{"range": "Sheet1!A1", "type": "custom-formula", "expr": "=IMPORTRANGE(\"foreign\",\"A1\")", "format_json": `{"textFormat":{"bold":true}}`}},
		{"sheets_validation_set", map[string]any{"range": "Sheet1!A1", "type": "CUSTOM_FORMULA", "value": "=IMPORTRANGE(\"foreign\",\"A1\")"}},
		{"sheets_table_create", map[string]any{"range": "Sheet1!A1:B3", "name": "Table", "columns_json": `[{"columnName":"x","columnType":"DROPDOWN","dataValidationRule":{"condition":{"type":"CUSTOM_FORMULA","values":[{"userEnteredValue":"\u003dIMPO\u0052TRANGE(\"foreign\",\"A1\")"}]}}}]`}},
	} {
		for _, spec := range hostedSheetsRemainingTools() {
			if spec.Name == test.name {
				test.args["spreadsheet_id"] = "selected-sheet"
				req := mcp.CallToolRequest{}
				req.Params.Arguments = test.args
				if _, err := spec.BuildArgs(req); err == nil {
					t.Errorf("%s admitted cross-file formula", spec.Name)
				}
			}
		}
	}
	for _, raw := range []string{`[[{"formula":"=1"}]]`, `[[[1]]]`, `@-`, `[]`} {
		if _, err := hostedSheetsRemainingValue(raw, "rows"); err == nil {
			t.Errorf("accepted rows %s", raw)
		}
	}
	if _, err := hostedSheetsRemainingValue(`[["=IMPORTRANGE(\"foreign\",\"A1\")",true,42,null]]`, "rows"); err != nil {
		t.Fatalf("RAW-only append must preserve literal strings: %v", err)
	}
}

func TestHostedSheetsRemainingFormulaSemanticParity(t *testing.T) {
	for _, raw := range []string{
		`"\u003d\u0049MPO\u0052TRANGE(\"other\",\"A1\")"`,
		`{"condition":{"values":[{"userEnteredValue":"  =importrange (\"other\",\"A1\")"}]}}`,
		`[{"values":[["=IF(TRUE,IMPORTRANGE(\"other\",\"A1\"),0)"]]}]`,
	} {
		if !hostedSheetsRemainingJSONFormula(raw) {
			t.Errorf("escaped formula not detected: %s", raw)
		}
	}
	for _, raw := range []string{`"IMPORTRANGE("`, `"'=IMPORTRANGE(\"other\",\"A1\")"`, `"=SUM(A1:A2)"`} {
		if hostedSheetsRemainingJSONFormula(raw) {
			t.Errorf("non-cross-file formula rejected: %s", raw)
		}
	}
	for _, spec := range hostedSheetsRemainingTools() {
		if spec.Name == "sheets_table_append" {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{"spreadsheet_id": "selected-sheet", "table_id": "table-1", "values_json": `[["=IMPORTRANGE(\"other\",\"A1\")"]]`}
			argv, err := spec.BuildArgs(req)
			if err != nil || !slices.Contains(argv, "--input=RAW") {
				t.Fatalf("append RAW guarantee: %q %v", argv, err)
			}
		}
	}
}
