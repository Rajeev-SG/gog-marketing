package cmd

import (
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/alecthomas/kong"
	"github.com/mark3labs/mcp-go/mcp"
)

// Hosted MCP tools wrapping the canonical `forms questions` add/move/delete
// commands. Form IDs are opaque Drive file IDs validated against
// hostedSlidesReadIDPattern and passed positionally after a literal `--`;
// optional flags are only emitted when provided so native defaults hold.

var (
	errFormsHostedArgs   = errors.New("invalid argument")
	errFormsHostedFormID = errors.New("invalid form_id: expected an opaque Google file ID")
)

func formsHostedAsString(key string, v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("%w: %s must be a string", errFormsHostedArgs, key)
}

func formsHostedAsInt(key string, v any) (int, error) {
	var n int64
	switch raw := v.(type) {
	case int:
		n = int64(raw)
	case int64:
		n = raw
	case float64:
		if raw != math.Trunc(raw) {
			return 0, fmt.Errorf("%w: %s must be a whole number", errFormsHostedArgs, key)
		}
		n = int64(raw)
	default:
		return 0, fmt.Errorf("%w: %s must be an integer", errFormsHostedArgs, key)
	}
	if n < -1<<31 || n > 1<<31-1 {
		return 0, fmt.Errorf("%w: %s out of range", errFormsHostedArgs, key)
	}
	return int(n), nil
}

func formsHostedAsStringList(key string, v any) ([]string, error) {
	switch list := v.(type) {
	case []string:
		return list, nil
	case []any:
		out := make([]string, 0, len(list))
		for i, item := range list {
			s, err := formsHostedAsString(fmt.Sprintf("%s[%d]", key, i), item)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %s must be an array of strings", errFormsHostedArgs, key)
	}
}

func formsHostedReqString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", fmt.Errorf("%w: missing %s", errFormsHostedArgs, key)
	}
	return formsHostedAsString(key, v)
}

func formsHostedReqInt(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, fmt.Errorf("%w: missing %s", errFormsHostedArgs, key)
	}
	return formsHostedAsInt(key, v)
}

func formsHostedFormID(args map[string]any) (string, error) {
	id, err := formsHostedReqString(args, "form_id")
	if err != nil {
		return "", err
	}
	if !hostedSlidesReadIDPattern.MatchString(id) {
		return "", errFormsHostedFormID
	}
	return id, nil
}

func hostedFormsAddQuestionArgs(req mcp.CallToolRequest) ([]string, error) {
	args := req.GetArguments()
	formID, err := formsHostedFormID(args)
	if err != nil {
		return nil, err
	}
	title, err := formsHostedReqString(args, "title")
	if err != nil {
		return nil, err
	}
	vals := make(map[string]any)
	var argErr error
	setErr := func(e error) {
		if argErr == nil {
			argErr = e
		}
	}
	for key, raw := range args {
		switch key {
		case "form_id", "title":
		case "type", "description", "scale_low_label", "scale_high_label":
			if s, e := formsHostedAsString(key, raw); e != nil {
				setErr(e)
			} else {
				vals[key] = s
			}
		case "required", "include_time", "include_year", "duration":
			if b, ok := raw.(bool); ok {
				vals[key] = b
			} else {
				setErr(fmt.Errorf("%w: %s must be a boolean", errFormsHostedArgs, key))
			}
		case "index", "points", "scale_low", "scale_high":
			if n, e := formsHostedAsInt(key, raw); e != nil {
				setErr(e)
			} else {
				vals[key] = n
			}
		case "options", "correct":
			if l, e := formsHostedAsStringList(key, raw); e != nil {
				setErr(e)
			} else {
				vals[key] = l
			}
		default:
			setErr(fmt.Errorf("%w: unknown argument %s", errFormsHostedArgs, key))
		}
	}
	input := formsAddQuestionInput{
		FormID:    formID,
		Title:     title,
		Type:      "text",
		Index:     -1,
		ScaleLow:  1,
		ScaleHigh: 5,
	}
	argv := []string{"forms", "questions", "add", "--title=" + title}
	setStr := func(key, flag string, dst *string) {
		if s, ok := vals[key].(string); ok {
			*dst = s
			argv = append(argv, flag+"="+s)
		}
	}
	setNum := func(key, flag string, dst *int) {
		if n, ok := vals[key].(int); ok {
			*dst = n
			argv = append(argv, flag+"="+strconv.Itoa(n))
		}
	}
	setBool := func(key, flag string, dst *bool) {
		if b, ok := vals[key].(bool); ok && b {
			*dst = true
			argv = append(argv, flag)
		}
	}
	setStr("type", "--type", &input.Type)
	setBool("required", "--required", &input.Required)
	if l, ok := vals["options"].([]string); ok {
		input.Options = l
		for _, s := range l {
			argv = append(argv, "--option="+s)
		}
	}
	setNum("index", "--index", &input.Index)
	if l, ok := vals["correct"].([]string); ok {
		input.Correct = l
		for _, s := range l {
			argv = append(argv, "--correct="+s)
		}
	}
	setNum("points", "--points", &input.Points)
	setStr("description", "--description", &input.Description)
	setNum("scale_low", "--scale-low", &input.ScaleLow)
	setNum("scale_high", "--scale-high", &input.ScaleHigh)
	setStr("scale_low_label", "--scale-low-label", &input.ScaleLowLabel)
	setStr("scale_high_label", "--scale-high-label", &input.ScaleHighLabel)
	setBool("include_time", "--include-time", &input.IncludeTime)
	setBool("include_year", "--include-year", &input.IncludeYear)
	setBool("duration", "--duration", &input.Duration)
	if _, err := newFormsAddQuestionPlan(input); err != nil {
		return nil, err
	}
	if argErr != nil {
		return nil, argErr
	}
	return append(argv, "--", formID), nil
}

