package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
	scriptapi "google.golang.org/api/script/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func appScriptDeploymentUpdateServer(t *testing.T, getCount, putCount *int, check func(t *testing.T, r *http.Request, body string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/projects/script-123/deployments/deployment-456") {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			*getCount++
			fmt.Fprint(w, `{"deploymentConfig":{"scriptId":"script-123","versionNumber":1,"description":"old","manifestFileName":"old.json"}}`)
		case http.MethodPut:
			*putCount++
			body, _ := io.ReadAll(r.Body)
			if check != nil {
				check(t, r, string(body))
			}
			fmt.Fprint(w, `{"deploymentConfig":{"scriptId":"script-123","versionNumber":2,"description":"old","manifestFileName":"old.json"}}`)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
}

func TestAppScriptDeploymentUpdateSuccess(t *testing.T) {
	var getCount, putCount int
	srv := appScriptDeploymentUpdateServer(t, &getCount, &putCount, func(t *testing.T, _ *http.Request, body string) {
		var req struct {
			DeploymentConfig struct {
				ScriptId         string   `json:"scriptId"`
				VersionNumber    int64    `json:"versionNumber"`
				ManifestFileName *string  `json:"manifestFileName,omitempty"`
				ForceSendFields  []string `json:"forceSendFields,omitempty"`
			} `json:"deploymentConfig"`
		}
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatalf("bad PUT body: %v", err)
		}
		if req.DeploymentConfig.ScriptId != "script-123" || req.DeploymentConfig.VersionNumber != 2 {
			t.Fatalf("unexpected config: %#v", req.DeploymentConfig)
		}
	})
	defer srv.Close()
	svc := newAppScriptTestService(t, srv)
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--description=new", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if getCount != 1 || putCount != 1 {
		t.Fatalf("expected 1 GET and 1 PUT, got %d/%d", getCount, putCount)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if got["updated"] != true || got["deployment"] == nil {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestAppScriptDeploymentUpdateOmittedFieldsPreserved(t *testing.T) {
	var getCount, putCount int
	srv := appScriptDeploymentUpdateServer(t, &getCount, &putCount, func(t *testing.T, _ *http.Request, body string) {
		var cfg struct {
			DeploymentConfig struct {
				Description      string `json:"description"`
				ManifestFileName string `json:"manifestFileName"`
			} `json:"deploymentConfig"`
		}
		if err := json.Unmarshal([]byte(body), &cfg); err != nil {
			t.Fatalf("bad PUT body: %v", err)
		}
		if cfg.DeploymentConfig.Description != "old" || cfg.DeploymentConfig.ManifestFileName != "old.json" {
			t.Fatalf("omitted fields not preserved: %#v", cfg.DeploymentConfig)
		}
	})
	defer srv.Close()
	svc := newAppScriptTestService(t, srv)
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if getCount != 1 || putCount != 1 {
		t.Fatalf("expected 1 GET and 1 PUT, got %d/%d", getCount, putCount)
	}
}

func TestAppScriptDeploymentUpdateExplicitEmptyDescription(t *testing.T) {
	var getCount, putCount int
	srv := appScriptDeploymentUpdateServer(t, &getCount, &putCount, func(t *testing.T, _ *http.Request, body string) {
		var raw struct {
			DeploymentConfig struct {
				Description *string `json:"description"`
			} `json:"deploymentConfig"`
		}
		if err := json.Unmarshal([]byte(body), &raw); err != nil {
			t.Fatalf("bad PUT body: %v", err)
		}
		if raw.DeploymentConfig.Description == nil || *raw.DeploymentConfig.Description != "" {
			t.Fatalf("explicit empty description missing from wire JSON: %s", body)
		}
	})
	defer srv.Close()
	svc := newAppScriptTestService(t, srv)
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--description=", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
}

func TestAppScriptDeploymentUpdateExplicitManifestReplacement(t *testing.T) {
	var getCount, putCount int
	srv := appScriptDeploymentUpdateServer(t, &getCount, &putCount, func(t *testing.T, _ *http.Request, body string) {
		var cfg struct {
			DeploymentConfig struct {
				ManifestFileName string `json:"manifestFileName"`
			} `json:"deploymentConfig"`
		}
		if err := json.Unmarshal([]byte(body), &cfg); err != nil {
			t.Fatalf("bad PUT body: %v", err)
		}
		if cfg.DeploymentConfig.ManifestFileName != "new.json" {
			t.Fatalf("expected manifestFileName new.json, got %q", cfg.DeploymentConfig.ManifestFileName)
		}
	})
	defer srv.Close()
	svc := newAppScriptTestService(t, srv)
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--manifest-file-name=new.json", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
}

func TestAppScriptDeploymentUpdateRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty script", args: []string{"appscript", "deployment-update", "", "deployment-456", "--version-number=2"}, want: "script id is required"},
		{name: "empty deployment", args: []string{"appscript", "deployment-update", "script-123", "", "--version-number=2"}, want: "deployment id is required"},
		{name: "zero version", args: []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=0"}, want: "version number must be a positive 32-bit integer"},
		{name: "negative version", args: []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=-1"}, want: "version number must be a positive 32-bit integer"},
		{name: "blank manifest", args: []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--manifest-file-name="}, want: "manifest file name must not be blank"},
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

func TestAppScriptDeploymentUpdateDryRun(t *testing.T) {
	res := executeWithAppScriptTestServiceFactory(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--description=new", "--dry-run", "--json"}, unexpectedAppScriptTestService(t, "dry-run must not create service"))
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if got["dry_run"] != true || got["op"] != "appscript.deployment.update" {
		t.Fatalf("unexpected result: %v", got)
	}
	req, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected request payload: %#v", got["request"])
	}
	if req["script_id"] != "script-123" || req["deployment_id"] != "deployment-456" || req["version_number"] != float64(2) || req["description"] != "new" {
		t.Fatalf("unexpected request payload: %v", req)
	}
}

func TestAppScriptDeploymentUpdateConfirmation(t *testing.T) {
	res := executeWithAppScriptTestServiceFactory(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--no-input"}, unexpectedAppScriptTestService(t, "confirmation must happen before factory"))
	if res.err == nil || !strings.Contains(res.err.Error(), "refusing to update Apps Script deployment without --force (non-interactive)") {
		t.Fatalf("expected confirmation rejection, got %v", res.err)
	}
}

func TestAppScriptDeploymentUpdateGetFailurePreventsPut(t *testing.T) {
	var putCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCount++
		}
		http.Error(w, "get failure", http.StatusNotFound)
	}))
	defer srv.Close()
	svc := newAppScriptTestService(t, srv)
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err == nil {
		t.Fatal("expected error")
	}
	if putCount != 0 {
		t.Fatalf("expected no PUT after GET failure, got %d", putCount)
	}
}

func TestAppScriptDeploymentUpdateWithoutRetries(t *testing.T) {
	var getCount, putCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getCount++
			fmt.Fprint(w, `{"deploymentConfig":{"scriptId":"script-123","versionNumber":1}}`)
		case http.MethodPut:
			putCount++
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	retry := googleapi.NewRetryTransport(srv.Client().Transport)
	retry.MaxRetries5xx = 1
	svc, err := scriptapi.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"),
		option.WithHTTPClient(&http.Client{Transport: retry}))
	if err != nil {
		t.Fatalf("NewService() failed: %v", err)
	}
	res := executeWithAppScriptTestService(t, []string{"appscript", "deployment-update", "script-123", "deployment-456", "--version-number=2", "--force", "--json", "--account=a@b.com"}, svc)
	if res.err == nil {
		t.Fatal("expected error")
	}
	if putCount != 1 {
		t.Fatalf("expected exactly one PUT without retry, got %d", putCount)
	}
}
