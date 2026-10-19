package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclaw/gogcli/internal/app"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestDocsTextCommandsDoNotReplayUncertainWrites(t *testing.T) {
	for _, args := range [][]string{
		{"insert", "selected-doc", "X", "--index=1"},
		{"delete", "selected-doc", "--start=1", "--end=2"},
		{"clear", "selected-doc"},
		{"find-replace", "selected-doc", "a", "aa", "--format=plain"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var effects atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"documentId":"selected-doc","revisionId":"rev-1","body":{"content":[{"startIndex":1,"endIndex":5,"paragraph":{"elements":[{"startIndex":1,"endIndex":5,"textRun":{"content":"abc\n"}}]}}]}}`))
					return
				}
				effects.Add(1)
				if effects.Load() == 1 {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":{"code":503,"message":"uncertain after effect"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"documentId":"selected-doc","replies":[{"replaceAllText":{"occurrencesChanged":1}}]}`))
			}))
			defer server.Close()
			retry := googleapi.NewRetryTransport(server.Client().Transport)
			retry.MaxRetries5xx = 1
			svc, err := docs.NewService(t.Context(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(&http.Client{Transport: retry}))
			if err != nil {
				t.Fatal(err)
			}
			result := executeDocsTextTestService(t, append([]string{"--account=a@b.com", "--json", "docs"}, args...), svc)
			if result.err == nil || effects.Load() != 1 {
				t.Fatalf("uncertain text edit replayed effects=%d result=%+v", effects.Load(), result)
			}
		})
	}
}

func TestDocsClearPreservesNativeBodyAndNoOpSemantics(t *testing.T) {
	for _, text := range []string{"abc\n", "  \n", "\n"} {
		posts := 0
		svc, _ := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(map[string]any{"documentId": "selected-doc", "revisionId": "rev-1", "body": map[string]any{"content": []any{map[string]any{"startIndex": 1, "endIndex": 1 + len(text), "paragraph": map[string]any{"elements": []any{map[string]any{"textRun": map[string]any{"content": text}}}}}}}})
				return
			}
			posts++
			var batch docs.BatchUpdateDocumentRequest
			if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
				t.Fatal(err)
			}
			if len(batch.Requests) != 1 || batch.Requests[0].DeleteContentRange == nil || batch.Requests[0].DeleteContentRange.Range.StartIndex != 1 || batch.Requests[0].DeleteContentRange.Range.EndIndex != int64(len(text)) || batch.WriteControl == nil || batch.WriteControl.RequiredRevisionId != "rev-1" {
				t.Errorf("bad clear batch %+v", batch)
			}
			_, _ = w.Write([]byte(`{"documentId":"selected-doc"}`))
		}))
		result := executeDocsTextTestService(t, []string{"--account=a@b.com", "--json", "docs", "clear", "selected-doc"}, svc)
		if result.err != nil {
			t.Fatalf("clear%q %+v", text, result)
		}
		want := 0
		if strings.TrimSpace(text) != "" {
			want = 1
		}
		if posts != want {
			t.Fatalf("clear%q posts%d want%d", text, posts, want)
		}
	}
}

func executeDocsTextTestService(t *testing.T, args []string, svc *docs.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{Docs: func(context.Context, string) (*docs.Service, error) { return svc, nil }}})
}
