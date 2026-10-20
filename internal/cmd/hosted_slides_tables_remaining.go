package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedSlidesTableArgument = errors.New("hosted slides table: invalid argument")

func hostedSlidesTableRemainingTools() []mcpToolSpec {
	specs := make([]mcpToolSpec, 0, 11)
	for _, name := range []string{
		"slides_table_border_style",
		"slides_table_cell_style",
		"slides_table_column_delete",
		"slides_table_column_insert",
		"slides_table_column_size",
		"slides_table_create",
		"slides_table_merge",
		"slides_table_row_delete",
		"slides_table_row_insert",
		"slides_table_row_size",
		"slides_table_unmerge",
	} {
		options := []mcp.ToolOption{mcp.WithString("presentation_id", mcp.Required())}
		switch name {
		case "slides_table_create":
			options = append(options,
				mcp.WithString("slide_id", mcp.Required()),
				mcp.WithString("object_id"),
				mcp.WithInteger("rows", mcp.Required(), mcp.Min(1), mcp.Max(100)),
				mcp.WithInteger("cols", mcp.Required(), mcp.Min(1), mcp.Max(100)),
			)
		case "slides_table_border_style":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("row", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("col", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("row_span", mcp.Min(1), mcp.Max(2147483647), mcp.DefaultNumber(1)),
				mcp.WithInteger("col_span", mcp.Min(1), mcp.Max(2147483647), mcp.DefaultNumber(1)),
				mcp.WithString("position", mcp.Enum("ALL", "BOTTOM", "INNER", "INNER_HORIZONTAL", "INNER_VERTICAL", "LEFT", "OUTER", "RIGHT", "TOP"), mcp.DefaultString("ALL")),
				mcp.WithString("border_color"),
				mcp.WithBoolean("transparent"),
				mcp.WithNumber("weight"),
				mcp.WithString("dash", mcp.Enum("SOLID", "DOT", "DASH", "DASH_DOT", "LONG_DASH", "LONG_DASH_DOT")),
			)
		case "slides_table_cell_style":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("row", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("col", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithString("fill_color"),
				mcp.WithBoolean("fill_transparent"),
				mcp.WithString("content_align", mcp.Enum("TOP", "MIDDLE", "BOTTOM")),
				mcp.WithString("range"),
				mcp.WithBoolean("bold"), mcp.WithBoolean("no_bold"),
				mcp.WithBoolean("italic"), mcp.WithBoolean("no_italic"),
				mcp.WithBoolean("underline"), mcp.WithBoolean("no_underline"),
				mcp.WithString("text_color"),
				mcp.WithNumber("size"),
				mcp.WithString("font"),
			)
		case "slides_table_column_insert":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("col", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("count", mcp.Min(1), mcp.Max(20), mcp.DefaultNumber(1)),
				mcp.WithBoolean("right"),
			)
		case "slides_table_column_delete":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("col", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
			)
		case "slides_table_column_size":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("col", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithNumber("width", mcp.Required(), mcp.Min(32)),
			)
		case "slides_table_merge", "slides_table_unmerge":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("row", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("col", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("row_span", mcp.Min(1), mcp.Max(2147483647), mcp.DefaultNumber(1)),
				mcp.WithInteger("col_span", mcp.Min(1), mcp.Max(2147483647), mcp.DefaultNumber(1)),
			)
		case "slides_table_row_insert":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("row", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithInteger("count", mcp.Min(1), mcp.Max(20), mcp.DefaultNumber(1)),
				mcp.WithBoolean("below"),
			)
		case "slides_table_row_delete":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("row", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
			)
		case "slides_table_row_size":
			options = append(options,
				mcp.WithString("table_object_id", mcp.Required()),
				mcp.WithInteger("row", mcp.Required(), mcp.Min(0), mcp.Max(2147483647)),
				mcp.WithNumber("height", mcp.Required(), mcp.Min(0)),
			)
		}
		specs = append(specs, mcpToolSpec{
			Name: name, Service: "slides", Risk: mcpRiskWrite,
			Description: "Update one explicitly granted presentation via native " + strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(name, "slides_table_"), "table_"), "_", " ") + ". Native revision, table and range checks remain enforced; exact write-tool/file authority is required. No generic requests, files or automatic retries.",
			Options:     options,
			BuildArgs:   func(req mcp.CallToolRequest) ([]string, error) { return hostedSlidesTableRemainingArgs(name, req) },
		})
	}
	return specs
}

func hostedSlidesTableRemainingArgs(name string, req mcp.CallToolRequest) ([]string, error) {
	presentationID, err := hostedSlidesReadTool(req)
	if err != nil {
		return nil, err
	}
	switch name {
	case "slides_table_create":
		return hostedSlidesTableCreateArgs(req, presentationID)
	case "slides_table_border_style":
		return hostedSlidesTableBorderStyleArgs(req)
	case "slides_table_cell_style":
		return hostedSlidesTableCellStyleArgs(req)
	case "slides_table_column_insert":
		return hostedSlidesTableColumnInsertArgs(req)
	case "slides_table_column_size":
		return hostedSlidesTableColumnSizeArgs(req)
	case "slides_table_row_insert":
		return hostedSlidesTableRowInsertArgs(req)
	case "slides_table_row_size":
		return hostedSlidesTableRowSizeArgs(req)
	case "slides_table_merge":
		return hostedSlidesTableMergeArgs(req)
	case "slides_table_unmerge":
		return hostedSlidesTableUnmergeArgs(req)
	default:
		return hostedSlidesTableDeleteArgs(name, req, presentationID)
	}
}

func hostedSlidesTableCreateArgs(req mcp.CallToolRequest, presentationID string) ([]string, error) {
	argv := []string{"slides", "table"}
	argv = append(argv, "create")
	slideID, err := hostedSlidesTableString(req, "slide_id", 256)
	if err != nil {
		return nil, err
	}
	if _, present := req.GetArguments()["object_id"]; present {
		objectID, argumentErr := hostedSlidesTableString(req, "object_id", 256)
		if argumentErr != nil {
			return nil, argumentErr
		}
		argv = append(argv, "--object-id="+objectID)
	}
	rows, err := hostedSlidesTableInt(req, "rows", 1, 100, true)
	if err != nil {
		return nil, err
	}
	cols, err := hostedSlidesTableInt(req, "cols", 1, 100, true)
	if err != nil {
		return nil, err
	}
	argv = append(argv, "--rows="+strconv.FormatInt(rows, 10), "--cols="+strconv.FormatInt(cols, 10), "--", presentationID, slideID)
	return argv, nil
}

func hostedSlidesTableBorderStyleArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table"}
	argv = append(argv, "border", "style")
	targetArgs, selectedID, tableObjectID, err := hostedSlidesTableTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	argv = targetArgs
	rowSpan, err := hostedSlidesTableInt(req, "row_span", 1, 2147483647, false)
	if err != nil {
		return nil, err
	}
	colSpan, err := hostedSlidesTableInt(req, "col_span", 1, 2147483647, false)
	if err != nil {
		return nil, err
	}
	position := "ALL"
	if _, provided := req.GetArguments()["position"]; provided {
		var positionErr error
		position, positionErr = hostedSlidesTableString(req, "position", 32)
		if positionErr != nil {
			return nil, positionErr
		}
	}
	argv = append(argv,
		"--row-span="+strconv.FormatInt(rowSpan, 10),
		"--col-span="+strconv.FormatInt(colSpan, 10),
		"--position="+position,
	)
	borderColor, present, err := hostedSlidesTableOptionalString(req, "border_color", 16)
	if err != nil {
		return nil, err
	}
	transparent, presentBool, err := hostedSlidesTableOptionalBool(req, "transparent")
	if err != nil {
		return nil, err
	}
	if present && transparent {
		return nil, errHostedSlidesTableArgument
	}
	weight, weightPresent, err := hostedSlidesTableOptionalNumber(req, "weight")
	if err != nil {
		return nil, err
	}
	dash, dashPresent, err := hostedSlidesTableOptionalString(req, "dash", 32)
	if err != nil {
		return nil, err
	}
	if !present && !transparent && !weightPresent && !dashPresent {
		return nil, errHostedSlidesTableArgument
	}
	if present {
		argv = append(argv, "--border-color="+borderColor)
	}
	if presentBool && transparent {
		argv = append(argv, "--transparent")
	}
	if weightPresent {
		argv = append(argv, "--weight="+hostedSlidesTableFloat(weight))
	}
	if dashPresent {
		argv = append(argv, "--dash="+dash)
	}
	argv = append(argv, "--", selectedID, tableObjectID)
	return argv, nil
}

func hostedSlidesTableCellStyleArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table"}
	argv = append(argv, "cell", "style")
	targetArgs, selectedID, tableObjectID, err := hostedSlidesTableTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	argv = targetArgs
	_, fillColorPresent, err := hostedSlidesTableOptionalString(req, "fill_color", 16)
	if err != nil {
		return nil, err
	}
	_, fillTransparentPresent, err := hostedSlidesTableOptionalBool(req, "fill_transparent")
	if err != nil {
		return nil, err
	}
	if fillColorPresent && fillTransparentPresent {
		return nil, errHostedSlidesTableArgument
	}
	styleProvided := fillColorPresent || fillTransparentPresent
	for _, key := range []string{"fill_color", "text_color", "font", "range"} {
		if _, exists := req.GetArguments()[key]; exists {
			styleProvided = true
			value, argumentErr := hostedSlidesTableString(req, key, 256)
			if argumentErr != nil {
				return nil, argumentErr
			}
			argv = append(argv, "--"+strings.ReplaceAll(key, "_", "-")+"="+value)
		}
	}
	if _, exists := req.GetArguments()["fill_transparent"]; exists {
		styleProvided = true
		value, argumentErr := hostedSlidesTableBool(req, "fill_transparent")
		if argumentErr != nil {
			return nil, argumentErr
		}
		if value {
			argv = append(argv, "--fill-transparent")
		}
	}
	if _, exists := req.GetArguments()["content_align"]; exists {
		styleProvided = true
		value, argumentErr := hostedSlidesTableString(req, "content_align", 32)
		if argumentErr != nil {
			return nil, argumentErr
		}
		argv = append(argv, "--content-align="+value)
	}
	if _, exists := req.GetArguments()["size"]; exists {
		styleProvided = true
		value, argumentErr := hostedSlidesTableNumber(req, "size", 0, 1000, true)
		if argumentErr != nil {
			return nil, argumentErr
		}
		argv = append(argv, "--size="+hostedSlidesTableFloat(value))
	}
	for _, key := range []string{"bold", "no_bold", "italic", "no_italic", "underline", "no_underline"} {
		if _, exists := req.GetArguments()[key]; exists {
			styleProvided = true
			value, argumentErr := hostedSlidesTableBool(req, key)
			if argumentErr != nil {
				return nil, argumentErr
			}
			if value {
				argv = append(argv, "--"+strings.ReplaceAll(key, "_", "-"))
			}
		}
	}
	if !styleProvided {
		return nil, errHostedSlidesTableArgument
	}
	argv = append(argv, "--", selectedID, tableObjectID)
	return argv, nil
}

func hostedSlidesTableColumnInsertArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table"}
	argv = append(argv, "column", "insert")
	targetArgs, selectedID, tableObjectID, err := hostedSlidesTableColumnTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	argv = targetArgs
	count, err := hostedSlidesTableInt(req, "count", 1, 20, false)
	if err != nil {
		return nil, err
	}
	if _, exists := req.GetArguments()["right"]; exists {
		right, argumentErr := hostedSlidesTableBool(req, "right")
		if argumentErr != nil {
			return nil, argumentErr
		}
		if right {
			argv = append(argv, "--right")
		}
	}
	argv = append(argv, "--count="+strconv.FormatInt(count, 10), "--", selectedID, tableObjectID)
	return argv, nil
}

func hostedSlidesTableColumnSizeArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table"}
	argv = append(argv, "column", "size")
	targetArgs, selectedID, tableObjectID, err := hostedSlidesTableColumnTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	argv = targetArgs
	width, err := hostedSlidesTableNumber(req, "width", 32, 10000, true)
	if err != nil {
		return nil, err
	}
	argv = append(argv, "--width="+hostedSlidesTableFloat(width), "--", selectedID, tableObjectID)
	return argv, nil
}

func hostedSlidesTableRowInsertArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table", "row", "insert"}
	targetArgs, selectedID, tableObjectID, err := hostedSlidesTableRowTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	argv = targetArgs
	count, err := hostedSlidesTableInt(req, "count", 1, 20, false)
	if err != nil {
		return nil, err
	}
	if _, exists := req.GetArguments()["below"]; exists {
		below, argumentErr := hostedSlidesTableBool(req, "below")
		if argumentErr != nil {
			return nil, argumentErr
		}
		if below {
			argv = append(argv, "--below")
		}
	}
	argv = append(argv, "--count="+strconv.FormatInt(count, 10), "--", selectedID, tableObjectID)
	return argv, nil
}

func hostedSlidesTableRowSizeArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table"}
	argv = append(argv, "row", "size")
	targetArgs, selectedID, tableObjectID, err := hostedSlidesTableRowTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	argv = targetArgs
	height, err := hostedSlidesTableNumber(req, "height", 0, 10000, true)
	if err != nil {
		return nil, err
	}
	argv = append(argv, "--height="+hostedSlidesTableFloat(height), "--", selectedID, tableObjectID)
	return argv, nil
}

func hostedSlidesTableMergeArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table", "merge"}
	rowSpan, err := hostedSlidesTableInt(req, "row_span", 1, 2147483647, false)
	if err != nil {
		return nil, err
	}
	colSpan, err := hostedSlidesTableInt(req, "col_span", 1, 2147483647, false)
	if err != nil {
		return nil, err
	}
	if rowSpan == 1 && colSpan == 1 {
		return nil, errHostedSlidesTableArgument
	}
	argv, err = hostedSlidesTableRangeArgs(req, argv)
	if err != nil {
		return nil, err
	}
	return argv, nil
}

func hostedSlidesTableUnmergeArgs(req mcp.CallToolRequest) ([]string, error) {
	argv := []string{"slides", "table", "unmerge"}
	argv, err := hostedSlidesTableRangeArgs(req, argv)
	if err != nil {
		return nil, err
	}
	return argv, nil
}

func hostedSlidesTableDeleteArgs(name string, req mcp.CallToolRequest, presentationID string) ([]string, error) {
	argv := []string{"slides", "table"}
	tableObjectID, err := hostedSlidesTableString(req, "table_object_id", 256)
	if err != nil {
		return nil, err
	}
	if name == "slides_table_column_delete" {
		col, argumentErr := hostedSlidesTableInt(req, "col", 0, 2147483647, true)
		if argumentErr != nil {
			return nil, argumentErr
		}
		argv = append(argv, "column", "delete", "--col="+strconv.FormatInt(col, 10), "--", presentationID, tableObjectID)
	} else {
		row, argumentErr := hostedSlidesTableInt(req, "row", 0, 2147483647, true)
		if argumentErr != nil {
			return nil, argumentErr
		}
		argv = append(argv, "row", "delete", "--row="+strconv.FormatInt(row, 10), "--", presentationID, tableObjectID)
	}
	return argv, nil
}

func hostedSlidesTableTargetArgs(req mcp.CallToolRequest, argv []string) ([]string, string, string, error) {
	presentationID, err := hostedSlidesReadTool(req)
	if err != nil {
		return nil, "", "", err
	}
	tableObjectID, err := hostedSlidesTableString(req, "table_object_id", 256)
	if err != nil {
		return nil, "", "", err
	}
	row, err := hostedSlidesTableInt(req, "row", 0, 2147483647, true)
	if err != nil {
		return nil, "", "", err
	}
	col, err := hostedSlidesTableInt(req, "col", 0, 2147483647, true)
	if err != nil {
		return nil, "", "", err
	}
	return append(argv, "--row="+strconv.FormatInt(row, 10), "--col="+strconv.FormatInt(col, 10)), presentationID, tableObjectID, nil
}

func hostedSlidesTableRowTargetArgs(req mcp.CallToolRequest, argv []string) ([]string, string, string, error) {
	presentationID, err := hostedSlidesReadTool(req)
	if err != nil {
		return nil, "", "", err
	}
	tableObjectID, err := hostedSlidesTableString(req, "table_object_id", 256)
	if err != nil {
		return nil, "", "", err
	}
	row, err := hostedSlidesTableInt(req, "row", 0, 2147483647, true)
	if err != nil {
		return nil, "", "", err
	}
	return append(argv, "--row="+strconv.FormatInt(row, 10)), presentationID, tableObjectID, nil
}

func hostedSlidesTableColumnTargetArgs(req mcp.CallToolRequest, argv []string) ([]string, string, string, error) {
	presentationID, err := hostedSlidesReadTool(req)
	if err != nil {
		return nil, "", "", err
	}
	tableObjectID, err := hostedSlidesTableString(req, "table_object_id", 256)
	if err != nil {
		return nil, "", "", err
	}
	col, err := hostedSlidesTableInt(req, "col", 0, 2147483647, true)
	if err != nil {
		return nil, "", "", err
	}
	return append(argv, "--col="+strconv.FormatInt(col, 10)), presentationID, tableObjectID, nil
}

