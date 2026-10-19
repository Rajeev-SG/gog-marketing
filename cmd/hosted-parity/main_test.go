package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"sort"
	"testing"

	"github.com/openclaw/gogcli/internal/cmd"
	"github.com/openclaw/gogcli/internal/hosted/runner"
)

func canonicalMatrix(t *testing.T) parityMatrix {
	t.Helper()
	command := exec.CommandContext(context.Background(), "go", "run", "./cmd/gog", "schema", "--json")
	command.Dir = "../.."
	payload, err := command.Output()
	if err != nil {
		t.Fatalf("build native schema: %v", err)
	}
	var schema schemaDocument
	err = json.Unmarshal(payload, &schema)
	if err != nil {
		t.Fatalf("decode native schema: %v", err)
	}
	bindings := map[string]bindingMetadata{}
	for _, binding := range runner.NativeBindingMetadata() {
		bindings[binding.Name] = bindingMetadata{
			AcceptedScopes:   append([]string(nil), binding.Scopes...),
			HTTPMethods:      append([]string(nil), binding.Methods...),
			HTTPMethodsKnown: len(binding.Methods) > 0,
			Destructive:      binding.Destructive,
		}
	}
	for _, metadata := range bindings {
		sort.Strings(metadata.AcceptedScopes)
		sort.Strings(metadata.HTTPMethods)
	}
	matrix, err := BuildMatrix(schema.Command, runner.NativeToolCatalog(cmd.HostedExistingToolCatalog()), bindings)
	if err != nil {
		t.Fatalf("build matrix: %v", err)
	}
	return matrix
}

func TestBaselineMatrix(t *testing.T) {
	matrix := canonicalMatrix(t)

	if matrix.NativeLeafCount != 353 {
		t.Fatalf("native leaf count = %d, want 353", matrix.NativeLeafCount)
	}
	if matrix.HostedToolCount != 180 || matrix.TotalHostedCatalogCount != 218 || matrix.ExcludedHostedToolCount != 38 {
		t.Fatalf("hosted counts = %d/%d/%d, want 180/218/38", matrix.HostedToolCount, matrix.TotalHostedCatalogCount, matrix.ExcludedHostedToolCount)
	}
	if matrix.MappedNativeLeafCount != 189 || matrix.MappedHostedToolCount != 180 {
		t.Fatalf("mapped counts = %d native leaves/%d tools, want 189/180", matrix.MappedNativeLeafCount, matrix.MappedHostedToolCount)
	}
	if matrix.Classifications["unclassified"] != 0 || len(matrix.UnmappedHostedTools) != 0 {
		t.Fatalf("matrix has unclassified native commands or unmapped hosted tools: %+v/%v", matrix.Classifications, matrix.UnmappedHostedTools)
	}
}

func TestTasksAndTagManagerComplete(t *testing.T) {
	matrix := canonicalMatrix(t)
	summaries := map[string]serviceSummary{}
	for _, summary := range matrix.ByService {
		summaries[summary.Service] = summary
	}
	tasks := summaries["tasks"]
	if tasks.NativeLeaves != 11 || tasks.HostedTools != 11 || tasks.MappedNativeLeaves != 11 {
		t.Fatalf("Tasks = %+v, want 11 leaves/tools mapped", tasks)
	}
	gtm := summaries["tagmanager"]
	if gtm.NativeLeaves != 34 || gtm.HostedTools != 34 || gtm.MappedNativeLeaves != 34 {
		t.Fatalf("GTM = %+v, want 34 leaves/tools mapped", gtm)
	}
}

func TestEveryActiveHostedToolMapped(t *testing.T) {
	matrix := canonicalMatrix(t)
	mapped := map[string]bool{}
	for _, leaf := range matrix.Leaves {
		if leaf.HostedTool != nil {
			mapped[*leaf.HostedTool] = true
		}
		if leaf.LiveVerified {
			t.Fatalf("%s %s claims live verification", leaf.Service, leaf.Command)
		}
	}
	for service, count := range map[string]int{"analytics": 31, "appscript": 10, "bigquery": 6, "docs": 21, "drive": 8, "forms": 10, "searchconsole": 8, "sheets": 32, "slides": 9, "tagmanager": 34, "tasks": 11} {
		found := 0
		for name := range mapped {
			if len(name) > len(service)+1 && name[:len(service)+1] == service+"_" {
				found++
			}
		}
		if found != count {
			t.Fatalf("%s mapped hosted tools = %d, want %d", service, found, count)
		}
	}
}

