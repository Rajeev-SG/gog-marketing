package cmd

import "github.com/mark3labs/mcp-go/mcp"

func hostedBigQueryDatasetGetTool() mcpToolSpec {
	return mcpToolSpec{
		Name: "bigquery_dataset_get", Service: "bigquery", Risk: mcpRiskRead,
		Description: "Get native BigQuery dataset metadata in an explicitly enabled customer project. No SQL job execution or platform billing.",
		Options:     []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("dataset", mcp.Required())},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			project, err := requireMCPString(req, "project")
			if err != nil {
				return nil, err
			}
			dataset, err := requireMCPString(req, "dataset")
			if err != nil {
				return nil, err
			}
			return []string{"bigquery", "datasets", "get", "--billing-project=" + project, "--", dataset}, nil
		},
	}
}
