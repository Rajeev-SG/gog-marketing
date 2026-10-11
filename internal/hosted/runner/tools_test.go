package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/cmd"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestGeneratedWorkerCatalogMatchesExecutableNativeSchemas(t *testing.T) {
	data, err := os.ReadFile("../app/src/tool-catalog.generated.json")
	if err != nil {
		t.Fatal(err)
	}

	var generated any
	if decodeErr := json.Unmarshal(data, &generated); decodeErr != nil {
		t.Fatal(decodeErr)
	}

	encoded, err := json.Marshal(NativeToolCatalog())
	if err != nil {
		t.Fatal(err)
	}

	var current any
	if decodeErr := json.Unmarshal(encoded, &current); decodeErr != nil {
		t.Fatal(decodeErr)
	}

	if !reflect.DeepEqual(generated, current) {
		t.Fatal("Worker catalogue drifted from executable Go schemas; rebuild/run cmd/hosted-catalog")
	}
}

var errUnexpectedNativeFactory = errors.New("unexpected native handler")

func nativeToolBody(t *testing.T, mutate func(*Request)) []byte {
	t.Helper()

	request := Request{
		TenantID: "tenant-a", ConnectionID: "connection-a", GoogleSubject: "google-sub-a",
		GoogleEmail: "owner@example.com", AccessToken: "synthetic-ephemeral-token",
		Operation: OperationToolCall,
		Tool: &ToolInvocation{
			Name: "sheets_read_range", Arguments: map[string]any{"spreadsheet_id": "selected-sheet", "range": "A1"},
			Authorization: ToolAuthorization{
				Service: "sheets", AllowedTools: []string{"sheets_read_range"},
				Scopes:   []string{"https://www.googleapis.com/auth/drive.file"},
				Resource: &ResourceGrantInput{Service: "sheets", ResourceType: "file", ResourceID: "selected-sheet", Enabled: true},
			},
		},
	}
	if mutate != nil {
		mutate(&request)
	}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	return body
}

func TestNativeToolRequestRejectsAuthorityBypassBeforeHandler(t *testing.T) {
	var factoryCalls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Sheets = func(context.Context, string) (*sheets.Service, error) {
			factoryCalls.Add(1)
			return nil, errUnexpectedNativeFactory
		}
	})

	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{"disabled tool", func(r *Request) { r.Tool.Authorization.AllowedTools = nil }},
		{"wildcard tool", func(r *Request) { r.Tool.Authorization.AllowedTools = []string{"*"} }},
		{"excluded tool", func(r *Request) { r.Tool.Name = "gmail_search" }},
		{"restricted scope", func(r *Request) { r.Tool.Authorization.Scopes = []string{"https://www.googleapis.com/auth/drive"} }},
		{"unknown transitive scope", func(r *Request) {
			r.Tool.Authorization.Scopes = append(r.Tool.Authorization.Scopes, "https://www.googleapis.com/auth/unknown.scope")
		}},
		{"missing scope", func(r *Request) { r.Tool.Authorization.Scopes = []string{"openid"} }},
		{"disabled resource", func(r *Request) { r.Tool.Authorization.Resource.Enabled = false }},
		{"foreign resource", func(r *Request) { r.Tool.Arguments["spreadsheet_id"] = "unselected-sheet" }},
		{"resource path traversal", func(r *Request) {
			r.Tool.Arguments["spreadsheet_id"] = "../other"
			r.Tool.Authorization.Resource.ResourceID = "../other"
		}},
		{"foreign service", func(r *Request) { r.Tool.Authorization.Service = "docs" }},
		{"write disabled", func(r *Request) {
			r.Tool.Name = "sheets_update_range"
			r.Tool.Authorization.AllowedTools = []string{r.Tool.Name}
			r.Tool.Arguments["values_json"] = `[["new"]]`
		}},
		{"unselected formula source", func(r *Request) {
			r.Tool.Name = "sheets_update_range"
			r.Tool.Authorization.AllowedTools = []string{r.Tool.Name}
			r.Tool.Authorization.AllowWrite = true
			r.Tool.Arguments["values_json"] = `[["=IMPORTRANGE(\"other-sheet\",\"A1\")"]]`
		}},
		{"report requires data-read scope, not merely admin edit", func(r *Request) {
			r.Tool.Name = "analytics_report"
			r.Tool.Arguments = map[string]any{"property": "properties/123"}
			r.Tool.Authorization = ToolAuthorization{Service: "analytics", AllowedTools: []string{r.Tool.Name}, Scopes: []string{"https://www.googleapis.com/auth/analytics.edit"}, Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := nativeToolBody(t, test.mutate)

			response := postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
			if response.Status != 400 || response.Error.Code != "invalid_request" {
				t.Fatalf("authority bypass was not rejected: %d %s", response.Status, response.Body)
			}
		})
	}

	if factoryCalls.Load() != 0 {
		t.Fatal("denied authority reached native handler")
	}
}

