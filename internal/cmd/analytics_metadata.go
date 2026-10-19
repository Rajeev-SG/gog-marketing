package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openclaw/gogcli/internal/outfmt"
)

type AnalyticsMetadataCmd struct {
	Get AnalyticsMetadataGetCmd `cmd:"" name:"get" help:"Read GA4 Analytics Data API metadata for one property"`
}

type AnalyticsMetadataGetCmd struct {
	Property string `arg:"" name:"property" help:"GA4 numeric property ID"`
}

func analyticsMetadataPropertyPath(raw string) (string, error) {
	property := strings.TrimSpace(raw)
	property = strings.TrimPrefix(property, "properties/")
	if property == "0" || property == "" {
		return "", usage("a positive numeric GA4 property ID is required")
	}
	if strings.Trim(property, "0123456789") != "" {
		return "", usage("a positive numeric GA4 property ID is required")
	}
	id, err := strconv.ParseUint(property, 10, 64)
	if err != nil || id == 0 {
		return "", usage("a positive numeric GA4 property ID is required")
	}
	return "properties/" + strconv.FormatUint(id, 10), nil
}

func (c *AnalyticsMetadataGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	path, err := analyticsMetadataPropertyPath(c.Property)
	if err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := analyticsDataService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Properties.GetMetadata(path + "/metadata").Context(ctx).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"property":   path,
			"dimensions": resp.Dimensions,
			"metrics":    resp.Metrics,
		})
	}
	fmt.Fprintf(stdoutWriter(ctx), "GA4 metadata %s: %d dimensions, %d metrics\n",
		path, len(resp.Dimensions), len(resp.Metrics))
	return nil
}
