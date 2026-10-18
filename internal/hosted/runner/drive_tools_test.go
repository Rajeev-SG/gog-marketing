package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const (
	driveSelectedID = "selectedfile"
	driveCopiedID   = "copiedfile"
)

var driveNotFoundBody = `{"error":{"code":404,"message":"File not found","errors":[{"domain":"global","reason":"notFound","message":"File not found"}],"status":"NOT_FOUND"}}`

type driveFileState struct {
	ID       string
	Name     string
	MimeType string
	Trashed  bool
	NotFound bool
}

type driveFixture struct {
	files     map[string]*driveFileState
	calls     atomic.Int64
	gets      atomic.Int64
	mutations atomic.Int64
	lastPatch string
	lastCopy  string
}

func driveAuthorization(name string, allowWrite, allowDestructive bool, resourceID string) ToolAuthorization {
	return ToolAuthorization{
		Service:          "drive",
		AllowedTools:     []string{name},
		Scopes:           []string{"https://www.googleapis.com/auth/drive.file"},
		AllowWrite:       allowWrite,
		AllowDestructive: allowDestructive,
		Resource:         &ResourceGrantInput{Service: "drive", ResourceType: "file", ResourceID: resourceID, Enabled: true},
	}
}

func (f *driveFixture) transport(t *testing.T) http.RoundTripper {
	t.Helper()

	return nativeFixtureTransport(func(r *http.Request) (*http.Response, error) {
		f.calls.Add(1)
		respond := func(status int, payload string) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(payload)),
				Request:    r,
			}, nil
		}

		if r.URL.Hostname() != "www.googleapis.com" {
			t.Errorf("unexpected Drive host: %s", r.URL)
		}
		metadata := func(file *driveFileState) string {
			encoded, err := json.Marshal(struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				MimeType string `json:"mimeType"`
				Trashed  bool   `json:"trashed"`
			}{file.ID, file.Name, file.MimeType, file.Trashed})
			if err != nil {
				t.Errorf("metadata encode: %v", err)
			}

			return string(encoded)
		}

		switch {
		case r.Method == http.MethodGet:
			f.gets.Add(1)

			file := f.files[strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")]
			if file == nil || file.NotFound {
				return respond(404, driveNotFoundBody)
			}

			return respond(200, metadata(file))
		case r.Method == http.MethodPatch:
			f.mutations.Add(1)

			file := f.files[strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")]
			if file == nil || file.NotFound {
				return respond(404, driveNotFoundBody)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("patch body read: %v", err)
			}
			f.lastPatch = string(body)
			var patch struct {
				Name    string `json:"name"`
				Trashed *bool  `json:"trashed"`
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()

			if err := decoder.Decode(&patch); err != nil {
				t.Errorf("unexpected patch body: %s", body)
			}

			switch {
			case patch.Trashed != nil:
				file.Trashed = *patch.Trashed
			case patch.Name != "":
				file.Name = patch.Name
			default:
				t.Errorf("patch changed nothing: %s", body)
			}

			return respond(200, metadata(file))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/copy"):
			f.mutations.Add(1)

			file := f.files[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/drive/v3/files/"), "/copy")]
			if file == nil || file.NotFound {
				return respond(404, driveNotFoundBody)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("copy body read: %v", err)
			}
			f.lastCopy = string(body)
			var copyArgs struct {
				Name string `json:"name"`
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()

			if err := decoder.Decode(&copyArgs); err != nil {
				t.Errorf("copy must send only a new name, got: %s", body)
			}

			if copyArgs.Name == "" {
				t.Errorf("copy missing new name: %s", body)
			}
			copied := &driveFileState{ID: driveCopiedID, Name: copyArgs.Name, MimeType: "text/plain"}
			f.files[driveCopiedID] = copied

			return respond(200, metadata(copied))
		default:
			t.Errorf("restricted/undeclared Drive endpoint: %s %s", r.Method, r.URL)
			return respond(403, `{"error":{"code":403,"message":"forbidden"}}`)
		}
	})
}

func driveServiceFixture(t *testing.T, state *driveFixture) func(context.Context, string) (*drive.Service, error) {
	t.Helper()

	return func(ctx context.Context, account string) (*drive.Service, error) {
		if account != "owner@example.com" || authclient.AccessTokenFromContext(ctx) != "synthetic-ephemeral-token" || !googleapi.NoInputFromContext(ctx) {
			t.Error("trusted execution identity was lost")
		}

		return drive.NewService(ctx, option.WithHTTPClient(&http.Client{Transport: googleapi.GuardedTransport(ctx, state.transport(t))}))
	}
}

func TestSignedNativeDriveRenameCopyTrashUnderSelectedFileGrant(t *testing.T) {
	state := &driveFixture{files: map[string]*driveFileState{
		driveSelectedID: {ID: driveSelectedID, Name: "Report.pdf", MimeType: "text/plain"},
	}}
	harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
		engine.NativeServices = app.Services{Drive: driveServiceFixture(t, state)}
	})

	call := func(name string, args map[string]any, auth ToolAuthorization, mutate func(*Request)) capturedResponse {
		body := nativeToolBody(t, func(req *Request) {
			req.Tool = &ToolInvocation{Name: name, Arguments: args, Authorization: auth}
			if mutate != nil {
				mutate(req)
			}
		})

		return postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
	}

	renameAuth := driveAuthorization("drive_rename", true, false, driveSelectedID)
	renameArgs := map[string]any{"file_id": driveSelectedID, "new_name": "Renamed.pdf"}

	rename := call("drive_rename", renameArgs, renameAuth, nil)
	if rename.Status != 200 || !rename.OK {
		t.Fatalf("native rename failed: %d %s", rename.Status, rename.Body)
	}

	if state.calls.Load() != 1 || state.gets.Load() != 0 || state.mutations.Load() != 1 {
		t.Fatalf("rename issued unexpected Google traffic: calls=%d gets=%d mutations=%d", state.calls.Load(), state.gets.Load(), state.mutations.Load())
	}

	if strings.TrimSpace(state.lastPatch) != `{"name":"Renamed.pdf"}` {
		t.Fatalf("rename patch body was not name-only: %s", state.lastPatch)
	}

	if state.files[driveSelectedID].Name != "Renamed.pdf" {
		t.Fatalf("rename was not persisted: %s", state.files[driveSelectedID].Name)
	}

	copyResp := call("drive_copy", map[string]any{"file_id": driveSelectedID, "new_name": "Copy.pdf"}, driveAuthorization("drive_copy", true, false, driveSelectedID), nil)
	if copyResp.Status != 200 || !copyResp.OK || !strings.Contains(string(copyResp.Result), driveCopiedID) {
		t.Fatalf("native copy failed: %d %s", copyResp.Status, copyResp.Body)
	}

	if state.calls.Load() != 3 || state.gets.Load() != 1 || state.mutations.Load() != 2 {
		t.Fatalf("copy issued unexpected Google traffic: calls=%d gets=%d mutations=%d", state.calls.Load(), state.gets.Load(), state.mutations.Load())
	}

	if strings.TrimSpace(state.lastCopy) != `{"name":"Copy.pdf"}` {
		t.Fatalf("copy body was not name-only: %s", state.lastCopy)
	}

	copied := state.files[driveCopiedID]
	if copied == nil || copied.MimeType != "text/plain" || copied.Trashed {
		t.Fatalf("copied file was not persisted correctly: %+v", copied)
	}

	// The copy is not covered by the selected-file grant: reads of the copy
	// must fail before any Google traffic.
	before := state.calls.Load()

	foreignRead := call("drive_get", map[string]any{"file_id": driveCopiedID}, driveAuthorization("drive_get", false, false, driveSelectedID), nil)
	if foreignRead.OK {
		t.Fatalf("copy read was granted without a matching resource: %d %s", foreignRead.Status, foreignRead.Body)
	}

	if state.calls.Load() != before {
		t.Fatal("ungranted copy read reached Google")
	}

	metadata := call("drive_get", map[string]any{"file_id": driveSelectedID}, driveAuthorization("drive_get", false, false, driveSelectedID), nil)
	if metadata.Status != 200 || !metadata.OK ||
		!strings.Contains(string(metadata.Result), `"id":"selectedfile"`) ||
		!strings.Contains(string(metadata.Result), "Renamed.pdf") ||
		!strings.Contains(string(metadata.Result), `"mimeType":"text/plain"`) {
		t.Fatalf("fresh metadata read failed: %d %s", metadata.Status, metadata.Result)
	}

	trashAuth := driveAuthorization("drive_trash", true, true, driveSelectedID)
	beforeMutations := state.mutations.Load()

	trash := call("drive_trash", map[string]any{"file_id": driveSelectedID}, trashAuth, nil)
	if trash.Status != 200 || !trash.OK {
		t.Fatalf("native trash failed: %d %s", trash.Status, trash.Body)
	}

	if state.mutations.Load() != beforeMutations+1 || strings.TrimSpace(state.lastPatch) != `{"trashed":true}` {
		t.Fatalf("trash did not issue exactly one trashed patch: %d %s", state.mutations.Load(), state.lastPatch)
	}

	if !state.files[driveSelectedID].Trashed {
		t.Fatal("trash was not persisted")
	}

	trashedMeta := call("drive_get", map[string]any{"file_id": driveSelectedID}, driveAuthorization("drive_get", false, false, driveSelectedID), nil)
	if trashedMeta.Status != 200 || !trashedMeta.OK || !strings.Contains(string(trashedMeta.Result), `"trashed":true`) {
		t.Fatalf("trashed state was not readable: %d %s", trashedMeta.Status, trashedMeta.Result)
	}

	// Permission denials must be rejected before any Google traffic, preflight included.
	deny := func(label string, name string, args map[string]any, auth ToolAuthorization, mutate func(*Request)) {
		t.Helper()
		beforeCalls := state.calls.Load()

		resp := call(name, args, auth, mutate)
		if resp.OK {
			t.Errorf("%s was allowed: %d %s", label, resp.Status, resp.Body)
		}

		if state.calls.Load() != beforeCalls {
			t.Errorf("%s reached Google", label)
		}
	}

	wrongScope := driveAuthorization("drive_rename", true, false, driveSelectedID)
	wrongScope.Scopes = []string{"https://www.googleapis.com/auth/drive.readonly"}
	deny("wrong_scope", "drive_rename", renameArgs, wrongScope, nil)

	missingTool := driveAuthorization("drive_rename", true, false, driveSelectedID)
	missingTool.AllowedTools = []string{"drive_trash"}
	deny("missing_tool", "drive_rename", renameArgs, missingTool, nil)

	deny("missing_write", "drive_rename", renameArgs, driveAuthorization("drive_rename", false, false, driveSelectedID), nil)
	deny("missing_destructive", "drive_trash", map[string]any{"file_id": driveSelectedID}, driveAuthorization("drive_trash", true, false, driveSelectedID), nil)
	deny("permanent_arg", "drive_trash", map[string]any{"file_id": driveSelectedID, "permanent": true}, trashAuth, nil)
	deny("unknown_parent", "drive_copy", map[string]any{"file_id": driveSelectedID, "new_name": "Copy.pdf", "parent": "folderP"}, driveAuthorization("drive_copy", true, false, driveSelectedID), nil)
	deny("foreign_resource", "drive_rename", renameArgs, driveAuthorization("drive_rename", true, false, "otherfile"), nil)
	deny("foreign_tenant", "drive_rename", renameArgs, renameAuth, func(req *Request) { req.TenantID = "tenant-b" })
}

