package cmd

import (
	"errors"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedFormTitle = errors.New("invalid form title")

func hostedFormsCreateTools() []mcpToolSpec {
	return []mcpToolSpec{{
		Name: "forms_create_blank", Service: "forms", Risk: mcpRiskWrite,
		Description: "Create a blank Google Form by literal title under an explicit account-level write-tool grant. No description, parent, template, publish or any other option is accepted. The returned form is not automatically granted for later reads or writes.",
		Options:     []mcp.ToolOption{mcp.WithString("title", mcp.Required())},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			title, err := requireMCPString(req, "title")
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(title) == "" || len(title) > 512 || strings.ContainsFunc(title, unicode.IsControl) {
				return nil, errHostedFormTitle
			}
			return []string{"forms", "create", "--title=" + title}, nil
		},
	}}
}
