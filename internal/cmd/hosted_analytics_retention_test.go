package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedAnalyticsRetentionArguments(t *testing.T) {
	for _, tc := range []struct {
		update bool
		args   map[string]any
		want   []string
	}{
		{false, map[string]any{"property": "properties/123"}, []string{"analytics", "retention", "get", "--", "properties/123"}},
		{true, map[string]any{"property": "properties/123", "reset_user_data_on_new_activity": false}, []string{"analytics", "retention", "update", "--reset-user-data-on-new-activity=false", "--", "properties/123"}},
		{true, map[string]any{"property": "properties/123", "event_data_retention": "FOURTEEN_MONTHS", "user_data_retention": "TWO_MONTHS"}, []string{"analytics", "retention", "update", "--event-data-retention=FOURTEEN_MONTHS", "--user-data-retention=TWO_MONTHS", "--", "properties/123"}},
	} {
		got, err := hostedAnalyticsRetentionArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: tc.args}}, tc.update)
		if err != nil || strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Fatalf("args %v err=%v want%v", got, err, tc.want)
		}
	}
	for _, args := range []map[string]any{
		{"property": "123", "reset_user_data_on_new_activity": false},
		{"property": "properties/123/../456", "reset_user_data_on_new_activity": false},
		{"property": "properties/123"},
		{"property": "properties/123", "event_data_retention": ""},
		{"property": "properties/123", "event_data_retention": true},
		{"property": "properties/123", "reset_user_data_on_new_activity": "false"},
		{"property": "properties/123", "user_data_retention": "FIFTY_MONTHS"},
		{"property": "properties/123", "body_json": "@/tmp/retention.json", "update_mask": "*"},
	} {
		if _, err := hostedAnalyticsRetentionArgs(mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}, true); err == nil {
			t.Fatalf("invalid update accepted %v", args)
		}
	}
}
