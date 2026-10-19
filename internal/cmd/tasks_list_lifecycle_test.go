package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestTasksListLifecycleAndMoveRequests(t *testing.T) {
	const list = "list-source-1234567890"
	title := "Original"
	deleted := false
	events := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		events = append(events, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/v1/users/@me/lists/" + list:
			switch r.Method {
			case http.MethodGet:
			case http.MethodPatch:
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body) != 1 || body["title"] != "Renamed 😀" {
					t.Errorf("rename changed non-title fields: %v", body)
				}
				title = body["title"].(string)
			case http.MethodDelete:
				deleted = true
				w.WriteHeader(204)
				return
			default:
				t.Errorf("wrong list method %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": list, "title": title, "etag": "keep-etag", "updated": "2026-10-19T00:00:00Z"})
		case "/tasks/v1/lists/" + list + "/tasks/task-1/move":
			if r.Method != http.MethodPost {
				t.Error("move must POST")
			}
			q := r.URL.Query()
			if len(events) == 4 {
				if q.Get("parent") != "parent-1" || q.Get("previous") != "previous-1" || q.Get("destinationTasklist") != "list-dest-1234567890" {
					t.Errorf("cross-list move query %v", q)
				}
			} else if q.Has("parent") || q.Has("previous") || q.Has("destinationTasklist") {
				t.Errorf("omitted selectors were sent %v", q)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "task-new", "title": "Moved", "parent": q.Get("parent"), "position": "00000001"})
		default:
			t.Errorf("undeclared/lookup endpoint %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	svc := newTasksServiceFromServer(t, server)
	for _, args := range [][]string{
		{"lists", "get", list},
		{"lists", "rename", list, "Renamed 😀"},
		{"lists", "get", list},
		{"move", list, "task-1", "--parent=parent-1", "--previous=previous-1", "--destination-tasklist-id=list-dest-1234567890"},
		{"move", list, "task-1"},
		{"lists", "delete", list, "--force"},
	} {
		result := executeWithTasksTestService(t, append([]string{"--account=a@b.com", "--json", "tasks"}, args...), svc)
		if result.err != nil {
			t.Fatalf("args%v result%+v", args, result)
		}
	}
	if len(events) != 6 || title != "Renamed 😀" || !deleted {
		t.Fatalf("lifecycle %v title=%s deleted=%t", events, title, deleted)
	}
}

func TestTasksLifecycleInputAndDryRunBeforeService(t *testing.T) {
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--dry-run", "tasks", "lists", "rename", "list-source-1234567890", "New"}, true},
		{[]string{"--dry-run", "tasks", "lists", "delete", "list-source-1234567890"}, true},
		{[]string{"--dry-run", "tasks", "move", "list-source-1234567890", "task-1"}, true},
		{[]string{"tasks", "lists", "delete", "@default", "--force"}, false},
		{[]string{"tasks", "lists", "delete", "default", "--force"}, false},
		{[]string{"tasks", "lists", "delete", "list-source-1234567890"}, false},
		{[]string{"tasks", "lists", "rename", "list-source-1234567890", strings.Repeat("😀", 1025)}, false},
		{[]string{"tasks", "lists", "rename", "a title", "New"}, false},
		{[]string{"tasks", "move", "list-source-1234567890", "task-1", "--parent=task-1"}, false},
		{[]string{"tasks", "move", "list-source-1234567890", "task-1", "--destination-tasklist-id=../foreign"}, false},
		{[]string{"--read-only", "tasks", "lists", "rename", "list-source-1234567890", "New"}, false},
	} {
		result := executeWithTasksTestServiceFactory(t, append([]string{"--account=a@b.com", "--no-input"}, tc.args...), unexpectedTasksTestService(t, "invalid/dry-run action requested a service"))
		if (result.err == nil) != tc.ok {
			t.Fatalf("args%v result%+v", tc.args, result)
		}
	}
	if title, err := tasksLifecycleTitle(strings.Repeat("😀", 1024)); err != nil || len(title) != 4096 {
		t.Fatal("valid1024unicodecharacters rejected")
	}
}

func TestTasksLifecycleMutationsDoNotRetryUncertainEffects(t *testing.T) {
	for _, args := range [][]string{
		{"lists", "rename", "list-source-1234567890", "New"},
		{"lists", "delete", "list-source-1234567890", "--force"},
		{"move", "list-source-1234567890", "task-1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var effects atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				effects.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if effects.Load() == 1 {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":{"code":503,"message":"uncertain"}}`))
					return
				}
				if r.Method == http.MethodDelete {
					w.WriteHeader(204)
					return
				}
				_, _ = w.Write([]byte(`{"id":"id-after-effect","title":"New"}`))
			}))
			defer server.Close()
			retry := googleapi.NewRetryTransport(server.Client().Transport)
			retry.MaxRetries5xx = 1
			svc, err := tasks.NewService(t.Context(), option.WithEndpoint(server.URL+"/"), option.WithHTTPClient(&http.Client{Transport: retry}))
			if err != nil {
				t.Fatal(err)
			}
			result := executeWithTasksTestService(t, append([]string{"--account=a@b.com", "--json", "tasks"}, args...), svc)
			if result.err == nil || effects.Load() != 1 {
				t.Fatalf("uncertain mutation repeated effects=%d result=%+v", effects.Load(), result)
			}
		})
	}
}
