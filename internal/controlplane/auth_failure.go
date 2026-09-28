package controlplane

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

type AuthFailureCategory string

const (
	AuthFailureInvalidGrant   AuthFailureCategory = "google_invalid_grant"
	AuthFailureSessionControl AuthFailureCategory = "google_session_control"
	AuthFailureScopeMismatch  AuthFailureCategory = "google_scope_mismatch"
	AuthFailureOAuthClient    AuthFailureCategory = "oauth_client_unavailable"
	AuthFailurePermission     AuthFailureCategory = "google_permission_denied"
	AuthFailureTransient      AuthFailureCategory = "transient"
	AuthFailureUnknown        AuthFailureCategory = "unknown"
)

type AuthFailure struct {
	Category  AuthFailureCategory
	Operation string
	Err       error
}

func (e *AuthFailure) Error() string {
	if e == nil {
		return ""
	}

	if e.Operation == "" {
		return string(e.Category) + ": " + e.Err.Error()
	}

	return e.Operation + ": " + string(e.Category) + ": " + e.Err.Error()
}

func (e *AuthFailure) Unwrap() error { return e.Err }

func classifyAuthError(err error) AuthFailureCategory {
	if err == nil {
		return ""
	}

	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		code := strings.ToLower(strings.TrimSpace(retrieve.ErrorCode))

		subtype := strings.ToLower(strings.TrimSpace(retrieve.ErrorDescription))
		switch {
		case strings.Contains(subtype, "invalid_rapt"):
			return AuthFailureSessionControl
		case code == "invalid_grant":
			return AuthFailureInvalidGrant
		case code == "invalid_client" || code == "deleted_client":
			return AuthFailureOAuthClient
		case code == "access_denied" || code == "insufficient_scope":
			return AuthFailurePermission
		}
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Code == 429 || apiErr.Code >= 500:
			return AuthFailureTransient
		case apiErr.Code == 401 || apiErr.Code == 403:
			return AuthFailurePermission
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return AuthFailureTransient
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return AuthFailureTransient
	}

	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "invalid_rapt"):
		return AuthFailureSessionControl
	case strings.Contains(lower, "invalid_grant"):
		return AuthFailureInvalidGrant
	case strings.Contains(lower, "invalid_client"), strings.Contains(lower, "deleted_client"):
		return AuthFailureOAuthClient
	case strings.Contains(lower, "insufficient_scope"), strings.Contains(lower, "access_denied"):
		return AuthFailurePermission
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "temporarily"), strings.Contains(lower, "connection reset"):
		return AuthFailureTransient
	default:
		return AuthFailureUnknown
	}
}

func wrapAuthFailure(operation string, err error) error {
	if err == nil {
		return nil
	}

	return &AuthFailure{Category: classifyAuthError(err), Operation: operation, Err: err}
}

func AuthFailureCategoryFor(err error) AuthFailureCategory { return authFailureCategory(err) }

func authFailureCategory(err error) AuthFailureCategory {
	var failure *AuthFailure
	if errors.As(err, &failure) {
		return failure.Category
	}

	return classifyAuthError(err)
}
