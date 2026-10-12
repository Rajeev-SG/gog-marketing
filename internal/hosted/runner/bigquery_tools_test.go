package runner

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeBigQueryReadToolsUseGrantedProjectAndNeverCreateJobs(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.BigQuery = func(ctx context.Context, _ string, project string) (googleapi.BigQueryClient, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodGet || r.URL.Host != "bigquery.googleapis.com" {
					t.Errorf("unexpected BigQuery operation %s %s", r.Method, r.URL)
				}
				body := ""

				switch r.URL.Path {
				case "/bigquery/v2/projects/customer-proj/datasets":
					body = `{"datasets":[{"datasetReference":{"projectId":"customer-proj","datasetId":"demo"}}]}`
				case "/bigquery/v2/projects/customer-proj/datasets/demo":
					body = `{"id":"customer-proj:demo","datasetReference":{"projectId":"customer-proj","datasetId":"demo"},"location":"US"}`
				case "/bigquery/v2/projects/customer-proj/datasets/demo/tables":
					body = `{"tables":[{"tableReference":{"projectId":"customer-proj","datasetId":"demo","tableId":"events"}}]}`
				case "/bigquery/v2/projects/customer-proj/datasets/demo/tables/events":
					body = `{"tableReference":{"projectId":"customer-proj","datasetId":"demo","tableId":"events"},"numRows":"2","schema":{"fields":[{"name":"id","type":"STRING"}]}}`
				case "/bigquery/v2/projects/customer-proj/datasets/demo/tables/events/data":
					body = `{"rows":[{"f":[{"v":"1"}]},{"f":[{"v":"2"}]}]}`
				default:
					t.Errorf("undeclared BigQuery path %s", r.URL.Path)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})

			return googleapi.NewBigQueryWithHTTPClient(ctx, project, &http.Client{Transport: googleapi.GuardedTransport(ctx, transport)})
		}
	})

	call := func(name, project, scope string, args map[string]any, enabled bool) capturedResponse {
		args["project"] = project
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "bigquery", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, Resource: &ResourceGrantInput{Service: "bigquery", ResourceType: "project", ResourceID: "customer-proj", Enabled: enabled}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, name := range []string{"bigquery_datasets_list", "bigquery_dataset_get", "bigquery_tables_list", "bigquery_table_get", "bigquery_table_schema", "bigquery_table_rows"} {
		args := map[string]any{}
		if name != "bigquery_datasets_list" {
			args["dataset"] = "demo"
		}

		if strings.HasPrefix(name, "bigquery_table_") {
			args["table"] = "events"
		}

		if name == "bigquery_table_rows" {
			args["max"] = 1
		}

		response := call(name, "customer-proj", "bigquery", args, true)
		if !response.OK {
			t.Fatalf("native %s: %s", name, response.Body)
		}

		if name == "bigquery_table_rows" && !strings.Contains(string(response.Result), `"id":"1"`) {
			t.Fatalf("native row data missing: %s", response.Body)
		}
	}
	before := calls.Load()

	for _, entry := range []struct {
		name, project, scope string
		args                 map[string]any
		enabled              bool
	}{
		{"bigquery_datasets_list", "foreign-proj", "bigquery", map[string]any{}, true},
		{"bigquery_datasets_list", "gog-marketing-prod", "bigquery", map[string]any{}, true},
		{"bigquery_datasets_list", "customer-proj", "bigquery", map[string]any{}, false},
		{"bigquery_datasets_list", "customer-proj", "bigquery.readonly", map[string]any{}, true},
		{"bigquery_table_get", "customer-proj", "bigquery", map[string]any{"dataset": "../foreign", "table": "events"}, true},
		{"bigquery_query", "customer-proj", "bigquery", map[string]any{"sql": "DELETE FROM demo.events WHERE TRUE", "acknowledge_cost": true}, true},
	} {
		if response := call(entry.name, entry.project, entry.scope, entry.args, entry.enabled); response.OK {
			t.Fatalf("unsafe invocation allowed: %#v", entry)
		}
	}

	if calls.Load() != before {
		t.Fatal("denied invocation reached Google")
	}
	invocation := &ToolInvocation{Name: "bigquery_table_rows", Arguments: map[string]any{"project": "customer-proj", "dataset": "demo", "table": "events"}}
	guard := nativeRequestGuard(invocation)

	for _, target := range []string{"https://bigquery.googleapis.com/bigquery/v2/projects/customer-proj/jobs", "https://bigquery.googleapis.com/bigquery/v2/projects/foreign-proj/datasets/demo/tables/events"} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		if guard(req) == nil {
			t.Fatalf("foreign endpoint admitted: %s", target)
		}
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://bigquery.googleapis.com/bigquery/v2/projects/customer-proj/datasets/demo/tables/events", nil)
	if guard(req) == nil {
		t.Fatal("write method admitted for BigQuery read")
	}
}