func hostedSlidesTableRangeArgs(req mcp.CallToolRequest, argv []string) ([]string, error) {
	argv, presentationID, tableObjectID, err := hostedSlidesTableTargetArgs(req, argv)
	if err != nil {
		return nil, err
	}
	rowSpan, err := hostedSlidesTableInt(req, "row_span", 1, 2147483647, false)
	if err != nil {
		return nil, err
	}
	colSpan, err := hostedSlidesTableInt(req, "col_span", 1, 2147483647, false)
	if err != nil {
		return nil, err
	}
	return append(argv, "--row-span="+strconv.FormatInt(rowSpan, 10), "--col-span="+strconv.FormatInt(colSpan, 10), "--", presentationID, tableObjectID), nil
}

func hostedSlidesTableString(req mcp.CallToolRequest, key string, maximum int) (string, error) {
	value, exists := req.GetArguments()[key]
	if !exists || value == nil {
		return "", fmt.Errorf("%w: missing %s", errHostedSlidesTableArgument, key)
	}
	text, ok := value.(string)
	if !ok || len(text) > maximum {
		return "", fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	if key == "slide_id" || key == "object_id" || key == "table_object_id" {
		if !hostedSlidesObjectIDPattern.MatchString(text) {
			return "", fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
		}
	}
	return text, nil
}

func hostedSlidesTableOptionalString(req mcp.CallToolRequest, key string, maximum int) (string, bool, error) {
	value, exists := req.GetArguments()[key]
	if !exists || value == nil {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok || len(text) > maximum {
		return "", false, fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	return text, true, nil
}

func hostedSlidesTableInt(req mcp.CallToolRequest, key string, minimum, maximum int64, required bool) (int64, error) {
	raw, exists := req.GetArguments()[key]
	if !exists || raw == nil {
		if required {
			return 0, fmt.Errorf("%w: missing %s", errHostedSlidesTableArgument, key)
		}
		return minimum, nil
	}
	var value int64
	switch n := raw.(type) {
	case int:
		value = int64(n)
	case int64:
		value = n
	case float64:
		if n != float64(int64(n)) {
			return 0, fmt.Errorf("%w: fractional %s", errHostedSlidesTableArgument, key)
		}
		value = int64(n)
	default:
		return 0, fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: out-of-range %s", errHostedSlidesTableArgument, key)
	}
	return value, nil
}

func hostedSlidesTableNumber(req mcp.CallToolRequest, key string, minimum, maximum float64, required bool) (float64, error) {
	raw, exists := req.GetArguments()[key]
	if !exists || raw == nil {
		if required {
			return 0, fmt.Errorf("%w: missing %s", errHostedSlidesTableArgument, key)
		}
		return 0, nil
	}
	var value float64
	switch n := raw.(type) {
	case int:
		value = float64(n)
	case int64:
		value = float64(n)
	case float64:
		value = n
	default:
		return 0, fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: out-of-range %s", errHostedSlidesTableArgument, key)
	}
	return value, nil
}

func hostedSlidesTableOptionalNumber(req mcp.CallToolRequest, key string) (float64, bool, error) {
	raw, exists := req.GetArguments()[key]
	if !exists || raw == nil {
		return 0, false, nil
	}
	var value float64
	switch n := raw.(type) {
	case int:
		value = float64(n)
	case int64:
		value = float64(n)
	case float64:
		value = n
	default:
		return 0, false, fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	if value <= 0 {
		return 0, false, fmt.Errorf("%w: non-positive %s", errHostedSlidesTableArgument, key)
	}
	return value, true, nil
}

func hostedSlidesTableBool(req mcp.CallToolRequest, key string) (bool, error) {
	raw, exists := req.GetArguments()[key]
	if !exists || raw == nil {
		return false, fmt.Errorf("%w: missing %s", errHostedSlidesTableArgument, key)
	}
	value, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	return value, nil
}

func hostedSlidesTableOptionalBool(req mcp.CallToolRequest, key string) (bool, bool, error) {
	raw, exists := req.GetArguments()[key]
	if !exists || raw == nil {
		return false, false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, false, fmt.Errorf("%w: invalid %s", errHostedSlidesTableArgument, key)
	}
	return value, true, nil
}

func hostedSlidesTableFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
