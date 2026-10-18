package cmd

import (
	"errors"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

var (
	errHostedCommentsID  = errors.New("invalid id")
	errHostedCommentsMax = errors.New("max must be an integer between 1 and 100")
)

// hostedCommentsWatchReadTools registers the three fixed read-only tools for
// the drive and forms services: comment listing, comment retrieval and forms
// watch listing. Each tool targets exactly one selected resource id and never
// exposes all/since selectors, write operations or provider plumbing options.
func hostedCommentsWatchReadTools() []mcpToolSpec {
	idOf := func(req mcp.CallToolRequest, arg string) (string, error) {
		id, err := requireMCPString(req, arg)
		if err != nil {
			return "", err
		}
		if !hostedDocsResourceIDPattern.MatchString(id) {
			return "", errHostedCommentsID
		}
		return id, nil
	}

	buildCommentsList := func(req mcp.CallToolRequest) ([]string, error) {
		fileID, err := idOf(req, "file_id")
		if err != nil {
			return nil, err
		}
		argv := []string{"drive", "comments", "list", "--max=50"}
		if raw, ok := req.GetArguments()["max"]; ok {
			pageSize, err := hostedCommentsWatchReadMax(raw)
			if err != nil {
				return nil, err
			}
			argv[3] = "--max=" + strconv.Itoa(pageSize)
		}
		if page := req.GetString("page", ""); page != "" {
			argv = append(argv, "--page="+page)
		}
		if req.GetArguments()["include_quoted"] == true {
			argv = append(argv, "--include-quoted")
		}
		return append(argv, "--", fileID), nil
	}

	buildCommentsGet := func(req mcp.CallToolRequest) ([]string, error) {
		fileID, err := idOf(req, "file_id")
		if err != nil {
			return nil, err
		}
		commentID, err := idOf(req, "comment_id")
		if err != nil {
			return nil, err
		}
		return []string{"drive", "comments", "get", "--", fileID, commentID}, nil
	}

	buildFormsWatchList := func(req mcp.CallToolRequest) ([]string, error) {
		formID, err := idOf(req, "form_id")
		if err != nil {
			return nil, err
		}
		return []string{"forms", "watch", "list", "--", formID}, nil
	}

	return []mcpToolSpec{
		{
			Name:        "drive_comments_list",
			Service:     "drive",
			Risk:        mcpRiskRead,
			Description: "List comments on one explicitly granted Google Drive file under an exact account/tool/file read grant. Supports paging and quoted-content inclusion; no all/since selectors or write access.",
			Options: []mcp.ToolOption{
				mcp.WithString("file_id", mcp.Required()),
				mcp.WithInteger("max", mcp.Min(1), mcp.Max(100)),
				mcp.WithString("page"),
				mcp.WithBoolean("include_quoted"),
			},
			BuildArgs: buildCommentsList,
		},
		{
			Name:        "drive_comments_get",
			Service:     "drive",
			Risk:        mcpRiskRead,
			Description: "Read one comment by id from one explicitly granted Google Drive file under an exact account/tool/file read grant.",
			Options: []mcp.ToolOption{
				mcp.WithString("file_id", mcp.Required()),
				mcp.WithString("comment_id", mcp.Required()),
			},
			BuildArgs: buildCommentsGet,
		},
		{
			Name:        "forms_watch_list",
			Service:     "forms",
			Risk:        mcpRiskRead,
			Description: "List watches configured on one explicitly granted Google Form under an exact account/tool/file read grant. No billing, create, renew or pubsub-target options.",
			Options: []mcp.ToolOption{
				mcp.WithString("form_id", mcp.Required()),
			},
			BuildArgs: buildFormsWatchList,
		},
	}
}

// hostedCommentsWatchReadMax coerces the optional max option to an integer in
// the 1..100 range; anything else is rejected before argv is built.
func hostedCommentsWatchReadMax(raw any) (int, error) {
	switch v := raw.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, errHostedCommentsMax
		}
		pageSize := int(v)
		if pageSize < 1 || pageSize > 100 {
			return 0, errHostedCommentsMax
		}
		return pageSize, nil
	case int:
		if v < 1 || v > 100 {
			return 0, errHostedCommentsMax
		}
		return v, nil
	default:
		return 0, errHostedCommentsMax
	}
}
