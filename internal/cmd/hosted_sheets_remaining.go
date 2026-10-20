package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetschart"
	"github.com/openclaw/gogcli/internal/sheetsconditional"
	"github.com/openclaw/gogcli/internal/sheetsdimension"
	"github.com/openclaw/gogcli/internal/sheetsformat"
	"github.com/openclaw/gogcli/internal/sheetsvalidation"
)

var hostedSheetsRemainingCrossFileFormula = regexp.MustCompile(`(?i)\bIMPORTRANGE\s*\(`)

var errHostedSheetsRemaining = errors.New("invalid fixed Sheets operation arguments")

type hostedSheetsRemainingField struct {
	name, flag, kind string
	required         bool
}
type hostedSheetsRemainingRecord struct {
	command    string
	positional []string
	fields     []hostedSheetsRemainingField
}

// Deliberately limited variants: no files, stdin, generic requests, external
// data sources, destination spreadsheet or arbitrary field masks.
func hostedSheetsRemainingRecords() []hostedSheetsRemainingRecord {
	f := func(name, flag, kind string, required bool) hostedSheetsRemainingField {
		return hostedSheetsRemainingField{name, flag, kind, required}
	}
	return []hostedSheetsRemainingRecord{
		{"copy", []string{"title"}, nil},
		{"banding clear", nil, []hostedSheetsRemainingField{f("id", "id", "positive", true)}},
		{"banding set", []string{"range"}, []hostedSheetsRemainingField{f("row_properties_json", "row-properties-json", "banding", true)}},
		{"batch-update", nil, []hostedSheetsRemainingField{f("data_json", "data-json", "values", true)}},
		{"chart create", nil, []hostedSheetsRemainingField{f("spec_json", "spec-json", "chart", true), f("sheet", "sheet", "text", true)}},
		{"chart delete", []string{"chart_id"}, nil},
		{"chart update", []string{"chart_id"}, []hostedSheetsRemainingField{f("spec_json", "spec-json", "chart", true)}},
		{"conditional-format add", []string{"range"}, []hostedSheetsRemainingField{f("type", "type", "text", true), f("expr", "expr", "text", false), f("format_json", "format-json", "format", true), f("index", "index", "integer", false)}},
		{"conditional-format clear", nil, []hostedSheetsRemainingField{f("sheet", "sheet", "text", true), f("index", "index", "integer", true)}},
		{"copy-paste", []string{"source", "dest"}, []hostedSheetsRemainingField{f("type", "type", "paste", false), f("transpose", "transpose", "bool", false)}},
		{"delete-dimension", []string{"target"}, []hostedSheetsRemainingField{f("dimension", "dimension", "dimension", true), f("start", "start", "positive", false), f("end", "end", "positive", false)}},
		{"duplicate-tab", []string{"source_tab", "new_name"}, []hostedSheetsRemainingField{f("index", "index", "integer", false)}},
		{"filter set", []string{"range"}, nil},
		{"find-replace", []string{"find", "replace"}, []hostedSheetsRemainingField{f("sheet", "sheet", "text", true), f("match_case", "match-case", "bool", false), f("match_entire", "match-entire", "bool", false)}},
		{"format", []string{"range"}, []hostedSheetsRemainingField{f("format_json", "format-json", "format", true)}},
		{"insert", []string{"sheet", "dimension", "start"}, []hostedSheetsRemainingField{f("count", "count", "positive", false), f("after", "after", "bool", false)}},
		{"links set", []string{"cell", "url", "text"}, nil},
		{"raw", nil, []hostedSheetsRemainingField{f("sheet", "sheet", "text", false), f("include_grid_data", "include-grid-data", "bool", false)}},
		{"table create", []string{"range"}, []hostedSheetsRemainingField{f("name", "name", "text", true), f("columns_json", "columns-json", "columns", true)}},
		{"table append", []string{"table_id"}, []hostedSheetsRemainingField{f("values_json", "values-json", "rows", true)}},
		{"table clear", []string{"table_id"}, nil},
		{"table delete", []string{"table_id"}, nil},
		{"validation clear", []string{"range"}, []hostedSheetsRemainingField{f("filtered_rows_included", "filtered-rows-included", "bool", false)}},
		{"validation set", []string{"range"}, []hostedSheetsRemainingField{f("type", "type", "text", true), f("value", "value", "text", false), f("strict", "strict", "bool", false), f("filtered_rows_included", "filtered-rows-included", "bool", false)}},
	}
}

