// Package runner implements the private Cloud Run execution service for
// hosted v1 (#63). It exposes a fixed typed endpoint that authenticates a
// trusted control-plane capability snapshot and executes only the two
// explicitly allowed operations on top of the existing guarded Google
// engine packages.
package runner

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	OperationDiscover              = "discover"
	OperationAnalyticsPropertyRead = "analytics_property_read"

	ServiceAnalytics     = "analytics"
	ResourceTypeProperty = "property"
)

// establishedDiscoveryServices is the only service set discovery accepts.
// Operator-dependent services (for example Google Ads without a developer
// token) stay inside this set and surface honest unavailable statuses from
// the engine; they are never fabricated as successes.
var establishedDiscoveryServices = map[string]bool{
	"analytics":     true,
	"tagmanager":    true,
	"googleads":     true,
	"searchconsole": true,
	"bigquery":      true,
}

var canonicalPropertyID = regexp.MustCompile(`^properties/[0-9]+$`)

// ResourceGrantInput carries the trusted control-plane resource snapshot for
// analytics_property_read. It is deliberately a narrow whitelist of the
// controlplane.ResourceGrant fields the runner is allowed to act on.
type ResourceGrantInput struct {
	Service      string `json:"service"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Enabled      bool   `json:"enabled"`
}

// Request is the exact JSON body contract for POST /v1/execute. Unknown
// fields, trailing values, and oversized values are rejected before any
// Google engine work starts.
type Request struct {
	RequestID     string              `json:"request_id"`
	TenantID      string              `json:"tenant_id"`
	ConnectionID  string              `json:"connection_id"`
	Operation     string              `json:"operation"`
	GoogleEmail   string              `json:"google_email"`
	GoogleSubject string              `json:"google_subject,omitempty"`
	AccessToken   string              `json:"access_token"`
	Services      []string            `json:"services,omitempty"`
	Resource      *ResourceGrantInput `json:"resource,omitempty"`
}

type validator struct{}

var errInvalidRequest = errors.New("invalid request")

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errInvalidRequest, fmt.Sprintf(format, args...))
}

func safeIdentifier(value string, maxLength int, what string) error {
	if value == "" {
		return invalidf("missing %s", what)
	}

	if len(value) > maxLength {
		return invalidf("%s too long", what)
	}

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return invalidf("%s contains unsupported characters", what)
		}
	}

	return nil
}

func (v validator) validateRequest(req *Request) error {
	if req == nil {
		return invalidf("empty request")
	}

	if req.RequestID != "" {
		if len(req.RequestID) > 64 {
			return invalidf("request_id too long")
		}

		for _, r := range req.RequestID {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			default:
				return invalidf("request_id contains unsupported characters")
			}
		}
	}

	if err := safeIdentifier(req.TenantID, 128, "tenant_id"); err != nil {
		return err
	}

	if err := safeIdentifier(req.ConnectionID, 128, "connection_id"); err != nil {
		return err
	}

	if req.Operation != OperationDiscover && req.Operation != OperationAnalyticsPropertyRead {
		return invalidf("unsupported operation")
	}

	if err := validateGoogleEmail(req.GoogleEmail); err != nil {
		return err
	}

	if req.GoogleSubject != "" {
		if len(req.GoogleSubject) > 128 {
			return invalidf("google_subject too long")
		}

		for _, r := range req.GoogleSubject {
			if unicode.IsControl(r) || r == ' ' {
				return invalidf("google_subject contains unsupported characters")
			}
		}
	}

	if req.AccessToken == "" {
		return invalidf("missing access_token")
	}

	if len(req.AccessToken) > 8192 {
		return invalidf("access_token too long")
	}

	for _, r := range req.AccessToken {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return invalidf("access_token contains whitespace")
		}
	}

	switch req.Operation {
	case OperationDiscover:
		return v.validateDiscoveryServices(req.Services)
	case OperationAnalyticsPropertyRead:
		return validateReadGrant(req.Resource)
	}

	return invalidf("unsupported operation")
}

func (v validator) validateDiscoveryServices(raw []string) error {
	if len(raw) == 0 {
		return invalidf("discover requires services")
	}

	if len(raw) > 10 {
		return invalidf("too many services")
	}

	seen := make(map[string]bool, len(raw))
	for _, service := range raw {
		service = strings.ToLower(strings.TrimSpace(service))
		if service == "" || len(service) > 64 {
			return invalidf("invalid service name")
		}

		if !establishedDiscoveryServices[service] {
			return invalidf("unsupported discovery service")
		}

		if seen[service] {
			return invalidf("duplicate service")
		}
		seen[service] = true
	}

	return nil
}

func validateReadGrant(grant *ResourceGrantInput) error {
	if grant == nil {
		return invalidf("missing resource grant")
	}

	if grant.Service != ServiceAnalytics || grant.ResourceType != ResourceTypeProperty {
		return invalidf("unsupported resource type")
	}

	if !canonicalPropertyID.MatchString(grant.ResourceID) {
		return invalidf("resource_id is not a canonical GA4 property")
	}

	if !grant.Enabled {
		return invalidf("resource grant is disabled")
	}

	return nil
}

func validateGoogleEmail(email string) error {
	if email == "" {
		return invalidf("missing google_email")
	}

	if len(email) > 254 {
		return invalidf("google_email too long")
	}

	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return invalidf("invalid google_email")
	}

	for _, r := range email {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return invalidf("google_email contains unsupported characters")
		}
	}

	return nil
}
