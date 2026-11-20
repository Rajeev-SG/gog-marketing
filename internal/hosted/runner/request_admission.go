package runner

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// admitInvocationSpecificRequest runs the extra body/query admissions that
// nativeRequestGuard applies after its common host, scope, method and grant
// checks. Keeping the dispatch here keeps the shared request guard under its
// complexity limit without weakening any admission.
func admitInvocationSpecificRequest(invocation *ToolInvocation, request *http.Request) error {
	switch invocation.Name {
	case "analytics_properties_list":
		return admitAnalyticsPropertyInventory(invocation, request)
	case "analytics_properties_create":
		return admitAnalyticsPropertyCreate(invocation, request)
	case "searchconsole_verification_token":
		return admitSiteVerificationRequest(invocation, request, false)
	case "searchconsole_verification_verify":
		return admitSiteVerificationRequest(invocation, request, true)
	default:
		return nil
	}
}

// readRequestJSONBody decodes a bounded request body and restores it so the
// Google client can send the exact bytes that were inspected.
func readRequestJSONBody(request *http.Request) (map[string]any, error) {
	if request == nil || request.Body == nil {
		return nil, invalidf("missing request body")
	}

	payload, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	_ = request.Body.Close()

	request.Body = io.NopCloser(bytes.NewReader(payload))

	if err != nil {
		return nil, invalidf("unreadable request body")
	}

	var body map[string]any
	if json.Unmarshal(payload, &body) != nil || body == nil {
		return nil, invalidf("invalid request body")
	}

	return body, nil
}

// admitSiteVerificationRequest pins the Site Verification wire body to the
// fixed DNS TXT flow for the selected domain. The least-privilege verify_only
// grant must never be able to record or delegate another owner, so an `owners`
// field is rejected rather than forwarded.
func admitSiteVerificationRequest(invocation *ToolInvocation, request *http.Request, verify bool) error {
	domain, _ := invocation.Arguments["domain"].(string)
	if !validVerificationDomain(domain) {
		return invalidf("invalid verification domain")
	}

	body, err := readRequestJSONBody(request)
	if err != nil {
		return err
	}

	if _, present := body["owners"]; present {
		return invalidf("owner delegation is not permitted")
	}

	site, _ := body["site"].(map[string]any)
	if site["type"] != "INET_DOMAIN" || site["identifier"] != domain {
		return invalidf("verification body does not match the selected domain")
	}

	if verify {
		if len(body) != 1 {
			return invalidf("unexpected verification body fields")
		}

		methods := request.URL.Query()["verificationMethod"]
		if len(methods) != 1 || methods[0] != "DNS_TXT" {
			return invalidf("verification requires DNS_TXT only")
		}

		return nil
	}

	if body["verificationMethod"] != "DNS_TXT" || len(body) != 2 {
		return invalidf("verification token requires DNS_TXT only")
	}

	return nil
}
