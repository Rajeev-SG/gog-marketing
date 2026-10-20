package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedDocsTableTools() []mcpToolSpec {
	specs := make([]mcpToolSpec, 0, 7)
	for _, command := range []string{"table-row insert", "table-row delete", "table-column insert", "table-column delete", "table-merge", "table-unmerge", "table-row pin-header"} {
		options := []mcp.ToolOption{mcp.WithString("document_id", mcp.Required()), mcp.WithString("table"), mcp.WithString("tab")}
		switch command {
		case "table-row insert", "table-column insert":
			options = append(options, mcp.WithString("at"))
		case "table-row delete":
			options = append(options, mcp.WithInteger("row", mcp.Required(), mcp.Min(1), mcp.Max(2147483647)))
		case "table-column delete":
			options = append(options, mcp.WithInteger("col", mcp.Required(), mcp.Min(1), mcp.Max(2147483647)))
		case "table-merge":
			options = append(options, mcp.WithString("range", mcp.Required()))
		case "table-unmerge":
			options = append(options, mcp.WithString("cell", mcp.Required()))
		case "table-row pin-header":
			options = append(options, mcp.WithInteger("rows", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)))
		}
		specs = append(specs, mcpToolSpec{Name: "docs_" + strings.NewReplacer("-", "_", " ", "_").Replace(command), Service: "docs", Risk: mcpRiskWrite, Description: "Native " + command + " within one explicitly granted document. Native table/merged-cell validation and revision checks retained. Exact write grant plus one-use chat approval; row/column deletion also requires destructive permission. No files, raw request bodies, persisted batches or automatic retries.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedDocsTableArgs(command, req) }})
	}
	return specs
}

func hostedDocsTableArgs(command string, req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	args := append([]string{"docs"}, strings.Fields(command)...)
	args, err = hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}
	if _, ok := req.GetArguments()["table"]; ok {
		table, e := hostedDocsNamedEditString(req, "table", 4096)
		if e != nil {
			return nil, e
		}
		if _, e = validateDocsTableMutationArgs(id, table); e != nil {
			return nil, e
		}
		args = append(args, "--table="+table)
	}
	switch command {
	case "table-row insert", "table-column insert":
		if _, ok := req.GetArguments()["at"]; ok {
			value, e := hostedDocsNamedEditString(req, "at", 32)
			if e != nil {
				return nil, e
			}
			if _, _, e = parseDocsTableInsertAt(value, "index"); e != nil {
				return nil, e
			}
			args = append(args, "--at="+value)
		}
	case "table-row delete", "table-column delete", "table-row pin-header":
		key := "row"
		if command == "table-column delete" {
			key = "col"
		}
		if command == "table-row pin-header" {
			key = "rows"
		}
		value, e := hostedDocsNamedEditIndex(req, key)
		if e != nil {
			return nil, e
		}
		if value == nil || (*value == 0 && key != "rows") {
			return nil, errHostedDocsNamedEditArgument
		}
		args = append(args, "--"+key+"="+strconv.FormatInt(*value, 10))
	case "table-merge":
		value, e := hostedDocsNamedEditString(req, "range", 128)
		if e != nil {
			return nil, e
		}
		if _, _, _, _, e = parseDocsTableRange(value); e != nil {
			return nil, e
		}
		args = append(args, "--range="+value)
	case "table-unmerge":
		value, e := hostedDocsNamedEditString(req, "cell", 64)
		if e != nil {
			return nil, e
		}
		if _, _, e = parseDocsTableCell(value, "cell"); e != nil {
			return nil, e
		}
		args = append(args, "--cell="+value)
	}
	return append(args, "--", id), nil
}
