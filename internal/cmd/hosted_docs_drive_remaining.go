package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// These descriptors only bridge fixed native flags, never caller-supplied argv,
// API bodies, file names, field masks or endpoint paths.
type hostedRemainingField struct {
	key      string
	kind     byte
	required bool
	values   []string
}
type hostedRemainingDefinition struct {
	name, service, command, idKey string
	write                         bool
	fields                        []hostedRemainingField
}

func hostedDocsDriveRemainingDefinitions() []hostedRemainingDefinition {
	text := func(key string, required bool) hostedRemainingField {
		return hostedRemainingField{key: key, kind: 's', required: required}
	}
	integer := func(key string, required bool) hostedRemainingField {
		return hostedRemainingField{key: key, kind: 'i', required: required}
	}
	boolean := func(key string) hostedRemainingField { return hostedRemainingField{key: key, kind: 'b'} }
	enum := func(key string, required bool, values ...string) hostedRemainingField {
		return hostedRemainingField{key: key, kind: 's', required: required, values: values}
	}
	cell := []hostedRemainingField{integer("table_index", false), integer("row", true), integer("col", true), text("tab", false)}
	cellStyle := append(append([]hostedRemainingField{}, cell...), integer("row_span", false), integer("col_span", false))
	for _, key := range []string{"background_color", "border_all", "border_top", "border_bottom", "border_left", "border_right", "padding_all", "padding_top", "padding_bottom", "padding_left", "padding_right", "content_align", "text_color"} {
		cellStyle = append(cellStyle, text(key, false))
	}
	cellStyle = append(cellStyle, boolean("bold"), boolean("italic"), boolean("underline"))
	return []hostedRemainingDefinition{
		{"docs_cell_style", "docs", "cell-style", "document_id", true, cellStyle},
		{"docs_cell_update", "docs", "cell-update", "document_id", true, append(append([]hostedRemainingField{}, cell...), text("content", true), boolean("append"))},
		{"docs_table_column_width", "docs", "table-column-width", "document_id", true, []hostedRemainingField{integer("table_index", false), integer("col", false), {key: "width", kind: 'n'}, boolean("evenly_distributed"), text("tab", false)}},
		{"docs_table_row_style", "docs", "table-row style", "document_id", true, []hostedRemainingField{text("table", false), integer("row", false), text("min_height", false), boolean("prevent_overflow"), text("tab", false)}},
		{"docs_update", "docs", "update", "document_id", true, []hostedRemainingField{text("text", true), integer("index", false), text("replace_range", false), text("at", false), integer("occurrence", false), boolean("match_case"), text("tab", false), text("segment", false)}},
		{"docs_copy", "docs", "copy", "document_id", true, []hostedRemainingField{text("title", true)}},
		{"docs_raw", "docs", "raw", "document_id", false, []hostedRemainingField{text("tab", false), boolean("all_tabs")}},
		{"drive_raw", "drive", "raw", "file_id", false, nil},
		{"drive_url", "drive", "url", "file_id", false, nil},
		{"drive_mkdir", "drive", "mkdir", "parent_id", true, []hostedRemainingField{text("name", true)}},
		{"drive_share", "drive", "share", "file_id", true, []hostedRemainingField{enum("to", true, "anyone", "user", "domain"), text("email", false), text("domain", false), enum("role", false, "reader", "writer", "commenter"), boolean("discoverable"), boolean("notify")}},
		{"drive_unshare", "drive", "unshare", "file_id", true, []hostedRemainingField{text("permission_id", true)}},
		{"drive_bulk_remove_public", "drive", "bulk remove-public", "file_id", true, nil},
		{"drive_bulk_update_role", "drive", "bulk update-role", "file_id", true, []hostedRemainingField{enum("from", true, "reader", "writer", "commenter"), enum("to", true, "reader", "writer", "commenter"), enum("type", false, "user", "group", "domain", "anyone"), text("target", false)}},
	}
}

