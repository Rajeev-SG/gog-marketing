package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type AnalyticsPropertiesCmd struct {
	List AnalyticsPropertiesListCmd `cmd:"" default:"withargs" aliases:"ls"`
	Get  AnalyticsPropertyGetCmd    `cmd:"" name:"get" aliases:"info,show"`
}

type AnalyticsPropertiesListCmd struct {
	PageSize  int64  `name:"page-size" aliases:"max" default:"50"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *AnalyticsPropertiesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	var items []*analyticsadmin.GoogleAnalyticsAdminV1betaProperty
	next := c.PageToken
	for {
		call := svc.Properties.List().PageSize(c.PageSize).Context(ctx)
		if next != "" {
			call = call.PageToken(next)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return callErr
		}
		items = append(items, resp.Properties...)
		next = resp.NextPageToken
		if !c.All || next == "" {
			break
		}
	}
	return writeAnalyticsAdminList(ctx, "properties", items, next, c.FailEmpty, func(item *analyticsadmin.GoogleAnalyticsAdminV1betaProperty) map[string]any {
		return map[string]any{"name": item.Name, "display_name": item.DisplayName, "create_time": item.CreateTime, "update_time": item.UpdateTime}
	})
}

type AnalyticsPropertyGetCmd struct {
	Property string `arg:"" name:"property"`
}

func (c *AnalyticsPropertyGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Properties.Get(analyticsPropertyPath(c.Property)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, "property", item)
}

type AnalyticsMutationFlags struct {
	JSONFile   string `name:"json-file" aliases:"body" help:"Complete resource JSON, inline or @file"`
	UpdateMask string `name:"update-mask" help:"Comma-separated update mask for update operations"`
}

type AnalyticsDataStreamsCmd struct {
	List   AnalyticsDataStreamsListCmd   `cmd:"" default:"withargs" aliases:"ls"`
	Get    AnalyticsDataStreamsGetCmd    `cmd:"" name:"get" aliases:"info,show"`
	Create AnalyticsDataStreamsCreateCmd `cmd:"" name:"create"`
	Update AnalyticsDataStreamsUpdateCmd `cmd:"" name:"update"`
	Delete AnalyticsDataStreamsDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove"`
}

type AnalyticsDataStreamsListCmd struct {
	Property  string `arg:"" name:"property"`
	PageSize  int64  `name:"page-size" aliases:"max" default:"50"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *AnalyticsDataStreamsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAnalyticsAdminList(ctx, flags, "data_streams", analyticsAdminListConfig{Property: c.Property, PageSize: c.PageSize, PageToken: c.PageToken, All: c.All, FailEmpty: c.FailEmpty}, fetchAnalyticsDataStreamsPage, func(item *analyticsadmin.GoogleAnalyticsAdminV1betaDataStream) map[string]any {
		return map[string]any{"name": item.Name, "display_name": item.DisplayName, "type": item.Type, "create_time": item.CreateTime}
	})
}

type AnalyticsDataStreamsGetCmd struct {
	DataStream string `arg:"" name:"datastream"`
}

func (c *AnalyticsDataStreamsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Properties.DataStreams.Get(analyticsResourcePath(c.DataStream)).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, "data_stream", item)
}

type AnalyticsDataStreamsCreateCmd struct {
	Property               string `arg:"" name:"property"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsDataStreamsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	if err := dryRunExit(ctx, flags, "analytics.datastreams.create", map[string]any{"parent": analyticsPropertyPath(c.Property), "data_stream": body}); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := svc.Properties.DataStreams.Create(analyticsPropertyPath(c.Property), body).Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, "data_stream", item)
}

