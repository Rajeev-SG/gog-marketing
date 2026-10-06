package runner

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/controlplane"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/googleauth"
)

const testSecret = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type recordedDiscoverer struct {
	report           controlplane.DiscoveryReport
	err              error
	calls            atomic.Int64
	blockUntil       time.Duration
	lastAccessToken  string
	lastTenantID     string
	lastConnectionID string
}

func (d *recordedDiscoverer) DiscoverReport(
	ctx context.Context, connection controlplane.Connection, token controlplane.OAuthToken,
) (controlplane.DiscoveryReport, error) {
	d.calls.Add(1)
	d.lastAccessToken = authclient.AccessTokenFromContext(ctx)
	d.lastTenantID = connection.OrganizationID
	d.lastConnectionID = connection.ID

	if d.blockUntil > 0 {
		select {
		case <-ctx.Done():
			return controlplane.DiscoveryReport{}, fmt.Errorf("engine context ended: %w", ctx.Err())
		case <-time.After(d.blockUntil):
		}
	}

	return d.report, d.err
}

type capturedResponse struct {
	Status    int
	OK        bool
	RequestID string `json:"request_id"`
	Operation string `json:"operation"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Result json.RawMessage `json:"result"`
	Body   string
}

type testHarness struct {
	server     *httptest.Server
	discoverer *recordedDiscoverer
	engine     *Engine
}

func newHarness(t *testing.T, mutate func(*Engine, *recordedDiscoverer)) *testHarness {
	t.Helper()

	discoverer := &recordedDiscoverer{}

	engine := &Engine{
		Secret:           testSecret,
		Discoverer:       discoverer,
		Timeout:          5 * time.Second,
		MaxRequestBytes:  64 * 1024,
		MaxResponseBytes: 256 * 1024,
	}
	if mutate != nil {
		mutate(engine, discoverer)
	}

	server := httptest.NewServer((&Server{Engine: engine}).Handler())
	t.Cleanup(server.Close)

	return &testHarness{server: server, discoverer: discoverer, engine: engine}
}

func signInvocation(t *testing.T, body []byte, mutate func(map[string]any)) string {
	t.Helper()

	now := time.Now().Unix()

	claims := map[string]any{
		"iss":            "gog-marketing-worker",
		"sub":            "gog-marketing-worker",
		"aud":            "gog-marketing-runner",
		"iat":            now,
		"exp":            now + 30,
		"tenant_id":      "tenant-a",
		"connection_id":  "connection-a",
		"operation":      OperationDiscover,
		"request_sha256": bodyDigest(body),
	}
	if mutate != nil {
		mutate(claims)
	}

	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	headerSegment := base64.RawURLEncoding.EncodeToString(header)
	payloadSegment := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(headerSegment + "." + payloadSegment))

	return headerSegment + "." + payloadSegment + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func bodyDigest(body []byte) string {
	digest := sha256.Sum256(body)

	return hex.EncodeToString(digest[:])
}

func discoverBody(t *testing.T, mutate func(*Request)) []byte {
	t.Helper()

	req := Request{
		TenantID:     "tenant-a",
		ConnectionID: "connection-a",
		Operation:    OperationDiscover,
		GoogleEmail:  "owner@example.com",
		AccessToken:  "ya29.synthetic-access-token",
		Services:     []string{"analytics", "googleads"},
	}
	if mutate != nil {
		mutate(&req)
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	return body
}

func readBody(t *testing.T, mutate func(*Request)) []byte {
	t.Helper()

	req := Request{
		TenantID:     "tenant-a",
		ConnectionID: "connection-a",
		Operation:    OperationAnalyticsPropertyRead,
		GoogleEmail:  "owner@example.com",
		AccessToken:  "ya29.synthetic-access-token",
		Resource: &ResourceGrantInput{
			Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true,
		},
	}
	if mutate != nil {
		mutate(&req)
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	return body
}

func postExecute(t *testing.T, server *httptest.Server, token string, body []byte) capturedResponse {
	t.Helper()

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/v1/execute", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var parsed capturedResponse
	if jsonErr := json.Unmarshal(raw, &parsed); jsonErr != nil {
		t.Fatalf("parse response %q: %v", string(raw), jsonErr)
	}
	parsed.Status = response.StatusCode
	parsed.Body = string(raw)

	return parsed
}

func TestExecuteRejectsBeforeEngine(t *testing.T) {
	validBody := discoverBody(t, nil)

	cases := []struct {
		name       string
		body       func(*testing.T) []byte
		claims     func(map[string]any)
		wantStatus int
		wantCode   string
	}{
		{"anonymous", func(*testing.T) []byte { return validBody }, nil, 401, "anonymous"},
		{"bad signature", func(*testing.T) []byte { return validBody }, nil, 401, "bad_signature"},
		{"wrong issuer", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["iss"] = "other-worker" }, 401, "token_invalid"},
		{"wrong audience", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["aud"] = "other-runner" }, 401, "token_invalid"},
		{"unsupported algorithm", func(*testing.T) []byte { return validBody }, nil, 401, "token_invalid"},
		{"expired", func(*testing.T) []byte { return validBody }, func(claims map[string]any) {
			now := time.Now().Unix()
			claims["iat"], claims["exp"] = now-70, now-10
		}, 401, "token_expired"},
		{"not yet valid", func(*testing.T) []byte { return validBody }, func(claims map[string]any) {
			now := time.Now().Unix()
			claims["iat"], claims["exp"] = now+40, now+60
		}, 401, "token_not_yet_valid"},
		{"window too long", func(*testing.T) []byte { return validBody }, func(claims map[string]any) {
			now := time.Now().Unix()
			claims["iat"], claims["exp"] = now-90, now-20
		}, 401, "token_invalid"},
		{"wrong body digest", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["request_sha256"] = strings.Repeat("0", 64) }, 401, "request_digest_mismatch"},
		{"foreign tenant claim", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["tenant_id"] = "tenant-b" }, 403, "context_mismatch"},
		{"foreign connection claim", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["connection_id"] = "connection-b" }, 403, "context_mismatch"},
		{"tampered operation claim", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["operation"] = OperationAnalyticsPropertyRead }, 403, "context_mismatch"},
		{"tampered body context", func(t *testing.T) []byte {
			t.Helper()
			return discoverBody(t, func(req *Request) { req.TenantID = "tenant-b" })
		}, nil, 403, "context_mismatch"},
		{"unknown operation", func(*testing.T) []byte { return validBody }, func(claims map[string]any) { claims["operation"] = "gog_exec" }, 403, "context_mismatch"},
		{"unknown field", func(t *testing.T) []byte {
			t.Helper()
			return unknownFieldBody(t, validBody)
		}, nil, 400, "invalid_request"},
		{"trailing value", func(*testing.T) []byte { return append(bytes.Clone(validBody), []byte(" 42")...) }, nil, 400, "invalid_request"},
		{"malformed json", func(*testing.T) []byte { return []byte("{") }, nil, 400, "invalid_request"},
		{"unsupported discovery service", func(t *testing.T) []byte {
			t.Helper()
			return discoverBody(t, func(req *Request) { req.Services = []string{"sheets"} })
		}, nil, 400, "invalid_request"},
		{"disabled grant", func(t *testing.T) []byte {
			t.Helper()
			return readBody(t, func(req *Request) { req.Resource.Enabled = false })
		}, func(claims map[string]any) { claims["operation"] = OperationAnalyticsPropertyRead }, 400, "invalid_request"},
		{"non canonical property", func(t *testing.T) []byte {
			t.Helper()
			return readBody(t, func(req *Request) { req.Resource.ResourceID = "properties/abc" })
		}, func(claims map[string]any) { claims["operation"] = OperationAnalyticsPropertyRead }, 400, "invalid_request"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			harness := newHarness(t, nil)
			harness.engine.AnalyticsAdminFactory = func(context.Context, string) (*analyticsadmin.Service, error) {
				t.Error("engine must not be reached for rejection")
				return nil, errEngineReached
			}

			body := test.body(t)

			var token string

			switch test.name {
			case "anonymous":
			case "bad signature":
				token = tamperSignature(signInvocation(t, body, test.claims))
			case "unsupported algorithm":
				token = unsignedAlgorithmNone(t, body)
			default:
				token = signInvocation(t, body, test.claims)
			}

			response := postExecute(t, harness.server, token, body)
			if response.Status != test.wantStatus || response.Error.Code != test.wantCode {
				t.Fatalf("got %d %q want %d %q: %s", response.Status, response.Error.Code, test.wantStatus, test.wantCode, response.Body)
			}

			if harness.discoverer.calls.Load() != 0 {
				t.Fatal("engine was invoked for a rejected request")
			}

			if strings.Contains(response.Body, "ya29.synthetic-access-token") || strings.Contains(response.Body, "owner@example.com") {
				t.Fatalf("response leaked secrets: %s", response.Body)
			}
		})
	}
}

func unsignedAlgorithmNone(t *testing.T, body []byte) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": "none"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	now := time.Now().Unix()

	payload, err := json.Marshal(map[string]any{
		"iss": "gog-marketing-worker", "sub": "gog-marketing-worker", "aud": "gog-marketing-runner",
		"iat": now, "exp": now + 30, "tenant_id": "tenant-a", "connection_id": "connection-a",
		"operation": OperationDiscover, "request_sha256": bodyDigest(body),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func unknownFieldBody(t *testing.T, body []byte) []byte {
	t.Helper()

	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	generic["argv"] = []string{"rm", "-rf"}

	mutated, err := json.Marshal(generic)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return mutated
}

func tamperSignature(token string) string {
	segment := token[strings.LastIndex(token, ".")+1:]

	replacement := "A"
	if segment[0] == 'A' {
		replacement = "B"
	}

	return token[:strings.LastIndex(token, ".")+1] + replacement + segment[1:]
}

func TestExecuteDiscoveryDelegatesToControlPlaneEngine(t *testing.T) {
	harness := newHarness(t, func(_ *Engine, discoverer *recordedDiscoverer) {
		discoverer.report = controlplane.DiscoveryReport{
			Resources: []controlplane.ResourceGrant{
				{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", DisplayName: "Dev property"},
			},
			Statuses: map[string]controlplane.DiscoveryServiceStatus{
				"analytics": {State: controlplane.DiscoveryServiceOK, ResourceCount: 1, CheckedAt: time.Now().UTC()},
				"googleads": {State: controlplane.DiscoveryServiceUnavailable, Detail: "google_ads_unconfigured"},
			},
		}
	})

	body := discoverBody(t, nil)
	response := postExecute(t, harness.server, signInvocation(t, body, nil), body)

	if response.Status != 200 || !response.OK {
		t.Fatalf("unexpected response: %d %s", response.Status, response.Body)
	}

	var result struct {
		Resources []ResourceResult `json:"resources"`
		Statuses  map[string]struct {
			State string `json:"state"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(result.Resources) != 1 || result.Resources[0].Service != "analytics" || result.Resources[0].ResourceID != "properties/123" {
		t.Fatalf("wrong resources: %+v", result.Resources)
	}

	if result.Statuses["googleads"].State != "unavailable" {
		t.Fatalf("googleads status must stay honest: %+v", result.Statuses)
	}

	if harness.discoverer.lastAccessToken != "ya29.synthetic-access-token" {
		t.Fatal("ephemeral access token was not propagated through the guarded context")
	}

	if harness.discoverer.lastConnectionID != "connection-a" || harness.discoverer.lastTenantID != "tenant-a" {
		t.Fatalf("connection context wrong: %s %s", harness.discoverer.lastTenantID, harness.discoverer.lastConnectionID)
	}
}

