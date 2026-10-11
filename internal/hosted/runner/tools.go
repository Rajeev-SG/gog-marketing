package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/openclaw/gogcli/internal/cmd"
	"github.com/openclaw/gogcli/internal/hosted/policy"
)

const OperationToolCall = "tool_call"

// ToolInvocation carries an exact-body-signed authorization snapshot. No
// authority is read from model arguments, local config or another account.
type ToolInvocation struct {
	Name          string            `json:"name"`
	Arguments     map[string]any    `json:"arguments"`
	Authorization ToolAuthorization `json:"authorization"`
}

type ToolAuthorization struct {
	Service          string              `json:"service"`
	AllowedTools     []string            `json:"allowed_tools"`
	Scopes           []string            `json:"scopes"`
	Resource         *ResourceGrantInput `json:"resource"`
	AllowWrite       bool                `json:"allow_write"`
	AllowDestructive bool                `json:"allow_destructive"`
}

type NativeToolBinding struct {
	Name             string `json:"name"`
	Service          string `json:"service"`
	ResourceType     string `json:"resource_type"`
	ResourceArgument string `json:"resource_argument"`
	Write            bool   `json:"write"`
	hosts            []string
	paths            func(string) []string
	scopes           []string
}

func exactPaths(prefix, suffix string) func(string) []string {
	return func(id string) []string { return []string{prefix + id + suffix} }
}

// Bind only tools for which request paths and resource admission are explicit.
// Native schemas/handlers are reused; this is policy, not new API integration.
var nativeToolBindings = []NativeToolBinding{
	{Name: "analytics_properties_get", Service: "analytics", ResourceType: "property", ResourceArgument: "property", hosts: []string{"analyticsadmin.googleapis.com"}, paths: exactPaths("/v1beta/", ""), scopes: []string{"analytics.readonly", "analytics.edit"}},
	{Name: "analytics_report", Service: "analytics", ResourceType: "property", ResourceArgument: "property", hosts: []string{"analyticsdata.googleapis.com"}, paths: exactPaths("/v1beta/", ":runReport"), scopes: []string{"analytics.readonly"}},
	{Name: "analytics_datastreams_list", Service: "analytics", ResourceType: "property", ResourceArgument: "property", hosts: []string{"analyticsadmin.googleapis.com"}, paths: exactPaths("/v1beta/", "/dataStreams"), scopes: []string{"analytics.readonly", "analytics.edit"}},
	{Name: "drive_get", Service: "drive", ResourceType: "file", ResourceArgument: "file_id", hosts: []string{"www.googleapis.com"}, paths: exactPaths("/drive/v3/files/", ""), scopes: []string{"drive.file"}},
	{Name: "docs_get", Service: "docs", ResourceType: "file", ResourceArgument: "document_id", hosts: []string{"docs.googleapis.com"}, paths: exactPaths("/v1/documents/", ""), scopes: []string{"documents.readonly", "documents", "drive.file"}},
	{Name: "docs_write", Service: "docs", ResourceType: "file", ResourceArgument: "document_id", Write: true, hosts: []string{"docs.googleapis.com"}, paths: func(id string) []string {
		return []string{"/v1/documents/" + id, "/v1/documents/" + id + ":batchUpdate"}
	}, scopes: []string{"documents", "drive.file"}},
	{Name: "sheets_read_range", Service: "sheets", ResourceType: "file", ResourceArgument: "spreadsheet_id", hosts: []string{"sheets.googleapis.com"}, paths: exactPaths("/v4/spreadsheets/", "/values/"), scopes: []string{"spreadsheets.readonly", "spreadsheets", "drive.file"}},
	{Name: "sheets_update_range", Service: "sheets", ResourceType: "file", ResourceArgument: "spreadsheet_id", Write: true, hosts: []string{"sheets.googleapis.com"}, paths: exactPaths("/v4/spreadsheets/", "/values/"), scopes: []string{"spreadsheets", "drive.file"}},
	{Name: "calendar_events", Service: "calendar", ResourceType: "calendar", ResourceArgument: "calendar_id", hosts: []string{"www.googleapis.com"}, paths: func(id string) []string {
		return []string{"/calendar/v3/calendars/" + id, "/calendar/v3/calendars/" + id + "/events", "/calendar/v3/users/me/settings/timezone"}
	}, scopes: []string{"calendar.readonly", "calendar"}},
}