type nativeFixtureTransport func(*http.Request) (*http.Response, error)

func (f nativeFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSignedNativeToolUsesExistingWriteAndReadWithPersistentFixture(t *testing.T) {
	value := "before"
	var requests atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices = app.Services{Sheets: func(ctx context.Context, account string) (*sheets.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted execution identity was lost")
			}
			base := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)

				if r.URL.Hostname() != "sheets.googleapis.com" || r.URL.Path != "/v4/spreadsheets/selected-sheet/values/A1" {
					t.Errorf("unexpected native request: %s", r.URL)
				}
				body := `{"range":"A1","values":[["` + value + `"]]}`

				if r.Method == http.MethodPut {
					var input struct {
						Values [][]string `json:"values"`
					}
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					value = input.Values[0][0]
					body = `{"updatedRange":"A1","updatedCells":1}`
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			client := &http.Client{Transport: googleapi.GuardedTransport(ctx, base)}

			return sheets.NewService(ctx, option.WithHTTPClient(client))
		}}
	})
	write := nativeToolBody(t, func(r *Request) {
		r.Tool.Name = "sheets_update_range"
		r.Tool.Arguments["values_json"] = `[["after"]]`
		r.Tool.Authorization.AllowedTools = []string{r.Tool.Name}
		r.Tool.Authorization.AllowWrite = true
	})

	writeResponse := postExecute(t, harness.server, signInvocation(t, write, func(c map[string]any) { c["operation"] = OperationToolCall }), write)
	if writeResponse.Status != 200 || !writeResponse.OK {
		t.Fatalf("native write failed: %d %s", writeResponse.Status, writeResponse.Body)
	}
	read := nativeToolBody(t, nil)

	readResponse := postExecute(t, harness.server, signInvocation(t, read, func(c map[string]any) { c["operation"] = OperationToolCall }), read)
	if readResponse.Status != 200 || !readResponse.OK || !strings.Contains(string(readResponse.Result), "after") || requests.Load() != 2 {
		t.Fatalf("native persisted read failed: %d %s requests=%d", readResponse.Status, readResponse.Body, requests.Load())
	}
	// Altered tenant/connection context cannot reuse the original capability.
	var tampered Request
	if err := json.Unmarshal(read, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.TenantID = "tenant-b"
	changed, _ := json.Marshal(tampered)

	denial := postExecute(t, harness.server, signInvocation(t, read, func(c map[string]any) { c["operation"] = OperationToolCall }), changed)
	if denial.Status != 401 || requests.Load() != 2 {
		t.Fatal("tampered context reached native execution")
	}
}

