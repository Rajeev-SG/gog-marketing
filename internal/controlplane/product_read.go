package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

var ErrReadUnavailable = errors.New("resource store temporarily unavailable")

// ReadAnalyticsProperty is the initial fixed-schema product tool allowlist.
// No account-level or other-service grant implies access to this property.
func (s *Service) ReadAnalyticsProperty(ctx context.Context, actor Actor, connectionID, resourceID string) (*analyticsadmin.GoogleAnalyticsAdminV1betaProperty, error) {
	const action = "agent.analytics.property.get"

	var requests atomic.Int64
	audit := func(result, detail string) {
		s.audit(ctx, actor, connectionID, action, result, fmt.Sprintf("%s; google_api_requests=%d", detail, requests.Load()))
	}

	deny := func(err error) (*analyticsadmin.GoogleAnalyticsAdminV1betaProperty, error) {
		audit("deny", "resource access denied")
		return nil, err
	}
	if actor.UserID == "" || actor.OrganizationID == "" {
		return deny(ErrForbidden)
	}

	number, err := strconv.ParseUint(strings.TrimPrefix(resourceID, "properties/"), 10, 64)
	if err != nil || number == 0 || resourceID != fmt.Sprintf("properties/%d", number) {
		return deny(ErrInvalid)
	}
	// Policy runs before secret retrieval, token refresh, or any Google request.
	if policyErr := (Policy{Store: s.Store}).Allow(ctx, actor, connectionID, "analytics", resourceID); policyErr != nil {
		if errors.Is(policyErr, ErrForbidden) || errors.Is(policyErr, ErrNotFound) {
			return deny(ErrForbidden)
		}

		audit("error", "resource store unavailable")

		return nil, ErrReadUnavailable
	}

	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil && !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrNotFound) {
		audit("error", "connection store unavailable")
		return nil, ErrReadUnavailable
	}

	if err != nil || !slices.Contains(connection.Services, "analytics") || (connection.Status != ConnectionHealthy && connection.Status != ConnectionExpired) {
		return deny(ErrForbidden)
	}

	connection, token, err := s.FreshToken(ctx, actor, connectionID)
	if err != nil {
		audit("error", string(AuthFailureCategoryFor(err)))
		return nil, err
	}

	if token.AccessToken == "" {
		audit("error", "missing access token")
		return nil, ErrForbidden
	}
	// The product always uses its selected central token, never ambient ADC or keyring auth.
	ctx = googleapi.WithAuthDependencies(ctx, googleapi.AuthDependencies{Mode: googleapi.AuthModeStored})
	ctx = authclient.WithAccessToken(ctx, token.AccessToken)
	ctx = googleapi.WithReadOnly(googleapi.WithNoInput(ctx), true)

	// Count only requests made by the guarded GA4 client, not OAuth refresh.
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { requests.Add(1) }})

	factory := s.AnalyticsAdminFactory
	if factory == nil {
		factory = googleapi.NewAnalyticsAdmin
	}

	engine, err := factory(ctx, connection.GoogleEmail)
	if err == nil {
		var property *analyticsadmin.GoogleAnalyticsAdminV1betaProperty

		property, err = engine.Properties.Get(resourceID).Context(ctx).Do()
		if err == nil {
			audit("ok", resourceID)
			return property, nil
		}
	}

	audit("error", string(AuthFailureCategoryFor(err)))

	return nil, wrapControlPlaneError(err)
}

// AnalyticsPropertyData is the fixed product response, not the upstream schema.
type AnalyticsPropertyData struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	TimeZone     string `json:"time_zone"`
	CurrencyCode string `json:"currency_code"`
}

func (h *ProductHandler) analyticsPropertyRequest(r *http.Request) (AnalyticsPropertyData, int, string) {
	session, ok := h.config.Sessions.FromProductRequest(r)
	if !ok {
		return AnalyticsPropertyData{}, http.StatusUnauthorized, "sign_in_required"
	}
	// Browsers cannot forge Fetch Metadata. Native agents must supply the
	// session's private CSRF header when Fetch Metadata is absent.
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "same-origin" && !(site == "" && constantTimeEqual(r.Header.Get("X-CSRF-Token"), session.CSRF)) {
		return AnalyticsPropertyData{}, http.StatusForbidden, "same_origin_required"
	}
	actor := Actor{UserID: session.UserID, OrganizationID: session.OrgID, Role: session.Role}

	property, err := h.config.Service.ReadAnalyticsProperty(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("resource"))
	if err != nil {
		status, code := http.StatusBadGateway, "read_failed"

		_, needsReconnect := productFailureMessage(err)
		switch {
		case errors.Is(err, ErrForbidden):
			status, code = http.StatusForbidden, "access_denied"
		case errors.Is(err, ErrReadUnavailable):
			status, code = http.StatusServiceUnavailable, "temporarily_unavailable"
		case errors.Is(err, ErrInvalid):
			status, code = http.StatusBadRequest, "invalid_resource"
		case needsReconnect:
			status, code = http.StatusConflict, "reconnect_required"
		}

		return AnalyticsPropertyData{}, status, code
	}

	if property == nil {
		return AnalyticsPropertyData{}, http.StatusBadGateway, "read_failed"
	}

	return AnalyticsPropertyData{Name: property.Name, DisplayName: property.DisplayName, TimeZone: property.TimeZone, CurrencyCode: property.CurrencyCode}, http.StatusOK, ""
}

func (h *ProductHandler) readAnalyticsProperty(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	property, status, code := h.analyticsPropertyRequest(r)
	w.WriteHeader(status)

	if code != "" {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Operation string                `json:"operation"`
		Property  AnalyticsPropertyData `json:"property"`
	}{Operation: "analytics.property.get", Property: property})
}

func (h *ProductHandler) readAnalyticsPropertyPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	property, status, code := h.analyticsPropertyRequest(r)
	w.WriteHeader(status)
	h.render(w, "property", map[string]any{"Property": property, "Error": code, "ConnectionID": r.PathValue("id")})
}