// Real control-plane engine path: the operator-dependent Google Ads service
// short-circuits to an honest unavailable status with no fabricated results
// and no network calls.
func TestExecuteDiscoveryUsesRealEngineUnavailableStatus(t *testing.T) {
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.Discoverer = controlplane.EngineDiscoverer{}
	})

	body := discoverBody(t, func(req *Request) { req.Services = []string{"googleads"} })
	response := postExecute(t, harness.server, signInvocation(t, body, nil), body)

	if response.Status != 200 || !response.OK {
		t.Fatalf("unexpected response: %d %s", response.Status, response.Body)
	}

	if !strings.Contains(string(response.Result), `"google_ads_unconfigured"`) || strings.Contains(string(response.Result), "customers/") {
		t.Fatalf("honest unavailable status contract failed: %s", response.Result)
	}
}

func TestExecuteReadUsesTypedGuardedGoogleClient(t *testing.T) {
	var apiCalls atomic.Int64

	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)

		if r.Method != http.MethodGet || r.URL.Path != "/v1beta/properties/123" || r.Header.Get("Authorization") != "Bearer ya29.synthetic-access-token" {
			t.Errorf("wrong guarded request: %s %s %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"properties/123","displayName":"Dev property","timeZone":"Europe/London","currencyCode":"GBP","industryCategory":"SECRET"}`))
	}))
	defer google.Close()

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.AnalyticsAdminFactory = func(ctx context.Context, email string) (*analyticsadmin.Service, error) {
			if email != "owner@example.com" || !googleapi.ReadOnly(ctx) || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "ya29.synthetic-access-token" {
				t.Error("guarded stored-token read-only/no-input context not propagated")
			}

			client, clientErr := googleapi.NewHTTPClient(ctx, googleauth.ServiceAnalytics, email)
			if clientErr != nil {
				return nil, fmt.Errorf("mock GA4 transport: %w", clientErr)
			}

			return analyticsadmin.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(google.URL+"/"))
		}
	})

	body := readBody(t, nil)
	response := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationAnalyticsPropertyRead }), body)

	if response.Status != 200 || !response.OK {
		t.Fatalf("unexpected response: %d %s", response.Status, response.Body)
	}

	if strings.Contains(response.Body, "SECRET") || strings.Contains(response.Body, "industryCategory") {
		t.Fatalf("whitelisted result contract leaked upstream fields: %s", response.Body)
	}

	if !strings.Contains(response.Body, `"time_zone":"Europe/London"`) || apiCalls.Load() != 1 {
		t.Fatalf("wrong whitelisted property result: %s", response.Body)
	}
}