func hostedSheetsRemainingTools() []mcpToolSpec {
	records := hostedSheetsRemainingRecords()
	tools := make([]mcpToolSpec, 0, len(records))
	for _, record := range records {
		record := record
		options := []mcp.ToolOption{mcp.WithString("spreadsheet_id", mcp.Required())}
		for _, name := range record.positional {
			if name == "chart_id" || name == "start" {
				options = append(options, mcp.WithInteger(name, mcp.Required(), mcp.Min(0)))
			} else {
				options = append(options, mcp.WithString(name, mcp.Required()))
			}
		}
		for _, field := range record.fields {
			var opts []mcp.PropertyOption
			if field.required {
				opts = append(opts, mcp.Required())
			}
			switch field.kind {
			case "integer", "positive":
				options = append(options, mcp.WithInteger(field.name, opts...))
			case "bool":
				options = append(options, mcp.WithBoolean(field.name, opts...))
			default:
				options = append(options, mcp.WithString(field.name, opts...))
			}
		}
		risk := mcpRiskWrite
		if record.command == "raw" {
			risk = mcpRiskRead
		}
		tools = append(tools, mcpToolSpec{Name: "sheets_" + strings.NewReplacer(" ", "_", "-", "_").Replace(record.command), Service: "sheets", Risk: risk, Description: "Fixed native Sheets " + record.command + " on one selected spreadsheet. Writes require approval; deletion and replacement may discard data. Inline typed inputs only.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedSheetsRemainingArgs(record, req) }})
	}
	return tools
}

func hostedSheetsRemainingArgs(record hostedSheetsRemainingRecord, req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedSheetsLayoutSpreadsheetID(req)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{"spreadsheet_id": true}
	for _, p := range record.positional {
		allowed[p] = true
	}
	for _, f := range record.fields {
		allowed[f.name] = true
	}
	for key := range req.GetArguments() {
		if !allowed[key] {
			return nil, errHostedSheetsRemaining
		}
	}
	argv := append([]string{"sheets"}, strings.Fields(record.command)...)
	values := map[string]string{}
	for _, field := range record.fields {
		raw, ok := req.GetArguments()[field.name]
		if !ok {
			if field.required {
				return nil, errHostedSheetsRemaining
			}
			continue
		}
		value, e := hostedSheetsRemainingValue(raw, field.kind)
		if e != nil {
			return nil, e
		}
		values[field.name] = value
		argv = append(argv, "--"+field.flag+"="+value)
	}
	if record.command == "table append" {
		argv = append(argv, "--input=RAW")
	}
	if record.command == "table delete" {
		argv = append(argv, "--discard-data")
	}
	positional := []string{id}
	for _, name := range record.positional {
		kind := "text"
		if name == "chart_id" {
			kind = "integer"
		}
		if name == "start" {
			kind = "positive"
		}
		value, e := hostedSheetsRemainingValue(req.GetArguments()[name], kind)
		if e != nil {
			return nil, e
		}
		if name == "range" || name == "source" || name == "dest" || name == "cell" {
			if _, e = parseSheetRange(value, name); e != nil {
				return nil, e
			}
		}
		values[name] = value
		positional = append(positional, value)
	}
	// All formula-bearing native inputs are checked after JSON unescaping.
	for _, key := range []string{"replace", "expr", "value"} {
		if hostedSheetsRemainingUnsafeFormula(values[key]) {
			return nil, errHostedSheetsRemaining
		}
	}
	switch record.command {
	case "conditional-format add":
		if _, _, err = sheetsconditional.BuildCondition(values["type"], values["expr"]); err != nil {
			return nil, err
		}
	case "validation set":
		var v []string
		if s, ok := values["value"]; ok {
			v = []string{s}
		}
		if _, err = sheetsvalidation.BuildCondition(values["type"], v); err != nil {
			return nil, err
		}
	case "delete-dimension":
		start, _ := strconv.ParseInt(values["start"], 10, 64)
		end, _ := strconv.ParseInt(values["end"], 10, 64)
		if _, err = sheetsdimension.ParseDeleteSpec(values["target"], values["dimension"], start, end); err != nil {
			return nil, err
		}
	case "insert":
		if values["dimension"] != "rows" && values["dimension"] != "cols" {
			return nil, errHostedSheetsRemaining
		}
	case "links set":
		if _, err = resolveLinkCell(linkCellSpec{Cell: values["cell"], URL: values["url"], Text: values["text"]}, 0); err != nil {
			return nil, err
		}
	}
	return append(append(argv, "--"), positional...), nil
}

func hostedSheetsRemainingValue(raw any, kind string) (string, error) {
	if kind == "integer" || kind == "positive" {
		n, e := validateHostedTabIndexArg(raw)
		if e != nil || n < 0 || (kind == "positive" && n == 0) {
			return "", errHostedSheetsRemaining
		}
		return strconv.Itoa(n), nil
	}
	if kind == "bool" {
		v, ok := raw.(bool)
		if !ok {
			return "", errHostedSheetsRemaining
		}
		return strconv.FormatBool(v), nil
	}
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" || len(s) > 65536 || strings.ContainsAny(s, "\x00\r\n") {
		return "", errHostedSheetsRemaining
	}
	switch kind {
	case "paste":
		return normalizePasteType(s)
	case "dimension":
		if s != "ROWS" && s != "COLUMNS" {
			return "", errHostedSheetsRemaining
		}
	case "format":
		var v sheets.CellFormat
		if err := hostedSheetsRemainingJSON(s, &v); err != nil {
			return "", err
		}
		if err := sheetsformat.Decode([]byte(s), &v); err != nil {
			return "", err
		}
		if _, _, err := sheetsformat.InferMask([]byte(s)); err != nil {
			return "", err
		}
	case "chart":
		var v sheets.ChartSpec
		if err := hostedSheetsRemainingJSON(s, &v); err != nil {
			return "", err
		}
		if _, err := sheetschart.ParseSpec([]byte(s)); err != nil {
			return "", err
		}
		if v.DataSourceChartProperties != nil || hostedSheetsRemainingDataSource([]byte(s)) {
			return "", errHostedSheetsRemaining
		}
	case "banding":
		var v sheets.BandingProperties
		if err := hostedSheetsRemainingJSON(s, &v); err != nil {
			return "", err
		}
		if _, err := parseBandingProperties(s, nil, strings.NewReader("")); err != nil {
			return "", err
		}
	case "rows":
		if !strings.HasPrefix(strings.TrimSpace(s), "[") {
			return "", errHostedSheetsRemaining
		}
		rows, err := parseSheetsAppendValues(s, nil, strings.NewReader(""))
		if err != nil {
			return "", err
		}
		if err := hostedSheetsRemainingScalarRows(rows); err != nil {
			return "", err
		}
	case "columns":
		if !strings.HasPrefix(strings.TrimSpace(s), "[") {
			return "", errHostedSheetsRemaining
		}
		if hostedSheetsRemainingJSONFormula(s) {
			return "", errHostedSheetsRemaining
		}
		if _, err := parseSheetsTableColumnsJSON(s, strings.NewReader("")); err != nil {
			return "", err
		}
	case "values":
		if !strings.HasPrefix(strings.TrimSpace(s), "[") {
			return "", errHostedSheetsRemaining
		}
		var ranges []*sheets.ValueRange
		dec := json.NewDecoder(strings.NewReader(s))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ranges); err != nil {
			return "", err
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			return "", errHostedSheetsRemaining
		}
		parsed, err := parseSheetsBatchUpdateData(s, strings.NewReader(""))
		if err != nil {
			return "", err
		}
		for _, r := range parsed {
			if _, err := parseSheetRange(r.Range, "range"); err != nil {
				return "", err
			}
			if hostedSheetsRemainingJSONFormula(s) {
				return "", errHostedSheetsRemaining
			}
			for _, row := range r.Values {
				for _, cell := range row {
					switch cell.(type) {
					case nil, string, bool, float64:
					default:
						return "", errHostedSheetsRemaining
					}
				}
			}
		}
	}
	return s, nil
}