type AnalyticsDataStreamsUpdateCmd struct {
	DataStream             string `arg:"" name:"datastream"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsDataStreamsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaDataStream{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	path := analyticsResourcePath(c.DataStream)
	if err := dryRunExit(ctx, flags, "analytics.datastreams.update", map[string]any{"name": path, "data_stream": body, "update_mask": c.UpdateMask}); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	call := svc.Properties.DataStreams.Patch(path, body).Context(ctx)
	if strings.TrimSpace(c.UpdateMask) != "" {
		call = call.UpdateMask(c.UpdateMask)
	}
	item, err := call.Do()
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, "data_stream", item)
}

type AnalyticsDataStreamsDeleteCmd struct {
	DataStream string `arg:"" name:"datastream"`
}

func (c *AnalyticsDataStreamsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := analyticsResourcePath(c.DataStream)
	if err := dryRunAndConfirmDestructive(ctx, flags, "analytics.datastreams.delete", map[string]any{"name": path}, "delete GA4 data stream "+path); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	if _, err := svc.Properties.DataStreams.Delete(path).Context(ctx).Do(); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("deleted", true), kv("name", path))
}

type AnalyticsKeyEventsCmd struct {
	List   AnalyticsKeyEventsListCmd   `cmd:"" default:"withargs" aliases:"ls"`
	Get    AnalyticsKeyEventsGetCmd    `cmd:"" name:"get" aliases:"info,show"`
	Create AnalyticsKeyEventsCreateCmd `cmd:"" name:"create"`
	Update AnalyticsKeyEventsUpdateCmd `cmd:"" name:"update"`
	Delete AnalyticsKeyEventsDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove"`
}

type AnalyticsKeyEventsListCmd struct {
	Property  string `arg:"" name:"property"`
	PageSize  int64  `name:"page-size" aliases:"max" default:"50"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *AnalyticsKeyEventsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAnalyticsAdminList(ctx, flags, "key_events", analyticsAdminListConfig{Property: c.Property, PageSize: c.PageSize, PageToken: c.PageToken, All: c.All, FailEmpty: c.FailEmpty}, fetchAnalyticsKeyEventsPage, func(item *analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent) map[string]any {
		return map[string]any{"name": item.Name, "event_name": item.EventName, "counting_method": item.CountingMethod}
	})
}

type AnalyticsKeyEventsGetCmd struct {
	KeyEvent string `arg:"" name:"keyevent"`
}

func (c *AnalyticsKeyEventsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return analyticsGetAndWrite(ctx, flags, "key_event", c.KeyEvent, func(svc *analyticsadmin.Service, name string) (any, error) {
		return svc.Properties.KeyEvents.Get(name).Context(ctx).Do()
	})
}

type AnalyticsKeyEventsCreateCmd struct {
	Property               string `arg:"" name:"property"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsKeyEventsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsCreateAndWrite(ctx, flags, "key_event", "analytics.keyevents.create", analyticsPropertyPath(c.Property), body, func(svc *analyticsadmin.Service, parent string, value any) (any, error) {
		return svc.Properties.KeyEvents.Create(parent, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent)).Context(ctx).Do()
	})
}

type AnalyticsKeyEventsUpdateCmd struct {
	KeyEvent               string `arg:"" name:"keyevent"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsKeyEventsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsUpdateAndWrite(ctx, flags, "key_event", "analytics.keyevents.update", analyticsResourcePath(c.KeyEvent), c.UpdateMask, body, func(svc *analyticsadmin.Service, name string, value any, mask string) (any, error) {
		call := svc.Properties.KeyEvents.Patch(name, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent)).Context(ctx)
		if mask != "" {
			call = call.UpdateMask(mask)
		}
		return call.Do()
	})
}

type AnalyticsKeyEventsDeleteCmd struct {
	KeyEvent string `arg:"" name:"keyevent"`
}

func (c *AnalyticsKeyEventsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return analyticsDelete(ctx, flags, "analytics.keyevents.delete", analyticsResourcePath(c.KeyEvent), func(svc *analyticsadmin.Service, name string) error {
		_, err := svc.Properties.KeyEvents.Delete(name).Context(ctx).Do()
		return err
	})
}

type AnalyticsCustomDimensionsCmd struct {
	List    AnalyticsCustomDimensionsListCmd    `cmd:"" default:"withargs" aliases:"ls"`
	Get     AnalyticsCustomDimensionsGetCmd     `cmd:"" name:"get" aliases:"info,show"`
	Create  AnalyticsCustomDimensionsCreateCmd  `cmd:"" name:"create"`
	Update  AnalyticsCustomDimensionsUpdateCmd  `cmd:"" name:"update"`
	Archive AnalyticsCustomDimensionsArchiveCmd `cmd:"" name:"archive"`
}
type AnalyticsCustomDimensionsListCmd struct {
	Property  string `arg:"" name:"property"`
	PageSize  int64  `name:"page-size" aliases:"max" default:"50"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *AnalyticsCustomDimensionsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAnalyticsAdminList(ctx, flags, "custom_dimensions", analyticsAdminListConfig{Property: c.Property, PageSize: c.PageSize, PageToken: c.PageToken, All: c.All, FailEmpty: c.FailEmpty}, fetchAnalyticsCustomDimensionsPage, func(item *analyticsadmin.GoogleAnalyticsAdminV1betaCustomDimension) map[string]any {
		return map[string]any{"name": item.Name, "display_name": item.DisplayName, "parameter_name": item.ParameterName, "scope": item.Scope}
	})
}

