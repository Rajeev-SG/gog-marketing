package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/openclaw/gogcli/internal/controlplane"
)

// Engine holds the guarded execution configuration and injected seams. It is
// the single wiring point; there is no CLI spawn, shell, argv, or generic
// HTTP escape hatch.
type Engine struct {
	// Secret is the dedicated >=32-byte invocation-token signing material.
	// It is not a Google credential and grants nothing beyond this service.
	Secret string

	// Discoverer is the existing control-plane reporting engine. The default
	// composition is controlplane.EngineDiscoverer{}.
	Discoverer controlplane.ReportingDiscoverer

	// AnalyticsAdminFactory defaults to googleapi.NewAnalyticsAdmin.
	AnalyticsAdminFactory func(context.Context, string) (*analyticsadmin.Service, error)

	Counter          Counter
	Timeout          time.Duration
	MaxRequestBytes  int64
	MaxResponseBytes int
	Logger           *slog.Logger
}

// Server is the fixed-endpoint execution service.
type Server struct {
	Engine *Engine

	counterOnce sync.Once
	execCounter Counter
}

func (s *Server) logger() *slog.Logger {
	if s.Engine != nil && s.Engine.Logger != nil {
		return s.Engine.Logger
	}

	return slog.Default()
}

func (s *Server) counter() Counter {
	if s.Engine != nil && s.Engine.Counter != nil {
		return s.Engine.Counter
	}

	s.counterOnce.Do(func() { s.execCounter = NewDefaultCounter() })

	return s.execCounter
}

// Handler returns the fixed route set: an unauthenticated container probe and
// the capability-gated execute endpoint.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /v1/execute", s.handleExecute)

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"executions": s.counter().Total(),
	})
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	maxBytes := s.Engine.MaxRequestBytes
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}

	body, bodyErr := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if bodyErr != nil {
		s.finish(w, start, "", "", "", outcome{
			status: 400,
			err:    &apiError{Code: "invalid_request", Message: "request body exceeds limit or could not be read"},
		})

		return
	}

	token := bearerToken(r)
	if token == "" {
		s.finish(w, start, "", "", "", capabilityOutcome("anonymous", http.StatusUnauthorized))
		return
	}

	claims, capabilityErr := verifyCapability(s.Engine.Secret, token, body, time.Now())
	if capabilityErr != nil {
		var reject *CapabilityError
		code, status := "token_invalid", http.StatusUnauthorized

		if errors.As(capabilityErr, &reject) {
			code, status = reject.Code, reject.HTTPStatus
		}

		s.finish(w, start, "", "", "", capabilityOutcome(code, status))

		return
	}

	req, decodeErr := decodeRequest(body)
	if decodeErr != nil {
		s.finish(w, start, "", claims.TenantID, claims.Operation, invalidOutcome("malformed request"))
		return
	}

	if claims.TenantID != req.TenantID || claims.ConnectionID != req.ConnectionID || claims.Operation != req.Operation {
		s.finish(w, start, req.RequestID, claims.TenantID, claims.Operation, capabilityOutcome("context_mismatch", http.StatusForbidden))
		return
	}

	if validationErr := (validator{}).validateRequest(req); validationErr != nil {
		s.finish(w, start, req.RequestID, req.TenantID, req.Operation, invalidOutcome("request validation failed"))
		return
	}

	result := s.execute(ctx, req)
	s.finish(w, start, req.RequestID, req.TenantID, req.Operation, result)
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "

	header := r.Header.Get("Authorization")
	if len(header) < len(prefix) || header[:len(prefix)] != prefix {
		return ""
	}

	return header[len(prefix):]
}

func capabilityOutcome(code string, status int) outcome {
	if status == 0 {
		status = http.StatusUnauthorized
	}

	return outcome{status: status, err: &apiError{Code: code, Message: "invocation rejected"}}
}

// finish writes exactly one structured response and one small safe audit
// line. Tenant and connection identifiers are hashed; credentials, emails,
// authorization headers, request bodies, and provider exception text are
// never written to responses or logs.
func (s *Server) finish(w http.ResponseWriter, start time.Time, requestID, tenantID, operation string, result outcome) {
	durationMS := time.Since(start).Milliseconds()

	outcomeCode := "ok"
	if result.err != nil {
		outcomeCode = result.err.Code
	}

	counter := s.counter()
	counter.Record(operation, outcomeCode)
	count := counter.Total()

	resp := response{
		RequestID:  requestID,
		Operation:  operation,
		OK:         result.err == nil,
		Result:     result.result,
		Error:      result.err,
		DurationMS: durationMS,
	}

	status := result.status
	if status == 0 {
		status = http.StatusOK
	}

	data, encodeErr := json.Marshal(resp)
	if encodeErr != nil {
		data = []byte(`{"ok":false,"error":{"code":"internal","message":"response encoding failed"}}`)
		status = http.StatusInternalServerError
	}

	tenantHash := "unknown"
	if tenantID != "" {
		tenantHash = hashContext(tenantID)
	}

	connectionHash := "unknown"
	if requestID != "" {
		connectionHash = hashContext(requestID)
	}

	if requestID == "" {
		requestID = "unknown"
	}

	if operation == "" {
		operation = "unknown"
	}

	s.logger().Info("runner.execution",
		"request_id", requestID,
		"operation", operation,
		"tenant_hash", tenantHash,
		"connection_hash", connectionHash,
		"outcome", outcomeCode,
		"duration_ms", durationMS,
		"executions", count,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func decodeRequest(body []byte) (*Request, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	var req Request

	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}

	if err := requireEOF(decoder); err != nil {
		return nil, err
	}

	if req.RequestID == "" {
		req.RequestID = newRequestID()
	}

	return &req, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
