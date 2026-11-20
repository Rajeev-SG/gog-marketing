package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestExecute_AnalyticsPropertiesCreate(t *testing.T) {
	var calls int
	svc := newAnalyticsAdminTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/properties" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("parent"); got != "" {
			t.Fatalf("parent must travel in the body, got query %q", got)
		}
		var body struct {
			Parent       string `json:"parent"`
			DisplayName  string `json:"displayName"`
			TimeZone     string `json:"timeZone"`
			CurrencyCode string `json:"currencyCode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.DisplayName != "Paid Media Decoded" || body.TimeZone != "Europe/London" || body.CurrencyCode != "GBP" {
			t.Fatalf("unexpected body: %#v", body)
		}
		if body.Parent != "accounts/123" {
			t.Fatalf("body parent = %q, want accounts/123", body.Parent)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":         "properties/456",
			"parent":       "accounts/123",
			"displayName":  "Paid Media Decoded",
			"timeZone":     "Europe/London",
			"currencyCode": "GBP",
		})
	}))
	result := executeWithAnalyticsAdminTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"analytics", "properties", "create", "123",
		"--display-name", "Paid Media Decoded",
		"--time-zone", "Europe/London",
		"--currency", "gbp",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	var parsed struct {
		Property struct {
			Name         string `json:"name"`
			Parent       string `json:"parent"`
			CurrencyCode string `json:"currencyCode"`
		} `json:"property"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Property.Name != "properties/456" || parsed.Property.Parent != "accounts/123" || parsed.Property.CurrencyCode != "GBP" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_AnalyticsPropertiesCreate_DryRun(t *testing.T) {
	svc := newAnalyticsAdminTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry run reached Google: %s %s", r.Method, r.URL)
	}))
	result := executeWithAnalyticsAdminTestService(t, []string{
		"--json",
		"--dry-run",
		"--account", "a@b.com",
		"analytics", "properties", "create", "accounts/123",
		"--display-name", "Paid Media Decoded",
		"--time-zone", "Europe/London",
		"--currency", "GBP",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			Parent   string `json:"parent"`
			Resource struct {
				DisplayName string `json:"displayName"`
			} `json:"resource"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.DryRun || parsed.Op != "analytics.properties.create" ||
		parsed.Request.Parent != "accounts/123" || parsed.Request.Resource.DisplayName != "Paid Media Decoded" {
		t.Fatalf("unexpected dry-run payload: %#v", parsed)
	}
}

func TestExecute_AnalyticsPropertiesCreate_ReadOnlyBlocked(t *testing.T) {
	svc := newAnalyticsAdminTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("read-only request reached Google: %s %s", r.Method, r.URL)
	}))
	result := executeWithAnalyticsAdminTestService(t, []string{
		"--json",
		"--readonly",
		"--account", "a@b.com",
		"analytics", "properties", "create", "accounts/123",
		"--display-name", "Paid Media Decoded",
		"--time-zone", "Europe/London",
		"--currency", "GBP",
	}, svc)
	if !errors.Is(result.err, googleapi.ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", result.err)
	}
}

func TestExecute_AnalyticsPropertiesCreate_Validation(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "account",
			args: []string{"properties/123", "--display-name", "X", "--time-zone", "Europe/London", "--currency", "GBP"},
			want: "invalid account",
		},
		{
			name: "account non-numeric",
			args: []string{"accounts/abc", "--display-name", "X", "--time-zone", "Europe/London", "--currency", "GBP"},
			want: "invalid account",
		},
		{
			name: "time zone",
			args: []string{"accounts/123", "--display-name", "X", "--time-zone", "Europe London", "--currency", "GBP"},
			want: "invalid --time-zone",
		},
		{
			name: "currency",
			args: []string{"accounts/123", "--display-name", "X", "--time-zone", "Europe/London", "--currency", "POUND"},
			want: "invalid --currency",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			args := append([]string{"--json", "--dry-run", "--account", "a@b.com", "analytics", "properties", "create"}, testCase.args...)
			result := executeWithAnalyticsAdminTestService(t, args, nil)
			if result.err == nil || !strings.Contains(result.err.Error(), testCase.want) {
				t.Fatalf("err = %v, want %q", result.err, testCase.want)
			}
		})
	}
}
