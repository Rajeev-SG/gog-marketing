package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecute_AnalyticsRealtime(t *testing.T) {
	svc := newAnalyticsDataTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/properties/123:runRealtimeReport" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Dimensions []struct {
				Name string `json:"name"`
			} `json:"dimensions"`
			Metrics []struct {
				Name string `json:"name"`
			} `json:"metrics"`
			Limit string `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Dimensions) != 1 || body.Dimensions[0].Name != "eventName" {
			t.Fatalf("unexpected dimensions: %#v", body.Dimensions)
		}
		if len(body.Metrics) != 1 || body.Metrics[0].Name != "eventCount" {
			t.Fatalf("unexpected metrics: %#v", body.Metrics)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rowCount": 2,
			"dimensionHeaders": []map[string]any{
				{"name": "eventName"},
			},
			"metricHeaders": []map[string]any{
				{"name": "eventCount", "type": "TYPE_INTEGER"},
			},
			"rows": []map[string]any{
				{
					"dimensionValues": []map[string]any{{"value": "page_view"}},
					"metricValues":    []map[string]any{{"value": "42"}},
				},
				{
					"dimensionValues": []map[string]any{{"value": "purchase"}},
					"metricValues":    []map[string]any{{"value": "3"}},
				},
			},
		})
	}))
	result := executeWithAnalyticsDataTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"analytics", "realtime", "123",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		Property   string   `json:"property"`
		Dimensions []string `json:"dimensions"`
		Metrics    []string `json:"metrics"`
		RowCount   int64    `json:"row_count"`
		Rows       []struct {
			DimensionValues []struct {
				Value string `json:"value"`
			} `json:"dimensionValues"`
			MetricValues []struct {
				Value string `json:"value"`
			} `json:"metricValues"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Property != "properties/123" || parsed.RowCount != 2 || len(parsed.Rows) != 2 {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
	if parsed.Rows[0].DimensionValues[0].Value != "page_view" || parsed.Rows[0].MetricValues[0].Value != "42" {
		t.Fatalf("unexpected first row: %#v", parsed.Rows[0])
	}
	if len(parsed.Dimensions) != 1 || parsed.Dimensions[0] != "eventName" ||
		len(parsed.Metrics) != 1 || parsed.Metrics[0] != "eventCount" {
		t.Fatalf("unexpected headers: %#v/%#v", parsed.Dimensions, parsed.Metrics)
	}
}

func TestNewAnalyticsRealtimePlan(t *testing.T) {
	plan, err := newAnalyticsRealtimePlan(" /123 ", " eventName, country ", " eventCount ", 25)
	if err != nil {
		t.Fatalf("newAnalyticsRealtimePlan: %v", err)
	}
	if plan.Property != "properties/123" {
		t.Fatalf("unexpected property: %q", plan.Property)
	}
	if len(plan.Dimensions) != 2 || plan.Dimensions[0] != "eventName" || plan.Dimensions[1] != "country" {
		t.Fatalf("unexpected dimensions: %#v", plan.Dimensions)
	}
	if plan.Request == nil || plan.Request.Limit != 25 || len(plan.Request.Metrics) != 1 || plan.Request.Metrics[0].Name != "eventCount" {
		t.Fatalf("unexpected request: %#v", plan.Request)
	}
	if len(plan.Request.Dimensions) != 2 || plan.Request.Dimensions[0].Name != "eventName" {
		t.Fatalf("unexpected request dimensions: %#v", plan.Request.Dimensions)
	}
}

func TestNewAnalyticsRealtimePlanValidation(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		property  string
		metrics   string
		max       int64
		wantError string
	}{
		{name: "property", metrics: "eventCount", max: 100, wantError: "empty property"},
		{name: "metrics", property: "123", max: 100, wantError: "empty --metrics"},
		{name: "max zero", property: "123", metrics: "eventCount", wantError: "--max must be between"},
		{name: "max high", property: "123", metrics: "eventCount", max: analyticsRealtimeMaxRows + 1, wantError: "--max must be between"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := newAnalyticsRealtimePlan(testCase.property, "eventName", testCase.metrics, testCase.max)
			if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("err = %v, want %q", err, testCase.wantError)
			}
		})
	}
}
