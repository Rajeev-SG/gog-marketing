package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func appScriptDeploymentDeleteServer(t *testing.T, count *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/projects/script-123/deployments/deployment-456") {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		*count++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	}))
}

func TestAppScriptDeploymentDeleteSuccess(t *testing.T) {
	count := 0
	srv := appScriptDeploymentDeleteServer(t, &count)
	defer srv.Close()
	svc := newAppScriptTestService(t, srv)
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-delete", "script-123", "deployment-456", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if count != 1 {
		t.Fatalf("expected 1 delete, got %d", count)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if got["deleted"] != true || got["deployment_id"] != "deployment-456" {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestAppScriptDeploymentDeleteRejectsInvalidID(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty script", args: []string{"appscript", "deployment-delete", "", "deployment-456"}, want: "script id is required"},
		{name: "empty deployment", args: []string{"appscript", "deployment-delete", "script-123", ""}, want: "deployment id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := executeWithAppScriptTestServiceFactory(t, tc.args, unexpectedAppScriptTestService(t, "service must not be called"))
			if res.err == nil || !strings.Contains(res.err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, res.err)
			}
		})
	}
}

func TestAppScriptDeploymentDeleteDryRun(t *testing.T) {
	res := executeWithAppScriptTestServiceFactory(t, []string{"appscript", "deployment-delete", "script-123", "deployment-456", "--dry-run", "--json"}, unexpectedAppScriptTestService(t, "dry-run must not create service"))
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if got["dry_run"] != true {
		t.Fatalf("expected dry_run true: %v", got)
	}
	if got["op"] != "appscript.deployment.delete" {
		t.Fatalf("unexpected op: %v", got)
	}
	req, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected request payload: %#v", got["request"])
	}
	if req["script_id"] != "script-123" || req["deployment_id"] != "deployment-456" {
		t.Fatalf("unexpected request payload: %v", req)
	}
}

func TestAppScriptDeploymentDeleteDestructiveConfirmation(t *testing.T) {
	res := executeWithAppScriptTestServiceFactory(t, []string{"appscript", "deployment-delete", "script-123", "deployment-456", "--no-input"}, unexpectedAppScriptTestService(t, "confirmation must happen before factory"))
	if res.err == nil || !strings.Contains(res.err.Error(), "refusing to delete Apps Script deployment without --force (non-interactive)") {
		t.Fatalf("expected confirmation rejection, got %v", res.err)
	}
}

func TestAppScriptDeploymentDeleteWithoutRetries(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		http.Error(w, "temporary failure", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	retry := googleapi.NewRetryTransport(srv.Client().Transport)
	retry.MaxRetries5xx = 1
	svc, err := scriptapi.NewService(t.Context(),
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(&http.Client{Transport: retry}))
	if err != nil {
		t.Fatalf("NewService() failed: %v", err)
	}
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-delete", "script-123", "deployment-456", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err == nil {
		t.Fatal("expected error")
	}
	if count != 1 {
		t.Fatalf("expected exactly one request without retry, got %d", count)
	}
}