func TestExecuteTimesOutWithStructuredError(t *testing.T) {
	harness := newHarness(t, func(engine *Engine, discoverer *recordedDiscoverer) {
		engine.Timeout = 80 * time.Millisecond
		discoverer.blockUntil = time.Second
	})

	body := discoverBody(t, nil)
	response := postExecute(t, harness.server, signInvocation(t, body, nil), body)

	if response.Status != 504 || response.Error.Code != "timeout" {
		t.Fatalf("unexpected timeout response: %d %s", response.Status, response.Body)
	}

	if strings.Contains(strings.ToLower(response.Body), "context deadline exceeded") {
		t.Fatalf("raw provider exception leaked: %s", response.Body)
	}
}

func TestExecuteEnforcesOutputLimit(t *testing.T) {
	harness := newHarness(t, func(engine *Engine, discoverer *recordedDiscoverer) {
		engine.MaxResponseBytes = 1024
		discoverer.report = controlplane.DiscoveryReport{
			Resources: []controlplane.ResourceGrant{{
				Service: "analytics", ResourceType: "property", ResourceID: "properties/123",
				DisplayName: strings.Repeat("x", 4096),
			}},
		}
	})

	body := discoverBody(t, nil)
	response := postExecute(t, harness.server, signInvocation(t, body, nil), body)

	if response.Status != 422 || response.Error.Code != "output_limit" {
		t.Fatalf("unexpected output-limit response: %d %s", response.Status, response.Body)
	}

	if strings.Contains(response.Body, strings.Repeat("x", 512)) {
		t.Fatalf("oversized result leaked: %s", response.Body)
	}
}

