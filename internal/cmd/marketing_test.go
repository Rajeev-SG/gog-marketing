package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleads"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestParseAuthServicesMarketingAlias(t *testing.T) {
	services, err := parseAuthServices("marketing")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(services))
	for _, service := range services {
		got = append(got, string(service))
	}
	if strings.Join(got, ",") != "analytics,tagmanager,googleads,searchconsole,bigquery" {
		t.Fatalf("marketing services = %q", got)
	}
}

func TestGoogleAdsClientSearchHeadersPaginationAndRequestID(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("developer-token") != "dev-token" || r.Header.Get("login-customer-id") != "1234567890" {
			t.Errorf("headers = %#v", r.Header)
		}
		if r.URL.Path != "/v25/customers/1234567890/googleAds:search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("x-request-id", "req-"+string(rune('0'+calls)))
		if calls == 1 {
			_, _ = io.WriteString(w, `{"results":[{"campaign":{"id":"1"}}],"nextPageToken":"next"}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"campaign":{"id":"2"}}]}`)
	}))
	defer server.Close()
	client := &googleads.Client{HTTP: server.Client(), BaseURL: server.URL, DeveloperToken: "dev-token", LoginCustomerID: "123-456-7890"}
	var rows []map[string]any
	next := ""
	for {
		response, err := client.Search(context.Background(), "123-456-7890", googleads.SearchRequest{Query: "SELECT campaign.id FROM campaign", PageToken: next})
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range response.Results {
			var row map[string]any
			if err := json.Unmarshal(raw, &row); err != nil {
				t.Fatal(err)
			}
			rows = append(rows, row)
		}
		if response.NextPageToken == "" {
			break
		}
		next = response.NextPageToken
	}
	if calls != 2 || len(rows) != 2 {
		t.Fatalf("calls = %d rows = %d", calls, len(rows))
	}
}

func TestGoogleAdsClientAPIErrorKeepsRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "req-error")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"bad query","status":"INVALID_ARGUMENT"}}`)
	}))
	defer server.Close()
	client := &googleads.Client{HTTP: server.Client(), BaseURL: server.URL, DeveloperToken: "dev-token"}
	_, err := client.Search(context.Background(), "1234567890", googleads.SearchRequest{Query: "SELECT 1"})
	var apiErr *googleads.APIError
	if !errors.As(err, &apiErr) || apiErr.RequestID != "req-error" || !strings.Contains(apiErr.Error(), "req-error") {
		t.Fatalf("error = %#v", err)
	}
}

func TestBigQueryDryRunCommandUsesInjectedClient(t *testing.T) {
	fake := &fakeBigQueryClient{}
	result := executeWithTestRuntime(t, []string{"--account", "a@b.com", "--json", "--dry-run", "bigquery", "query", "--project", "billing-proj", "--sql", "SELECT 1"}, &app.Runtime{Services: app.Services{
		BigQuery: func(_ context.Context, account, project string) (googleapi.BigQueryClient, error) {
			if account != "a@b.com" || project != "billing-proj" {
				t.Fatalf("factory args = %q %q", account, project)
			}
			return fake, nil
		},
	}})
	if result.err != nil {
		t.Fatal(result.err)
	}
	if fake.querySQL != "SELECT 1" || !fake.queryDryRun {
		t.Fatalf("query = %#v", fake)
	}
	if !strings.Contains(result.stdout, `"dry_run": true`) {
		t.Fatalf("stdout = %s", result.stdout)
	}
}

func TestMarketingMCPToolsAreTypedAndRiskClassified(t *testing.T) {
	readTools := mcpEnabledTools(McpCmd{}, nil)
	for _, name := range []string{"analytics_report", "tagmanager_tags_list", "googleads_query", "searchconsole_query", "bigquery_query_dry_run", "bigquery_datasets_list"} {
		if !hasMCPTool(readTools, name) {
			t.Fatalf("missing read tool %s", name)
		}
	}
	if hasMCPTool(readTools, "bigquery_query") || hasMCPTool(readTools, "tagmanager_versions_publish") || hasMCPTool(readTools, "searchconsole_sitemaps_submit") {
		t.Fatal("write tool exposed by default")
	}
	writeTools := mcpEnabledTools(McpCmd{AllowWrite: true}, nil)
	if !hasMCPTool(writeTools, "bigquery_query") || !hasMCPTool(writeTools, "tagmanager_versions_publish") || !hasMCPTool(writeTools, "searchconsole_sitemaps_submit") || !hasMCPTool(writeTools, "searchconsole_sitemaps_delete") {
		t.Fatal("write tools missing after --allow-write")
	}
	args, err := findMCPTool(t, "bigquery_query_dry_run").BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"project": "p", "sql": "SELECT 1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != "bigquery query --sql SELECT 1 --dry-run --billing-project p" {
		t.Fatalf("dry-run args = %#v", args)
	}
}

type fakeBigQueryClient struct {
	querySQL    string
	queryDryRun bool
}

func (f *fakeBigQueryClient) Project() string { return "billing-proj" }
func (f *fakeBigQueryClient) ListDatasets(context.Context) ([]googleapi.BigQueryDataset, error) {
	return []googleapi.BigQueryDataset{{ProjectID: "billing-proj", DatasetID: "demo"}}, nil
}

func (f *fakeBigQueryClient) GetDataset(context.Context, string) (*googleapi.BigQueryDataset, error) {
	return &googleapi.BigQueryDataset{ProjectID: "billing-proj", DatasetID: "demo"}, nil
}

func (f *fakeBigQueryClient) ListTables(context.Context, string) ([]googleapi.BigQueryTable, error) {
	return []googleapi.BigQueryTable{{ProjectID: "billing-proj", DatasetID: "demo", TableID: "events"}}, nil
}

func (f *fakeBigQueryClient) GetTable(context.Context, string, string) (*googleapi.BigQueryTable, error) {
	return &googleapi.BigQueryTable{ProjectID: "billing-proj", DatasetID: "demo", TableID: "events"}, nil
}

func (f *fakeBigQueryClient) ReadRows(context.Context, string, string, int64) ([]map[string]any, []googleapi.BigQuerySchemaField, error) {
	return []map[string]any{{"id": "1"}}, []googleapi.BigQuerySchemaField{{Name: "id", Type: "STRING"}}, nil
}

func (f *fakeBigQueryClient) Query(_ context.Context, sql string, dryRun bool) (*googleapi.BigQueryQueryResult, error) {
	f.querySQL, f.queryDryRun = sql, dryRun
	return &googleapi.BigQueryQueryResult{DryRun: dryRun, Rows: []map[string]any{}}, nil
}
func (f *fakeBigQueryClient) Close() error { return nil }
