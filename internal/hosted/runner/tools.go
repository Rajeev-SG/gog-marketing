package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/openclaw/gogcli/internal/hosted/native"
	"github.com/openclaw/gogcli/internal/hosted/policy"
)

const OperationToolCall = "tool_call"

const resourceTypeFile = "file"

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
	Name                   string `json:"name"`
	Service                string `json:"service"`
	ResourceType           string `json:"resource_type"`
	ResourceArgument       string `json:"resource_argument"`
	Write                  bool   `json:"write"`
	hosts                  []string
	paths                  func(string) []string
	scopes                 []string
	methods                []string
	escapedPaths           bool
	calendarEventOperation string
	destructive            bool
	argumentPaths          func(map[string]any) ([]string, error)
}

// NativeToolDescription exposes only existing executable schemas and their
// resource mapping. The Worker consumes a generated copy, not a second schema.
type NativeToolDescription struct {
	Name             string `json:"name"`
	Service          string `json:"service"`
	ResourceType     string `json:"resource_type"`
	ResourceArgument string `json:"resource_argument"`
	Write            bool   `json:"write"`
	Tool             any    `json:"tool"`
}

func NativeToolCatalog(tools []native.Tool) []NativeToolDescription {
	catalog := make([]NativeToolDescription, 0, len(nativeToolBindings))

	for _, tool := range tools {
		binding, ok := nativeBinding(tool.Tool.Name)
		if !ok {
			continue
		}
		// Hosted resource admission always requires an explicit selector,
		// even when the local tool offers an account-dependent default.
		if binding.ResourceArgument != "" && !slices.Contains(tool.Tool.InputSchema.Required, binding.ResourceArgument) {
			tool.Tool.InputSchema.Required = append(tool.Tool.InputSchema.Required, binding.ResourceArgument)
		}
		catalog = append(catalog, NativeToolDescription{
			Name: binding.Name, Service: binding.Service, ResourceType: binding.ResourceType,
			ResourceArgument: binding.ResourceArgument, Write: binding.Write, Tool: tool.Tool,
		})
	}

	return catalog
}

func exactPaths(prefix, suffix string) func(string) []string {
	return func(id string) []string { return []string{prefix + id + suffix} }
}

