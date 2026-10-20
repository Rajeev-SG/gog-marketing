package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

type hostedSlidesRemainingField struct {
	key, flag            string
	kind                 byte
	required, positional bool
	values               []string
}
type hostedSlidesRemainingRecord struct {
	command string
	fields  []hostedSlidesRemainingField
}

// Fixed flag descriptors mirror the native typed commands; local files,
// destination folders, arbitrary requests and thumbnail downloads are excluded.
func hostedSlidesRemainingRecords() []hostedSlidesRemainingRecord {
	return []hostedSlidesRemainingRecord{
		{"bullets", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"range", "range", 's', true, false, nil}, {"on", "on", 'b', false, false, nil}, {"off", "off", 'b', false, false, nil}, {"preset", "preset", 's', false, false, nil}}},
		{"copy", []hostedSlidesRemainingField{{"title", "title", 's', true, true, nil}}},
		{"element alt-text", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"title", "title", 's', false, false, nil}, {"description", "description", 's', false, false, nil}}},
		{"element create-line", []hostedSlidesRemainingField{{"slide_id", "slideId", 's', true, true, nil}, {"category", "category", 's', false, false, []string{"STRAIGHT", "BENT", "CURVED"}}, {"x", "x", 'n', false, false, nil}, {"y", "y", 'n', false, false, nil}, {"width", "width", 'n', false, false, nil}, {"height", "height", 'n', false, false, nil}, {"unit", "unit", 's', false, false, []string{"PT", "EMU"}}, {"object_id", "object-id", 's', false, false, nil}}},
		{"element create-shape", []hostedSlidesRemainingField{{"slide_id", "slideId", 's', true, true, nil}, {"type", "type", 's', false, false, nil}, {"x", "x", 'n', false, false, nil}, {"y", "y", 'n', false, false, nil}, {"width", "width", 'n', false, false, nil}, {"height", "height", 'n', false, false, nil}, {"unit", "unit", 's', false, false, []string{"PT", "EMU"}}, {"object_id", "object-id", 's', false, false, nil}}},
		{"element delete", []hostedSlidesRemainingField{{"object_ids", "objectId", 'a', true, true, nil}}},
		{"element group", []hostedSlidesRemainingField{{"object_ids", "objectId", 'a', true, true, nil}, {"group_id", "group-id", 's', false, false, nil}}},
		{"element style", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"kind", "kind", 's', false, false, []string{"shape", "line"}}, {"fill_color", "fill-color", 's', false, false, nil}, {"fill_transparent", "fill-transparent", 'b', false, false, nil}, {"outline_color", "outline-color", 's', false, false, nil}, {"outline_transparent", "outline-transparent", 'b', false, false, nil}, {"outline_weight", "outline-weight", 'n', false, false, nil}, {"outline_dash", "outline-dash", 's', false, false, []string{"SOLID", "DOT", "DASH", "DASH_DOT", "LONG_DASH", "LONG_DASH_DOT"}}}},
		{"element transform", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"scale_x", "scale-x", 'n', false, false, nil}, {"scale_y", "scale-y", 'n', false, false, nil}, {"shear_x", "shear-x", 'n', false, false, nil}, {"shear_y", "shear-y", 'n', false, false, nil}, {"translate_x", "translate-x", 'n', false, false, nil}, {"translate_y", "translate-y", 'n', false, false, nil}, {"rotate", "rotate", 'n', false, false, nil}, {"unit", "unit", 's', false, false, []string{"PT", "EMU"}}, {"apply_mode", "apply-mode", 's', false, false, []string{"RELATIVE", "ABSOLUTE"}}}},
		{"element ungroup", []hostedSlidesRemainingField{{"group_ids", "groupId", 'a', true, true, nil}}},
		{"element z-order", []hostedSlidesRemainingField{{"object_ids", "objectId", 'a', true, true, nil}, {"operation", "operation", 's', true, false, []string{"BRING_TO_FRONT", "BRING_FORWARD", "SEND_BACKWARD", "SEND_TO_BACK"}}}},
		{"insert-text", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"text", "text", 's', true, true, nil}, {"insertion_index", "insertion-index", 'i', false, false, nil}, {"replace", "replace", 'b', false, false, nil}, {"row", "row", 'i', false, false, nil}, {"col", "col", 'i', false, false, nil}}},
		{"link", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"range", "range", 's', true, false, nil}, {"url", "url", 's', false, false, nil}, {"clear", "clear", 'b', false, false, nil}}},
		{"paragraph-style", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"range", "range", 's', false, false, nil}, {"row", "row", 'i', false, false, nil}, {"col", "col", 'i', false, false, nil}, {"align", "align", 's', false, false, nil}, {"direction", "direction", 's', false, false, nil}, {"line_spacing", "line-spacing", 'n', false, false, nil}, {"space_above", "space-above", 'n', false, false, nil}, {"space_below", "space-below", 'n', false, false, nil}, {"indent_start", "indent-start", 'n', false, false, nil}, {"indent_end", "indent-end", 'n', false, false, nil}, {"indent_first_line", "indent-first-line", 'n', false, false, nil}}},
		{"raw", []hostedSlidesRemainingField{}},
		{"replace-slide", []hostedSlidesRemainingField{{"slide_id", "slideId", 's', true, true, nil}, {"url", "url", 's', true, false, nil}, {"notes", "notes", 's', false, false, nil}}},
		{"style-text", []hostedSlidesRemainingField{{"object_id", "objectId", 's', true, true, nil}, {"range", "range", 's', true, false, nil}, {"bold", "bold", 'b', false, false, nil}, {"no_bold", "no-bold", 'b', false, false, nil}, {"italic", "italic", 'b', false, false, nil}, {"no_italic", "no-italic", 'b', false, false, nil}, {"underline", "underline", 'b', false, false, nil}, {"no_underline", "no-underline", 'b', false, false, nil}, {"text_color", "text-color", 's', false, false, nil}, {"size", "size", 'n', false, false, nil}, {"font", "font", 's', false, false, nil}}},
		{"thumbnail", []hostedSlidesRemainingField{{"slide_id", "slideId", 's', true, true, nil}, {"size", "size", 's', false, false, nil}, {"format", "format", 's', false, false, nil}}},
	}
}

