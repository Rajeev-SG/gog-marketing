package cmd

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/docsedit"
)

func hostedDocsFormatLayoutTools() []mcpToolSpec {
	format := make([]mcp.ToolOption, 0, 30)
	format = append(format, []mcp.ToolOption{mcp.WithString("document_id", mcp.Required()), mcp.WithString("match"), mcp.WithBoolean("match_all"), mcp.WithBoolean("match_case"), mcp.WithString("tab"), mcp.WithString("segment")}...)
	for _, key := range []string{"font_family", "text_color", "bg_color", "alignment", "spacing_mode", "named_style", "bullet_preset", "link"} {
		format = append(format, mcp.WithString(key))
	}
	for _, key := range []string{"font_size", "line_spacing", "indent_start", "indent_first_line", "indent_end", "space_above", "space_below"} {
		format = append(format, mcp.WithNumber(key))
	}
	for _, key := range []string{"bold", "italic", "underline", "strikethrough", "keep_with_next", "keep_lines_together"} {
		format = append(format, mcp.WithBoolean(key))
	}
	format = append(format, mcp.WithInteger("heading_level", mcp.Min(1), mcp.Max(6)), mcp.WithString("bullet_style", mcp.Enum("unordered", "ordered", "none")), mcp.WithString("code_style", mcp.Enum("apply")))
	page := make([]mcp.ToolOption, 0, 10)
	page = append(page, []mcp.ToolOption{mcp.WithString("document_id", mcp.Required()), mcp.WithString("tab"), mcp.WithString("layout", mcp.Enum("pageless", "pages", "paged"))}...)
	for _, key := range []string{"page_size", "page_width", "page_height", "margin_left", "margin_right", "margin_top", "margin_bottom"} {
		page = append(page, mcp.WithString(key))
	}
	return []mcpToolSpec{
		{Name: "docs_format", Service: "docs", Risk: mcpRiskWrite, Description: "Format native document text/paragraphs, first/all literal matches or default whole body, optionally on a tab/header/footer/footnote. Boolean false clears bold/italic/underline/strike; paragraph pointer booleans and zero spacing/indents are preserved. Empty link clears it; links are set, not fetched. Native style/conflict validation retained. Exact document/tool write grants and one-use chat approval; no files/raw masks/batches or automatic retries.", Options: format, BuildArgs: hostedDocsFormatArgs},
		{Name: "docs_page_layout", Service: "docs", Risk: mcpRiskWrite, Description: "Set native paged/pageless layout, named/custom size or margins on an exact document/tab. Dimension strings accept pt/in/cm/mm including zero margins. Margin-only updates preserve existing mode; with no fields native default is pageless. Exact write-tool/document grants and one-use chat approval; no wildcard/raw requests or automatic retries.", Options: page, BuildArgs: hostedDocsPageLayoutArgs},
		{Name: "docs_section_columns", Service: "docs", Risk: mcpRiskWrite, Description: "Set one-to-three columns and none/between separator at a UTF-16 index, literal anchor or native end position in one document/tab. Native body placement, ambiguity, table exclusion and revision checks retained. Exact write-tool/document grants and one-use chat approval; no segments/files/persisted batch or automatic retries.", Options: []mcp.ToolOption{mcp.WithString("document_id", mcp.Required()), mcp.WithInteger("count", mcp.Required(), mcp.Min(1), mcp.Max(3)), mcp.WithString("separator", mcp.Enum("none", "between")), mcp.WithInteger("index", mcp.Min(1), mcp.Max(2147483647)), mcp.WithString("at"), mcp.WithInteger("occurrence", mcp.Min(1), mcp.Max(2147483647)), mcp.WithBoolean("match_case"), mcp.WithBoolean("at_end"), mcp.WithString("tab")}, BuildArgs: hostedDocsSectionColumnsArgs},
	}
}

