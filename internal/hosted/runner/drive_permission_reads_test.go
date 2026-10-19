package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeDrivePermissionReads(t *testing.T) {
	calls := 0
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Drive = func(ctx context.Context, account string) (*drive.Service, error) {
			if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
				t.Error("trusted identity lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls++

				if r.Method != http.MethodGet || r.URL.Hostname() != "www.googleapis.com" {
					t.Errorf("foreign request %s %s", r.Method, r.URL)
				}
				payload := `{}`

				switch r.URL.Path {
				case "/drive/v3/files/selected-file":
					payload = `{"id":"selected-file","name":"Selected","mimeType":"application/vnd.google-apps.document","webViewLink":"https://foreign.example/never-follow","owners":[{"emailAddress":"owner@example.com"}]}`
				case "/drive/v3/files/selected-file/permissions":
					switch r.URL.Query().Get("pageToken") {
					case "":
						payload = `{"permissions":[{"id":"public","type":"anyone","role":"reader"},{"id":"guest","type":"user","role":"reader","emailAddress":"guest@outside.test"}],"nextPageToken":"next&cursor=2"}`
					case "next&cursor=2":
						payload = `{"permissions":[{"id":"owner","type":"user","role":"owner","emailAddress":"owner@example.com"}]}`
					default:
						t.Errorf("wrong page token %s", r.URL)
					}
				default:
					t.Errorf("enumeration/foreign path reached transport: %s", r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}, nil
			})

			return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name string, args map[string]any, change func(*ToolAuthorization)) capturedResponse {
		auth := ToolAuthorization{Service: "drive", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, Resource: &ResourceGrantInput{Service: "drive", ResourceType: "file", ResourceID: "selected-file", Enabled: true}}
		if change != nil {
			change(&auth)
		}
		body := nativeToolBody(t, func(req *Request) { req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth} })

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	page := call("drive_permissions", map[string]any{"file_id": "selected-file", "max_results": 2}, nil)
	var list struct {
		Permissions   []drive.Permission `json:"permissions"`
		NextPageToken string             `json:"nextPageToken"`
	}

	if !page.OK {
		t.Fatal(page.Body)
	}

	if err := json.Unmarshal(page.Result, &list); err != nil || len(list.Permissions) != 2 || list.NextPageToken != "next&cursor=2" || calls != 1 {
		t.Fatalf("bad bounded page %s calls=%d err=%v", page.Result, calls, err)
	}

	for _, tc := range []struct {
		name string
		args map[string]any
		want int
	}{
		{"drive_audit_sharing", map[string]any{"file_id": "selected-file"}, 2},
		{"drive_audit_user", map[string]any{"file_id": "selected-file", "user": "guest@outside.test"}, 1},
	} {
		before := calls
		result := call(tc.name, tc.args, nil)
		var findings struct {
			Count   int `json:"findingCount"`
			Scanned int `json:"scannedFileCount"`
		}

		if !result.OK {
			t.Fatalf("%s failed %s", tc.name, result.Body)
		}

		if err := json.Unmarshal(result.Result, &findings); err != nil || findings.Count != tc.want || findings.Scanned != 1 || calls != before+3 {
			t.Fatalf("wrong audit %s calls=%d err=%v", result.Result, calls-before, err)
		}
	}
	before := calls

	for _, name := range []string{"drive_permissions", "drive_audit_sharing", "drive_audit_user"} {
		args := map[string]any{"file_id": "selected-file"}
		if name == "drive_audit_user" {
			args["user"] = "guest@outside.test"
		}

		for _, mutate := range []func(*ToolAuthorization){
			func(a *ToolAuthorization) { a.AllowedTools = []string{"drive_get"} },
			func(a *ToolAuthorization) { a.Resource.Enabled = false },
			func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-file" },
			func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/documents"} },
		} {
			if call(name, args, mutate).OK {
				t.Fatalf("%s accepted missing authority", name)
			}
		}
	}

	if calls != before {
		t.Fatalf("denials reached Google %d extra calls", calls-before)
	}
}

func TestDrivePermissionReadRequestGuards(t *testing.T) {
	for _, name := range []string{"drive_permissions", "drive_audit_sharing", "drive_audit_user"} {
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"file_id": "selected-file"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-file"}}}
		guard := nativeRequestGuard(invocation)
		base := "https://www.googleapis.com/drive/v3/files/selected-file"

		for _, suffix := range []string{"/permissions", ""} {
			for _, query := range []string{"", "?alt=json", "?alt=media", "?alt=json&alt=media", "?alt=media&alt=json"} {
				request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+suffix+query, nil)
				if err != nil {
					t.Fatal(err)
				}

				want := (suffix == "/permissions" || name != "drive_permissions") && (query == "" || query == "?alt=json")
				if err := guard(request); (err == nil) != want {
					t.Errorf("%s %s allowed=%t err=%v", name, request.URL, want, err)
				}
			}
		}

		for _, target := range []struct{ method, url string }{
			{http.MethodPost, base + "/permissions"}, {http.MethodDelete, base + "/permissions"}, {http.MethodGet, "https://foreign.example/drive/v3/files/selected-file/permissions"}, {http.MethodGet, "https://www.googleapis.com/drive/v3/files/foreign/permissions"}, {http.MethodGet, "https://www.googleapis.com/drive/v3/files"}, {http.MethodGet, base + "/permissions/user"},
		} {
			request, err := http.NewRequestWithContext(t.Context(), target.method, target.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); err == nil {
				t.Errorf("%s accepted foreign request %s %s", name, target.method, target.url)
			}
		}
	}
}
