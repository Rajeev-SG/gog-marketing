package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestEveryMarketingMutationHonorsReadOnly(t *testing.T) {
	for _, tt := range marketingMutationCases() {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--account", "a@b.com", "--readonly", "--no-input", "--force"}, tt.args...)
			result := executeWithTestRuntime(t, args, &app.Runtime{})
			if !errors.Is(result.err, googleapi.ErrReadOnly) {
				t.Fatalf("error = %v, want ErrReadOnly", result.err)
			}
		})
	}
}

func TestEveryMarketingMutationHasDryRunContract(t *testing.T) {
	for _, tt := range marketingMutationCases() {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--account", "a@b.com", "--dry-run", "--no-input", "--force", "--json"}, tt.args...)
			result := executeWithTestRuntime(t, args, &app.Runtime{})
			if result.err != nil {
				t.Fatalf("dry-run error = %v", result.err)
			}
			if !strings.Contains(result.stdout, `"op": "`+tt.op+`"`) {
				t.Fatalf("dry-run output = %s, want op %q", result.stdout, tt.op)
			}
		})
	}
}

func TestEveryDestructiveMarketingMutationRequiresConfirmation(t *testing.T) {
	for _, tt := range marketingMutationCases() {
		if !strings.Contains(tt.name, "delete") && !strings.Contains(tt.name, "archive") && !strings.Contains(tt.name, "publish") {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--account", "a@b.com", "--no-input", "--json"}, tt.args...)
			result := executeWithTestRuntime(t, args, &app.Runtime{})
			if result.err == nil {
				t.Fatal("expected confirmation error")
			}
			message := strings.ToLower(result.err.Error())
			if !strings.Contains(message, "confirm") && !strings.Contains(message, "force") && !strings.Contains(message, "no-input") {
				t.Fatalf("error = %v, want confirmation guidance", result.err)
			}
		})
	}
}

func TestBigQueryExecutionHonorsReadOnly(t *testing.T) {
	result := executeWithTestRuntime(t, []string{"--account", "a@b.com", "--readonly", "bigquery", "query", "--project", "billing-proj", "--sql", "SELECT 1", "--acknowledge-cost"}, &app.Runtime{})
	if !errors.Is(result.err, googleapi.ErrReadOnly) {
		t.Fatalf("error = %v, want ErrReadOnly", result.err)
	}
}

type marketingMutationCase struct {
	name string
	op   string
	args []string
}

