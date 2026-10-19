package cmd

import (
	"context"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/openclaw/gogcli/internal/googleapi"
)

type AnalyticsRetentionCmd struct {
	Get    AnalyticsRetentionGetCmd    `cmd:"" name:"get" help:"Read GA4 retention settings for one property"`
	Update AnalyticsRetentionUpdateCmd `cmd:"" name:"update" help:"Change GA4 retention settings; shortening retention can irreversibly delete historical data"`
}
type AnalyticsRetentionGetCmd struct {
	Property string `arg:"" name:"property" help:"GA4 property ID or properties/ID"`
}
type AnalyticsRetentionUpdateCmd struct {
	Property                   string  `arg:"" name:"property" help:"GA4 property ID or properties/ID"`
	EventDataRetention         *string `name:"event-data-retention" help:"TWO_MONTHS, FOURTEEN_MONTHS, or 360-only TWENTY_SIX_MONTHS, THIRTY_EIGHT_MONTHS, FIFTY_MONTHS"`
	UserDataRetention          *string `name:"user-data-retention" help:"TWO_MONTHS or FOURTEEN_MONTHS"`
	ResetUserDataOnNewActivity *bool   `name:"reset-user-data-on-new-activity" help:"Reset the user retention period after each new event; explicitly accepts false"`
}

func analyticsRetentionName(raw string) (string, error) {
	property := normalizeAnalyticsProperty(raw)
	id := strings.TrimPrefix(property, "properties/")
	if len(property) > 256 || id == "" || strings.Trim(id, "0123456789") != "" {
		return "", usage("a numeric GA4 property ID is required")
	}
	return property + "/dataRetentionSettings", nil
}

func (c *AnalyticsRetentionGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	name, err := analyticsRetentionName(c.Property)
	if err != nil {
		return err
	}
	return analyticsGetAndWrite(ctx, flags, "data_retention", name, func(svc *analyticsadmin.Service, name string) (any, error) {
		return svc.Properties.GetDataRetentionSettings(name).Context(ctx).Do()
	})
}

// plan validates the property name and supplied flags, then builds the
// canonical update body and its updateMask with no service or network use.
func (c *AnalyticsRetentionUpdateCmd) plan() (*analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings, string, error) {
	name, err := analyticsRetentionName(c.Property)
	if err != nil {
		return nil, "", err
	}
	if c.EventDataRetention == nil && c.UserDataRetention == nil && c.ResetUserDataOnNewActivity == nil {
		return nil, "", usagef("nothing to update: set --event-data-retention, --user-data-retention and/or --reset-user-data-on-new-activity")
	}
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaDataRetentionSettings{Name: name}
	var mask []string
	if c.EventDataRetention != nil {
		switch v := strings.ToUpper(strings.TrimSpace(*c.EventDataRetention)); v {
		case "TWO_MONTHS", "FOURTEEN_MONTHS", "TWENTY_SIX_MONTHS", "THIRTY_EIGHT_MONTHS", "FIFTY_MONTHS":
			body.EventDataRetention = v
			mask = append(mask, "event_data_retention")
		default: // provided empty, UNSPECIFIED, or unknown all rejected here
			return nil, "", usagef("invalid --event-data-retention %q: allowed TWO_MONTHS, FOURTEEN_MONTHS, TWENTY_SIX_MONTHS, THIRTY_EIGHT_MONTHS, FIFTY_MONTHS", *c.EventDataRetention)
		}
	}
	if c.UserDataRetention != nil {
		switch v := strings.ToUpper(strings.TrimSpace(*c.UserDataRetention)); v {
		case "TWO_MONTHS", "FOURTEEN_MONTHS":
			body.UserDataRetention = v
			mask = append(mask, "user_data_retention")
		default: // provided empty, UNSPECIFIED, or unknown all rejected here
			return nil, "", usagef("invalid --user-data-retention %q: allowed TWO_MONTHS, FOURTEEN_MONTHS (advanced durations are event-only per the official schema)", *c.UserDataRetention)
		}
	}
	if c.ResetUserDataOnNewActivity != nil {
		body.ResetUserDataOnNewActivity = *c.ResetUserDataOnNewActivity
		if !*c.ResetUserDataOnNewActivity {
			// Force false onto the wire; the Go client omits zero-valued fields otherwise.
			body.ForceSendFields = append(body.ForceSendFields, "ResetUserDataOnNewActivity")
		}
		mask = append(mask, "reset_user_data_on_new_activity")
	}
	return body, strings.Join(mask, ","), nil
}

func (c *AnalyticsRetentionUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body, mask, err := c.plan()
	if err != nil {
		return err
	}
	if guardErr := marketingDryRunAndConfirmDestructive(ctx, flags, "analytics.retention.update", map[string]any{
		"name":        body.Name,
		"update_mask": mask,
		"settings":    body,
	}, "change GA4 retention settings; shortening retention can irreversibly delete historical data"); guardErr != nil {
		return guardErr
	}
	ctx = googleapi.WithoutRetries(ctx)
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	result, err := svc.Properties.UpdateDataRetentionSettings(body.Name, body).UpdateMask(mask).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, "data_retention", result)
}
