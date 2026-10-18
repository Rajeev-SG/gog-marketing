package cmd

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedSheetsBlankCreateKeepsTitleLiteral(t *testing.T) {
	spec := hostedSheetsCreateTools()[0]
	title := "--account=foreign@example.com"
	args, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"title": title}}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(args, []string{"sheets", "create", "--", title}) {
		t.Fatalf("title escaped its positional argument: %v", args)
	}
}
