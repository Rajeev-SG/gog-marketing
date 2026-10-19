package cmd

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var hostedSlidesLayouts = map[string]bool{
	"BLANK":                         true,
	"CAPTION_ONLY":                  true,
	"TITLE":                         true,
	"TITLE_AND_BODY":                true,
	"TITLE_AND_TWO_COLUMNS":         true,
	"TITLE_ONLY":                    true,
	"SECTION_HEADER":                true,
	"SECTION_TITLE_AND_DESCRIPTION": true,
	"ONE_COLUMN_TEXT":               true,
	"MAIN_POINT":                    true,
	"BIG_NUMBER":                    true,
}

func hostedSlidesLifecycleTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name:        "slides_new_slide",
			Service:     "slides",
			Risk:        mcpRiskWrite,
			Description: "Add a slide to a selected Google Slides presentation via gog slides new-slide. Omitted layout chooses the native BLANK default; layout and layout_id are mutually exclusive.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Selected presentation ID.")),
				mcp.WithString("layout", mcp.Description("Optional predefined layout. Valid values: BLANK, CAPTION_ONLY, TITLE, TITLE_AND_BODY, TITLE_AND_TWO_COLUMNS, TITLE_ONLY, SECTION_HEADER, SECTION_TITLE_AND_DESCRIPTION, ONE_COLUMN_TEXT, MAIN_POINT, BIG_NUMBER.")),
				mcp.WithString("layout_id", mcp.Description("Optional exact presentation layout object ID; mutually exclusive with layout.")),
				mcp.WithInteger("index", mcp.Min(0), mcp.Max(2147483647), mcp.Description("Optional zero-based insertion index.")),
			},
			BuildArgs: hostedSlidesNewSlideArgs,
		},
		{
			Name:        "slides_duplicate_slide",
			Service:     "slides",
			Risk:        mcpRiskWrite,
			Description: "Duplicate a slide in a selected Google Slides presentation via gog slides duplicate-slide.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Selected presentation ID.")),
				mcp.WithString("slide_id", mcp.Required(), mcp.Description("Slide object ID to duplicate.")),
				mcp.WithInteger("to_index", mcp.Min(0), mcp.Max(2147483647), mcp.Description("Optional zero-based insertion index.")),
			},
			BuildArgs: hostedSlidesSlideLifecycleObjectArgs("duplicate-slide", false),
		},
		{
			Name:        "slides_move_slide",
			Service:     "slides",
			Risk:        mcpRiskWrite,
			Description: "Move a slide in a selected Google Slides presentation to a zero-based position via gog slides move-slide.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Selected presentation ID.")),
				mcp.WithString("slide_id", mcp.Required(), mcp.Description("Slide object ID to move.")),
				mcp.WithInteger("to_index", mcp.Required(), mcp.Min(0), mcp.Max(2147483647), mcp.Description("Required zero-based insertion index.")),
			},
			BuildArgs: hostedSlidesSlideLifecycleObjectArgs("move-slide", true),
		},
		{
			Name:        "slides_delete_slide",
			Service:     "slides",
			Risk:        mcpRiskWrite,
			Description: "Delete a slide from a selected Google Slides presentation via gog slides delete-slide. The runner additionally requires destructive permission.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required(), mcp.Description("Selected presentation ID.")),
				mcp.WithString("slide_id", mcp.Required(), mcp.Description("Slide object ID to delete.")),
			},
			BuildArgs: hostedSlidesSlideToggleArgs("delete-slide"),
		},
	}
}

func hostedSlidesNewSlideArgs(req mcp.CallToolRequest) ([]string, error) {
	presentationID, err := hostedSlidesReadTool(req)
	if err != nil {
		return nil, err
	}
	if !hostedSlidesReadIDPattern.MatchString(presentationID) {
		return nil, fmt.Errorf("%w: %q", errHostedSlidesBadPID, presentationID)
	}

	args := req.GetArguments()
	if rawLayout, present := args["layout"]; present {
		if value, ok := rawLayout.(string); !ok || value == "" || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%w: layout", errHostedSlidesArgType)
		}
	}
	layout, err := hostedSlidesOptString(args, "layout")
	if err != nil {
		return nil, err
	}
	if layout != "" {
		if strings.TrimSpace(layout) == "" || !hostedSlidesLayouts[layout] {
			return nil, fmt.Errorf("%w: invalid layout %q", errHostedSlidesArgType, layout)
		}
	}

	if rawLayoutID, present := args["layout_id"]; present {
		if value, ok := rawLayoutID.(string); !ok || value == "" || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%w: layout_id", errHostedSlidesArgType)
		}
	}
	layoutID, err := hostedSlidesOptString(args, "layout_id")
	if err != nil {
		return nil, err
	}
	if layoutID != "" {
		if strings.TrimSpace(layoutID) == "" || !hostedSlidesObjectIDPattern.MatchString(layoutID) {
			return nil, fmt.Errorf("%w: invalid layout_id %q", errHostedSlidesBadObjectID, layoutID)
		}
	}
	if layout != "" && layoutID != "" {
		return nil, errHostedSlidesModeConflict
	}

	argv := []string{"slides", "new-slide"}
	if layout != "" {
		argv = append(argv, "--layout="+layout)
	}
	if layoutID != "" {
		argv = append(argv, "--layout-id="+layoutID)
	}
	if _, present := args["index"]; present {
		index, err := validateHostedTabIndexArg(args["index"])
		if err != nil {
			return nil, fmt.Errorf("%w: index", errHostedSlidesArgType)
		}
		argv = append(argv, fmt.Sprintf("--index=%d", index))
	}
	return append(argv, "--", presentationID), nil
}

func hostedSlidesSlideLifecycleObjectArgs(verb string, requireIndex bool) func(mcp.CallToolRequest) ([]string, error) {
	return func(req mcp.CallToolRequest) ([]string, error) {
		presentationID, err := hostedSlidesReadTool(req)
		if err != nil {
			return nil, err
		}
		if !hostedSlidesReadIDPattern.MatchString(presentationID) {
			return nil, fmt.Errorf("%w: %q", errHostedSlidesBadPID, presentationID)
		}

		slideID, err := req.RequireString("slide_id")
		if err != nil {
			return nil, err
		}
		if slideID != req.GetString("slide_id", "") || !hostedSlidesObjectIDPattern.MatchString(slideID) {
			return nil, fmt.Errorf("%w: slide_id %q", errHostedSlidesBadObjectID, slideID)
		}

		argv := []string{"slides", verb}
		args := req.GetArguments()
		rawIndex, present := args["to_index"]
		if present || requireIndex {
			if !present {
				return nil, fmt.Errorf("%w: to_index", errHostedSlidesNilArg)
			}
			index, err := validateHostedTabIndexArg(rawIndex)
			if err != nil {
				return nil, fmt.Errorf("%w: to_index", errHostedSlidesArgType)
			}
			argv = append(argv, fmt.Sprintf("--to-index=%d", index))
		}
		return append(argv, "--", presentationID, slideID), nil
	}
}
