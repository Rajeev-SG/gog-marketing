package cmd

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var (
	errHostedDocsTabLabel = errors.New("invalid tab label")
	errHostedDocsTabIndex = errors.New("invalid tab index")
)

// hostedLabelControlPattern rejects every Unicode C-category character
// (control, format, surrogate, private use, unassigned) in tab labels.
var hostedLabelControlPattern = regexp.MustCompile(`\p{C}`)

// validateHostedTabLabelArg enforces the shared label contract: the raw
// argument must be an actual string (nil and every other type are rejected),
// at most 512 UTF-8 bytes, and free of Unicode C-category characters.
// Required labels must be non-empty after trimming; optional labels may be
// empty (auto-title, root parent, no icon). The literal value is returned
// unchanged so the flag value is preserved verbatim.
func validateHostedTabLabelArg(raw any, required bool) (string, error) {
	label, ok := raw.(string)
	if !ok {
		return "", errHostedDocsTabLabel
	}
	if len(label) > 512 || hostedLabelControlPattern.MatchString(label) {
		return "", errHostedDocsTabLabel
	}
	if required && strings.TrimSpace(label) == "" {
		return "", errHostedDocsTabLabel
	}
	return label, nil
}

// validateHostedTabIndexArg enforces the strict index contract: an actual
// int, int64, or integral float64 within 0..2^31-1. nil, non-integral
// floats and every other type are rejected; no truncation occurs.
func validateHostedTabIndexArg(raw any) (int, error) {
	switch value := raw.(type) {
	case int:
		if value < 0 || value > math.MaxInt32 {
			return 0, errHostedDocsTabIndex
		}
		return value, nil
	case int64:
		if value < 0 || value > math.MaxInt32 {
			return 0, errHostedDocsTabIndex
		}
		return int(value), nil
	case float64:
		if value != math.Trunc(value) || value < 0 || value > math.MaxInt32 {
			return 0, errHostedDocsTabIndex
		}
		return int(value), nil
	default:
		return 0, errHostedDocsTabIndex
	}
}

// hostedDocsTabTools registers the three fixed tab-management tools for the
// docs service. Each builds one existing docs tabs CLI command against a
// selected document id. Optional flags are emitted only when provided, with
// literal values preserved verbatim; there is no generic proxy path.
func hostedDocsTabTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "docs_add_tab", Service: "docs", Risk: mcpRiskWrite,
			Description: "Add a new tab to a Google Doc under an explicit account-level write-tool grant. An omitted or empty title generates an automatic title; an omitted or empty parent tab places the tab at the root; an omitted or empty icon emoji means no icon.",
			Options: []mcp.ToolOption{
				mcp.WithString("document_id", mcp.Required()),
				mcp.WithString("title", mcp.Description("Optional literal tab title; empty or omitted auto-titles the tab.")),
				mcp.WithInteger("index", mcp.Min(0), mcp.Max(2147483647), mcp.Description("Optional zero-based insertion position.")),
				mcp.WithString("parent_tab", mcp.Description("Optional parent tab title or ID; empty or omitted places the tab at the root.")),
				mcp.WithString("icon_emoji", mcp.Description("Optional emoji for the tab; empty or omitted means no icon.")),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "document_id")
				if err != nil {
					return nil, err
				}
				if id != req.GetString("document_id", "") {
					return nil, errHostedDocsInventoryID
				}
				if validationErr := validateHostedDocsInventoryID(id); validationErr != nil {
					return nil, validationErr
				}
				args := []string{"docs", "tabs", "add"}
				arguments := req.GetArguments()
				if raw, present := arguments["title"]; present {
					title, err := validateHostedTabLabelArg(raw, false)
					if err != nil {
						return nil, err
					}
					args = append(args, "--title="+title)
				}
				if raw, present := arguments["index"]; present {
					index, err := validateHostedTabIndexArg(raw)
					if err != nil {
						return nil, err
					}
					args = append(args, "--index="+strconv.Itoa(index))
				}
				if raw, present := arguments["parent_tab"]; present {
					parent, err := validateHostedTabLabelArg(raw, false)
					if err != nil {
						return nil, err
					}
					args = append(args, "--parent-tab="+parent)
				}
				if raw, present := arguments["icon_emoji"]; present {
					icon, err := validateHostedTabLabelArg(raw, false)
					if err != nil {
						return nil, err
					}
					args = append(args, "--icon-emoji="+icon)
				}
				return append(args, "--", id), nil
			},
		},
		{
			Name: "docs_rename_tab", Service: "docs", Risk: mcpRiskWrite,
			Description: "Rename an existing tab in a Google Doc under an explicit account-level write-tool grant. The tab and new title are matched literally by title or ID.",
			Options: []mcp.ToolOption{
				mcp.WithString("document_id", mcp.Required()),
				mcp.WithString("tab", mcp.Required()),
				mcp.WithString("title", mcp.Required()),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "document_id")
				if err != nil {
					return nil, err
				}
				if id != req.GetString("document_id", "") {
					return nil, errHostedDocsInventoryID
				}
				if validationErr := validateHostedDocsInventoryID(id); validationErr != nil {
					return nil, validationErr
				}
				arguments := req.GetArguments()
				tab, err := validateHostedTabLabelArg(arguments["tab"], true)
				if err != nil {
					return nil, err
				}
				title, err := validateHostedTabLabelArg(arguments["title"], true)
				if err != nil {
					return nil, err
				}
				return []string{"docs", "tabs", "rename", "--tab=" + tab, "--title=" + title, "--", id}, nil
			},
		},
		{
			Name: "docs_delete_tab", Service: "docs", Risk: mcpRiskWrite,
			Description: "Delete a tab from a Google Doc by title or ID. This destructive action requires a separate destructive permission grant in addition to the write-tool grant, and the deleted tab content cannot be recovered.",
			Options: []mcp.ToolOption{
				mcp.WithString("document_id", mcp.Required()),
				mcp.WithString("tab", mcp.Required()),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				id, err := requireMCPString(req, "document_id")
				if err != nil {
					return nil, err
				}
				if id != req.GetString("document_id", "") {
					return nil, errHostedDocsInventoryID
				}
				if validationErr := validateHostedDocsInventoryID(id); validationErr != nil {
					return nil, validationErr
				}
				tab, err := validateHostedTabLabelArg(req.GetArguments()["tab"], true)
				if err != nil {
					return nil, err
				}
				return []string{"docs", "tabs", "delete", "--tab=" + tab, "--", id}, nil
			},
		},
	}
}
