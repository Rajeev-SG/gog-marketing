package cmd

import (
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// Sentinel errors for hosted Slides write-tool argument validation.
var (
	errHostedSlidesArgType      = errors.New("hosted slides: argument has the wrong type")
	errHostedSlidesNilArg       = errors.New("hosted slides: argument must not be null")
	errHostedSlidesBadPID       = errors.New("hosted slides: invalid presentation_id")
	errHostedSlidesBadObjectID  = errors.New("hosted slides: invalid object id")
	errHostedSlidesEmptyFind    = errors.New("hosted slides: find must not be empty")
	errHostedSlidesModeConflict = errors.New("hosted slides: exactly one of all, page_ids, object_id is required")
)

// hostedSlidesEditTools returns the Slides write tool specs surfaced through
// the hosted MCP bridge. Every spec invokes an existing typed gog CLI
// write; no new capability is introduced here.
func hostedSlidesEditTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name:        "slides_skip_slide",
			Service:     "slides",
			Risk:        mcpRiskWrite,
			Description: "Write to Google Slides (write/slides tool grant): skip (hide) a slide via gog slides skip-slide.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Presentation ID.")),
				mcp.WithString("slide_id", mcp.Required(), mcp.Description("Slide (page) ID to skip.")),
			},
			BuildArgs: hostedSlidesSlideToggleArgs("skip-slide"),
		},
		{
			Name:        "slides_unskip_slide",
			Service:     "slides",
			Risk:        mcpRiskWrite,
			Description: "Write to Google Slides (write/slides tool grant): unskip (show) a previously skipped slide via gog slides unskip-slide.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Presentation ID.")),
				mcp.WithString("slide_id", mcp.Required(), mcp.Description("Slide (page) ID to unskip.")),
			},
			BuildArgs: hostedSlidesSlideToggleArgs("unskip-slide"),
		},
		{
			Name:    "slides_replace_text",
			Service: "slides",
			Risk:    mcpRiskWrite,
			Description: "Write to Google Slides (write/slides tool grant): replace text via gog slides replace-text. " +
				"Exactly one mode is required: all slides (--all), specific page IDs (--page), or a single object (--object). " +
				"An empty replacement deletes matching text and additionally requires the destructive permission; the runner enforces this.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Presentation ID.")),
				mcp.WithString("find", mcp.Required(), mcp.Description("Text to find. Must be non-empty; spaces are valid.")),
				mcp.WithString("replacement", mcp.Required(), mcp.Description("Replacement text. May be empty to delete matching text (requires destructive permission).")),
				mcp.WithBoolean("match_case", mcp.Description("Match the find text case-sensitively.")),
				mcp.WithBoolean("all", mcp.Description("Replace in every slide. Mutually exclusive with page_ids and object_id.")),
				mcp.WithArray("page_ids", mcp.WithStringItems(), mcp.Description("Page IDs to restrict the replacement to. Mutually exclusive with all and object_id.")),
				mcp.WithString("object_id", mcp.Description("Single object ID to restrict the replacement to. Mutually exclusive with all and page_ids.")),
			},
			BuildArgs: hostedSlidesReplaceTextArgs,
		},
	}
}

// hostedSlidesSlideToggleArgs builds the argv for slides skip-slide and
// unskip-slide: slides <verb> -- <presentation_id> <slide_id>.
func hostedSlidesSlideToggleArgs(verb string) func(mcp.CallToolRequest) ([]string, error) {
	return func(req mcp.CallToolRequest) ([]string, error) {
		presentationID, err := hostedSlidesReadTool(req, "presentation_id")
		if err != nil {
			return nil, err
		}

		slideID, err := requireMCPString(req, "slide_id")
		if err != nil {
			return nil, err
		}

		if slideID != req.GetString("slide_id", "") || !hostedSlidesObjectIDPattern.MatchString(slideID) {
			return nil, errHostedSlidesBadObjectID
		}
		if !hostedSlidesReadIDPattern.MatchString(presentationID) {
			return nil, fmt.Errorf("%w: %q", errHostedSlidesBadPID, presentationID)
		}

		return []string{"slides", verb, "--", presentationID, slideID}, nil
	}
}

