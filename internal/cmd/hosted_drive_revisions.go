package cmd

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

var errHostedDriveRevisionArgument = errors.New("invalid Drive revision argument")

func hostedDriveRevisionID(request mcp.CallToolRequest, name string) (string, error) {
	value, ok := request.GetArguments()[name].(string)
	if !ok || !hostedDocsResourceIDPattern.MatchString(value) {
		return "", errHostedDriveRevisionArgument
	}
	return value, nil
}

func hostedDriveRevisionMax(raw any) (int, error) {
	var value float64
	switch number := raw.(type) {
	case int:
		value = float64(number)
	case int64:
		value = float64(number)
	case float64:
		value = number
	default:
		return 0, errHostedDriveRevisionArgument
	}
	if math.IsNaN(value) || value < 1 || value > 1000 || math.Trunc(value) != value {
		return 0, errHostedDriveRevisionArgument
	}
	return int(value), nil
}

func hostedDriveRevisionTools() []mcpToolSpec {
	return []mcpToolSpec{
		{
			Name: "drive_revisions_list", Service: "drive", Risk: mcpRiskRead,
			Description: "List one bounded page of revision metadata for an explicitly granted Drive file. Returns a next-page token when available; no content downloads or automatic page traversal.",
			Options:     []mcp.ToolOption{mcp.WithString("file_id", mcp.Required()), mcp.WithInteger("max_results", mcp.Min(1), mcp.Max(1000)), mcp.WithString("page_token")},
			BuildArgs: func(request mcp.CallToolRequest) ([]string, error) {
				id, err := hostedDriveRevisionID(request, "file_id")
				if err != nil {
					return nil, err
				}
				limit := 200
				if raw, present := request.GetArguments()["max_results"]; present {
					limit, err = hostedDriveRevisionMax(raw)
					if err != nil {
						return nil, err
					}
				}
				args := []string{"drive", "revisions", "list", "--max=" + strconv.Itoa(limit)}
				if raw, present := request.GetArguments()["page_token"]; present {
					token, ok := raw.(string)
					if !ok || len(token) > 4096 || strings.IndexFunc(token, unicode.IsControl) >= 0 || strings.TrimSpace(token) != token {
						return nil, errHostedDriveRevisionArgument
					}
					if token != "" {
						args = append(args, "--page="+token)
					}
				}
				return append(args, "--", id), nil
			},
		},
		{
			Name: "drive_revisions_get", Service: "drive", Risk: mcpRiskRead,
			Description: "Read metadata for one explicit revision of an explicitly granted Drive file. Returned export links are metadata only and are not fetched.",
			Options:     []mcp.ToolOption{mcp.WithString("file_id", mcp.Required()), mcp.WithString("revision_id", mcp.Required())},
			BuildArgs: func(request mcp.CallToolRequest) ([]string, error) {
				id, err := hostedDriveRevisionID(request, "file_id")
				if err != nil {
					return nil, err
				}
				revision, err := hostedDriveRevisionID(request, "revision_id")
				if err != nil {
					return nil, err
				}
				return []string{"drive", "revisions", "get", "--", id, revision}, nil
			},
		},
	}
}