// Bind only tools for which request paths and resource admission are explicit.
// Native schemas/handlers are reused; this is policy, not new API integration.
var nativeToolBindings = append([]NativeToolBinding{
	{Name: "sites_get", Service: "sites", ResourceType: resourceTypeFile, ResourceArgument: "site_id", hosts: []string{"www.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"drive.file"}, paths: exactPaths("/drive/v3/files/", "")},
	{Name: "sites_url", Service: "sites", ResourceType: resourceTypeFile, ResourceArgument: "site_id", hosts: []string{"www.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"drive.file"}, paths: exactPaths("/drive/v3/files/", "")},
	{Name: "groups_members", Service: "groups", hosts: []string{"cloudidentity.googleapis.com"}, scopes: []string{"cloud-identity.groups.readonly"}, paths: exactPaths("", ""), argumentPaths: validateGroupEmail},
	{Name: "groups_list", Service: "groups", hosts: []string{"cloudidentity.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"cloud-identity.groups.readonly"}, paths: exactPaths("/v1/groups/-/memberships:searchTransitiveGroups", "")},
	{Name: "tagmanager_versions_publish", Service: "tagmanager", ResourceType: "container", ResourceArgument: "container_path", Write: true, hosts: []string{"tagmanager.googleapis.com"}, scopes: []string{"tagmanager.publish"}, methods: []string{http.MethodPost}, destructive: true, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
		container, _ := args["container_path"].(string)

		version, _ := args["version"].(string)
		if !canonicalContainerID.MatchString(container) || !numericVersionID.MatchString(version) {
			return nil, invalidf("invalid container or version selector")
		}

		return []string{"/tagmanager/v2/" + container + "/versions/" + version + ":publish"}, nil
	}},
	{Name: "tasks_lists", Service: "tasks", hosts: []string{"tasks.googleapis.com"}, paths: exactPaths("/tasks/v1/users/@me/lists", ""), scopes: []string{"tasks.readonly", "tasks"}},
	{Name: "tasks_lists_create", Service: "tasks", Write: true, hosts: []string{"tasks.googleapis.com"}, paths: exactPaths("/tasks/v1/users/@me/lists", ""), scopes: []string{"tasks"}},
	{Name: "slides_list_slides", Service: "slides", ResourceType: "file", ResourceArgument: "presentation_id", hosts: []string{"slides.googleapis.com"}, paths: exactPaths("/v1/presentations/", ""), scopes: []string{"presentations.readonly", "presentations", "drive.file"}},
	{Name: "slides_update_notes", Service: "slides", ResourceType: "file", ResourceArgument: "presentation_id", Write: true, hosts: []string{"slides.googleapis.com"}, paths: func(id string) []string {
		return []string{"/v1/presentations/" + id, "/v1/presentations/" + id + ":batchUpdate"}
	}, scopes: []string{"presentations", "drive.file"}},
	{Name: "analytics_properties_get", Service: "analytics", ResourceType: "property", ResourceArgument: "property", hosts: []string{"analyticsadmin.googleapis.com"}, paths: exactPaths("/v1beta/", ""), scopes: []string{"analytics.readonly", "analytics.edit"}},
	{Name: "analytics_report", Service: "analytics", ResourceType: "property", ResourceArgument: "property", hosts: []string{"analyticsdata.googleapis.com"}, paths: exactPaths("/v1beta/", ":runReport"), scopes: []string{"analytics.readonly"}},
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
}, append(append(nativeTasksItemBindings(), nativeAnalyticsAdminBindings()...), append(nativeSearchConsoleBindings(), append(nativeContactsBindings(), append(nativeCalendarEventBindings(), append(nativeDriveLabelsBindings(), append(nativeMeetSpaceBindings(), append(nativeAdSenseAccountBindings(), append(nativeFormsBindings(), append(nativeAppScriptProjectBindings(), nativeChatBindings()...)...)...)...)...)...)...)...)...)...)

var (
	fileResourceID       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
	canonicalContainerID = regexp.MustCompile(`^accounts/[0-9]+/containers/[0-9]+$`)
	numericVersionID     = regexp.MustCompile(`^[0-9]+$`)
	calendarResourceID   = regexp.MustCompile(`^[A-Za-z0-9._@#+-]{1,256}$`)
	crossFileFormula     = regexp.MustCompile(`(?i)\bIMPORTRANGE\s*\(`)
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

	if binding.destructive && !auth.AllowDestructive {
		return invalidf("destructive grant is disabled")
	}

	if err := validateAdminReplacement(invocation); err != nil {
		return err
	}

	if invocation.Name == "forms_publish" && invocation.Arguments["unpublish"] == true && !auth.AllowDestructive {
		return invalidf("unpublishing requires destructive permission")
	}

	if binding.argumentPaths != nil {
		if _, err := binding.argumentPaths(invocation.Arguments); err != nil {
			return err
		}
	}

	if invocation.Name == "slides_update_notes" && !auth.AllowDestructive {
		return invalidf("destructive grant is disabled")
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
	if binding.ResourceType == "" {
		if grant != nil {
			return invalidf("account tool cannot carry a resource grant")
		}

		return nil
	}

	if grant == nil || !grant.Enabled || grant.Service != binding.Service || grant.ResourceType != binding.ResourceType {
		return invalidf("resource grant is disabled")
	}

	argument, ok := invocation.Arguments[binding.ResourceArgument].(string)
	if !ok || argument != grant.ResourceID {
		return invalidf("resource does not match grant")
	}

	return validateNativeResourceArgument(binding.ResourceType, argument)
}

func validateNativeResourceArgument(resourceType, argument string) error {
	switch resourceType {
	case "site":
		if !validSearchSite(argument) {
			return invalidf("invalid site resource")
		}
	case "container":
		if !canonicalContainerID.MatchString(argument) {
			return invalidf("invalid container resource")
		}
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

func validateAdminReplacement(invocation *ToolInvocation) error {
	if invocation.Authorization.Service != "analytics" || !strings.HasSuffix(invocation.Name, "_update") {
		return nil
	}

	mask, _ := invocation.Arguments["update_mask"].(string)
	if strings.Contains(mask, "*") && !invocation.Authorization.AllowDestructive {
		return invalidf("full replacement requires destructive permission")
	}

	return nil
}

func nativeRequestGuard(invocation *ToolInvocation) func(*http.Request) error {
	binding, _ := nativeBinding(invocation.Name)

	resourceID := ""
	if invocation.Authorization.Resource != nil {
		resourceID = invocation.Authorization.Resource.ResourceID
	}

	paths := binding.paths(resourceID)
	if binding.argumentPaths != nil {
		var err error

		paths, err = binding.argumentPaths(invocation.Arguments)
		if err != nil {
			return func(*http.Request) error { return native.ErrDenied }
		}
	}

	return func(request *http.Request) error {
		if request == nil || request.URL == nil || request.URL.Scheme != "https" || request.URL.User != nil || request.URL.Port() != "" || !slices.Contains(binding.hosts, request.URL.Hostname()) {
			return native.ErrDenied
		}

		if request.Header.Get("X-HTTP-Method-Override") != "" || request.Header.Get("X-Goog-User-Project") != "" || request.URL.RawQuery != "" && request.URL.Query().Has("access_token") {
			return native.ErrDenied
		}

		path := request.URL.Path
		if binding.escapedPaths {
			path = request.URL.EscapedPath()
		}

		if strings.Contains(path, "/../") || strings.Contains(path, "/./") || strings.Contains(path, "\\") {
			return native.ErrDenied
		}

		if invocation.Name == "groups_members" {
			return admitGroupMembershipRequest(invocation, request)
		}

		if binding.calendarEventOperation != "" {
			return admitCalendarEventRequest(invocation, request)
		}

		allowed := slices.Contains(paths, path)
		if binding.Service == "sheets" {
			// The ID is exact and the remaining suffix is an A1 range, not an
			// additional spreadsheet. Other Sheets endpoints remain unavailable.
			allowed = strings.HasPrefix(path, paths[0]) && len(path) > len(paths[0])
		}

		if !allowed {
			return native.ErrDenied
		}

		if len(binding.methods) != 0 && !slices.Contains(binding.methods, request.Method) {
			return native.ErrDenied
		}

		if err := admitNamedActionMethod(invocation.Name, path, request.Method); err != nil {
			return err
		}

		if !binding.Write && len(binding.methods) == 0 && request.Method != http.MethodGet && request.Method != http.MethodHead {
			if invocation.Name != "analytics_report" || request.Method != http.MethodPost {
				return native.ErrDenied
			}
		}

		if binding.Write {
			switch invocation.Name {
			case "contacts_update":
				updatePath := strings.HasSuffix(path, ":updateContact")
				if (request.Method == http.MethodGet && updatePath) || (request.Method == http.MethodPatch && !updatePath) {
					return native.ErrDenied
				}
			case "tasks_lists_create":
				if request.Method != http.MethodPost {
					return native.ErrDenied
				}
			case "docs_write", "slides_update_notes":
				if request.Method != http.MethodGet && !(request.Method == http.MethodPost && strings.HasSuffix(path, ":batchUpdate")) {
					return native.ErrDenied
				}
			case "sheets_update_range":
				if request.Method != http.MethodPut {
					return native.ErrDenied
				}
			}
		}

		return nil
	}
}

func admitNamedActionMethod(name, path, method string) error {
	var suffix string

	switch name {
	case "meet_space_end":
		suffix = ":endActiveConference"
	case "forms_publish":
		suffix = ":setPublishSettings"
	default:
		return nil
	}

	action := strings.HasSuffix(path, suffix)
	if (action && method != http.MethodPost) || (!action && method != http.MethodGet) {
		return native.ErrDenied
	}

	return nil
}

func (s *Server) executeNativeTool(ctx context.Context, req *Request) outcome {
	if s.Engine.ExecuteNative == nil {
		return engineOutcome("native tool execution unavailable")
	}

	result, err := s.Engine.ExecuteNative(ctx, req.Tool.Name, req.Tool.Arguments, native.Options{
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
		case errors.Is(err, native.ErrDenied):
			return outcome{status: 403, err: &apiError{Code: "permission_denied", Message: "tool request is not permitted"}}
		case errors.Is(err, native.ErrInput):
			return invalidOutcome("invalid typed tool input")
		case errors.Is(err, native.ErrOutputLimit):
			return outcome{status: 422, err: &apiError{Code: "output_limit", Message: "result exceeded output limit"}}
		default:
			return engineOutcome("typed tool execution failed")
		}
	}

	if req.Tool.Name == "tagmanager_versions_publish" {
		if envelope, ok := result.(map[string]any); ok {
			if published, ok := envelope["publish"].(map[string]any); ok && published["compilerError"] == true {
				return engineOutcome("GTM version compilation failed")
			}
		}
	}

	return outcome{result: result}
}
