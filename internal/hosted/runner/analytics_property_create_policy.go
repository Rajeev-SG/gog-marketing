package runner

import (
	"net/http"
	"strings"
)

// admitAnalyticsPropertyCreate pins GA4 property creation to the granted
// account. The Admin SDK carries the parent only in the request body (the
// HTTP template has no parent query parameter), so the common grant check is
// not enough: the body parent must be exactly the selected account, no other
// query parameter may widen the request, and the body cannot be replayed
// after inspection.
func admitAnalyticsPropertyCreate(invocation *ToolInvocation, request *http.Request) error {
	if invocation == nil {
		return invalidf("analytics property create requires a signed invocation")
	}

	if invocation.Authorization.Service != ServiceAnalytics || !invocation.Authorization.AllowWrite {
		return invalidf("analytics property create requires an analytics write grant")
	}

	resource := invocation.Authorization.Resource
	if resource == nil || resource.ResourceType != "account" || !resource.Enabled || resource.Service != ServiceAnalytics {
		return invalidf("analytics property create requires an account resource grant")
	}

	account := strings.TrimSpace(resource.ResourceID)
	if !analyticsAccountResourcePattern.MatchString(account) {
		return invalidf("invalid analytics account resource grant %q", resource.ResourceID)
	}

	argument, ok := invocation.Arguments["account"].(string)
	if !ok || argument != account {
		return invalidf("invalid selected account authority")
	}

	if request == nil || request.URL == nil {
		return invalidf("missing request")
	}

	if request.Method != http.MethodPost {
		return invalidf("analytics property create requires POST, got %s", request.Method)
	}

	if request.URL.Path != "/v1beta/properties" {
		return invalidf("unexpected analytics property create path %q", request.URL.Path)
	}

	if request.Header.Get("x-goog-user-project") != "" {
		return invalidf("billing project override is not permitted")
	}

	query := request.URL.Query()
	for key := range query {
		switch key {
		case "alt":
		case "prettyPrint":
			values := query[key]
			if len(values) != 1 || values[0] != "false" {
				return invalidf("invalid SDK formatting query")
			}
		default:
			return invalidf("unexpected analytics property create query parameter %q", key)
		}
	}

	body, err := readRequestJSONBody(request)
	if err != nil {
		return err
	}

	parent, _ := body["parent"].(string)
	if strings.TrimSpace(parent) != account {
		return invalidf("analytics property create parent does not match the granted account")
	}

	return nil
}