func hostedDocsDriveRemainingTools() []mcpToolSpec {
	definitions := hostedDocsDriveRemainingDefinitions()
	specs := make([]mcpToolSpec, 0, len(definitions))
	for _, definition := range definitions {
		options := []mcp.ToolOption{mcp.WithString(definition.idKey, mcp.Required())}
		for _, field := range definition.fields {
			switch field.kind {
			case 's':
				opts := []mcp.PropertyOption{}
				if field.required {
					opts = append(opts, mcp.Required())
				}
				if len(field.values) > 0 {
					opts = append(opts, mcp.Enum(field.values...))
				}
				options = append(options, mcp.WithString(field.key, opts...))
			case 'i':
				opts := []mcp.PropertyOption{mcp.Min(0), mcp.Max(2147483647)}
				if field.required {
					opts = append(opts, mcp.Required())
				}
				options = append(options, mcp.WithInteger(field.key, opts...))
			case 'n':
				options = append(options, mcp.WithNumber(field.key, mcp.Min(5), mcp.Max(100000)))
			case 'b':
				options = append(options, mcp.WithBoolean(field.key))
			}
		}
		risk := mcpRiskRead
		if definition.write {
			risk = mcpRiskWrite
		}
		specs = append(specs, mcpToolSpec{Name: definition.name, Service: definition.service, Risk: risk, Description: "Fixed native " + definition.service + " " + definition.command + " on one selected resource. Exact tool/resource grants required; writes are single-attempt. Permission mutations require destructive authority. Inline bounded values only; no files/stdin, arbitrary API bodies, masks, destination folders or folder traversal. Bulk variants are forced to the selected file. Raw responses keep native redaction and hosted output limits; links are not followed. Native flag/placement/revision validators retained. Not in the chat starter allowlist.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedDocsDriveRemainingArgs(definition, req) }})
	}
	specs = append(specs, mcpToolSpec{Name: "docs_sed", Service: "docs", Risk: mcpRiskWrite, Description: "Apply bounded inline native sed expressions to one selected document, retaining native regex, formatting, structural and table semantics. Exact write/destructive tool and document grants required. No expression files/stdin, arbitrary API requests or automatic retries; Google performs any requested public image-URI fetch, not the runner. Partial failure is not retried. Not in chat starter allowlist.", Options: []mcp.ToolOption{mcp.WithString("document_id", mcp.Required()), mcp.WithArray("expressions", mcp.Required(), mcp.WithStringItems()), mcp.WithString("tab")}, BuildArgs: hostedDocsSedRemainingArgs})
	return specs
}

func hostedDocsDriveRemainingArgs(definition hostedRemainingDefinition, req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditString(req, definition.idKey, 256)
	if err != nil {
		return nil, err
	}
	if !hostedDocsResourceIDPattern.MatchString(id) {
		return nil, errHostedCommentsID
	}
	args := append([]string{definition.service}, strings.Fields(definition.command)...)
	positionals := []string{id}
	if definition.name == "drive_mkdir" {
		args = append(args, "--parent="+id)
		positionals = nil
	}
	if strings.HasPrefix(definition.name, "drive_bulk_") {
		args = append(args, "--file="+id)
		positionals = nil
	}
	if definition.name == "docs_cell_update" {
		args = append(args, "--format=plain")
	}
	for _, field := range definition.fields {
		_, present := req.GetArguments()[field.key]
		if !present {
			if field.required {
				return nil, errHostedDocsNamedEditArgument
			}
			continue
		}
		flag := "--" + strings.ReplaceAll(field.key, "_", "-")
		value := ""
		switch field.kind {
		case 's':
			v, e := hostedDocsNamedEditString(req, field.key, 262144)
			if e != nil {
				return nil, e
			}
			if len(field.values) > 0 {
				found := false
				for _, candidate := range field.values {
					found = found || v == candidate
				}
				if !found {
					return nil, errHostedDocsNamedEditArgument
				}
			}
			value = v
		case 'i':
			v, e := hostedDocsNamedEditIndex(req, field.key)
			if e != nil {
				return nil, e
			}
			value = strconv.FormatInt(*v, 10)
		case 'n':
			v, e := hostedDocsFormatNumber(req.GetArguments()[field.key])
			if e != nil || v < 5 || v > 100000 {
				return nil, errHostedDocsNamedEditArgument
			}
			value = strconv.FormatFloat(v, 'f', -1, 64)
		case 'b':
			v, e := hostedDocsNamedEditBool(req, field.key, false)
			if e != nil {
				return nil, e
			}
			if !v {
				if field.key == "prevent_overflow" {
					args = append(args, "--no-prevent-overflow")
				}
				continue
			}
			args = append(args, flag)
			continue
		}
		if field.key == "title" || field.key == "name" || field.key == "permission_id" {
			if strings.TrimSpace(value) == "" {
				return nil, errHostedDocsNamedEditArgument
			}
			if field.key == "permission_id" && !hostedDocsResourceIDPattern.MatchString(value) {
				return nil, errHostedCommentsID
			}
			positionals = append(positionals, value)
			continue
		}
		args = append(args, flag+"="+value)
	}
	return append(append(args, "--"), positionals...), nil
}

func hostedDocsSedRemainingArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	raw, ok := req.GetArguments()["expressions"].([]any)
	if !ok || len(raw) == 0 || len(raw) > 20 {
		return nil, errHostedDocsNamedEditArgument
	}
	args := []string{"docs", "sed"}
	for _, value := range raw {
		expression, valid := value.(string)
		if !valid || len(expression) == 0 || len(expression) > 65536 {
			return nil, errHostedDocsNamedEditArgument
		}
		if _, parseErr := parseFullExpr(expression); parseErr != nil {
			return nil, parseErr
		}
		args = append(args, "--expressions="+expression)
	}
	args, err = hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}
	return append(args, "--", id), nil
}
