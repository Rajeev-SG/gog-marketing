package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/controlplane"
	"github.com/openclaw/gogcli/internal/googleapi"
)

// ResourceResult is the canonical lower-case resource wire contract.
type ResourceResult struct {
	Service      string `json:"service"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	DisplayName  string `json:"display_name,omitempty"`
	Parent       string `json:"parent,omitempty"`
	Enabled      bool   `json:"enabled"`
}

// DiscoveryResult preserves the control-plane reporting contract, including
// honest unavailable/unsupported statuses for operator-dependent services.
type DiscoveryResult struct {
	Resources []ResourceResult                               `json:"resources"`
	Statuses  map[string]controlplane.DiscoveryServiceStatus `json:"statuses"`
}

// response is the single structured wire response for successes and errors.
// Error messages are fixed safe strings; provider exceptions never reach the
// response or logs.
type response struct {
	RequestID  string    `json:"request_id,omitempty"`
	Operation  string    `json:"operation,omitempty"`
	OK         bool      `json:"ok"`
	Result     any       `json:"result,omitempty"`
	Error      *apiError `json:"error,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type outcome struct {
	status int
	err    *apiError
	result any
}

func invalidOutcome(message string) outcome {
	return outcome{status: 400, err: &apiError{Code: "invalid_request", Message: message}}
}

func engineOutcome(message string) outcome {
	return outcome{status: 502, err: &apiError{Code: "engine_error", Message: message}}
}

func timeoutOutcome() outcome {
	return outcome{status: 504, err: &apiError{Code: "timeout", Message: "operation timed out"}}
}

// defaultAnalyticsAdminFactory is the real guarded googleapi client used in
// production. Tests inject a transport-backed factory of the same signature.
func defaultAnalyticsAdminFactory(ctx context.Context, email string) (*analyticsadmin.Service, error) {
	service, err := googleapi.NewAnalyticsAdmin(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("create analytics admin: %w", err)
	}

	return service, nil
}

// execute runs one fully validated request against the guarded engine inside
// the finite operation deadline. Engine calls use stored access-token mode,
// read-only, and no-input contracts only; there is no ADC or keyring path.
func (s *Server) execute(parent context.Context, req *Request) outcome {
	engineCtx, cancel := context.WithTimeout(parent, s.Engine.Timeout)
	defer cancel()

	engineCtx = googleapi.WithAuthDependencies(engineCtx, googleapi.AuthDependencies{Mode: googleapi.AuthModeStored})
	engineCtx = googleapi.WithReadOnly(googleapi.WithNoInput(engineCtx), true)
	engineCtx = authclient.WithAccessToken(engineCtx, req.AccessToken)

	var result outcome

	switch req.Operation {
	case OperationDiscover:
		result = s.executeDiscovery(engineCtx, req)
	case OperationAnalyticsPropertyRead:
		result = s.executeRead(engineCtx, req)
	default:
		result = invalidOutcome("unsupported operation")
	}

	if result.err == nil {
		payload, limitErr := boundedJSON(result.result, s.Engine.MaxResponseBytes)
		if limitErr != nil {
			return outcome{status: 422, err: &apiError{Code: "output_limit", Message: "result exceeded output limit"}}
		}
		result.result = json.RawMessage(payload)
	}

	return result
}

func (s *Server) executeDiscovery(ctx context.Context, req *Request) outcome {
	connection := controlplane.Connection{
		ID:             req.ConnectionID,
		OrganizationID: req.TenantID,
		GoogleEmail:    req.GoogleEmail,
		GoogleSubject:  req.GoogleSubject,
		Services:       normalizeServices(req.Services),
	}
	token := controlplane.OAuthToken{
		AccessToken: req.AccessToken,
		Email:       req.GoogleEmail,
		Subject:     req.GoogleSubject,
	}

	report, err := s.Engine.Discoverer.DiscoverReport(ctx, connection, token)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			return timeoutOutcome()
		}

		return engineOutcome("discovery engine failed")
	}

	resources := make([]ResourceResult, 0, len(report.Resources))
	for _, grant := range report.Resources {
		resources = append(resources, ResourceResult{
			Service:      strings.ToLower(grant.Service),
			ResourceType: strings.ToLower(grant.ResourceType),
			ResourceID:   grant.ResourceID,
			DisplayName:  grant.DisplayName,
			Parent:       grant.Parent,
			Enabled:      grant.Enabled,
		})
	}

	return outcome{result: DiscoveryResult{Resources: resources, Statuses: report.Statuses}}
}

func (s *Server) executeRead(ctx context.Context, req *Request) outcome {
	factory := s.Engine.AnalyticsAdminFactory
	if factory == nil {
		factory = defaultAnalyticsAdminFactory
	}

	service, err := factory(ctx, req.GoogleEmail)
	if err != nil {
		if ctx.Err() != nil {
			return timeoutOutcome()
		}

		return engineOutcome("analytics engine failed")
	}

	property, err := service.Properties.Get(req.Resource.ResourceID).Context(ctx).Do()
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			return timeoutOutcome()
		}

		return engineOutcome("analytics property read failed")
	}

	return outcome{result: controlplane.AnalyticsPropertyData{
		Name:         property.Name,
		DisplayName:  property.DisplayName,
		TimeZone:     property.TimeZone,
		CurrencyCode: property.CurrencyCode,
	}}
}

func normalizeServices(services []string) []string {
	out := make([]string, 0, len(services))
	for _, service := range services {
		out = append(out, strings.ToLower(strings.TrimSpace(service)))
	}

	return out
}

func boundedJSON(value any, limit int) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	if limit > 0 && len(data) > limit {
		return nil, errOutputLimit
	}

	return data, nil
}

func newRequestID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "req-unavailable"
	}

	return "req-" + hex.EncodeToString(raw)
}
