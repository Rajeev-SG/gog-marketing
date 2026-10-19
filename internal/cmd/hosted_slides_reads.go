package cmd

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

var (
	errHostedSlidesReadID     = errors.New("invalid presentation id")
	errHostedSlidesText       = errors.New("invalid search text")
	errHostedSlidesOccurrence = errors.New("occurrence must be a positive integer")
	errHostedSlidesReadOption = errors.New("invalid Slides read option")
)

// hostedSlidesReadIDPattern matches canonical Google Drive file IDs. Slide and
// page selectors have a separate rule because colon namespacing is valid there.
var hostedSlidesReadIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

func hostedSlidesReadTool(req mcp.CallToolRequest, argument string) (string, error) {
	value, err := requireMCPString(req, argument)
	if err != nil {
		return "", err
	}
	if value != req.GetString(argument, "") || !hostedSlidesReadIDPattern.MatchString(value) {
		return "", errHostedSlidesReadID
	}
	return value, nil
}

func hostedSlidesReadTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "slides_info", Service: "slides", Risk: mcpRiskRead,
			Description: "Read metadata for one explicitly granted Google Slides presentation. Confirms the selected Drive file is a Google presentation, then returns native presentation metadata under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("presentation_id", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := hostedSlidesReadTool(req, "presentation_id")
				if err != nil {
					return nil, err
				}
				return []string{"slides", "info", "--", id}, nil
			},
		},
		{
			Name: "slides_read_slide", Service: "slides", Risk: mcpRiskRead,
			Description: "Read speaker notes, text, images and tables for one selected slide in an explicitly granted Google Slides presentation. Optional detail adds normalized element geometry, text runs/styles, paragraphs and table cells.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required()),
				mcp.WithString("slide_id", mcp.Required()),
				mcp.WithBoolean("detail"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := hostedSlidesReadTool(req, "presentation_id")
				if err != nil {
					return nil, err
				}
				slide, err := requireMCPString(req, "slide_id")
				if err != nil {
					return nil, err
				}
				if !hostedSlidesObjectIDPattern.MatchString(slide) || slide != req.GetString("slide_id", "") {
					return nil, errHostedSlidesReadID
				}
				if raw, present := req.GetArguments()["detail"]; present {
					if _, ok := raw.(bool); !ok {
						return nil, errHostedSlidesReadOption
					}
				}
				argv := []string{"slides", "read-slide"}
				if req.GetArguments()["detail"] == true {
					argv = append(argv, "--detail")
				}
				return append(argv, "--", id, slide), nil
			},
		},
		{
			Name: "slides_locate", Service: "slides", Risk: mcpRiskRead,
			Description: "Locate literal text with UTF-16 indexes in an explicitly granted Google Slides presentation. Supports one selected page, case sensitivity, all matches or a one-based occurrence. Text is preserved literally; empty text is rejected and no regex or additional requests are made.",
			Options: []mcp.ToolOption{
				mcp.WithString("presentation_id", mcp.Required()),
				mcp.WithString("text", mcp.Required()),
				mcp.WithString("page_id"),
				mcp.WithBoolean("match_case"),
				mcp.WithBoolean("all"),
				mcp.WithInteger("occurrence", mcp.Min(1), mcp.Max(2147483647)),
				mcp.WithBoolean("fail_empty"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := hostedSlidesReadTool(req, "presentation_id")
				if err != nil {
					return nil, err
				}
				text, err := req.RequireString("text")
				if err != nil || text == "" {
					return nil, errHostedSlidesText
				}
				for _, key := range []string{"all", "match_case", "fail_empty"} {
					if raw, present := req.GetArguments()[key]; present {
						if _, ok := raw.(bool); !ok {
							return nil, errHostedSlidesReadOption
						}
					}
				}
				if req.GetArguments()["all"] == true {
					if _, present := req.GetArguments()["occurrence"]; present {
						return nil, errHostedSlidesReadOption
					}
				}
				argv := []string{"slides", "locate"}
				if raw, present := req.GetArguments()["page_id"]; present {
					page, ok := raw.(string)
					if !ok {
						return nil, errHostedSlidesReadID
					}
					if page != "" {
						if !hostedSlidesObjectIDPattern.MatchString(page) {
							return nil, errHostedSlidesReadID
						}
						argv = append(argv, "--page="+page)
					}
				}
				if req.GetArguments()["match_case"] == true {
					argv = append(argv, "--match-case")
				}
				if req.GetArguments()["all"] == true {
					argv = append(argv, "--all")
				}
				if value, present := req.GetArguments()["occurrence"]; present {
					count, err := hostedSlidesOccurrenceValue(value)
					if err != nil {
						return nil, err
					}
					argv = append(argv, "--occurrence="+strconv.Itoa(count))
				}
				if req.GetArguments()["fail_empty"] == true {
					argv = append(argv, "--fail-empty")
				}
				return append(argv, "--", id, text), nil
			},
		},
	}
}

var hostedSlidesObjectIDPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_:-]{0,255}$`)

func hostedSlidesOccurrenceValue(value any) (int, error) {
	switch n := value.(type) {
	case int:
		if n >= 1 && n <= 2147483647 {
			return n, nil
		}
	case int64:
		if n >= 1 && n <= 2147483647 {
			return int(n), nil
		}
	case float64:
		if n >= 1 && n <= 2147483647 && n == float64(int(n)) {
			return int(n), nil
		}
	}
	return 0, errHostedSlidesOccurrence
}