var (
	fileResourceID     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
	calendarResourceID = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,256}$`)
	crossFileFormula   = regexp.MustCompile(`(?i)\bIMPORTRANGE\s*\(`)
)

func nativeBinding(name string) (NativeToolBinding, bool) {
	for _, binding := range nativeToolBindings {
		if binding.Name == name {
			return binding, true
		}
	}

	return NativeToolBinding{}, false
}

func validateToolInvocation(invocation *ToolInvocation) error {
	if invocation == nil {
		return invalidf("missing tool invocation")
	}

	binding, ok := nativeBinding(invocation.Name)
	if !ok {
		return invalidf("unsupported typed tool")
	}

	auth := invocation.Authorization
	if _, known := policy.LookupService(auth.Service); !known || binding.Service != auth.Service {
		return invalidf("unsupported tool service")
	}

	if len(auth.AllowedTools) > 128 || !slices.Contains(auth.AllowedTools, invocation.Name) {
		return invalidf("tool grant is disabled")
	}

	if len(auth.Scopes) > 128 || policy.ValidateScopes(auth.Scopes) != nil {
		return invalidf("scope is blocked")
	}

	matchedScope := false
	for _, scope := range binding.scopes {
		matchedScope = matchedScope || slices.Contains(auth.Scopes, "https://www.googleapis.com/auth/"+scope)
	}

	if !matchedScope {
		return invalidf("required scope is missing")
	}

	if binding.Write && !auth.AllowWrite {
		return invalidf("write grant is disabled")
	}

	if invocation.Name == "docs_write" && invocation.Arguments["replace"] == true && !auth.AllowDestructive {
		return invalidf("destructive grant is disabled")
	}

	if invocation.Name == "sheets_update_range" && invocation.Arguments["input"] != "RAW" {
		// USER_ENTERED formulas must not import data from an unselected file.
		// The current snapshot authorizes one file; secondary-file admission is
		// not implied by write permission on the destination spreadsheet.
		values, _ := invocation.Arguments["values_json"].(string)

		var rows [][]any
		if json.Unmarshal([]byte(values), &rows) == nil {
			for _, row := range rows {
				for _, value := range row {
					text, isText := value.(string)
					if isText && strings.HasPrefix(strings.TrimSpace(text), "=") && crossFileFormula.MatchString(text) {
						return invalidf("secondary file grant is required")
					}
				}
			}
		}
	}

	grant := auth.Resource
	if grant == nil || !grant.Enabled || grant.Service != binding.Service || grant.ResourceType != binding.ResourceType {
		return invalidf("resource grant is disabled")
	}

	argument, ok := invocation.Arguments[binding.ResourceArgument].(string)
	if !ok || argument != grant.ResourceID {
		return invalidf("resource does not match grant")
	}

	switch binding.ResourceType {
	case "file":
		if !fileResourceID.MatchString(argument) {
			return invalidf("invalid file resource")
		}
	case "property":
		if !canonicalPropertyID.MatchString(argument) {
			return invalidf("invalid property resource")
		}
	case "calendar":
		if !calendarResourceID.MatchString(argument) {
			return invalidf("invalid calendar resource")
		}
	default:
		return invalidf("unsupported resource model")
	}

	return nil
}

func nativeRequestGuard(invocation *ToolInvocation) func(*http.Request) error {
	binding, _ := nativeBinding(invocation.Name)
	paths := binding.paths(invocation.Authorization.Resource.ResourceID)

	return func(request *http.Request) error {
		if request == nil || request.URL == nil || request.URL.Scheme != "https" || request.URL.User != nil || request.URL.Port() != "" || !slices.Contains(binding.hosts, request.URL.Hostname()) {
			return cmd.ErrHostedToolDenied
		}

		if request.Header.Get("X-HTTP-Method-Override") != "" || request.Header.Get("X-Goog-User-Project") != "" || request.URL.RawQuery != "" && request.URL.Query().Has("access_token") {
			return cmd.ErrHostedToolDenied
		}

		path := request.URL.Path
		if strings.Contains(path, "/../") || strings.Contains(path, "/./") || strings.Contains(path, "\\") {
			return cmd.ErrHostedToolDenied
		}

		allowed := slices.Contains(paths, path)
		if binding.Service == "sheets" {
			// The ID is exact and the remaining suffix is an A1 range, not an
			// additional spreadsheet. Other Sheets endpoints remain unavailable.
			allowed = strings.HasPrefix(path, paths[0]) && len(path) > len(paths[0])
		}

		if !allowed {
			return cmd.ErrHostedToolDenied
		}

		if !binding.Write && request.Method != http.MethodGet && request.Method != http.MethodHead {
			if invocation.Name != "analytics_report" || request.Method != http.MethodPost {
				return cmd.ErrHostedToolDenied
			}
		}

		if binding.Write {
			switch invocation.Name {
			case "docs_write":
				if request.Method != http.MethodGet && !(request.Method == http.MethodPost && strings.HasSuffix(path, ":batchUpdate")) {
					return cmd.ErrHostedToolDenied
				}
			case "sheets_update_range":
				if request.Method != http.MethodPut {
					return cmd.ErrHostedToolDenied
				}
			}
		}

		return nil
	}
}

func (s *Server) executeNativeTool(ctx context.Context, req *Request) outcome {
	result, err := cmd.ExecuteHostedExistingTool(ctx, req.Tool.Name, req.Tool.Arguments, cmd.HostedToolOptions{
		Account: req.GoogleEmail, AccessToken: req.AccessToken,
		AllowWrite:     req.Tool.Authorization.AllowWrite,
		Force:          req.Tool.Authorization.AllowDestructive,
		MaxOutputBytes: s.Engine.MaxResponseBytes, Services: s.Engine.NativeServices,
		RequestGuard: nativeRequestGuard(req.Tool),
	})
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return timeoutOutcome()
		case errors.Is(err, cmd.ErrHostedToolDenied):
			return outcome{status: 403, err: &apiError{Code: "permission_denied", Message: "tool request is not permitted"}}
		case errors.Is(err, cmd.ErrHostedToolInput):
			return invalidOutcome("invalid typed tool input")
		case errors.Is(err, cmd.ErrHostedToolOutputLimit):
			return outcome{status: 422, err: &apiError{Code: "output_limit", Message: "result exceeded output limit"}}
		default:
			return engineOutcome("typed tool execution failed")
		}
	}

	return outcome{result: result}
}
