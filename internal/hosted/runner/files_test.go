package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func selectedFilesBody(t *testing.T, scopes []string, ids []string) []byte {
	t.Helper()
	request := Request{
		TenantID: "tenant-a", ConnectionID: "connection-a", Operation: OperationVerifyFiles,
		GoogleEmail: "owner@example.com", GoogleSubject: "google-sub-a", AccessToken: "synthetic",
		FileSelection: &FileSelection{IDs: ids, Scopes: scopes},
	}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	return body
}

func TestSelectedFileVerificationUsesTypedDriveAndPreservesPartialFailures(t *testing.T) {
	var calls atomic.Int64
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Drive = func(ctx context.Context, _ string) (*drive.Service, error) {
			client := &http.Client{Transport: googleapi.GuardedTransport(ctx, nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				status := 200
				body := `{"id":"selected-doc","name":"Trusted Google name","mimeType":"application/vnd.google-apps.document"}`

				if r.URL.Path == "/drive/v3/files/denied" {
					status = 403
					body = `{"error":{"message":"provider-sensitive-text"}}`
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))}

			return drive.NewService(ctx, option.WithHTTPClient(client))
		}
	})
	body := selectedFilesBody(t, []string{"https://www.googleapis.com/auth/drive.file"}, []string{"selected-doc", "denied"})

	response := postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationVerifyFiles }), body)
	if response.Status != 200 || !response.OK {
		t.Fatalf("verification failed: %d %s", response.Status, response.Body)
	}

	var result FileVerificationResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 1 || result.Files[0].ID != "selected-doc" || result.Files[0].Name != "Trusted Google name" || len(result.Rejected) != 1 || result.Rejected[0] != "denied" || calls.Load() != 2 {
		t.Fatalf("wrong partial result: %+v", result)
	}

	if strings.Contains(response.Body, "provider-sensitive-text") {
		t.Fatal("provider error leaked")
	}
}

func TestSelectedFileVerificationRejectsRestrictedScopesAndInvalidIDs(t *testing.T) {
	harness := newHarness(t, nil)
	for _, selection := range []FileSelection{
		{Scopes: []string{"https://www.googleapis.com/auth/drive"}, IDs: []string{"x"}},
		{Scopes: []string{"https://www.googleapis.com/auth/drive.file", "https://www.googleapis.com/auth/adwords"}, IDs: []string{"x"}},
		{Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, IDs: []string{"../x"}},
		{Scopes: []string{"https://www.googleapis.com/auth/drive.file"}, IDs: []string{"x", "x"}},
	} {
		body := selectedFilesBody(t, selection.Scopes, selection.IDs)

		response := postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationVerifyFiles }), body)
		if response.Status != 400 {
			t.Fatalf("unsafe verification accepted: %d %s", response.Status, response.Body)
		}
	}
}
