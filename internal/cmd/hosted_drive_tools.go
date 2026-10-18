package cmd

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func hostedDriveRenameTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "drive_rename",
		Service:     "drive",
		Risk:        mcpRiskWrite,
		Description: "Rename a Picker-selected Drive file. Requires drive.file and an explicit selected-file grant.",
		Options: []mcp.ToolOption{
			mcp.WithString("file_id", mcp.Required()),
			mcp.WithString("new_name", mcp.Required()),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			fileID, err := requireMCPString(req, "file_id")
			if err != nil {
				return nil, err
			}
			newName, err := requireMCPString(req, "new_name")
			if err != nil {
				return nil, err
			}
			return []string{"drive", "rename", "--", fileID, newName}, nil
		},
	}
}

func hostedDriveCopyTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "drive_copy",
		Service:     "drive",
		Risk:        mcpRiskWrite,
		Description: "Copy a Picker-selected Drive file. Requires drive.file and an explicit selected-file grant.",
		Options: []mcp.ToolOption{
			mcp.WithString("file_id", mcp.Required()),
			mcp.WithString("new_name", mcp.Required()),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			fileID, err := requireMCPString(req, "file_id")
			if err != nil {
				return nil, err
			}
			newName, err := requireMCPString(req, "new_name")
			if err != nil {
				return nil, err
			}
			return []string{"drive", "copy", "--", fileID, newName}, nil
		},
	}
}

func hostedDriveTrashTool() mcpToolSpec {
	return mcpToolSpec{
		Name:        "drive_trash",
		Service:     "drive",
		Risk:        mcpRiskWrite,
		Description: "Trash a freshly verified non-folder Picker-selected Drive file. Requires drive.file, the exact write/file grant and destructive permission. Folders, shortcuts, unknown kinds and permanent deletion are not supported.",
		Options: []mcp.ToolOption{
			mcp.WithString("file_id", mcp.Required()),
		},
		BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			fileID, err := requireMCPString(req, "file_id")
			if err != nil {
				return nil, err
			}
			args := []string{"drive", "delete", "--", fileID}
			return args, nil
		},
	}
}
