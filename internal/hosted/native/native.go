// Package native defines the hosted boundary to existing typed command handlers.
// Executable entrypoints compose the implementation; runners never import cmd.
package native

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var (
	ErrDenied      = errors.New("hosted tool is not permitted")
	ErrInput       = errors.New("hosted tool input is invalid")
	ErrExecution   = errors.New("hosted tool execution failed")
	ErrOutputLimit = errors.New("hosted tool output exceeds limit")

	// ErrWorkspaceRequired marks a typed tool call that failed because the
	// account does not have a Google Workspace customer. Runners map it to a
	// fixed consumer-facing instruction, never raw provider text.
	ErrWorkspaceRequired = errors.New("Google Workspace required")
)

type Tool struct {
	Service string   `json:"service"`
	Risk    string   `json:"risk"`
	Tool    mcp.Tool `json:"tool"`
}

type Options struct {
	Account        string
	AccessToken    string
	AllowWrite     bool
	Force          bool
	QuotaProject   string
	AllowBillable  bool
	MaxOutputBytes int
	RequestGuard   googleapi.RequestGuard
	Services       app.Services
}

type Execute func(context.Context, string, map[string]any, Options) (any, error)
