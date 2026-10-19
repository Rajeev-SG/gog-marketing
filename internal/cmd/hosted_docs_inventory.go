package cmd

import (
	"errors"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedDocsInventoryID = errors.New("invalid document id")

// hostedDocsResourceIDPattern matches Drive/Docs file identifiers: opaque
// ASCII tokens of 1-256 characters. It rejects separators that could be
// interpreted as URLs or path traversal; the literal argument separator
// prevents flag interpretation.
var hostedDocsResourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// hostedDocsInventoryTools registers the fixed read-only inventory tools for
// the docs service. Each builds one existing docs CLI read command against a
// single selected document id. The caller never chooses the host, method,
// path, command shape or optional CLI selectors; there is no generic proxy
// path.
func hostedDocsInventoryTools() []mcpToolSpec {
	build := func(command string) func(mcp.CallToolRequest) ([]string, error) {
		return func(req mcp.CallToolRequest) ([]string, error) {
			id, err := requireMCPString(req, "document_id")
			if err != nil {
				return nil, err
			}
			if err := validateHostedDocsInventoryID(id); err != nil {
				return nil, err
			}
			parts := append([]string{"docs"}, strings.Split(command, " ")...)
			return append(parts, "--", id), nil
		}
	}
	return []mcpToolSpec{
		{
			Name: "docs_info", Service: "docs", Risk: mcpRiskRead,
			Description: "Read metadata (documentId, title, revisionId) for one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("info"),
		},
		{
			Name: "docs_list_tabs", Service: "docs", Risk: mcpRiskRead,
			Description: "List tab identifiers, titles, indexes and hierarchy for one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("list-tabs"),
		},
		{
			Name: "docs_structure", Service: "docs", Risk: mcpRiskRead,
			Description: "Show numbered default-tab structure for one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("structure"),
		},
		{
			Name: "docs_headings_list", Service: "docs", Risk: mcpRiskRead,
			Description: "List headings in the default tab of one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("headings list"),
		},
		{
			Name: "docs_paragraphs_list", Service: "docs", Risk: mcpRiskRead,
			Description: "List paragraphs in the default tab of one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("paragraphs list"),
		},
		{
			Name: "docs_tables_list", Service: "docs", Risk: mcpRiskRead,
			Description: "List native tables in the default tab of one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("tables list"),
		},
		{
			Name: "docs_images_list", Service: "docs", Risk: mcpRiskRead,
			Description: "List images in the default tab of one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("images list"),
		},
		{
			Name: "docs_named_range_list", Service: "docs", Risk: mcpRiskRead,
			Description: "List named ranges across one explicitly granted Google document under an exact account/tool/file read grant.",
			Options:     []mcp.ToolOption{mcp.WithString("document_id", mcp.Required())},
			BuildArgs:   build("named-range list"),
		},
	}
}

// validateHostedDocsInventoryID enforces the selected-document contract
// shared by docs_info and docs_list_tabs: an opaque 1-256 character
// alphanumeric identifier with no URL or path-traversal syntax.
func validateHostedDocsInventoryID(id string) error {
	if !hostedDocsResourceIDPattern.MatchString(id) {
		return errHostedDocsInventoryID
	}
	return nil
}