func TestDuplicateNativeAliasesCountedDistinctly(t *testing.T) {
	matrix := canonicalMatrix(t)
	mapped := 0
	for _, leaf := range matrix.Leaves {
		if leaf.Service == "searchconsole" && (leaf.Command == "query" || leaf.Command == "searchanalytics query") {
			if leaf.HostedTool == nil || *leaf.HostedTool != "searchconsole_query" {
				t.Fatalf("Search Console alias mapping = %+v", leaf)
			}
			mapped++
		}
	}
	if mapped != 2 {
		t.Fatalf("Search Console alias leaves = %d, want 2", mapped)
	}
}

func TestExactNameFutureToolAutomaps(t *testing.T) {
	hosted := map[string]map[string]bool{"sheets": {"sheets_future_feature": true}}
	tool, mapped := hostedMapping("sheets", "future feature", hosted)
	if !mapped || tool != "sheets_future_feature" {
		t.Fatalf("hostedMapping = %q/%v, want exact canonical tool", tool, mapped)
	}
}

func TestUnknownMappingsRemainUnknown(t *testing.T) {
	status, reason := classifyMissing("sheets", "future feature")
	if status != "unreviewed_mapping" || reason == "" {
		t.Fatalf("unknown command = %q/%q, want explicit unreviewed_mapping reason", status, reason)
	}

	fixtureHosted := []runner.NativeToolDescription{
		{Name: "docs_get", Service: "docs"},
		{Name: "docs_unknown", Service: "docs"},
	}
	fixtureSchema := &schemaNode{Name: "gog", Subcommands: []*schemaNode{{Name: "docs", Subcommands: []*schemaNode{{Name: "get"}}}}}
	matrix, err := BuildMatrix(fixtureSchema, fixtureHosted, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.UnmappedHostedTools) != 1 || matrix.UnmappedHostedTools[0] != "docs_unknown" {
		t.Fatalf("unknown hosted tools = %v, want docs_unknown", matrix.UnmappedHostedTools)
	}
}

func TestReviewClassificationKeepsEngineeringAndPaidAssessmentSeparate(t *testing.T) {
	for _, entry := range []struct{ service, command, want string }{
		{"forms", "watch create", "implementation_design_required"},
		{"sheets", "batch-request", "excluded"},
		{"drive", "drives", "scope_boundary_blocked"},
		{"docs", "raw", "eligible_missing"},
		{"slides", "table unmerge", "eligible_missing"},
		{"sheets", "future feature", "unreviewed_mapping"},
	} {
		status, _ := classifyMissing(entry.service, entry.command)
		if status != entry.want {
			t.Fatalf("%s %s = %s, want %s", entry.service, entry.command, status, entry.want)
		}
	}
}

func TestBindingMetadataScopesAndMethodGuard(t *testing.T) {
	bindings := runner.NativeBindingMetadata()
	if len(bindings) == 0 {
		t.Fatal("runner binding metadata is empty")
	}
	names := map[string]bool{}
	for _, binding := range bindings {
		if binding.Name == "" || binding.Service == "" || len(binding.Scopes) == 0 {
			t.Fatalf("binding %+v has missing name/service/scopes", binding)
		}
		names[binding.Name] = true
	}
	for _, name := range []string{"drive_get", "docs_get", "analytics_keyevents_create", "tagmanager_versions_publish", "tasks_add"} {
		if !names[name] {
			t.Fatalf("binding metadata missing %q", name)
		}
	}
}

func TestGeneratedMatrixIsReproducibleAndCheckedIn(t *testing.T) {
	matrix := canonicalMatrix(t)
	first, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	second := canonicalMatrix(t)
	secondPayload, err := json.MarshalIndent(second, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, secondPayload) {
		t.Fatal("matrix generation is not reproducible")
	}

	checkedIn, err := os.ReadFile("../../docs/hosted/cli-hosted-matrix.json")
	if err != nil {
		t.Fatalf("read checked-in matrix: %v", err)
	}
	generated := append(bytes.Clone(first), '\n')
	if !bytes.Equal(generated, checkedIn) {
		t.Fatal("docs/hosted/cli-hosted-matrix.json drifted; regenerate it")
	}
}
