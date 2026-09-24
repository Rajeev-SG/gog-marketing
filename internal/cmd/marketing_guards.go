package cmd

import (
	"context"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func marketingDryRunExit(ctx context.Context, flags *RootFlags, op string, plan map[string]any) error {
	if readOnlyEnabled(flags) && (flags == nil || !flags.DryRun) {
		return googleapi.ErrReadOnly
	}
	return dryRunExit(ctx, flags, op, plan)
}

func marketingDryRunAndConfirmDestructive(ctx context.Context, flags *RootFlags, op string, plan map[string]any, description string) error {
	if readOnlyEnabled(flags) && (flags == nil || !flags.DryRun) {
		return googleapi.ErrReadOnly
	}
	return dryRunAndConfirmDestructive(ctx, flags, op, plan, description)
}