func TestExecuteAuditAndLogsStaySecretFree(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.Logger = logger
	})

	body := discoverBody(t, nil)
	if success := postExecute(t, harness.server, signInvocation(t, body, nil), body); success.Status != 200 {
		t.Fatalf("success expected: %d %s", success.Status, success.Body)
	}

	if rejected := postExecute(t, harness.server, "", body); rejected.Status != 401 {
		t.Fatalf("anonymous rejection expected: %d", rejected.Status)
	}

	logs := logBuffer.String()
	for _, leak := range []string{"ya29.synthetic-access-token", "owner@example.com", "tenant-a", "connection-a", "Authorization"} {
		if strings.Contains(logs, leak) {
			t.Fatalf("audit log leaked %q: %s", leak, logs)
		}
	}

	for _, required := range []string{"request_id", "operation", "outcome", "duration_ms", "executions", "tenant_hash", "connection_hash"} {
		if !strings.Contains(logs, required) {
			t.Fatalf("audit log missing %q: %s", required, logs)
		}
	}
}

func TestCounterAndHealthTrackExecutions(t *testing.T) {
	counter := newCountingCounter()
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.Counter = counter
	})

	body := discoverBody(t, nil)
	_ = postExecute(t, harness.server, signInvocation(t, body, nil), body)
	_ = postExecute(t, harness.server, "", body)

	if counter.count(OperationDiscover, "ok") != 1 || counter.count("", "anonymous") != 1 {
		t.Fatalf("wrong counter buckets: %+v", counter)
	}

	healthRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, harness.server.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("health: %v", err)
	}

	health, err := harness.server.Client().Do(healthRequest)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer health.Body.Close()

	healthBody, _ := io.ReadAll(health.Body)

	if health.StatusCode != 200 || !strings.Contains(string(healthBody), `"executions":2`) {
		t.Fatalf("health probe failed: %d %s", health.StatusCode, healthBody)
	}
}

