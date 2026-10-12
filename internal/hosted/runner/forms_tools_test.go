package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/forms/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeSelectedFormsReadUpdatePublishAndResponses(t *testing.T) {
	var calls atomic.Int64
	title := "Before"
	published := false
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices.Forms = func(ctx context.Context, _ string) (*forms.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				form := map[string]any{"formId": "selected-form", "info": map[string]any{"title": title}, "responderUri": "https://docs.google.com/forms/d/e/respond/viewform", "publishSettings": map[string]any{"publishState": map[string]any{"isPublished": published, "isAcceptingResponses": published}}}
				var body any = form

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/forms/selected-form":
				case r.Method == http.MethodPost && r.URL.Path == "/v1/forms/selected-form:batchUpdate":
					var input forms.BatchUpdateFormRequest
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}

					for _, request := range input.Requests {
						if request.UpdateFormInfo != nil {
							title = request.UpdateFormInfo.Info.Title
						}
					}
					body = map[string]any{"form": map[string]any{"formId": "selected-form", "info": map[string]any{"title": title}}}
				case r.Method == http.MethodPost && r.URL.Path == "/v1/forms/selected-form:setPublishSettings":
					var input forms.SetPublishSettingsRequest
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					published = input.PublishSettings.PublishState.IsPublished
					body = map[string]any{"publishSettings": input.PublishSettings}
				case r.Method == http.MethodGet && r.URL.Path == "/v1/forms/selected-form/responses":
					body = map[string]any{"responses": []any{map[string]any{"responseId": "response-1"}}}
				case r.Method == http.MethodGet && r.URL.Path == "/v1/forms/selected-form/responses/response-1":
					body = map[string]any{"responseId": "response-1"}
				default:
					t.Errorf("foreign or undeclared Forms endpoint: %s %s", r.Method, r.URL)
				}

				data, err := json.Marshal(body)
				if err != nil {
					return nil, fmt.Errorf("encode fixture: %w", err)
				}

				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})

			return forms.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name, scope string, destructive bool, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "forms", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive, Resource: &ResourceGrantInput{Service: "forms", ResourceType: "file", ResourceID: "selected-form", Enabled: true}}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, name := range []string{"forms_get", "forms_update", "forms_get", "forms_publish", "forms_responses_list", "forms_responses_get"} {
		args := map[string]any{"form_id": "selected-form"}
		if name == "forms_update" {
			args["title"] = "After"
		}

		if name == "forms_responses_get" {
			args["response_id"] = "response-1"
		}

		if name == "forms_update" || name == "forms_publish" {
			before := calls.Load()

			denied := call(name, "forms.body.readonly", false, args)
			if denied.OK || calls.Load() != before {
				t.Fatal("read body scope authorized mutation")
			}
		}

		result := call(name, "drive.file", true, args)
		if !result.OK {
			t.Fatalf("native %s failed: %s", name, result.Body)
		}
	}

	if title != "After" || !published {
		t.Fatal("form update/publish did not persist")
	}
	args := map[string]any{"form_id": "selected-form", "unpublish": true}
	before := calls.Load()

	denied := call("forms_publish", "drive.file", false, args)
	if denied.OK || calls.Load() != before {
		t.Fatal("unpublish bypassed destructive permission")
	}

	ended := call("forms_publish", "drive.file", true, args)
	if !ended.OK || published {
		t.Fatalf("native unpublish failed: %s", ended.Body)
	}
	before = calls.Load()

	foreign := call("forms_get", "drive.file", false, map[string]any{"form_id": "foreign-form"})
	if foreign.OK || calls.Load() != before {
		t.Fatal("unselected form reached Google")
	}
}