func hostedSlidesRemainingTools() []mcpToolSpec {
	records := hostedSlidesRemainingRecords()
	tools := make([]mcpToolSpec, 0, len(records))
	for _, record := range records {
		options := []mcp.ToolOption{mcp.WithString("presentation_id", mcp.Required())}
		for _, field := range record.fields {
			opts := []mcp.PropertyOption{}
			if field.required {
				opts = append(opts, mcp.Required())
			}
			if len(field.values) > 0 {
				opts = append(opts, mcp.Enum(field.values...))
			}
			switch field.kind {
			case 's':
				options = append(options, mcp.WithString(field.key, opts...))
			case 'b':
				options = append(options, mcp.WithBoolean(field.key, opts...))
			case 'n':
				options = append(options, mcp.WithNumber(field.key, opts...))
			case 'i':
				options = append(options, mcp.WithInteger(field.key, append(opts, mcp.Min(0), mcp.Max(2147483647))...))
			case 'a':
				options = append(options, mcp.WithArray(field.key, append(opts, mcp.WithStringItems())...))
			}
		}
		risk := mcpRiskWrite
		if record.command == "raw" || record.command == "thumbnail" {
			risk = mcpRiskRead
		}
		tools = append(tools, mcpToolSpec{Name: "slides_" + strings.NewReplacer(" ", "_", "-", "_").Replace(record.command), Service: "slides", Risk: risk, Description: "Native " + record.command + " on one explicitly granted presentation, with exact tool/resource authority. Native object/range/style/revision validation retained. Writes are single-attempt; element deletion and image replacement require destructive permission. Inline inputs only; no files/stdin, destination folder, arbitrary API body or thumbnail download. Public HTTPS image URLs are fetched by Google, not this runner. Not in chat starter allowlist; live verification pending.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedSlidesRemainingArgs(record, req) }})
	}
	return tools
}

func hostedSlidesRemainingArgs(record hostedSlidesRemainingRecord, req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedSlidesReadTool(req)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{"presentation_id": true}
	for _, field := range record.fields {
		allowed[field.key] = true
	}
	for key := range req.GetArguments() {
		if !allowed[key] {
			return nil, errHostedSlidesReadOption
		}
	}
	args := append([]string{"slides"}, strings.Fields(record.command)...)
	positional := []string{id}
	for _, field := range record.fields {
		raw, present := req.GetArguments()[field.key]
		if !present {
			if field.required {
				return nil, errHostedSlidesReadOption
			}
			continue
		}
		values := []string{}
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
					return nil, errHostedSlidesReadOption
				}
			}
			if (strings.HasSuffix(field.key, "_id") || field.key == "object_id") && v != "" && !hostedSlidesObjectIDPattern.MatchString(v) {
				return nil, errHostedSlidesBadObjectID
			}
			if record.command == "insert-text" && field.key == "text" && v == "-" {
				return nil, errHostedSlidesReadOption
			}
			values = append(values, v)
		case 'i':
			v, e := hostedDocsNamedEditIndex(req, field.key)
			if e != nil {
				return nil, e
			}
			values = append(values, strconv.FormatInt(*v, 10))
		case 'n':
			v, e := hostedDocsFormatNumber(raw)
			if e != nil {
				return nil, e
			}
			values = append(values, strconv.FormatFloat(v, 'f', -1, 64))
		case 'b':
			v, e := hostedDocsNamedEditBool(req, field.key, false)
			if e != nil {
				return nil, e
			}
			if !v {
				continue
			}
			args = append(args, "--"+field.flag)
			continue
		case 'a':
			items, ok := raw.([]any)
			if !ok || len(items) == 0 || len(items) > 100 {
				return nil, errHostedSlidesReadOption
			}
			for _, item := range items {
				v, valid := item.(string)
				if !valid || !hostedSlidesObjectIDPattern.MatchString(v) {
					return nil, errHostedSlidesBadObjectID
				}
				values = append(values, v)
			}
		}
		if field.positional {
			positional = append(positional, values...)
		} else {
			for _, value := range values {
				args = append(args, "--"+field.flag+"="+value)
			}
		}
	}
	return append(append(args, "--"), positional...), nil
}
