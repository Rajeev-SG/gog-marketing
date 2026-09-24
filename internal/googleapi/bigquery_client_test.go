package googleapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
)

func TestBigQueryAdapterUsesOfficialClientAndBoundedQuery(t *testing.T) {
	var queryBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasets"):
			_, _ = io.WriteString(w, `{"datasets":[{"datasetReference":{"projectId":"billing-proj","datasetId":"demo"}}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasets/demo"):
			_, _ = io.WriteString(w, `{"id":"billing-proj:demo","datasetReference":{"projectId":"billing-proj","datasetId":"demo"},"location":"US"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasets/demo/tables"):
			_, _ = io.WriteString(w, `{"tables":[{"tableReference":{"projectId":"billing-proj","datasetId":"demo","tableId":"events"}}]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasets/demo/tables/events"):
			_, _ = io.WriteString(w, `{"tableReference":{"projectId":"billing-proj","datasetId":"demo","tableId":"events"},"numRows":"2","numBytes":"100","schema":{"fields":[{"name":"id","type":"STRING","mode":"NULLABLE"}]}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasets/demo/tables/events/data"):
			_, _ = io.WriteString(w, `{"rows":[{"f":[{"v":"1"}]},{"f":[{"v":"2"}]}],"schema":{"fields":[{"name":"id","type":"STRING","mode":"NULLABLE"}]}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/queries/"):
			_, _ = io.WriteString(w, `{"jobReference":{"projectId":"billing-proj","jobId":"job-1"},"jobComplete":true,"totalRows":"1","rows":[{"f":[{"v":"1"}]}],"schema":{"fields":[{"name":"id","type":"STRING","mode":"NULLABLE"}]}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-1"):
			_, _ = io.WriteString(w, `{"jobReference":{"projectId":"billing-proj","jobId":"job-1"},"status":{"state":"DONE"},"statistics":{"query":{"totalBytesProcessed":"42","totalBytesBilled":"42"}}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/jobs"):
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &queryBody)
			_, _ = io.WriteString(w, `{"jobReference":{"projectId":"billing-proj","jobId":"job-1"},"status":{"state":"DONE"},"statistics":{"query":{"totalBytesProcessed":"42","totalBytesBilled":"42"}}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/queries"):
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &queryBody)
			_, _ = io.WriteString(w, `{"jobReference":{"projectId":"billing-proj","jobId":"job-1"},"rows":[{"f":[{"v":"1"}]}],"schema":{"fields":[{"name":"id","type":"STRING","mode":"NULLABLE"}]},"totalBytesProcessed":"42"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := bigquery.NewClient(context.Background(), "billing-proj", option.WithoutAuthentication(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}

	adapter := &bigQueryAdapter{client: client}
	defer func() { _ = adapter.Close() }()

	datasets, err := adapter.ListDatasets(context.Background())
	if err != nil || len(datasets) != 1 || datasets[0].DatasetID != "demo" {
		t.Fatalf("datasets = %#v, %v", datasets, err)
	}

	table, err := adapter.GetTable(context.Background(), "demo", "events")
	if err != nil || table.Schema[0].Name != "id" {
		t.Fatalf("table = %#v, %v", table, err)
	}

	rows, schema, err := adapter.ReadRows(context.Background(), "demo", "events", 10)
	if err != nil || len(rows) != 2 || schema[0].Name != "id" || rows[0]["id"] != "1" {
		t.Fatalf("rows = %#v schema = %#v err = %v", rows, schema, err)
	}

	result, err := adapter.Query(context.Background(), "SELECT 1", false, 1024)
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["id"] != "1" {
		t.Fatalf("query result = %#v, %v", result, err)
	}

	if queryBody["maximumBytesBilled"] != float64(1024) && queryBody["maximumBytesBilled"] != "1024" {
		t.Fatalf("query body = %#v, want maximumBytesBilled 1024", queryBody)
	}
}

func TestBigQueryAdapterRejectsMissingByteCap(t *testing.T) {
	adapter := &bigQueryAdapter{}
	if _, err := adapter.Query(context.Background(), "SELECT 1", true, 0); !errors.Is(err, ErrBigQueryMaxBytesBilledRequired) {
		t.Fatalf("error = %v", err)
	}
}
