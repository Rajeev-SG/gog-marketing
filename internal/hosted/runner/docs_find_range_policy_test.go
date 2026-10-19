package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const nativeDocsFindRangeFixture = `{"documentId": "selected-doc", "revisionId": "rev-1", "body": {"content": [{"startIndex": 1, "endIndex": 28, "paragraph": {"elements": [{"startIndex": 1, "endIndex": 28, "textRun": {"content": "😀 Alpha   beta\nALPHA beta\n"}}]}}]}, "headers": {"header-1": {"headerId": "header-1", "content": [{"startIndex": 0, "endIndex": 15, "paragraph": {"elements": [{"startIndex": 0, "endIndex": 15, "textRun": {"content": "Header   Alpha\n"}}]}}]}}, "tabs": [{"tabProperties": {"tabId": "tab-main", "title": "Main"}, "documentTab": {"body": {"content": [{"startIndex": 1, "endIndex": 28, "paragraph": {"elements": [{"startIndex": 1, "endIndex": 28, "textRun": {"content": "😀 Alpha   beta\nALPHA beta\n"}}]}}]}, "headers": {"header-1": {"headerId": "header-1", "content": [{"startIndex": 0, "endIndex": 15, "paragraph": {"elements": [{"startIndex": 0, "endIndex": 15, "textRun": {"content": "Header   Alpha\n"}}]}}]}}}}, {"tabProperties": {"tabId": "tab-other", "title": "Other"}, "documentTab": {"body": {"content": [{"startIndex": 1, "endIndex": 18, "paragraph": {"elements": [{"startIndex": 1, "endIndex": 18, "textRun": {"content": "Other alpha beta\n"}}]}}]}}}]}`

func TestNativeDocsFindRangeSelection(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Docs = func(ctx context.Context, _ string) (*docs.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)

				if r.Method != http.MethodGet || r.URL.Path != "/v1/documents/selected-doc" {
					t.Errorf("unexpected search request %s %s", r.Method, r.URL)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(nativeDocsFindRangeFixture)), Request: r}, nil
			})

			return docs.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	for _, scope := range []string{"documents.readonly", "documents", "drive.file"} {
		for _, tc := range []struct {
			name, text string
			options    map[string]any
			want       [][2]int64
			fail       bool
		}{
			{"first", "alpha beta", nil, [][2]int64{{4, 16}}, false},
			{"all", "alpha beta", map[string]any{"all": true}, [][2]int64{{4, 16}, {17, 27}}, false},
			{"nth", "alpha beta", map[string]any{"occurrence": 2}, [][2]int64{{17, 27}}, false},
			{"case", "Alpha beta", map[string]any{"match_case": true, "all": true}, [][2]int64{{4, 16}}, false},
			{"literal spacing", "alpha beta", map[string]any{"normalize_whitespace": false, "all": true}, [][2]int64{{17, 27}}, false},
			{"utf16", "😀", nil, [][2]int64{{1, 3}}, false},
			{"header", "Header Alpha", map[string]any{"segment": "header-1"}, [][2]int64{{0, 14}}, false},
			{"empty", "missing", nil, nil, false},
			{"fail empty", "missing", map[string]any{"fail_empty": true}, nil, true},
		} {
			args := map[string]any{"document_id": "selected-doc", "tab": "Main", "text": tc.text}
			for key, value := range tc.options {
				args[key] = value
			}

			result := callDocsNamedEdit(t, harness, "docs_find_range", args, scope, false, false, nil)
			if result.OK == tc.fail {
				t.Fatalf("%s/%s success=%t: %s", scope, tc.name, result.OK, result.Body)
			}

			if tc.fail {
				continue
			}

			var output struct {
				Matches   []docsedit.TextRange `json:"matches"`
				TabID     string               `json:"tabId"`
				SegmentID string               `json:"segmentId"`
			}
			if err := json.Unmarshal(result.Result, &output); err != nil {
				t.Fatal(err)
			}

			if len(output.Matches) != len(tc.want) || output.TabID != "tab-main" {
				t.Fatalf("%s/%s wrong target/matches: %s", scope, tc.name, result.Result)
			}

			for i, want := range tc.want {
				got := output.Matches[i]
				if got.StartIndex != want[0] || got.EndIndex != want[1] {
					t.Fatalf("%s/%s UTF16 range=%+v want=%v", scope, tc.name, got, want)
				}
			}

			segment, _ := tc.options["segment"].(string)
			if output.SegmentID != segment {
				t.Fatalf("%s wrong segment: %s", tc.name, result.Result)
			}
		}
	}
	before := calls.Load()

	args := map[string]any{"document_id": "selected-doc", "text": "Alpha"}
	for _, change := range []func(*ToolAuthorization){
		func(a *ToolAuthorization) { a.Resource = nil },
		func(a *ToolAuthorization) { a.Resource.Enabled = false },
		func(a *ToolAuthorization) { a.Resource.ResourceID = "foreign-doc" },
		func(a *ToolAuthorization) { a.AllowedTools = []string{"docs_get"} },
		func(a *ToolAuthorization) { a.Scopes = []string{"https://www.googleapis.com/auth/bigquery"} },
	} {
		if result := callDocsNamedEdit(t, harness, "docs_find_range", args, "documents.readonly", false, false, change); result.OK {
			t.Fatal("ungranted search admitted")
		}
	}

	if result := callDocsNamedEdit(t, harness, "docs_find_range", map[string]any{"document_id": "selected-doc", "text": "Alpha", "all": true, "occurrence": 2}, "documents.readonly", false, false, nil); result.OK {
		t.Fatal("invalid all/occurrence admitted")
	}

	if calls.Load() != before {
		t.Fatal("denied search reached Google")
	}
}

func TestDocsNamedEditExactMethodGuard(t *testing.T) {
	for _, name := range []string{"docs_find_range", "docs_named_range_create", "docs_named_range_delete", "docs_named_range_replace"} {
		guard := nativeRequestGuard(&ToolInvocation{Name: name, Arguments: map[string]any{"document_id": "selected-doc"}, Authorization: ToolAuthorization{Resource: &ResourceGrantInput{ResourceID: "selected-doc"}}})

		base := "https://docs.googleapis.com/v1/documents/selected-doc"
		for _, tc := range []struct {
			method, url string
			allow       bool
		}{
			{http.MethodGet, base, true},
			{http.MethodPost, base + ":batchUpdate", name != "docs_find_range"},
			{http.MethodPost, base, false},
			{http.MethodGet, base + ":batchUpdate", false},
			{http.MethodDelete, base, false},
			{http.MethodGet, base + "/extra", false},
			{http.MethodGet, "https://docs.googleapis.com/v1/documents/foreign-doc", false},
			{http.MethodGet, "https://foreign.example/v1/documents/selected-doc", false},
		} {
			req, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			if err := guard(req); (err == nil) != tc.allow {
				t.Errorf("%s %s %s allow=%t err=%v", name, tc.method, tc.url, tc.allow, err)
			}
		}
	}
}