type AnalyticsCustomDimensionsGetCmd struct {
	CustomDimension string `arg:"" name:"custom-dimension"`
}

func (c *AnalyticsCustomDimensionsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return analyticsGetAndWrite(ctx, flags, "custom_dimension", c.CustomDimension, func(svc *analyticsadmin.Service, name string) (any, error) {
		return svc.Properties.CustomDimensions.Get(name).Context(ctx).Do()
	})
}

type AnalyticsCustomDimensionsCreateCmd struct {
	Property               string `arg:"" name:"property"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsCustomDimensionsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaCustomDimension{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsCreateAndWrite(ctx, flags, "custom_dimension", "analytics.custom-dimensions.create", analyticsPropertyPath(c.Property), body, func(svc *analyticsadmin.Service, parent string, value any) (any, error) {
		return svc.Properties.CustomDimensions.Create(parent, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaCustomDimension)).Context(ctx).Do()
	})
}

type AnalyticsCustomDimensionsUpdateCmd struct {
	CustomDimension        string `arg:"" name:"custom-dimension"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsCustomDimensionsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaCustomDimension{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsUpdateAndWrite(ctx, flags, "custom_dimension", "analytics.custom-dimensions.update", analyticsResourcePath(c.CustomDimension), c.UpdateMask, body, func(svc *analyticsadmin.Service, name string, value any, mask string) (any, error) {
		call := svc.Properties.CustomDimensions.Patch(name, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaCustomDimension)).Context(ctx)
		if mask != "" {
			call = call.UpdateMask(mask)
		}
		return call.Do()
	})
}

type AnalyticsCustomDimensionsArchiveCmd struct {
	CustomDimension string `arg:"" name:"custom-dimension"`
}

func (c *AnalyticsCustomDimensionsArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := analyticsResourcePath(c.CustomDimension)
	if err := dryRunAndConfirmDestructive(ctx, flags, "analytics.custom-dimensions.archive", map[string]any{"name": path}, "archive GA4 custom dimension "+path); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	if _, err := svc.Properties.CustomDimensions.Archive(path, &analyticsadmin.GoogleAnalyticsAdminV1betaArchiveCustomDimensionRequest{}).Context(ctx).Do(); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("archived", true), kv("name", path))
}

type AnalyticsCustomMetricsCmd struct {
	List    AnalyticsCustomMetricsListCmd    `cmd:"" default:"withargs" aliases:"ls"`
	Get     AnalyticsCustomMetricsGetCmd     `cmd:"" name:"get" aliases:"info,show"`
	Create  AnalyticsCustomMetricsCreateCmd  `cmd:"" name:"create"`
	Update  AnalyticsCustomMetricsUpdateCmd  `cmd:"" name:"update"`
	Archive AnalyticsCustomMetricsArchiveCmd `cmd:"" name:"archive"`
}
type AnalyticsCustomMetricsListCmd struct {
	Property  string `arg:"" name:"property"`
	PageSize  int64  `name:"page-size" aliases:"max" default:"50"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *AnalyticsCustomMetricsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	var items []*analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric
	next := c.PageToken
	for {
		call := svc.Properties.CustomMetrics.List(analyticsPropertyPath(c.Property)).PageSize(c.PageSize).Context(ctx)
		if next != "" {
			call = call.PageToken(next)
		}
		resp, callErr := call.Do()
		if callErr != nil {
			return callErr
		}
		items = append(items, resp.CustomMetrics...)
		next = resp.NextPageToken
		if !c.All || next == "" {
			break
		}
	}
	return writeAnalyticsAdminList(ctx, "custom_metrics", items, next, c.FailEmpty, func(item *analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric) map[string]any {
		return map[string]any{"name": item.Name, "display_name": item.DisplayName, "parameter_name": item.ParameterName, "scope": item.Scope, "measurement_unit": item.MeasurementUnit}
	})
}

type AnalyticsCustomMetricsGetCmd struct {
	CustomMetric string `arg:"" name:"custom-metric"`
}

func (c *AnalyticsCustomMetricsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	return analyticsGetAndWrite(ctx, flags, "custom_metric", c.CustomMetric, func(svc *analyticsadmin.Service, name string) (any, error) {
		return svc.Properties.CustomMetrics.Get(name).Context(ctx).Do()
	})
}

type AnalyticsCustomMetricsCreateCmd struct {
	Property               string `arg:"" name:"property"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsCustomMetricsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsCreateAndWrite(ctx, flags, "custom_metric", "analytics.custom-metrics.create", analyticsPropertyPath(c.Property), body, func(svc *analyticsadmin.Service, parent string, value any) (any, error) {
		return svc.Properties.CustomMetrics.Create(parent, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric)).Context(ctx).Do()
	})
}

