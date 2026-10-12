package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/hosted/native"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

var (
	ErrHostedToolDenied      = native.ErrDenied
	ErrHostedToolInput       = native.ErrInput
	ErrHostedToolExecution   = native.ErrExecution
	ErrHostedToolOutputLimit = native.ErrOutputLimit
)

// HostedTool describes an existing fixed-schema tool, not a generic CLI entry.
// Resource authorization and scope checks belong to the private runner before
// it invokes this bridge. The bridge never exposes a shell or accepts argv.
type HostedTool = native.Tool

func hostedExistingTools() []mcpToolSpec {
	var tools []mcpToolSpec
	for _, spec := range mcpAllTools() {
		if spec.Service == "gmail" || spec.Service == "googleads" || spec.Name == "drive_search" || spec.Name == "analytics_datastreams_list" {
			continue
		}
		if spec.Name == "tagmanager_versions_publish" {
			spec = hostedTagManagerPublishTool()
		}
		tools = append(tools, spec)
	}
	tools = append(tools, hostedSlidesTools()...)
	tools = append(tools, hostedTasksTools()...)
	tools = append(tools, hostedAnalyticsAdminTools()...)
	tools = append(tools, hostedSearchConsoleReadTools()...)
	tools = append(tools, hostedContactsTools()...)
	tools = append(tools, hostedCalendarEventTools()...)
	tools = append(tools, hostedDriveLabelsTools()...)
	return append(tools, hostedGroupsListTool(), hostedGroupsMembersTool())
}

// HostedExistingToolCatalog reuses the same schemas and typed argument builders
// as local MCP. It does not claim parity with every CLI command or live proof.
func HostedExistingToolCatalog() []HostedTool {
	specs := hostedExistingTools()
	tools := make([]HostedTool, 0, len(specs))
	for _, spec := range specs {
		tools = append(tools, HostedTool{Service: spec.Service, Risk: string(spec.Risk), Tool: newMCPTool(spec)})
	}
	return tools
}

// HostedToolOptions is trusted, code-only execution context, never client JSON.
// AllowWrite and Force must come from the signed server authorization snapshot.
type HostedToolOptions = native.Options

// ExecuteHostedExistingTool validates the existing MCP schema, builds only its
// fixed command fields and invokes the native Go handler in-process. It bypasses
// the CLI's root execution lifecycle: no global logger, config, ADC, token store,
// keyring, browser reauthorization or subprocess is initialized.
func ExecuteHostedExistingTool(parent context.Context, name string, arguments map[string]any, options HostedToolOptions) (any, error) {
	var selected *mcpToolSpec
	for _, spec := range hostedExistingTools() {
		if spec.Name == name {
			candidate := spec
			selected = &candidate
			break
		}
	}
	if selected == nil || (selected.Risk == mcpRiskWrite && !options.AllowWrite) {
		return nil, ErrHostedToolDenied
	}
	if !strings.Contains(options.Account, "@") || strings.TrimSpace(options.AccessToken) == "" {
		return nil, ErrHostedToolDenied
	}
	if selected.Name == "bigquery_query" {
		project, _ := arguments["project"].(string)
		if !options.AllowBillable || options.QuotaProject == "" || options.QuotaProject == "gog-marketing-prod" || project != options.QuotaProject {
			return nil, ErrHostedToolDenied
		}
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	var result any
	var callErr error
	invoked := false
	validator := newMCPServer()
	validator.AddTool(newMCPTool(*selected), func(callCtx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		invoked = true
		args, err := selected.BuildArgs(request)
		if err != nil {
			callErr = ErrHostedToolInput
			return nil, callErr
		}
		result, callErr = executeHostedCommand(callCtx, args, *selected, options)
		return mcp.NewToolResultText("completed"), nil
	})
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": arguments},
	})
	if err != nil {
		return nil, ErrHostedToolInput
	}
	validator.HandleMessage(ctx, payload)
	if !invoked {
		return nil, ErrHostedToolInput
	}
	return result, callErr
}

func executeHostedCommand(ctx context.Context, args []string, spec mcpToolSpec, options HostedToolOptions) (any, error) {
	limit := options.MaxOutputBytes
	if limit <= 0 {
		limit = 262144
	}
	output := newMCPLimitedBuffer(limit)
	parser, cli, err := newParserWithWriters("Hosted typed execution", &output, io.Discard)
	if err != nil {
		return nil, ErrHostedToolExecution
	}
	command, err := parser.Parse(args)
	if err != nil {
		return nil, ErrHostedToolInput
	}
	// Discard ambient root flags after parsing: account, credentials and write
	// authority can only come from the trusted runner, never env or tool arguments.
	cli.RootFlags = RootFlags{
		Account: options.Account, AccessToken: options.AccessToken,
		QuotaProject: options.QuotaProject, JSON: true, NoInput: true,
		ReadOnly: spec.Risk == mcpRiskRead, Force: options.Force,
		authMode:            googleapi.AuthModeStored,
		configStoreResolver: func() (*config.ConfigStore, error) { return nil, ErrHostedToolDenied },
		hostedExecution:     true,
	}
	runtime := &app.Runtime{
		IO:       app.IO{In: strings.NewReader(""), Out: &output, Err: io.Discard},
		Services: options.Services, ServicesManaged: true,
		// Explicitly fail closed if a native handler unexpectedly asks for local
		// config. No system/home resolver and no credential fallback are installed.
		LayoutResolver: config.NewResolver(config.Env{}, config.UserDirs{
			HomeDir:   func() (string, error) { return "", ErrHostedToolDenied },
			ConfigDir: func() (string, error) { return "", ErrHostedToolDenied },
			CacheDir:  func() (string, error) { return "", ErrHostedToolDenied },
		}),
	}
	auth := googleapi.AuthDependencies{Mode: googleapi.AuthModeStored}
	composeRuntimeGoogleServices(runtime, googleapi.NewFactory(auth, googleapi.FactoryOptions{}))
	ctx = app.WithRuntime(ctx, runtime)
	ctx = googleapi.WithAuthDependencies(ctx, auth)
	ctx = googleapi.WithNoInput(ctx)
	ctx = googleapi.WithReadOnly(ctx, spec.Risk == mcpRiskRead)
	ctx = authclient.WithAccessToken(ctx, options.AccessToken)
	ctx = authclient.WithQuotaProject(ctx, options.QuotaProject)
	ctx = googleapi.WithRequestGuard(ctx, options.RequestGuard)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	u, err := ui.New(ui.Options{Stdout: &output, Stderr: io.Discard, Color: "never"})
	if err != nil {
		return nil, ErrHostedToolExecution
	}
	ctx = ui.WithUI(ctx, u)
	command.BindTo(ctx, (*context.Context)(nil))
	command.Bind(&cli.RootFlags)
	if err := command.Run(); err != nil {
		if errors.Is(err, ErrHostedToolDenied) {
			return nil, ErrHostedToolDenied
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", ErrHostedToolExecution, ctx.Err())
		}
		// Provider text may contain account/credential data. Only stable errors
		// leave this bridge; the private runner supplies sanitized diagnostics.
		return nil, ErrHostedToolExecution
	}
	if output.truncated {
		return nil, ErrHostedToolOutputLimit
	}
	var result any
	if json.Unmarshal(output.buf.Bytes(), &result) == nil {
		return result, nil
	}
	return output.String(), nil
}
