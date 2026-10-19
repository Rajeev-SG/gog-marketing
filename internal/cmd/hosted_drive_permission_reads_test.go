package cmd

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDrivePermissionReadArguments(t *testing.T) {
	specs := map[string]mcpToolSpec{}
	for _, spec := range hostedDrivePermissionReadTools() {
		if spec.Risk != mcpRiskRead {
			t.Fatalf("not read-only: %s", spec.Name)
		}
		specs[spec.Name] = spec
	}
	if len(specs) != 3 {
		t.Fatalf("tool count %d", len(specs))
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"drive_permissions", map[string]any{"file_id": "file-1"}, []string{"drive", "permissions", "--max=100", "--", "file-1"}},
		{"drive_permissions", map[string]any{"file_id": "file-1", "max_results": 2, "page_token": "--account=else&cursor=2"}, []string{"drive", "permissions", "--max=2", "--page=--account=else&cursor=2", "--", "file-1"}},
		{"drive_audit_sharing", map[string]any{"file_id": "file-1", "public_only": true, "external_only": false, "internal_domains": []any{"example.com", "corp.test"}}, []string{"drive", "audit", "sharing", "--file=file-1", "--public-only=true", "--external-only=false", "--internal-domain=example.com", "--internal-domain=corp.test"}},
		{"drive_audit_sharing", map[string]any{"file_id": "file-1", "public_only": false, "external_only": false}, []string{"drive", "audit", "sharing", "--file=file-1", "--public-only=false", "--external-only=false"}},
		{"drive_audit_user", map[string]any{"file_id": "file-1", "user": "--account=else"}, []string{"drive", "audit", "user", "--file=file-1", "--", "--account=else"}},
	} {
		got, err := specs[tc.name].BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}})
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("%s got=%q want=%q err=%v", tc.name, got, tc.want, err)
		}
	}
	for name, spec := range specs {
		for _, bad := range []any{nil, 1, "", " file-1", "../foreign", "a/b", "https://drive.google.com/file/x"} {
			_, err := spec.BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"file_id": bad, "user": "guest@example.com"}}})
			if err == nil {
				t.Fatalf("%s accepted file=%v", name, bad)
			}
		}
	}
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"drive_permissions", "max_results", 101},
		{"drive_permissions", "max_results", 0},
		{"drive_permissions", "max_results", 1.5},
		{"drive_permissions", "max_results", "2"},
		{"drive_permissions", "page_token", nil},
		{"drive_permissions", "page_token", "bad\ncursor"},
		{"drive_audit_sharing", "public_only", nil},
		{"drive_audit_sharing", "external_only", "false"},
		{"drive_audit_sharing", "internal_domains", nil},
		{"drive_audit_sharing", "internal_domains", []any{1}},
		{"drive_audit_sharing", "internal_domains", []string{" "}},
		{"drive_audit_user", "user", nil},
		{"drive_audit_user", "user", " "},
		{"drive_audit_user", "user", "bad\nuser"},
	} {
		args := map[string]any{"file_id": "file-1", "user": "guest@example.com"}
		args[tc.key] = tc.value
		if _, err := specs[tc.name].BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}); err == nil {
			t.Fatalf("accepted %s %s=%v", tc.name, tc.key, tc.value)
		}
	}
	if _, err := specs["drive_audit_sharing"].BuildArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"file_id": "file-1", "public_only": true, "external_only": true}}}); err == nil {
		t.Fatal("accepted contradictory filters")
	}
}