func TestNativeRequestGuardRejectsRedirectsOtherResourcesAndUnexpectedMutations(t *testing.T) {
	var request Request
	if err := json.Unmarshal(nativeToolBody(t, nil), &request); err != nil {
		t.Fatal(err)
	}
	guard := nativeRequestGuard(request.Tool)

	for _, address := range []string{
		"https://attacker.example/v4/spreadsheets/selected-sheet/values/A1",
		"http://sheets.googleapis.com/v4/spreadsheets/selected-sheet/values/A1",
		"https://sheets.googleapis.com:444/v4/spreadsheets/selected-sheet/values/A1",
		"https://sheets.googleapis.com/v4/spreadsheets/other-sheet/values/A1",
		"https://sheets.googleapis.com/v4/spreadsheets/selected-sheet:batchUpdate",
		"https://sheets.googleapis.com/v4/spreadsheets/selected-sheet/values/../other",
		"https://sheets.googleapis.com/v4/spreadsheets/selected-sheet/values/A1?access_token=synthetic",
	} {
		parsed, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}

		if err := guard(&http.Request{Method: http.MethodGet, URL: parsed, Header: http.Header{}}); !errors.Is(err, cmd.ErrHostedToolDenied) {
			t.Errorf("unsafe request accepted: %s", address)
		}
	}

	valid, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://sheets.googleapis.com/v4/spreadsheets/selected-sheet/values/A1", nil)
	if err := guard(valid); err != nil {
		t.Fatal(err)
	}

	valid.Method = http.MethodPut
	if guard(valid) == nil {
		t.Fatal("read grant admitted write")
	}
	valid.Method = http.MethodGet
	valid.Header.Set("X-HTTP-Method-Override", "PUT")

	if guard(valid) == nil {
		t.Fatal("method override admitted")
	}
}

func TestNativeBindingsReuseMatchingExistingSchemas(t *testing.T) {
	existing := map[string]cmd.HostedTool{}
	for _, tool := range cmd.HostedExistingToolCatalog() {
		existing[tool.Tool.Name] = tool
	}

	for _, binding := range nativeToolBindings {
		tool, ok := existing[binding.Name]
		if !ok || tool.Service != binding.Service || (tool.Risk == "write") != binding.Write {
			t.Errorf("native schema/policy mismatch: %s", binding.Name)
		}
	}
}

func TestSignedNativeGA4ReadsUseExistingHandlersAndGuardedProperty(t *testing.T) {
	var calls atomic.Int64

	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		client := func(ctx context.Context) *http.Client {
			return &http.Client{Transport: googleapi.GuardedTransport(ctx, nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				body := `{"dataStreams":[{"name":"properties/123/dataStreams/456","type":"WEB_DATA_STREAM"}]}`

				switch r.URL.Hostname() {
				case "analyticsdata.googleapis.com":
					if r.URL.Path != "/v1beta/properties/123:runReport" || r.Method != http.MethodPost {
						t.Errorf("unexpected report request: %s %s", r.Method, r.URL)
					}
					body = `{"dimensionHeaders":[{"name":"date"}],"metricHeaders":[{"name":"activeUsers","type":"TYPE_INTEGER"}],"rows":[{"dimensionValues":[{"value":"20261011"}],"metricValues":[{"value":"3"}]}],"rowCount":1}`
				case "analyticsadmin.googleapis.com":
					if r.URL.Path != "/v1beta/properties/123/dataStreams" || r.Method != http.MethodGet {
						t.Errorf("unexpected stream request: %s %s", r.Method, r.URL)
					}
				default:
					t.Errorf("foreign Google service: %s", r.URL.Hostname())
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))}
		}
		engine.NativeServices.AnalyticsAdmin = func(ctx context.Context, _ string) (*analyticsadmin.Service, error) {
			return analyticsadmin.NewService(ctx, option.WithHTTPClient(client(ctx)))
		}
		engine.NativeServices.AnalyticsData = func(ctx context.Context, _ string) (*analyticsdata.Service, error) {
			return analyticsdata.NewService(ctx, option.WithHTTPClient(client(ctx)))
		}
	})
	for _, name := range []string{"analytics_report", "analytics_datastreams_list"} {
		body := nativeToolBody(t, func(r *Request) {
			r.Tool.Name = name
			r.Tool.Arguments = map[string]any{"property": "properties/123"}
			r.Tool.Authorization = ToolAuthorization{Service: "analytics", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/analytics.readonly"}, Resource: &ResourceGrantInput{Service: "analytics", ResourceType: "property", ResourceID: "properties/123", Enabled: true}}
		})

		response := postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
		if response.Status != 200 || !response.OK {
			t.Fatalf("native %s failed: %d %s", name, response.Status, response.Body)
		}
	}

	if calls.Load() != 2 {
		t.Fatalf("native calls=%d, want 2", calls.Load())
	}
}
