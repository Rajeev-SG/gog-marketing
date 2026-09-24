package googleapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestReadOnlyAllowsGoogleAdsSearchAndBigQueryDryRunOnly(t *testing.T) {
	ads, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://googleads.googleapis.com/v25/customers/1/googleAds:search", strings.NewReader(`{"query":"SELECT 1"}`))
	if err != nil {
		t.Fatal(err)
	}

	if !ReadOnlyRequestAllowed(ads) {
		t.Fatal("Google Ads search should be read-only")
	}

	dryRun, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://bigquery.googleapis.com/bigquery/v2/projects/p/queries", strings.NewReader(`{"query":"SELECT 1","dryRun":true}`))
	if err != nil {
		t.Fatal(err)
	}

	if !ReadOnlyRequestAllowed(dryRun) {
		t.Fatal("BigQuery dry-run should be read-only")
	}

	execute, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://bigquery.googleapis.com/bigquery/v2/projects/p/queries", strings.NewReader(`{"query":"SELECT 1"}`))
	if err != nil {
		t.Fatal(err)
	}

	if ReadOnlyRequestAllowed(execute) {
		t.Fatal("BigQuery execution must not be read-only")
	}
}