func hostedFormsMoveQuestionArgs(req mcp.CallToolRequest) ([]string, error) {
	args := req.GetArguments()
	formID, err := formsHostedFormID(args)
	if err != nil {
		return nil, err
	}
	oldIndex, err := formsHostedReqInt(args, "old_index")
	if err != nil {
		return nil, err
	}
	newIndex, err := formsHostedReqInt(args, "new_index")
	if err != nil {
		return nil, err
	}
	if oldIndex < 0 || newIndex < 0 {
		return nil, fmt.Errorf("%w: old_index and new_index must be >= 0", errFormsHostedArgs)
	}
	return []string{"forms", "questions", "move", "--", formID, strconv.Itoa(oldIndex), strconv.Itoa(newIndex)}, nil
}

func hostedFormsDeleteQuestionArgs(req mcp.CallToolRequest) ([]string, error) {
	args := req.GetArguments()
	formID, err := formsHostedFormID(args)
	if err != nil {
		return nil, err
	}
	index, err := formsHostedReqInt(args, "index")
	if err != nil {
		return nil, err
	}
	if index < 0 {
		return nil, fmt.Errorf("%w: index must be >= 0", errFormsHostedArgs)
	}
	return []string{"forms", "questions", "delete", "--", formID, strconv.Itoa(index)}, nil
}

func hostedFormsQuestionTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name:        "forms_add_question",
			Service:     "forms",
			Risk:        mcpRiskWrite,
			Description: "Adds a question to an existing Google Form: type (text/paragraph/radio/checkbox/dropdown/scale/date/time, default text), choices, quiz grading, position, scale bounds/labels, and date/time options.",
			Options: []mcp.ToolOption{
				mcp.WithString("form_id", mcp.Required(), mcp.Description("Opaque Google Drive file ID of the form")),
				mcp.WithString("title", mcp.Required(), mcp.Description("Question title/text")),
				mcp.WithString("type", mcp.Description("Question type (default text)"), mcp.Enum("text", "paragraph", "radio", "checkbox", "dropdown", "scale", "date", "time")),
				mcp.WithBoolean("required", mcp.Description("Whether an answer is required")),
				mcp.WithArray("options", mcp.WithStringItems(), mcp.Description("Choice options in order (radio/checkbox/dropdown)")),
				mcp.WithInteger("index", mcp.Description("0-based position to insert at; omit to append"), mcp.Min(-1)),
				mcp.WithArray("correct", mcp.WithStringItems(), mcp.Description("Correct answer value(s) for quiz grading")),
				mcp.WithInteger("points", mcp.Description("Quiz points (must be positive when correct is set)"), mcp.Min(0)),
				mcp.WithString("description", mcp.Description("Question description/help text")),
				mcp.WithInteger("scale_low", mcp.Description("Scale minimum value: 0 or 1"), mcp.Min(0), mcp.Max(1)),
				mcp.WithInteger("scale_high", mcp.Description("Scale maximum value: 2 through 10"), mcp.Min(2), mcp.Max(10)),
				mcp.WithString("scale_low_label", mcp.Description("Label for the low end of the scale")),
				mcp.WithString("scale_high_label", mcp.Description("Label for the high end of the scale")),
				mcp.WithBoolean("include_time", mcp.Description("Include time picker (date type)")),
				mcp.WithBoolean("include_year", mcp.Description("Include year field (date type)")),
				mcp.WithBoolean("duration", mcp.Description("Ask for duration instead of time (time type)")),
			},
			BuildArgs: hostedFormsAddQuestionArgs,
		},
		{
			Name:        "forms_move_question",
			Service:     "forms",
			Risk:        mcpRiskWrite,
			Description: "Moves an existing question in a Google Form from old_index to new_index (both 0-based).",
			Options: []mcp.ToolOption{
				mcp.WithString("form_id", mcp.Required(), mcp.Description("Opaque Google Drive file ID of the form")),
				mcp.WithInteger("old_index", mcp.Required(), mcp.Description("Current 0-based question position"), mcp.Min(0)),
				mcp.WithInteger("new_index", mcp.Required(), mcp.Description("Target 0-based question position"), mcp.Min(0)),
			},
			BuildArgs: hostedFormsMoveQuestionArgs,
		},
		{
			Name:        "forms_delete_question",
			Service:     "forms",
			Risk:        mcpRiskWrite,
			Description: "Destructively deletes the question at index from an existing Google Form; requires write permission to the form, enforced by the runner approval gates.",
			Options: []mcp.ToolOption{
				mcp.WithString("form_id", mcp.Required(), mcp.Description("Opaque Google Drive file ID of the form")),
				mcp.WithInteger("index", mcp.Required(), mcp.Description("0-based question position to delete"), mcp.Min(0)),
			},
			BuildArgs: hostedFormsDeleteQuestionArgs,
		},
	}
}

// Hosted arrays already carry element boundaries; only this fresh parser's
// Forms add flags must bypass the ordinary CLI comma-slice grammar.
func configureHostedFormsQuestionSlices(parser *kong.Kong) error {
	node := parser.Model.Node
	for _, name := range []string{"forms", "questions", "add"} {
		var found *kong.Node
		for _, child := range node.Children {
			if child.Name == name {
				found = child
				break
			}
		}
		if found == nil {
			return ErrHostedToolExecution
		}
		node = found
	}
	count := 0
	for _, flag := range node.Flags {
		if flag.Name != "option" && flag.Name != "correct" {
			continue
		}
		tag := *flag.Tag
		tag.Sep = -1
		flag.Tag = &tag
		count++
	}
	if count != 2 {
		return ErrHostedToolExecution
	}
	return nil
}
