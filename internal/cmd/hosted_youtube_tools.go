package cmd

import (
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedYouTubePlaylistTools() []mcpToolSpec {
	return []mcpToolSpec{
		{Name: "youtube_playlists_list", Service: "youtube", Risk: mcpRiskRead, Description: "List the connected account's native YouTube playlists with explicit tool permission; no API-key or arbitrary-channel fallback.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.Min(1), mcp.Max(50)), mcp.WithString("page")}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			args := []string{"youtube", "playlists", "list", "--mine", "--max=" + strconv.Itoa(req.GetInt("max", 25))}
			if page := req.GetString("page", ""); page != "" {
				args = append(args, "--page="+page)
			}
			return args, nil
		}},
		{Name: "youtube_playlists_create", Service: "youtube", Risk: mcpRiskWrite, Description: "Create a native YouTube playlist for the connected account with youtube scope and exact account/tool grant. Default privacy remains private.", Options: []mcp.ToolOption{mcp.WithString("title", mcp.Required()), mcp.WithString("description"), mcp.WithString("privacy", mcp.Enum("public", "unlisted", "private"))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			title, err := requireMCPString(req, "title")
			if err != nil {
				return nil, err
			}
			args := []string{"youtube", "playlists", "create", "--title=" + title}
			for _, key := range []string{"description", "privacy"} {
				if _, present := req.GetArguments()[key]; present {
					args = append(args, "--"+key+"="+req.GetString(key, ""))
				}
			}
			return args, nil
		}},
		{Name: "youtube_playlists_delete", Service: "youtube", Risk: mcpRiskWrite, Description: "Delete a native YouTube playlist using explicit write/destructive permission and youtube scope.", Options: []mcp.ToolOption{mcp.WithString("playlist_id", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			id, err := requireMCPString(req, "playlist_id")
			if err != nil {
				return nil, err
			}
			return []string{"youtube", "playlists", "delete", "--", id}, nil
		}},
	}
}
