package googleapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/bigquerydatatransfer/v1"
	"google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/serviceusage/v1"
)

func newCloudAdminTestAdapter(t *testing.T, handler http.Handler) (*cloudAdminAdapter, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	ctx := context.Background()

	resourceManager, err := cloudresourcemanager.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	billing, err := cloudbilling.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	serviceUsage, err := serviceusage.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	iamService, err := iam.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	dataTransfer, err := bigquerydatatransfer.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	return &cloudAdminAdapter{
		resourceManager: resourceManager,
		billing:         billing,
		serviceUsage:    serviceUsage,
		iam:             iamService,
		dataTransfer:    dataTransfer,
	}, server
}

func TestCloudAdminListProjectsParsesSearchResults(t *testing.T) {
	var body map[string]any

	adapter, server := newCloudAdminTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/projects:search") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)

			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = io.WriteString(w, `{"projects":[{"name":"projects/demo-proj","displayName":"Demo","state":"ACTIVE","projectId":"demo-proj"}]}`)
	}))
	defer server.Close()

	projects, err := adapter.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}

	if len(projects) != 1 || projects[0].ProjectID != "demo-proj" || projects[0].Name != "Demo" {
		t.Fatalf("unexpected projects: %#v", projects)
	}
}

func TestCloudAdminBillingAndServices(t *testing.T) {
	var serviceRead bool

	adapter, server := newCloudAdminTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/billingInfo"):
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/billingInfo","billingAccountName":"billingAccounts/ABC-DEF-123","billingEnabled":true}`)
		case strings.Contains(r.URL.Path, "/services"):
			serviceRead = true
			_, _ = io.WriteString(w, `{"services":[{"name":"projects/demo-proj/services/bigquery.googleapis.com","config":{"title":"BigQuery API"}}]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	billing, err := adapter.GetProjectBillingInfo(context.Background(), "demo-proj")
	if err != nil || !billing.BillingEnabled || billing.BillingAccount != "billingAccounts/ABC-DEF-123" {
		t.Fatalf("unexpected billing: %#v err=%v", billing, err)
	}

	services, err := adapter.ListEnabledServices(context.Background(), "demo-proj")
	if err != nil || !serviceRead {
		t.Fatalf("unexpected services: %#v err=%v", services, err)
	}

	if len(services) != 1 || services[0].APIID != "bigquery.googleapis.com" {
		t.Fatalf("unexpected services parsed: %#v", services)
	}
}

func TestCloudAdminTransferDisableAndDelete(t *testing.T) {
	var patchMask, patchDisabled string
	var deleteCalled bool

	adapter, server := newCloudAdminTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "transferConfigs/tc-1"):
			patchMask = r.URL.Query().Get("updateMask")
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)

			if disabled, ok := body["disabled"].(bool); ok && disabled {
				patchDisabled = "true"
			}
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/locations/us/transferConfigs/tc-1","dataSourceId":"scheduled_query","disabled":true}`)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "transferConfigs/tc-1"):
			deleteCalled = true
			_, _ = io.WriteString(w, "{}")
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	updated, err := adapter.DisableTransferConfig(context.Background(), "projects/demo-proj/locations/us/transferConfigs/tc-1")
	if err != nil || !updated.Disabled || patchMask != "disabled" || patchDisabled != "true" {
		t.Fatalf("unexpected disable: %#v err=%v mask=%q disabled=%q", updated, err, patchMask, patchDisabled)
	}

	if err := adapter.DeleteTransferConfig(context.Background(), "projects/demo-proj/locations/us/transferConfigs/tc-1"); err != nil || !deleteCalled {
		t.Fatalf("unexpected delete: err=%v deleted=%v", err, deleteCalled)
	}
}

func TestCloudAdminServiceEnableDisableAndServiceAccounts(t *testing.T) {
	var enablePath, disablePath string
	var saRead bool

	adapter, server := newCloudAdminTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/services/cloudbilling.googleapis.com:enable"):
			enablePath = r.URL.Path
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/operations/enable-1"}`)
		case strings.Contains(r.URL.Path, "/operations/"):
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/operations/enable-1","done":true}`)
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/services/cloudbilling.googleapis.com:disable"):
			disablePath = r.URL.Path
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/operations/disable-1"}`)
		case strings.Contains(r.URL.Path, "/serviceAccounts"):
			saRead = true
			_, _ = io.WriteString(w, `{"accounts":[{"name":"projects/demo-proj/serviceAccounts/sa@demo-proj.iam.gserviceaccount.com","email":"sa@demo-proj.iam.gserviceaccount.com","uniqueId":"123"}]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	enabled, err := adapter.EnableService(context.Background(), "demo-proj", "cloudbilling.googleapis.com")
	if err != nil || enabled.OperationName == "" || !strings.Contains(enablePath, "/services/cloudbilling.googleapis.com:enable") {
		t.Fatalf("unexpected enable: %#v err=%v path=%q", enabled, err, enablePath)
	}

	disabled, err := adapter.DisableService(context.Background(), "demo-proj", "cloudbilling.googleapis.com")
	if err != nil || disabled.OperationName == "" || !strings.Contains(disablePath, ":disable") {
		t.Fatalf("unexpected disable: %#v err=%v path=%q", disabled, err, disablePath)
	}

	accounts, err := adapter.ListServiceAccounts(context.Background(), "demo-proj")
	if err != nil || !saRead {
		t.Fatalf("unexpected service accounts: %#v err=%v", accounts, err)
	}

	if len(accounts) != 1 || accounts[0].Email != "sa@demo-proj.iam.gserviceaccount.com" {
		t.Fatalf("unexpected accounts parsed: %#v", accounts)
	}
}

func TestCloudAdminOperationPollsToCompletionAndSurfacesFailure(t *testing.T) {
	var polls int

	adapter, server := newCloudAdminTestAdapter(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":enable"):
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/operations/op-1","done":false}`)
		case strings.Contains(r.URL.Path, "/operations/op-1"):
			polls++
			if polls == 1 {
				_, _ = io.WriteString(w, `{"name":"projects/demo-proj/operations/op-1","done":false}`)
				return
			}
			_, _ = io.WriteString(w, `{"name":"projects/demo-proj/operations/op-1","done":true,"error":{"code":7,"message":"permission denied"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	op, err := adapter.EnableService(context.Background(), "demo-proj", "cloudbilling.googleapis.com")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	if op.OperationName == "" || !op.Done {
		t.Fatalf("unexpected operation: %#v", op)
	}

	if op.Error != "permission denied" {
		t.Fatalf("expected operation failure surfaced, got %#v", op)
	}
}
