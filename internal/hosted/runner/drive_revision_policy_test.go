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

func TestNativeDriveRevisionMetadata(t *testing.T) {
	calls := 0
	const fields = "id,mimeType,modifiedTime,keepForever,published,publishAuto,publishedOutsideDomain,publishedLink,lastModifyingUser,md5Checksum,size,originalFilename,exportLinks"
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Drive = func(ctx context.Context, account string) (*drive.Service, error) {
			if account != "owner@example.com" || !googleapi.NoInputFromContext(ctx) || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" {
				t.Error("trusted Drive identity lost")
			}
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls++

				if r.Method != http.MethodGet || r.URL.Hostname() != "www.googleapis.com" || r.URL.Query().Get("alt") != "json" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				q := r.URL.Query()
				var payload string

				switch r.URL.Path {
				case "/drive/v3/files/selected-file/revisions":
					if q.Get("pageSize") != "2" || q.Get("fields") != "nextPageToken,revisions("+fields+")" {
						t.Errorf("wrong list fields/page size %s", r.URL)
					}

					switch q.Get("pageToken") {
					case "":
						payload = `{"revisions":[{"id":"r1","modifiedTime":"2026-10-19T00:00:00Z"}],"nextPageToken":"next&cursor=2"}`
					case "next&cursor=2":
						payload = `{"revisions":[{"id":"r2","keepForever":true}]}`
					default:
						t.Errorf("unexpected cursor %q", q.Get("pageToken"))
						payload = `{}`
					}
				case "/drive/v3/files/selected-file/revisions/r2":
					if q.Get("fields") != fields {
						t.Errorf("wrong get fields %s", r.URL)
					}
					payload = `{"id":"r2","keepForever":true,"exportLinks":{"application/pdf":"https://foreign.test/content"}}`
				default:
					t.Errorf("foreign request reached transport %s", r.URL)
					payload = `{}`
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}, nil
			})

			return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})
	call := func(name string, args map[string]any, change func(*ToolAuthorization)) capturedResponse {
		authorization := ToolAuthorization{Service: "drive", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, Resource: &ResourceGrantInput{Service: "drive", ResourceType: "file", ResourceID: "selected-file", Enabled: true}}
		if change != nil {
			change(&authorization)
		}
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: authorization}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	first := call("drive_revisions_list", map[string]any{"file_id": "selected-file", "max_results": 2}, nil)
	var page struct {
		Revisions     []drive.Revision `json:"revisions"`
		NextPageToken string           `json:"nextPageToken"`
	}

	if !first.OK {
		t.Fatal(first.Body)
	}

	if err := json.Unmarshal(first.Result, &page); err != nil || len(page.Revisions) != 1 || page.Revisions[0].Id != "r1" || page.NextPageToken != "next&cursor=2" || calls != 1 {
		t.Fatalf("wrong first page %s err=%v calls=%d", first.Result, err, calls)
	}
	second := call("drive_revisions_list", map[string]any{"file_id": "selected-file", "max_results": 2, "page_token": page.NextPageToken}, nil)
	page.NextPageToken = ""

	if !second.OK {
		t.Fatal(second.Body)
	}

	if err := json.Unmarshal(second.Result, &page); err != nil || len(page.Revisions) != 1 || page.Revisions[0].Id != "r2" || !page.Revisions[0].KeepForever || page.NextPageToken != "" || calls != 2 {
		t.Fatalf("wrong second page %s err=%v", second.Result, err)
	}
	get := call("drive_revisions_get", map[string]any{"file_id": "selected-file", "revision_id": "r2"}, nil)
	var selected struct {
		FileID   string         `json:"fileId"`
		Revision drive.Revision `json:"revision"`
	}

	if !get.OK {
		t.Fatal(get.Body)
	}

	if err := json.Unmarshal(get.Result, &selected); err != nil || selected.FileID != "selected-file" || selected.Revision.Id != "r2" || selected.Revision.ExportLinks["application/pdf"] != "https://foreign.test/content" || calls != 3 {
		t.Fatalf("wrong selected metadata %s err=%v calls=%d", get.Result, err, calls)
	}

	for _, name := range []string{"drive_revisions_list", "drive_revisions_get"} {
		args := map[string]any{"file_id": "selected-file"}
		if name == "drive_revisions_get" {
			args["revision_id"] = "r2"
		}

		denials := []func(*ToolAuthorization){
			func(a *ToolAuthorization) { a.Resource = nil },
			func(a *ToolAuthorization) { a.Resource.Enabled = false },
			func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-file" },
			func(a *ToolAuthorization) { a.AllowedTools = []string{"drive_get"} },
			func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"} },
			func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/drive"} },
		}
		for _, change := range denials {
			if result := call(name, args, change); result.OK || calls != 3 {
				t.Fatalf("unauthorized revision reached Google: %s calls=%d", result.Body, calls)
			}
		}
	}
}

func TestDriveRevisionExactRequestGuard(t *testing.T) {
	for _, name := range []string{"drive_revisions_list", "drive_revisions_get"} {
		invocation := &ToolInvocation{Name: name, Arguments: map[string]any{"file_id": "selected-file", "revision_id": "r2"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-file"}}}
		guard := nativeRequestGuard(invocation)
		root := "https://www.googleapis.com/drive/v3/files/selected-file/revisions"

		selected := root
		if name == "drive_revisions_get" {
			selected += "/r2"
		}

		for _, test := range []struct {
			method, url string
			allowed     bool
		}{
			{http.MethodGet, selected, true},
			{http.MethodHead, selected, false},
			{http.MethodPost, selected, false},
			{http.MethodDelete, selected, false},
			{http.MethodGet, "https://www.googleapis.com/drive/v3/files/foreign-file/revisions/r2", false},
			{http.MethodGet, "https://foreign.test/drive/v3/files/selected-file/revisions/r2", false},
			{http.MethodGet, root + "/foreign-revision", false},
			{http.MethodGet, root + "/r2/download", false},
		} {
			request, err := http.NewRequestWithContext(context.Background(), test.method, test.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(request); (err == nil) != test.allowed {
				t.Errorf("%s %s allowed=%t err=%v", test.method, test.url, test.allowed, err)
			}
		}
	}
}
