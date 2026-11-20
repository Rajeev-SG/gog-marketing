package cmd

import (
	"context"
	"fmt"
	"strings"

	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

const analyticsRealtimeMaxRows = 250000

type AnalyticsRealtimeCmd struct {
	Property   string `arg:"" name:"property" help:"GA4 property ID or resource (e.g. 123456789 or properties/123456789)"`
	Dimensions string `name:"dimensions" help:"Comma-separated realtime dimensions (e.g. eventName,country)" default:"eventName"`
	Metrics    string `name:"metrics" help:"Comma-separated realtime metrics (e.g. eventCount,activeUsers)" default:"eventCount"`
	Max        int64  `name:"max" aliases:"limit" help:"Max rows to return (1-250000)" default:"100"`
	FailEmpty  bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

type analyticsRealtimePlan struct {
	Property   string
	Dimensions []string
	Metrics    []string
	Request    *analyticsdata.RunRealtimeReportRequest
}

func newAnalyticsRealtimePlan(property, dimensions, metrics string, limit int64) (analyticsRealtimePlan, error) {
	plan := analyticsRealtimePlan{
		Property:   normalizeAnalyticsProperty(property),
		Dimensions: splitCommaList(dimensions),
		Metrics:    splitCommaList(metrics),
	}
	if plan.Property == "" {
		return analyticsRealtimePlan{}, usage("empty property")
	}
	if len(plan.Metrics) == 0 {
		return analyticsRealtimePlan{}, usage("empty --metrics")
	}
	if limit <= 0 || limit > analyticsRealtimeMaxRows {
		return analyticsRealtimePlan{}, usagef("--max must be between 1 and %d", analyticsRealtimeMaxRows)
	}
	plan.Request = &analyticsdata.RunRealtimeReportRequest{
		Metrics: analyticsMetrics(plan.Metrics),
		Limit:   limit,
	}
	if len(plan.Dimensions) > 0 {
		plan.Request.Dimensions = analyticsDimensions(plan.Dimensions)
	}
	return plan, nil
}

func (c *AnalyticsRealtimeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	plan, err := newAnalyticsRealtimePlan(c.Property, c.Dimensions, c.Metrics, c.Max)
	if err != nil {
		return err
	}

	svc, err := analyticsDataService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Properties.RunRealtimeReport(plan.Property, plan.Request).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"property":         plan.Property,
			"dimensions":       plan.Dimensions,
			"metrics":          plan.Metrics,
			"row_count":        resp.RowCount,
			"dimensionHeaders": resp.DimensionHeaders,
			"metricHeaders":    resp.MetricHeaders,
			"rows":             resp.Rows,
		}); err != nil {
			return err
		}
		if len(resp.Rows) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(resp.Rows) == 0 {
		u.Err().Println("No realtime analytics rows")
		return failEmptyExit(c.FailEmpty)
	}

	headers := make([]string, 0, len(plan.Dimensions)+len(plan.Metrics))
	for _, d := range plan.Dimensions {
		headers = append(headers, strings.ToUpper(d))
	}
	for _, m := range plan.Metrics {
		headers = append(headers, strings.ToUpper(m))
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range resp.Rows {
		values := make([]string, 0, len(plan.Dimensions)+len(plan.Metrics))
		for i := range plan.Dimensions {
			values = append(values, sanitizeTab(analyticsDimensionValue(row, i)))
		}
		for i := range plan.Metrics {
			values = append(values, sanitizeTab(analyticsMetricValue(row, i)))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
	return nil
}
