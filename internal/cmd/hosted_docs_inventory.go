package cmd

import (
	"errors"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedDocsInventoryID = errors.New("invalid document id")

// hostedDocsResourceIDPattern matches Drive/Docs file identifiers: opaque
// ASCII tokens of 1-256 characters. It rejects separators that could be
// interpreted as URLs or path traversal; the literal argument separator
// prevents flag interpretation.
var hostedDocsResourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// hostedDocsInventoryTools registers the two fixed read-only inventory tools
// for the docs service. Both build the existing docs CLI info/list-tabs
// commands against a single selected document id. The caller never chooses
// the host, method, path or command shape, and there is no generic proxy path.
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
			return []string{"docs", command, "--", id}, nil
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