func hostedSheetsRemainingJSON(s string, target any) error {
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		return errHostedSheetsRemaining
	}
	dec := json.NewDecoder(bytes.NewBufferString(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errHostedSheetsRemaining
	}
	return nil
}

// Reject Connected Sheets references even when nested inside a typed chart.
func hostedSheetsRemainingDataSource(data []byte) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return true
	}
	var visit func(any) bool
	visit = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, w := range x {
				if strings.HasPrefix(strings.ToLower(k), "datasource") || visit(w) {
					return true
				}
			}
		case []any:
			for _, w := range x {
				if visit(w) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func hostedSheetsRemainingUnsafeFormula(text string) bool {
	trimmed := strings.TrimSpace(text)
	return (strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+")) && hostedSheetsRemainingCrossFileFormula.MatchString(trimmed)
}

func hostedSheetsRemainingJSONFormula(raw string) bool {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return true
	}
	var visit func(any) bool
	visit = func(v any) bool {
		switch x := v.(type) {
		case string:
			return hostedSheetsRemainingUnsafeFormula(x)
		case []any:
			for _, item := range x {
				if visit(item) {
					return true
				}
			}
		case map[string]any:
			for _, item := range x {
				if visit(item) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func hostedSheetsRemainingScalarRows(rows [][]interface{}) error {
	for _, row := range rows {
		for _, cell := range row {
			switch cell.(type) {
			case nil, string, bool, float64:
			default:
				return errHostedSheetsRemaining
			}
		}
	}
	return nil
}