// hostedSlidesReplaceTextArgs builds the argv for slides replace-text:
// slides replace-text [--match-case] [--all | --page=ID... | --object=ID] --
// <presentation_id> <find> <replacement>.
func hostedSlidesReplaceTextArgs(req mcp.CallToolRequest) ([]string, error) {
	presentationID, err := hostedSlidesReadTool(req, "presentation_id")
	if err != nil {
		return nil, err
	}

	if !hostedSlidesReadIDPattern.MatchString(presentationID) {
		return nil, fmt.Errorf("%w: %q", errHostedSlidesBadPID, presentationID)
	}

	find, err := req.RequireString("find")
	if err != nil {
		return nil, err
	}

	if len(find) == 0 {
		return nil, errHostedSlidesEmptyFind
	}

	replacement, err := req.RequireString("replacement")
	if err != nil {
		return nil, err
	}

	args := req.GetArguments()

	matchCase, err := hostedSlidesOptBool(args, "match_case")
	if err != nil {
		return nil, err
	}

	all, err := hostedSlidesOptBool(args, "all")
	if err != nil {
		return nil, err
	}

	pageIDs, err := hostedSlidesOptObjectIDs(args, "page_ids")
	if err != nil {
		return nil, err
	}

	objectID, err := hostedSlidesOptString(args, "object_id")
	if err != nil {
		return nil, err
	}

	if objectID != "" && !hostedSlidesObjectIDPattern.MatchString(objectID) {
		return nil, fmt.Errorf("%w: object_id %q", errHostedSlidesBadObjectID, objectID)
	}

	modes := 0
	if all {
		modes++
	}
	if len(pageIDs) > 0 {
		modes++
	}
	if objectID != "" {
		modes++
	}
	if modes != 1 {
		return nil, fmt.Errorf("%w (got %d)", errHostedSlidesModeConflict, modes)
	}

	argv := []string{"slides", "replace-text"}
	if matchCase {
		argv = append(argv, "--match-case")
	}

	switch {
	case all:
		argv = append(argv, "--all")
	case len(pageIDs) > 0:
		for _, pageID := range pageIDs {
			argv = append(argv, "--page="+pageID)
		}
	default:
		argv = append(argv, "--object="+objectID)
	}

	return append(argv, "--", presentationID, find, replacement), nil
}

// hostedSlidesOptBool reads an optional strictly-typed boolean argument.
func hostedSlidesOptBool(args map[string]any, key string) (bool, error) {
	value, present := args[key]
	if !present {
		return false, nil
	}

	flag, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %s must be a boolean", errHostedSlidesArgType, key)
	}

	return flag, nil
}

// hostedSlidesOptString reads an optional strictly-typed string argument.
func hostedSlidesOptString(args map[string]any, key string) (string, error) {
	value, present := args[key]
	if !present {
		return "", nil
	}

	if value == nil {
		return "", fmt.Errorf("%w: %s", errHostedSlidesNilArg, key)
	}

	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string", errHostedSlidesArgType, key)
	}

	return text, nil
}

// hostedSlidesOptObjectIDs reads an optional array-of-strings argument and
// validates each entry as a Slides object ID.
func hostedSlidesOptObjectIDs(args map[string]any, key string) ([]string, error) {
	value, present := args[key]
	if !present {
		return nil, nil
	}

	if value == nil {
		return nil, fmt.Errorf("%w: %s", errHostedSlidesNilArg, key)
	}

	var raw []any
	switch typed := value.(type) {
	case []string:
		raw = make([]any, len(typed))
		for i, item := range typed {
			raw[i] = item
		}
	case []any:
		raw = typed
	default:
		return nil, fmt.Errorf("%w: %s must be an array of strings", errHostedSlidesArgType, key)
	}

	ids := make([]string, 0, len(raw))
	for i, item := range raw {
		if item == nil {
			return nil, fmt.Errorf("%w: %s[%d]", errHostedSlidesNilArg, key, i)
		}

		id, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s[%d] must be a string", errHostedSlidesArgType, key, i)
		}

		if !hostedSlidesObjectIDPattern.MatchString(id) {
			return nil, fmt.Errorf("%w: %s[%d] %q", errHostedSlidesBadObjectID, key, i, id)
		}

		ids = append(ids, id)
	}

	return ids, nil
}
