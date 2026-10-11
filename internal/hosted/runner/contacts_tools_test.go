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
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestNativeContactsCRUDPreservesETagAndPermissionBoundaries(t *testing.T) {
	var calls atomic.Int64
	var stored people.Person
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.Timeout = 20 * time.Second
		engine.NativeServices.PeopleContacts = func(ctx context.Context, _ string) (*people.Service, error) {
			transport := nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				var body any = &stored
				status := 200

				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/people:createContact":
					if err := json.NewDecoder(r.Body).Decode(&stored); err != nil {
						t.Error(err)
					}
					stored.ResourceName = "people/c7"
					stored.Etag = "person-etag"
					stored.Metadata = &people.PersonMetadata{Sources: []*people.Source{{Type: "CONTACT", Id: "7", Etag: "source-etag"}}}
				case r.Method == http.MethodGet && r.URL.Path == "/v1/people/c7":
				case r.Method == http.MethodGet && r.URL.Path == "/v1/people/me/connections":
					body = map[string]any{"connections": []*people.Person{&stored}}
				case r.Method == http.MethodGet && r.URL.Path == "/v1/people:searchContacts":
					body = map[string]any{"results": []any{map[string]any{"person": &stored}}}
				case r.Method == http.MethodPatch && r.URL.Path == "/v1/people/c7:updateContact":
					var patch people.Person
					if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
						t.Error(err)
					}

					if patch.Etag != "person-etag" || patch.Metadata == nil || len(patch.Metadata.Sources) == 0 || patch.Metadata.Sources[0].Etag != "source-etag" {
						t.Errorf("native etag concurrency protection was lost: %+v", patch)
					}

					if !strings.Contains(r.URL.Query().Get("updatePersonFields"), "names") {
						t.Error("native update mask was lost")
					}
					stored = patch
				case r.Method == http.MethodDelete && r.URL.Path == "/v1/people/c7:deleteContact":
					stored = people.Person{}
					status = 204
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					status = 500
				}

				encoded, err := json.Marshal(body)
				if err != nil {
					return nil, fmt.Errorf("encode fixture: %w", err)
				}

				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r}, nil
			})

			return people.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, transport)}))
		}
	})

	call := func(name, scope string, destructive bool, args map[string]any) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: ToolAuthorization{Service: "contacts", AllowedTools: []string{name}, Scopes: []string{"https://www.googleapis.com/auth/" + scope}, AllowWrite: true, AllowDestructive: destructive}}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(claims map[string]any) { claims["operation"] = OperationToolCall }), body)
	}
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"contacts_create", map[string]any{"given": "Before", "email": "before@example.test", "url": []string{"https://example.test/one", "https://example.test/two"}, "custom": []string{"beta=value"}}},
		{"contacts_list", map[string]any{}},
		{"contacts_search", map[string]any{"query": "Before"}},
		{"contacts_get", map[string]any{"resource_name": "people/c7"}},
		{"contacts_update", map[string]any{"resource_name": "people/c7", "given": "After", "email": "", "url": []string{}}},
		{"contacts_get", map[string]any{"resource_name": "people/c7"}},
		{"contacts_delete", map[string]any{"resource_name": "people/c7"}},
	} {
		binding, _ := nativeBinding(test.name)
		if binding.Write {
			before := calls.Load()

			denied := call(test.name, "contacts.readonly", true, test.args)
			if denied.OK || calls.Load() != before {
				t.Fatal("read scope authorized contact mutation")
			}
		}

		if binding.destructive {
			before := calls.Load()

			denied := call(test.name, "contacts", false, test.args)
			if denied.OK || calls.Load() != before {
				t.Fatal("contact delete bypassed destructive permission")
			}
		}

		response := call(test.name, "contacts", true, test.args)
		if !response.OK {
			t.Fatalf("native %s failed: %s", test.name, response.Body)
		}

		if test.name == "contacts_update" && (len(stored.Names) == 0 || stored.Names[0].GivenName != "After" || len(stored.EmailAddresses) != 0 || len(stored.Urls) != 0) {
			t.Fatalf("native update/clear did not persist: %+v", stored)
		}
	}

	if stored.ResourceName != "" {
		t.Fatal("native deletion did not persist")
	}
	before := calls.Load()

	denied := call("contacts_get", "contacts", false, map[string]any{"resource_name": "people/../foreign"})
	if denied.OK || calls.Load() != before {
		t.Fatal("invalid contact selector reached Google")
	}
}
