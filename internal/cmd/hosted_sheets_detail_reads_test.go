package cmd

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedSheetsDetailReadArguments(t *testing.T) {
	byName := map[string]mcpToolSpec{}
	for _, spec := range hostedSheetsDetailReadTools() {
		byName[spec.Name] = spec
	}
	if len(byName) != 3 {
		t.Fatalf("expected 3 hosted sheets detail read tools, got %d", len(byName))
	}
	for name, spec := range byName {
		if spec.Service != "sheets" {
			t.Errorf("%s: Service = %q, want %q", name, spec.Service, "sheets")
		}
		if spec.Risk != mcpRiskRead {
			t.Errorf("%s: Risk = %q, want %q", name, spec.Risk, mcpRiskRead)
		}
	}

	newReq := func(args map[string]any) mcp.CallToolRequest {
		var req mcp.CallToolRequest
		req.Params.Arguments = args
		return req
	}

	chart, ok := byName["sheets_chart_get"]
	if !ok {
		t.Fatal("missing tool sheets_chart_get")
	}
	named, ok := byName["sheets_named_ranges_get"]
	if !ok {
		t.Fatal("missing tool sheets_named_ranges_get")
	}
	table, ok := byName["sheets_table_get"]
	if !ok {
		t.Fatal("missing tool sheets_table_get")
	}

	chartCases := []struct {
		name     string
		chartID  any
		expected string
	}{
		{name: "int one", chartID: 1, expected: "1"},
		{name: "int64 max", chartID: int64(math.MaxInt32), expected: "2147483647"},
		{name: "integral float", chartID: float64(42), expected: "42"},
	}
	for _, tc := range chartCases {
		argv, err := chart.BuildArgs(newReq(map[string]any{"spreadsheet_id": "abc123", "chart_id": tc.chartID}))
		if err != nil {
			t.Fatalf("chart %s: unexpected error: %v", tc.name, err)
		}
		want := []string{"sheets", "chart", "get", "--", "abc123", tc.expected}
		if !slices.Equal(argv, want) {
			t.Errorf("chart %s: argv = %v, want %v", tc.name, argv, want)
		}
	}

	argv, err := named.BuildArgs(newReq(map[string]any{"spreadsheet_id": "abc123", "name_or_id": "Revenue_2026"}))
	if err != nil {
		t.Fatalf("named ranges: unexpected error: %v", err)
	}
	want := []string{"sheets", "named-ranges", "get", "--", "abc123", "Revenue_2026"}
	if !slices.Equal(argv, want) {
		t.Errorf("named ranges: argv = %v, want %v", argv, want)
	}

	argv, err = table.BuildArgs(newReq(map[string]any{"spreadsheet_id": "abc123", "table_id": "--account=else"}))
	if err != nil {
		t.Fatalf("table: unexpected error: %v", err)
	}
	want = []string{"sheets", "table", "get", "--", "abc123", "--account=else"}
	if !slices.Equal(argv, want) {
		t.Errorf("table: argv = %v, want %v", argv, want)
	}

	spreadsheetBad := []any{nil, 42, "", "https://example.com/sheets/d/abc", "/tmp/sheets/abc", " abc ", "\tabc"}
	for _, spec := range byName {
		for _, v := range spreadsheetBad {
			args := map[string]any{"spreadsheet_id": v}
			switch spec.Name {
			case "sheets_chart_get":
				args["chart_id"] = 1
			case "sheets_named_ranges_get":
				args["name_or_id"] = "Revenue_2026"
			case "sheets_table_get":
				args["table_id"] = "T1"
			}
			if _, err := spec.BuildArgs(newReq(args)); err == nil {
				t.Errorf("%s: spreadsheet_id %#v: expected error", spec.Name, v)
			}
		}
	}

	chartIDBad := []any{nil, "seven", 0, -1, 1.5, math.NaN(), math.Inf(1), int64(2147483648)}
	for _, v := range chartIDBad {
		if _, err := chart.BuildArgs(newReq(map[string]any{"spreadsheet_id": "abc123", "chart_id": v})); err == nil {
			t.Errorf("chart_id %#v: expected error", v)
		}
	}

	selectorBad := []any{nil, 42, "", " ", "\n", strings.Repeat("a", 513)}
	for _, v := range selectorBad {
		if _, err := named.BuildArgs(newReq(map[string]any{"spreadsheet_id": "abc123", "name_or_id": v})); err == nil {
			t.Errorf("name_or_id %#v: expected error", v)
		}
		if _, err := table.BuildArgs(newReq(map[string]any{"spreadsheet_id": "abc123", "table_id": v})); err == nil {
			t.Errorf("table_id %#v: expected error", v)
		}
	}
}