type AnalyticsCustomMetricsUpdateCmd struct {
	CustomMetric           string `arg:"" name:"custom-metric"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsCustomMetricsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsUpdateAndWrite(ctx, flags, "custom_metric", "analytics.custom-metrics.update", analyticsResourcePath(c.CustomMetric), c.UpdateMask, body, func(svc *analyticsadmin.Service, name string, value any, mask string) (any, error) {
		call := svc.Properties.CustomMetrics.Patch(name, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaCustomMetric)).Context(ctx)
		if mask != "" {
			call = call.UpdateMask(mask)
		}
		return call.Do()
	})
}

type AnalyticsCustomMetricsArchiveCmd struct {
	CustomMetric string `arg:"" name:"custom-metric"`
}

func (c *AnalyticsCustomMetricsArchiveCmd) Run(ctx context.Context, flags *RootFlags) error {
	path := analyticsResourcePath(c.CustomMetric)
	if err := dryRunAndConfirmDestructive(ctx, flags, "analytics.custom-metrics.archive", map[string]any{"name": path}, "archive GA4 custom metric "+path); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	if _, err := svc.Properties.CustomMetrics.Archive(path, &analyticsadmin.GoogleAnalyticsAdminV1betaArchiveCustomMetricRequest{}).Context(ctx).Do(); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("archived", true), kv("name", path))
}

type AnalyticsGoogleAdsLinksCmd struct {
	List   AnalyticsGoogleAdsLinksListCmd   `cmd:"" default:"withargs" aliases:"ls"`
	Get    AnalyticsGoogleAdsLinksGetCmd    `cmd:"" name:"get" aliases:"info,show"`
	Create AnalyticsGoogleAdsLinksCreateCmd `cmd:"" name:"create"`
	Update AnalyticsGoogleAdsLinksUpdateCmd `cmd:"" name:"update"`
	Delete AnalyticsGoogleAdsLinksDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove"`
}
type AnalyticsGoogleAdsLinksListCmd struct {
	Property  string `arg:"" name:"property"`
	PageSize  int64  `name:"page-size" aliases:"max" default:"50"`
	PageToken string `name:"page-token" aliases:"page"`
	All       bool   `name:"all" aliases:"all-pages,allpages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results"`
}

func (c *AnalyticsGoogleAdsLinksListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runAnalyticsAdminList(ctx, flags, "google_ads_links", analyticsAdminListConfig{Property: c.Property, PageSize: c.PageSize, PageToken: c.PageToken, All: c.All, FailEmpty: c.FailEmpty}, fetchAnalyticsGoogleAdsLinksPage, func(item *analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink) map[string]any {
		return map[string]any{"name": item.Name, "customer_id": item.CustomerId, "ads_personalization_enabled": item.AdsPersonalizationEnabled}
	})
}

type AnalyticsGoogleAdsLinksGetCmd struct {
	Property      string `arg:"" name:"property"`
	GoogleAdsLink string `arg:"" name:"googleads-link"`
}

func (c *AnalyticsGoogleAdsLinksGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := svc.Properties.GoogleAdsLinks.List(analyticsPropertyPath(c.Property)).Context(ctx).Do()
	if err != nil {
		return err
	}
	for _, item := range resp.GoogleAdsLinks {
		if item.Name == analyticsResourcePath(c.GoogleAdsLink) || strings.HasSuffix(item.Name, "/"+strings.TrimSpace(c.GoogleAdsLink)) {
			return writeAnalyticsAdminItem(ctx, "google_ads_link", item)
		}
	}
	return usage("Google Ads link not found")
}

type AnalyticsGoogleAdsLinksCreateCmd struct {
	Property               string `arg:"" name:"property"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsGoogleAdsLinksCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsCreateAndWrite(ctx, flags, "google_ads_link", "analytics.googleads-links.create", analyticsPropertyPath(c.Property), body, func(svc *analyticsadmin.Service, parent string, value any) (any, error) {
		return svc.Properties.GoogleAdsLinks.Create(parent, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink)).Context(ctx).Do()
	})
}

type AnalyticsGoogleAdsLinksUpdateCmd struct {
	GoogleAdsLink          string `arg:"" name:"googleads-link"`
	AnalyticsMutationFlags `embed:""`
}

func (c *AnalyticsGoogleAdsLinksUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	body := &analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink{}
	if err := decodeAnalyticsJSON(c.JSONFile, body); err != nil {
		return err
	}
	return analyticsUpdateAndWrite(ctx, flags, "google_ads_link", "analytics.googleads-links.update", analyticsResourcePath(c.GoogleAdsLink), c.UpdateMask, body, func(svc *analyticsadmin.Service, name string, value any, mask string) (any, error) {
		call := svc.Properties.GoogleAdsLinks.Patch(name, value.(*analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink)).Context(ctx)
		if mask != "" {
			call = call.UpdateMask(mask)
		}
		return call.Do()
	})
}

type AnalyticsGoogleAdsLinksDeleteCmd struct {
	GoogleAdsLink string `arg:"" name:"googleads-link"`
}

func (c *AnalyticsGoogleAdsLinksDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return analyticsDelete(ctx, flags, "analytics.googleads-links.delete", analyticsResourcePath(c.GoogleAdsLink), func(svc *analyticsadmin.Service, name string) error {
		_, err := svc.Properties.GoogleAdsLinks.Delete(name).Context(ctx).Do()
		return err
	})
}

func analyticsAdminFor(ctx context.Context, flags *RootFlags) (*analyticsadmin.Service, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	return analyticsAdminService(ctx, account)
}

func analyticsPropertyPath(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "properties/") {
		return value
	}
	return "properties/" + value
}

func analyticsResourcePath(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "/") && (strings.HasPrefix(value, "properties/") || strings.HasPrefix(value, "accounts/")) {
		return value
	}
	if strings.HasPrefix(value, "properties/") {
		return value
	}
	if strings.Contains(value, "/") {
		return value
	}
	return value
}

func decodeAnalyticsJSON(spec string, dst any) error {
	raw, err := resolveInlineOrFileBytes(spec, strings.NewReader(""))
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return usage("resource JSON is required (--json-file)")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid Analytics Admin JSON: %w", err)
	}
	return nil
}

func writeAnalyticsAdminItem(ctx context.Context, key string, item any) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{key: item})
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return writeTagManagerMaps(ctx, []map[string]any{{key: value}})
}

func writeAnalyticsAdminList[T any](ctx context.Context, key string, items []T, next string, failEmpty bool, row func(T) map[string]any) error {
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{key: items, "nextPageToken": next}); err != nil {
			return err
		}
		return failEmptyExitIf(failEmpty, len(items) == 0)
	}
	if len(items) == 0 {
		return failEmptyExitIf(failEmpty, true)
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, row(item))
	}
	return writeTagManagerMaps(ctx, rows)
}

func collectAnalyticsAdminPages[T any](pageToken string, all bool, fetch func(string) ([]T, string, error)) ([]T, string, error) {
	var items []T
	next := pageToken
	for {
		pageItems, nextPage, err := fetch(next)
		if err != nil {
			return nil, "", err
		}
		items = append(items, pageItems...)
		next = nextPage
		if !all || next == "" {
			return items, next, nil
		}
	}
}

type analyticsAdminListConfig struct {
	Property  string
	PageSize  int64
	PageToken string
	All       bool
	FailEmpty bool
}

func runAnalyticsAdminList[T any](
	ctx context.Context,
	flags *RootFlags,
	key string,
	config analyticsAdminListConfig,
	fetch func(context.Context, *analyticsadmin.Service, analyticsAdminListConfig, string) ([]T, string, error),
	row func(T) map[string]any,
) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	items, next, err := collectAnalyticsAdminPages(config.PageToken, config.All, func(pageToken string) ([]T, string, error) {
		return fetch(ctx, svc, config, pageToken)
	})
	if err != nil {
		return err
	}
	return writeAnalyticsAdminList(ctx, key, items, next, config.FailEmpty, row)
}

func fetchAnalyticsDataStreamsPage(ctx context.Context, svc *analyticsadmin.Service, config analyticsAdminListConfig, pageToken string) ([]*analyticsadmin.GoogleAnalyticsAdminV1betaDataStream, string, error) {
	call := svc.Properties.DataStreams.List(analyticsPropertyPath(config.Property)).PageSize(config.PageSize).Context(ctx)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.DataStreams, resp.NextPageToken, nil
}

func fetchAnalyticsKeyEventsPage(ctx context.Context, svc *analyticsadmin.Service, config analyticsAdminListConfig, pageToken string) ([]*analyticsadmin.GoogleAnalyticsAdminV1betaKeyEvent, string, error) {
	call := svc.Properties.KeyEvents.List(analyticsPropertyPath(config.Property)).PageSize(config.PageSize).Context(ctx)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.KeyEvents, resp.NextPageToken, nil
}

func fetchAnalyticsCustomDimensionsPage(ctx context.Context, svc *analyticsadmin.Service, config analyticsAdminListConfig, pageToken string) ([]*analyticsadmin.GoogleAnalyticsAdminV1betaCustomDimension, string, error) {
	call := svc.Properties.CustomDimensions.List(analyticsPropertyPath(config.Property)).PageSize(config.PageSize).Context(ctx)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.CustomDimensions, resp.NextPageToken, nil
}

func fetchAnalyticsGoogleAdsLinksPage(ctx context.Context, svc *analyticsadmin.Service, config analyticsAdminListConfig, pageToken string) ([]*analyticsadmin.GoogleAnalyticsAdminV1betaGoogleAdsLink, string, error) {
	call := svc.Properties.GoogleAdsLinks.List(analyticsPropertyPath(config.Property)).PageSize(config.PageSize).Context(ctx)
	if pageToken != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.GoogleAdsLinks, resp.NextPageToken, nil
}

func analyticsGetAndWrite(ctx context.Context, flags *RootFlags, key, name string, get func(*analyticsadmin.Service, string) (any, error)) error {
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := get(svc, analyticsResourcePath(name))
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, key, item)
}

func analyticsCreateAndWrite(ctx context.Context, flags *RootFlags, key, op, parent string, body any, create func(*analyticsadmin.Service, string, any) (any, error)) error {
	if err := dryRunExit(ctx, flags, op, map[string]any{"parent": parent, "resource": body}); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := create(svc, parent, body)
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, key, item)
}

func analyticsUpdateAndWrite(ctx context.Context, flags *RootFlags, key, op, name, mask string, body any, update func(*analyticsadmin.Service, string, any, string) (any, error)) error {
	if err := dryRunExit(ctx, flags, op, map[string]any{"name": name, "resource": body, "update_mask": mask}); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	item, err := update(svc, analyticsResourcePath(name), body, mask)
	if err != nil {
		return err
	}
	return writeAnalyticsAdminItem(ctx, key, item)
}

func analyticsDelete(ctx context.Context, flags *RootFlags, op, name string, del func(*analyticsadmin.Service, string) error) error {
	name = analyticsResourcePath(name)
	if err := dryRunAndConfirmDestructive(ctx, flags, op, map[string]any{"name": name}, "delete GA4 resource "+name); err != nil {
		return err
	}
	svc, err := analyticsAdminFor(ctx, flags)
	if err != nil {
		return err
	}
	if err := del(svc, name); err != nil {
		return err
	}
	return writeResult(ctx, ui.FromContext(ctx), kv("deleted", true), kv("name", name))
}
