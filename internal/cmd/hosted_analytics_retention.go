package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedAnalyticsRetentionTools() []mcpToolSpec {
	property := mcp.WithString("property", mcp.Required())
	return []mcpToolSpec{
		{
			Name: "analytics_retention_get", Service: "analytics", Risk: mcpRiskRead,
			Description: "Read GA4 event/user retention settings for one explicitly enabled property. Requires exact tool/property authority and analytics.readonly or analytics.edit; no account-wide access.",
			Options:     []mcp.ToolOption{property}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedAnalyticsRetentionArgs(req, false) },
		},
		{
			Name: "analytics_retention_update", Service: "analytics", Risk: mcpRiskWrite,
			Description: "Partially change GA4 retention settings on one explicitly enabled property. Shortening retention can irreversibly delete historical data. Requires analytics.edit, exact tool/property write and destructive permission; chat requires one-use exact-argument approval. Only supplied fields change; false is preserved. Advanced event durations require an existing GA4 360 property. No wildcard mask, raw JSON, files or automatic retries.",
			Options: []mcp.ToolOption{
				property,
				mcp.WithString("event_data_retention", mcp.Enum("TWO_MONTHS", "FOURTEEN_MONTHS", "TWENTY_SIX_MONTHS", "THIRTY_EIGHT_MONTHS", "FIFTY_MONTHS")),
				mcp.WithString("user_data_retention", mcp.Enum("TWO_MONTHS", "FOURTEEN_MONTHS")),
				mcp.WithBoolean("reset_user_data_on_new_activity"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedAnalyticsRetentionArgs(req, true) },
		},
	}
}

func hostedAnalyticsRetentionArgs(req mcp.CallToolRequest, update bool) ([]string, error) {
	property, err := requireMCPString(req, "property")
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(property, "properties/") {
		return nil, usage("canonical properties/ID is required")
	}
	if _, nameErr := analyticsRetentionName(property); nameErr != nil {
		return nil, nameErr
	}
	if !update {
		return []string{"analytics", "retention", "get", "--", property}, nil
	}
	command := AnalyticsRetentionUpdateCmd{Property: property}
	args := []string{"analytics", "retention", "update"}
	for _, field := range []struct {
		key, flag   string
		destination **string
	}{
		{"event_data_retention", "event-data-retention", &command.EventDataRetention},
		{"user_data_retention", "user-data-retention", &command.UserDataRetention},
	} {
		raw, present := req.GetArguments()[field.key]
		if !present {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return nil, usage("retention durations must be strings")
		}
		*field.destination = &value
		args = append(args, "--"+field.flag+"="+value)
	}
	if raw, present := req.GetArguments()["reset_user_data_on_new_activity"]; present {
		value, ok := raw.(bool)
		if !ok {
			return nil, usage("reset flag must be boolean")
		}
		command.ResetUserDataOnNewActivity = &value
		args = append(args, "--reset-user-data-on-new-activity="+strconv.FormatBool(value))
	}
	if _, _, planErr := command.plan(); planErr != nil {
		return nil, planErr
	}
	return append(args, "--", property), nil
}
