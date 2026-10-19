package cmd

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestAppScriptRunDoesNotRetryEffects(t *testing.T) {
	var effects atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/scripts/deployment-1:run" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if effects.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":503,"message":"transient backend failure","status":"UNAVAILABLE"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":true,"response":{"result":3}}`))
	}))
	defer server.Close()

	retry := googleapi.NewRetryTransport(server.Client().Transport)
	retry.MaxRetries5xx = 1
	svc, err := scriptapi.NewService(t.Context(),
		option.WithEndpoint(server.URL+"/"),
		option.WithHTTPClient(&http.Client{Transport: retry}))
	if err != nil {
		t.Fatalf("NewService() failed: %v", err)
	}

	res := executeWithAppScriptTestService(t, []string{
		"--account=a@b.com", "--json", "appscript", "run",
		"deployment-1", "sum", "--params=[1,2]",
	}, svc)

	if res.err == nil {
		t.Fatalf("appscript run err = nil, want non-nil after 503 (effects=%d stdout=%q stderr=%q)", effects.Load(), res.stdout, res.stderr)
	}
	if got := effects.Load(); got != 1 {
		t.Errorf("effect count = %d, want 1; Run's googleapi.WithoutRetries(ctx) marker must reach RetryTransport", got)
	}
}
