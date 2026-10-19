package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
)

func TestAnalyticsMetadataGet(t *testing.T) {
	var gotPath string
	svc := newAnalyticsDataTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":       "properties/123/metadata",
			"dimensions": []map[string]any{{"apiName": "date", "uiName": "Date"}},
			"metrics":    []map[string]any{{"apiName": "activeUsers", "type": "TYPE_INTEGER"}},
		})
	}))
	result := executeWithAnalyticsDataTestService(t, []string{"--account=a@b.com", "--json", "analytics", "metadata", "get", "123"}, svc)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if gotPath != "/v1beta/properties/123/metadata" {
		t.Fatalf("path = %q", gotPath)
	}
	var got struct {
		Property   string
		Dimensions []analyticsdata.DimensionMetadata
		Metrics    []analyticsdata.MetricMetadata
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("stdout %s: %v", result.stdout, err)
	}
	if !strings.Contains(result.stdout, "properties/123") {
		t.Fatalf("stdout missing metadata JSON: %s", result.stdout)
	}
	if got.Property != "properties/123" || len(got.Dimensions) != 1 || got.Dimensions[0].ApiName != "date" || len(got.Metrics) != 1 || got.Metrics[0].ApiName != "activeUsers" {
		t.Fatalf("typed response = %+v", got)
	}
}

func TestAnalyticsMetadataGetRejectsBeforeFactory(t *testing.T) {
	for _, property := range []string{"0", "properties/0", "-1", "abc", "123/../456", "properties/123/", "999999999999999999999999"} {
		result := executeWithAnalyticsDataTestServiceFactory(t, []string{"--account=a@b.com", "analytics", "metadata", "get", property}, unexpectedAnalyticsDataTestService(t, "invalid metadata property"))
		if result.err == nil {
			t.Fatalf("property %q accepted", property)
		}
	}
}

func TestAnalyticsMetadataGetOutput(t *testing.T) {
	svc := newAnalyticsDataTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"properties/123/metadata","dimensions":[{"apiName":"date"}],"metrics":[{"apiName":"activeUsers"}]}`))
	}))
	result := executeWithAnalyticsDataTestService(t, []string{"--account=a@b.com", "--plain", "analytics", "metadata", "get", "properties/123"}, svc)
	if result.err != nil || !strings.Contains(result.stdout, "1 dimensions, 1 metrics") {
		t.Fatalf("result %+v", result)
	}
}

func TestAnalyticsMetadataGetFactoryErrorBeforeTypedCall(t *testing.T) {
	factoryErr := errors.New("auth unavailable")
	result := executeWithAnalyticsDataTestServiceFactory(t, []string{"--account=a@b.com", "analytics", "metadata", "get", "123"}, func(context.Context, string) (*analyticsdata.Service, error) {
		return nil, factoryErr
	})
	if !errors.Is(result.err, factoryErr) {
		t.Fatalf("error = %v", result.err)
	}
}