func marketingMutationCases() []marketingMutationCase {
	return []marketingMutationCase{
		{"analytics-datastreams-create", "analytics.datastreams.create", []string{"analytics", "datastreams", "create", "123", "--json-file", `{}`}},
		{"analytics-datastreams-update", "analytics.datastreams.update", []string{"analytics", "datastreams", "update", "properties/123/dataStreams/1", "--json-file", `{}`}},
		{"analytics-datastreams-delete", "analytics.datastreams.delete", []string{"analytics", "datastreams", "delete", "properties/123/dataStreams/1"}},
		{"analytics-keyevents-create", "analytics.keyevents.create", []string{"analytics", "keyevents", "create", "123", "--json-file", `{}`}},
		{"analytics-keyevents-update", "analytics.keyevents.update", []string{"analytics", "keyevents", "update", "properties/123/keyEvents/1", "--json-file", `{}`}},
		{"analytics-keyevents-delete", "analytics.keyevents.delete", []string{"analytics", "keyevents", "delete", "properties/123/keyEvents/1"}},
		{"analytics-custom-dimensions-create", "analytics.custom-dimensions.create", []string{"analytics", "custom-dimensions", "create", "123", "--json-file", `{}`}},
		{"analytics-custom-dimensions-update", "analytics.custom-dimensions.update", []string{"analytics", "custom-dimensions", "update", "properties/123/customDimensions/1", "--json-file", `{}`}},
		{"analytics-custom-dimensions-archive", "analytics.custom-dimensions.archive", []string{"analytics", "custom-dimensions", "archive", "properties/123/customDimensions/1"}},
		{"analytics-custom-metrics-create", "analytics.custom-metrics.create", []string{"analytics", "custom-metrics", "create", "123", "--json-file", `{}`}},
		{"analytics-custom-metrics-update", "analytics.custom-metrics.update", []string{"analytics", "custom-metrics", "update", "properties/123/customMetrics/1", "--json-file", `{}`}},
		{"analytics-custom-metrics-archive", "analytics.custom-metrics.archive", []string{"analytics", "custom-metrics", "archive", "properties/123/customMetrics/1"}},
		{"analytics-googleads-links-create", "analytics.googleads-links.create", []string{"analytics", "googleads-links", "create", "123", "--json-file", `{}`}},
		{"analytics-googleads-links-update", "analytics.googleads-links.update", []string{"analytics", "googleads-links", "update", "properties/123/googleAdsLinks/1", "--json-file", `{}`}},
		{"analytics-googleads-links-delete", "analytics.googleads-links.delete", []string{"analytics", "googleads-links", "delete", "properties/123/googleAdsLinks/1"}},
		{"tagmanager-containers-create", "tagmanager.containers.create", []string{"tagmanager", "containers", "create", "1", "--json-file", `{"name":"x"}`}},
		{"tagmanager-containers-update", "tagmanager.containers.update", []string{"tagmanager", "containers", "update", "1", "2", "--json-file", `{"name":"x"}`}},
		{"tagmanager-containers-delete", "tagmanager.containers.delete", []string{"tagmanager", "containers", "delete", "1", "2"}},
		{"tagmanager-workspaces-create", "tagmanager.workspaces.create", []string{"tagmanager", "workspaces", "create", "1", "2", "--json-file", `{"name":"x"}`}},
		{"tagmanager-workspaces-update", "tagmanager.workspaces.update", []string{"tagmanager", "workspaces", "update", "1", "2", "3", "--json-file", `{"name":"x"}`}},
		{"tagmanager-workspaces-delete", "tagmanager.workspaces.delete", []string{"tagmanager", "workspaces", "delete", "1", "2", "3"}},
		{"tagmanager-workspaces-sync", "tagmanager.workspaces.sync", []string{"tagmanager", "workspaces", "sync", "1", "2", "3"}},
		{"tagmanager-workspaces-create-version", "tagmanager.workspaces.create-version", []string{"tagmanager", "workspaces", "create-version", "1", "2", "3"}},
		{"tagmanager-tags-create", "tagmanager.tags.create", []string{"tagmanager", "tags", "create", "1", "2", "3", "--json-file", `{"name":"x","type":"y"}`}},
		{"tagmanager-tags-update", "tagmanager.tags.update", []string{"tagmanager", "tags", "update", "1", "2", "3", "4", "--json-file", `{"name":"x","type":"y"}`}},
		{"tagmanager-tags-delete", "tagmanager.tags.delete", []string{"tagmanager", "tags", "delete", "1", "2", "3", "4"}},
		{"tagmanager-triggers-create", "tagmanager.triggers.create", []string{"tagmanager", "triggers", "create", "1", "2", "3", "--json-file", `{"name":"x","type":"y"}`}},
		{"tagmanager-triggers-update", "tagmanager.triggers.update", []string{"tagmanager", "triggers", "update", "1", "2", "3", "4", "--json-file", `{"name":"x","type":"y"}`}},
		{"tagmanager-triggers-delete", "tagmanager.triggers.delete", []string{"tagmanager", "triggers", "delete", "1", "2", "3", "4"}},
		{"tagmanager-variables-create", "tagmanager.variables.create", []string{"tagmanager", "variables", "create", "1", "2", "3", "--json-file", `{"name":"x","type":"y"}`}},
		{"tagmanager-variables-update", "tagmanager.variables.update", []string{"tagmanager", "variables", "update", "1", "2", "3", "4", "--json-file", `{"name":"x","type":"y"}`}},
		{"tagmanager-variables-delete", "tagmanager.variables.delete", []string{"tagmanager", "variables", "delete", "1", "2", "3", "4"}},
		{"tagmanager-versions-delete", "tagmanager.versions.delete", []string{"tagmanager", "versions", "delete", "1", "2", "3"}},
		{"tagmanager-versions-publish", "tagmanager.versions.publish", []string{"tagmanager", "versions", "publish", "1", "2", "3"}},
		{"searchconsole-sitemaps-submit", "searchconsole.sitemaps.submit", []string{"searchconsole", "sitemaps", "submit", "https://example.com/", "https://example.com/sitemap.xml"}},
		{"searchconsole-sitemaps-delete", "searchconsole.sitemaps.delete", []string{"searchconsole", "sitemaps", "delete", "https://example.com/", "https://example.com/sitemap.xml"}},
	}
}