func TestWorkerBearerHeaderRequiredOverServerlessHeader(t *testing.T) {
	harness := newHarness(t, nil)

	body := discoverBody(t, nil)

	token := signInvocation(t, body, nil)

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, harness.server.URL+"/v1/execute", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	request.Header.Set("X-Serverless-Authorization", "Bearer "+token)

	response, err := harness.server.Client().Do(request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)

	if response.StatusCode != 401 || harness.discoverer.calls.Load() != 0 {
		t.Fatalf("X-Serverless-Authorization must not substitute capability auth: %d %s", response.StatusCode, raw)
	}
}

func TestRequestLimitRejection(t *testing.T) {
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.MaxRequestBytes = 1024
	})

	body := discoverBody(t, nil)
	response := postExecute(t, harness.server, signInvocation(t, body, nil), bytes.Repeat(body, 64))

	if response.Status != 400 || response.Error.Code != "invalid_request" || harness.discoverer.calls.Load() != 0 {
		t.Fatalf("oversized body must be rejected before the engine: %d %s", response.Status, response.Body)
	}
}

func TestServerRequiresConfiguredSecret(t *testing.T) {
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.Secret = "short"
	})

	body := discoverBody(t, nil)
	response := postExecute(t, harness.server, signInvocation(t, body, nil), body)

	if response.Status != 500 || response.Error.Code != "server_config" || harness.discoverer.calls.Load() != 0 {
		t.Fatalf("short secret must fail closed: %d %s", response.Status, response.Body)
	}
}