func TestSignedNativeDriveTrashRejectsUntrashableSelectedFiles(t *testing.T) {
	cases := []struct {
		label    string
		fileID   string
		mimeType string
		trashed  bool
		notFound bool
		code     string
		status   int
	}{
		{label: "folder", fileID: "folderfile", mimeType: "application/vnd.google-apps.folder", code: "permission_denied", status: http.StatusForbidden},
		{label: "shortcut", fileID: "shortcutfile", mimeType: "application/vnd.google-apps.shortcut", code: "permission_denied", status: http.StatusForbidden},
		{label: "empty_mime", fileID: "emptyfile", mimeType: ""},
		{label: "metadata_error", fileID: "errfile", notFound: true},
		{label: "already_trashed", fileID: "trashedfile", mimeType: "text/plain", trashed: true},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			state := &driveFixture{files: map[string]*driveFileState{
				tc.fileID: {ID: tc.fileID, Name: "Case.pdf", MimeType: tc.mimeType, Trashed: tc.trashed, NotFound: tc.notFound},
			}}
			harness := newHarness(t, func(engine *Engine, _ *recordedDiscoverer) {
				engine.NativeServices = app.Services{Drive: driveServiceFixture(t, state)}
			})

			body := nativeToolBody(t, func(req *Request) {
				req.Tool = &ToolInvocation{
					Name:          "drive_trash",
					Arguments:     map[string]any{"file_id": tc.fileID},
					Authorization: driveAuthorization("drive_trash", true, true, tc.fileID),
				}
			})

			resp := postExecute(t, harness.server, signInvocation(t, body, func(c map[string]any) { c["operation"] = OperationToolCall }), body)
			if resp.OK {
				t.Fatalf("trash of untrashable file was allowed: %d %s", resp.Status, resp.Body)
			}

			if state.mutations.Load() != 0 {
				t.Fatalf("%s trash mutated Google state", tc.label)
			}

			if state.gets.Load() != 1 {
				t.Fatalf("%s trash issued %d metadata GETs", tc.label, state.gets.Load())
			}

			if tc.code != "" && (resp.Error.Code != tc.code || resp.Status != tc.status) {
				t.Fatalf("%s trash was not rejected as %s: %d %s", tc.label, tc.code, resp.Status, resp.Body)
			}
		})
	}
}
