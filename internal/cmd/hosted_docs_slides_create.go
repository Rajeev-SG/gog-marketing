package cmd

import (
	"errors"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedDocsSlidesTitle = errors.New("invalid title")

// hostedDocsSlidesCreateTools registers the two fixed blank-creation tools for
// the docs and slides services. Both build the existing docs/slides CLI create
// commands, which issue a single Drive.Files.Create with a fixed MIME type when
// every optional flag (parent, template, source file, pageless, markdown) is
// omitted. The caller never chooses the MIME type, host, method or root
// command, and there is no generic proxy path.
func hostedDocsSlidesCreateTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "docs_create_blank", Service: "docs", Risk: mcpRiskWrite,
			Description: "Create a blank Google document by literal title under an explicit account-level write-tool grant. No parent, template, source file or markdown is accepted. The returned file is not automatically granted for later reads or writes.",
			Options:     []mcp.ToolOption{mcp.WithString("title", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				title, err := requireMCPString(req, "title")
				if err != nil {
					return nil, err
				}
				if err := validateHostedBlankCreateTitle(title); err != nil {
					return nil, err
				}
				return []string{"docs", "create", "--", title}, nil
			},
		},
		{
			Name: "slides_create_blank", Service: "slides", Risk: mcpRiskWrite,
			Description: "Create a blank Google Slides presentation by literal title under an explicit account-level write-tool grant. No parent or template is accepted. The returned file is not automatically granted for later reads or writes.",
			Options:     []mcp.ToolOption{mcp.WithString("title", mcp.Required())},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				title, err := requireMCPString(req, "title")
				if err != nil {
					return nil, err
				}
				if err := validateHostedBlankCreateTitle(title); err != nil {
					return nil, err
				}
				return []string{"slides", "create", "--", title}, nil
			},
		},
	}
}

// validateHostedBlankCreateTitle enforces the literal-title contract shared by
// docs_create_blank and slides_create_blank: non-blank, at most 512 UTF-8
// bytes, no Unicode control characters, passed as a single positional argument
// behind the "--" separator.
func validateHostedBlankCreateTitle(title string) error {
	if strings.TrimSpace(title) == "" || len(title) > 512 || strings.ContainsFunc(title, unicode.IsControl) {
		return errHostedDocsSlidesTitle
	}
	return nil
}