func hostedDocsFormatNumber(raw any) (float64, error) {
	var value float64
	switch v := raw.(type) {
	case float64:
		value = v
	case float32:
		value = float64(v)
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, err
		}
		value = parsed
	default:
		return 0, usage("style dimensions must be numeric")
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, usage("style dimensions must be finite")
	}
	return value, nil
}

func appendHostedDocsFormatStrings(req mcp.CallToolRequest, args []string, cmd *DocsFormatCmd) ([]string, error) {
	for _, field := range []struct {
		key, flag string
		target    *string
	}{
		{"font_family", "font-family", &cmd.Format.FontFamily}, {"text_color", "text-color", &cmd.Format.TextColor}, {"bg_color", "bg-color", &cmd.Format.BgColor}, {"alignment", "alignment", &cmd.Format.Alignment}, {"spacing_mode", "spacing-mode", &cmd.Format.SpacingMode}, {"named_style", "named-style", &cmd.Format.NamedStyle}, {"bullet_preset", "bullet-preset", &cmd.Format.BulletPreset},
	} {
		if _, present := req.GetArguments()[field.key]; !present {
			continue
		}
		value, err := hostedDocsNamedEditString(req, field.key, 4096)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(value) == "" {
			return nil, usage("style strings cannot be blank")
		}
		*field.target = value
		args = append(args, "--"+field.flag+"="+value)
	}
	if _, present := req.GetArguments()["link"]; present {
		value, err := hostedDocsNamedEditString(req, "link", 8192)
		if err != nil {
			return nil, err
		}
		if value == "" {
			cmd.NoLink = true
			args = append(args, "--no-link")
		} else {
			cmd.Link = value
			args = append(args, "--link="+value)
		}
	}
	return args, nil
}

func appendHostedDocsFormatNumbers(req mcp.CallToolRequest, args []string, cmd *DocsFormatCmd) ([]string, error) {
	for _, field := range []struct {
		key, flag string
		scalar    *float64
		optional  **float64
	}{
		{"font_size", "font-size", &cmd.Format.FontSize, nil},
		{"line_spacing", "line-spacing", &cmd.Format.LineSpacing, nil},
		{"indent_start", "indent-start", nil, &cmd.Format.IndentStart},
		{"indent_first_line", "indent-first-line", nil, &cmd.Format.IndentFirstLine},
		{"indent_end", "indent-end", nil, &cmd.Format.IndentEnd},
		{"space_above", "space-above", nil, &cmd.Format.SpaceAbove},
		{"space_below", "space-below", nil, &cmd.Format.SpaceBelow},
	} {
		raw, present := req.GetArguments()[field.key]
		if !present {
			continue
		}
		value, err := hostedDocsFormatNumber(raw)
		if err != nil {
			return nil, err
		}
		if field.scalar != nil {
			if value <= 0 {
				return nil, usage("font size and line spacing must be positive")
			}
			*field.scalar = value
		} else {
			*field.optional = &value
		}
		args = append(args, "--"+field.flag+"="+strconv.FormatFloat(value, 'g', -1, 64))
	}
	return args, nil
}

func appendHostedDocsFormatBools(req mcp.CallToolRequest, args []string, cmd *DocsFormatCmd) ([]string, error) {
	for _, field := range []struct {
		key, positive, negative string
		on, off                 *bool
	}{
		{"bold", "bold", "no-bold", &cmd.Format.Bold, &cmd.Format.NoBold}, {"italic", "italic", "no-italic", &cmd.Format.Italic, &cmd.Format.NoItalic}, {"underline", "underline", "no-underline", &cmd.Format.Underline, &cmd.Format.NoUnderline}, {"strikethrough", "strikethrough", "no-strikethrough", &cmd.Format.Strikethrough, &cmd.Format.NoStrike},
	} {
		if _, present := req.GetArguments()[field.key]; !present {
			continue
		}
		value, err := hostedDocsNamedEditBool(req, field.key, false)
		if err != nil {
			return nil, err
		}
		flag := field.positive
		if value {
			*field.on = true
		} else {
			*field.off = true
			flag = field.negative
		}
		args = append(args, "--"+flag)
	}
	for _, field := range []struct {
		key, flag string
		target    **bool
	}{{"keep_with_next", "keep-with-next", &cmd.Format.KeepWithNext}, {"keep_lines_together", "keep-lines-together", &cmd.Format.KeepLinesTogether}} {
		if _, present := req.GetArguments()[field.key]; !present {
			continue
		}
		value, err := hostedDocsNamedEditBool(req, field.key, false)
		if err != nil {
			return nil, err
		}
		*field.target = &value
		args = append(args, "--"+field.flag+"="+strconv.FormatBool(value))
	}
	return args, nil
}

func hostedDocsFormatArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	cmd := DocsFormatCmd{DocID: id}
	args := []string{"docs", "format"}
	args, err = appendHostedDocsFormatStrings(req, args, &cmd)
	if err != nil {
		return nil, err
	}
	args, err = appendHostedDocsFormatNumbers(req, args, &cmd)
	if err != nil {
		return nil, err
	}
	args, err = appendHostedDocsFormatBools(req, args, &cmd)
	if err != nil {
		return nil, err
	}
	if _, present := req.GetArguments()["match"]; present {
		value, matchErr := hostedDocsNamedEditString(req, "match", 262144)
		if matchErr != nil {
			return nil, matchErr
		}
		if strings.TrimSpace(value) == "" {
			return nil, usage("explicit match cannot be blank")
		}
		cmd.Match = value
		args = append(args, "--match="+value)
	}
	cmd.MatchAll, err = hostedDocsNamedEditBool(req, "match_all", false)
	if err != nil {
		return nil, err
	}
	if cmd.MatchAll {
		if cmd.Match == "" {
			return nil, usage("match_all requires match")
		}
		args = append(args, "--match-all")
	}
	cmd.MatchCase, err = hostedDocsNamedEditBool(req, "match_case", false)
	if err != nil {
		return nil, err
	}
	if cmd.MatchCase {
		args = append(args, "--match-case")
	}
	heading, err := hostedDocsNamedEditIndex(req, "heading_level")
	if err != nil {
		return nil, err
	}
	if heading != nil {
		if *heading < 1 || *heading > 6 {
			return nil, usage("heading_level must be1..6")
		}
		value := int(*heading)
		cmd.Format.HeadingLevel = &value
		args = append(args, "--heading-level="+strconv.Itoa(value))
	}
	if _, present := req.GetArguments()["bullet_style"]; present {
		value, styleErr := hostedDocsNamedEditString(req, "bullet_style", 32)
		if styleErr != nil {
			return nil, styleErr
		}
		switch value {
		case "unordered":
			cmd.Format.Bullets = true
			args = append(args, "--bullets")
		case "ordered":
			cmd.Format.Ordered = true
			args = append(args, "--ordered")
		case "none":
			cmd.Format.NoBullets = true
			args = append(args, "--no-bullets")
		default:
			return nil, usage("invalid bullet_style")
		}
	}
	if _, present := req.GetArguments()["code_style"]; present {
		value, styleErr := hostedDocsNamedEditString(req, "code_style", 32)
		if styleErr != nil {
			return nil, styleErr
		}
		if value != "apply" {
			return nil, usage("code_style must be apply")
		}
		cmd.Format.Code = true
		args = append(args, "--code")
	}
	formatting := cmd.Format.withLinkFlags(cmd.Link, cmd.NoLink)
	if !formatting.any() {
		return nil, usage("no formatting fields supplied")
	}
	if _, validationErr := formatting.buildRequests(1, 2, ""); validationErr != nil {
		return nil, validationErr
	}
	args, err = hostedDocsNamedEditSelection(args, req, "tab", "segment")
	if err != nil {
		return nil, err
	}
	return append(args, "--", id), nil
}

