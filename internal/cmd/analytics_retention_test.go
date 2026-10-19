package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestAnalyticsRetentionPlan(t *testing.T) {
	str := func(v string) *string { return &v }
	flag := func(v bool) *bool { return &v }
	for _, tc := range []struct {
		name string
		cmd  AnalyticsRetentionUpdateCmd
		mask string
		bad  bool
	}{
		{"partial event", AnalyticsRetentionUpdateCmd{Property: "123", EventDataRetention: str("two_months")}, "event_data_retention", false},
		{"false reset", AnalyticsRetentionUpdateCmd{Property: "properties/123", ResetUserDataOnNewActivity: flag(false)}, "reset_user_data_on_new_activity", false},
		{"all", AnalyticsRetentionUpdateCmd{Property: "123", EventDataRetention: str("FOURTEEN_MONTHS"), UserDataRetention: str("TWO_MONTHS"), ResetUserDataOnNewActivity: flag(false)}, "event_data_retention,user_data_retention,reset_user_data_on_new_activity", false},
		{"360 event", AnalyticsRetentionUpdateCmd{Property: "123", EventDataRetention: str("FIFTY_MONTHS")}, "event_data_retention", false},
		{"no fields", AnalyticsRetentionUpdateCmd{Property: "123"}, "", true},
		{"empty supplied", AnalyticsRetentionUpdateCmd{Property: "123", EventDataRetention: str("")}, "", true},
		{"unspecified", AnalyticsRetentionUpdateCmd{Property: "123", EventDataRetention: str("RETENTION_DURATION_UNSPECIFIED")}, "", true},
		{"bad enum", AnalyticsRetentionUpdateCmd{Property: "123", EventDataRetention: str("forever")}, "", true},
		{"user advanced", AnalyticsRetentionUpdateCmd{Property: "123", UserDataRetention: str("TWENTY_SIX_MONTHS")}, "", true},
		{"path injection", AnalyticsRetentionUpdateCmd{Property: "123/../456", EventDataRetention: str("TWO_MONTHS")}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, mask, err := tc.cmd.plan()
			if (err != nil) != tc.bad {
				t.Fatalf("plan error=%v bad=%t", err, tc.bad)
			}
			if tc.bad {
				return
			}
			if mask != tc.mask || body.Name != "properties/123/dataRetentionSettings" {
				t.Fatalf("plan %+v mask=%s", body, mask)
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if tc.cmd.ResetUserDataOnNewActivity != nil && wire["resetUserDataOnNewActivity"] != *tc.cmd.ResetUserDataOnNewActivity {
				t.Fatalf("explicit reset lost: %s", raw)
			}
			if tc.cmd.UserDataRetention == nil && wire["userDataRetention"] != nil {
				t.Fatal("unselected user retention emitted")
			}
		})
	}
}

func TestAnalyticsRetentionCLIRequestAndPartialReadback(t *testing.T) {
	stored := map[string]any{"name": "properties/123/dataRetentionSettings", "eventDataRetention": "FOURTEEN_MONTHS", "userDataRetention": "TWO_MONTHS", "resetUserDataOnNewActivity": true}
	calls := 0
	svc := newAnalyticsAdminTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1beta/properties/123/dataRetentionSettings" {
			t.Errorf("unexpected path %s", r.URL)
		}
		if r.Method == http.MethodPatch {
			if r.URL.Query().Get("updateMask") != "reset_user_data_on_new_activity" {
				t.Errorf("wrong mask %s", r.URL)
			}
			var patch map[string]any
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Fatal(err)
			}
			if len(patch) != 2 || patch["name"] != stored["name"] || patch["resetUserDataOnNewActivity"] != false {
				t.Errorf("wrong patch %v", patch)
			}
			stored["resetUserDataOnNewActivity"] = false
		} else if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stored)
	}))
	for _, args := range [][]string{
		{"get", "123"},
		{"update", "properties/123", "--reset-user-data-on-new-activity=false", "--force"},
		{"get", "properties/123"},
	} {
		result := executeWithAnalyticsAdminTestService(t, append([]string{"--account=a@b.com", "--json", "analytics", "retention"}, args...), svc)
		if result.err != nil || !strings.Contains(result.stdout, "FOURTEEN_MONTHS") {
			t.Fatalf("CLI result %+v", result)
		}
	}
	if calls != 3 || stored["eventDataRetention"] != "FOURTEEN_MONTHS" || stored["userDataRetention"] != "TWO_MONTHS" || stored["resetUserDataOnNewActivity"] != false {
		t.Fatal("partial update changed unrelated retention values")
	}
}

func TestAnalyticsRetentionValidationAndDryRunBeforeService(t *testing.T) {
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--dry-run", "analytics", "retention", "update", "123", "--event-data-retention=TWO_MONTHS"}, true},
		{[]string{"analytics", "retention", "update", "123", "--event-data-retention=TWO_MONTHS"}, false},
		{[]string{"analytics", "retention", "update", "123", "--force"}, false},
		{[]string{"--read-only", "analytics", "retention", "update", "123", "--event-data-retention=TWO_MONTHS", "--force"}, false},
	} {
		result := executeWithAnalyticsAdminTestServiceFactory(t, append([]string{"--account=a@b.com", "--no-input"}, tc.args...), func(_ctx context.Context, _ string) (*analyticsadmin.Service, error) {
			t.Fatal("invalid/dry-run mutation requested service")
			return nil, errors.New("unexpected service")
		})
		if (result.err == nil) != tc.ok {
			t.Fatalf("args=%v result=%+v", tc.args, result)
		}
	}
}

func TestAnalyticsRetentionUncertainUpdateDoesNotRetry(t *testing.T) {
	var effects atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if effects.Load() == 1 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":503,"message":"uncertain"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"properties/123/dataRetentionSettings","eventDataRetention":"TWO_MONTHS"}`))
	}))
	defer server.Close()
	retry := googleapi.NewRetryTransport(server.Client().Transport)
	retry.MaxRetries5xx = 1
	svc, err := analyticsadmin.NewService(t.Context(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(&http.Client{Transport: retry}))
	if err != nil {
		t.Fatal(err)
	}
	result := executeWithAnalyticsAdminTestService(t, []string{"--account=a@b.com", "--json", "--force", "analytics", "retention", "update", "123", "--event-data-retention=TWO_MONTHS"}, svc)
	if result.err == nil || effects.Load() != 1 {
		t.Fatalf("uncertain retention replayed: effects=%d result=%+v", effects.Load(), result)
	}
}
