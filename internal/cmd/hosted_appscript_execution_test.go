package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAppScriptExecutionArguments(t *testing.T) {
	spec := hostedAppScriptExecutionTool()
	if spec.Name != "appscript_run" || spec.Risk != mcpRiskWrite {
		t.Fatal("execution must require write authority")
	}
	params := `[1,{"nested":[true,null,"text"]}]`
	args, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"deployment_id": "deployment-1", "function": "sum", "params_json": params, "dev_mode": true}}})
	if err != nil || strings.Join(args, "\x00") != strings.Join([]string{"appscript", "run", "--params=" + params, "--", "deployment-1", "sum"}, "\x00") {
		t.Fatalf("arguments %v err=%v", args, err)
	}
	for _, raw := range []string{`null`, `{}`, `@/tmp/script.json`, `-`, `[] {}`, `[NaN]`, `"[]"`, ``} {
		if _, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"deployment_id": "deployment-1", "function": "sum", "params_json": raw}}}); err == nil {
			t.Errorf("accepted nonliteral/invalid parameter array %q", raw)
		}
	}
	for _, args := range []map[string]any{{"function": "sum"}, {"deployment_id": "deployment-1"}} {
		if _, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}); err == nil {
			t.Errorf("accepted missing selector %v", args)
		}
	}
}

func TestAppScriptRunDeploymentPayloadAndDryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/scripts/deployment-1:run" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["function"] != "sum" || len(body["parameters"].([]any)) != 2 || body["devMode"] != nil {
			t.Errorf("execution payload %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":true,"response":{"result":3}}`))
	}))
	defer server.Close()
	result := executeWithAppScriptTestService(t, []string{"--account=a@b.com", "--json", "appscript", "run", "deployment-1", "sum", "--params=[1,2]"}, newAppScriptTestService(t, server))
	if result.err != nil || !strings.Contains(result.stdout, `"result": 3`) {
		t.Fatalf("run result %+v", result)
	}
	dry := executeWithAppScriptTestServiceFactory(t, []string{"--account=a@b.com", "--dry-run", "appscript", "run", "deployment-1", "sum", "--params=[1,2]"}, unexpectedAppScriptTestService(t, "dry-run must not execute or access credentials"))
	if dry.err != nil {
		t.Fatalf("dry-run failed %+v", dry)
	}
}