func hostedDocsPageLayoutArgs(req mcp.CallToolRequest) ([]string, error) {
	cmd := DocsPageLayoutCmd{Layout: "pageless"}
	args := []string{"docs", "page-layout"}
	layoutPresent := false
	if raw, ok := req.GetArguments()["layout"]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, usage("layout must be a string")
		}
		layout := strings.TrimSpace(s)
		if layout == "" {
			return nil, usage("layout must not be blank")
		}
		cmd.Layout = layout
		layoutPresent = true
	}
	flags := []struct {
		key   string
		field *string
		name  string
	}{
		{"page_size", &cmd.LayoutFlags.PageSize, "page-size"},
		{"page_width", &cmd.LayoutFlags.PageWidth, "page-width"},
		{"page_height", &cmd.LayoutFlags.PageHeight, "page-height"},
		{"margin_left", &cmd.LayoutFlags.MarginLeft, "margin-left"},
		{"margin_right", &cmd.LayoutFlags.MarginRight, "margin-right"},
		{"margin_top", &cmd.LayoutFlags.MarginTop, "margin-top"},
		{"margin_bottom", &cmd.LayoutFlags.MarginBottom, "margin-bottom"},
	}
	for _, f := range flags {
		raw, ok := req.GetArguments()[f.key]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			return nil, usage(f.key + " must be a string")
		}
		v := strings.TrimSpace(s)
		if v == "" {
			return nil, usage(f.key + " must not be blank")
		}
		*f.field = v
	}
	mode := ""
	if layoutPresent || !cmd.LayoutFlags.any() {
		var err error
		mode, err = normalizePageLayout(cmd.Layout)
		if err != nil {
			return nil, err
		}
	}
	if _, err := buildUpdateDocumentStyleRequest(docsDocumentStyleOptions{Mode: mode, DocsLayoutFlags: cmd.LayoutFlags}); err != nil {
		return nil, err
	}
	if layoutPresent {
		args = append(args, "--layout="+cmd.Layout)
	}
	for _, f := range flags {
		if *f.field != "" {
			args = append(args, "--"+f.name+"="+*f.field)
		}
	}
	args, err := hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	args = append(args, "--", id)
	return args, nil
}

func hostedDocsSectionColumnsArgs(req mcp.CallToolRequest) ([]string, error) {
	count, err := hostedDocsNamedEditIndex(req, "count")
	if err != nil {
		return nil, err
	}
	if count == nil {
		return nil, usage("--count is required")
	}
	if *count < 1 || *count > 3 {
		return nil, usage("--count must be between 1 and 3")
	}
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	separator := "none"
	if _, present := req.GetArguments()["separator"]; present {
		separator, err = hostedDocsNamedEditString(req, "separator", 64)
		if err != nil {
			return nil, err
		}
		if _, err = normalizeDocsColumnSeparator(separator); err != nil {
			return nil, err
		}
	}
	index, err := hostedDocsNamedEditIndex(req, "index")
	if err != nil {
		return nil, err
	}
	atEnd, err := hostedDocsNamedEditBool(req, "at_end", false)
	if err != nil {
		return nil, err
	}
	anchor, err := hostedDocsTextAnchor(req)
	if err != nil {
		return nil, err
	}
	if _, planErr := docsedit.PlanEndInsertPlacement(docsedit.EndInsertPlacementOptions{Index: index, AtEnd: atEnd, Anchor: anchor}); planErr != nil {
		return nil, planErr
	}
	args := []string{
		"docs",
		"section-columns",
		"--count=" + strconv.FormatInt(*count, 10),
		"--separator=" + separator,
	}
	if index != nil {
		args = append(args, "--index="+strconv.FormatInt(*index, 10))
	}
	if atEnd {
		args = append(args, "--at-end")
	}
	args = appendDocsTextAnchorFlags(args, anchor)
	args, err = hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}
	args = append(args, "--", id)
	return args, nil
}
