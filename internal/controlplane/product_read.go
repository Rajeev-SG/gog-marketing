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

// ReadAnalyticsProperty is the initial fixed-schema product tool allowlist.
// No account-level or other-service grant implies access to this property.
func (s *Service) ReadAnalyticsProperty(ctx context.Context, actor Actor, connectionID, resourceID string) (*analyticsadmin.GoogleAnalyticsAdminV1betaProperty, error) {
	const action = "agent.analytics.property.get"

	var requests atomic.Int64
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { requests.Add(1) }})
	audit := func(result, detail string) {
		s.audit(ctx, actor, connectionID, action, result, fmt.Sprintf("%s; google_requests=%d", detail, requests.Load()))
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
		return deny(ErrForbidden)
	}

	connection, err := s.Store.GetConnection(ctx, actor.OrganizationID, connectionID)
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

	engine, err := googleapi.NewAnalyticsAdmin(ctx, connection.GoogleEmail)
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

func (h *ProductHandler) readAnalyticsProperty(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	session, ok := h.config.Sessions.FromRequest(r)
	if !ok || session.Admin {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "sign_in_required"})

		return
	}
	actor := Actor{UserID: session.UserID, OrganizationID: session.OrgID, Role: session.Role}

	property, err := h.config.Service.ReadAnalyticsProperty(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("resource"))
	if err != nil {
		status, code := http.StatusBadGateway, "read_failed"

		_, needsReconnect := productFailureMessage(err)
		switch {
		case errors.Is(err, ErrForbidden):
			status, code = http.StatusForbidden, "access_denied"
		case errors.Is(err, ErrInvalid):
			status, code = http.StatusBadRequest, "invalid_resource"
		case needsReconnect:
			status, code = http.StatusConflict, "reconnect_required"
		}

		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})

		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"operation": "analytics.property.get", "property": property})
}
