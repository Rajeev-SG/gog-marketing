package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/cmd"
	"github.com/openclaw/gogcli/internal/hosted/runner"
)

func fakeLiveTools() []runner.NativeToolDescription {
	return []runner.NativeToolDescription{
		{Name: "analytics_report", Service: "analytics", ResourceType: "account", ResourceArgument: "property_id", Write: false},
		{Name: "sheets_update_range", Service: "sheets", ResourceType: "file", ResourceArgument: "spreadsheet_id", Write: true},
		{Name: "calendar_list_calendars", Service: "calendar", Write: false},
	}
}

func fakeLiveBindings() []runner.BindingMetadata {
	return []runner.BindingMetadata{
		{
			Name:    "analytics_report",
			Service: "analytics",
			Scopes:  []string{"https://www.googleapis.com/auth/analytics.readonly"},
			Methods: []string{"GET"},
		},
		{
			Name:        "sheets_update_range",
			Service:     "sheets",
			Scopes:      []string{"https://www.googleapis.com/auth/spreadsheets", "https://www.googleapis.com/auth/drive.file"},
			Methods:     nil,
			Destructive: true,
		},
	}
}

func fakeLiveEvidence() []liveEvidenceRecord {
	allow := "https://github.com/Rajeev-SG/gog-marketing/issues/97#issuecomment-6251933114"
	deny := "https://github.com/Rajeev-SG/gog-marketing/issues/97#issuecomment-6252385397"
	sha := "5e1c30f9a1b2c3d4e5f60718293a4b5c6d7e8f90"
	return []liveEvidenceRecord{
		{
			Tool:              "analytics_report",
			Date:              "2026-10-18",
			DeploymentVersion: "df267a46-504b-411e-88af-61a45eaff8b9",
			SourceSHA:         nil,
			Variant:           "Bounded activeUsers report, 7daysAgo through yesterday, max=1",
			AllowEvidence:     allow,
			DenyEvidence:      deny,
			DenyVariant:       "Ungranted property rejected with policy_denied / grant_denied",
			State:             expectedEvidenceState,
		},
		{
			Tool:              "sheets_update_range",
			Date:              "2026-10-18",
			DeploymentVersion: "df267a46-504b-411e-88af-61a45eaff8b9",
			SourceSHA:         &sha,
			Variant:           "Selected-spreadsheet update of one range",
			AllowEvidence:     allow,
			DenyEvidence:      deny,
			DenyVariant:       "Ungranted file rejected with policy_denied",
			State:             expectedEvidenceState,
		},
	}
}

func TestBuildLiveMatrixFakeEvidence(t *testing.T) {
	tools, bindings := fakeLiveTools(), fakeLiveBindings()
	evidence := fakeLiveEvidence()
	matrix, err := BuildLiveMatrix(tools, bindings, evidence)
	if err != nil {
		t.Fatalf("BuildLiveMatrix: %v", err)
	}
	if matrix.SchemaVersion != liveMatrixSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", matrix.SchemaVersion, liveMatrixSchemaVersion)
	}
	if matrix.TotalCatalogCount != 3 {
		t.Fatalf("TotalCatalogCount = %d, want 3", matrix.TotalCatalogCount)
	}
	if matrix.ActiveToolCount != 2 {
		t.Fatalf("ActiveToolCount = %d, want 2 (calendar is deferred)", matrix.ActiveToolCount)
	}
	if len(matrix.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(matrix.Rows))
	}
	if matrix.HistoricalEvidenceRows != 2 {
		t.Fatalf("HistoricalEvidenceRows = %d, want 2", matrix.HistoricalEvidenceRows)
	}
	if matrix.Rows[0].Tool != "analytics_report" || matrix.Rows[1].Tool != "sheets_update_range" {
		t.Fatalf("rows not sorted by service/tool: %q then %q", matrix.Rows[0].Tool, matrix.Rows[1].Tool)
	}
	for _, row := range matrix.Rows {
		if row.CandidateLiveState != candidateLiveStateUnverified {
			t.Fatalf("tool %q candidate_live_state = %q", row.Tool, row.CandidateLiveState)
		}
		if row.HistoricalEvidence == nil {
			t.Fatalf("tool %q missing historical evidence", row.Tool)
		}
		if row.HistoricalEvidence.State != expectedEvidenceState {
			t.Fatalf("tool %q evidence state = %q", row.Tool, row.HistoricalEvidence.State)
		}
		if row.HistoricalEvidence.Date != "2026-10-18" {
			t.Fatalf("tool %q evidence date = %q", row.Tool, row.HistoricalEvidence.Date)
		}
		if row.HistoricalEvidence.DeploymentVersion != "df267a46-504b-411e-88af-61a45eaff8b9" {
			t.Fatalf("tool %q evidence deployment_version = %q", row.Tool, row.HistoricalEvidence.DeploymentVersion)
		}
	}

	analytics := matrix.Rows[0]
	if analytics.Write {
		t.Fatal("analytics_report write flag should be false")
	}
	if analytics.DestructiveBinding {
		t.Fatal("analytics_report destructive flag should be false")
	}
	if analytics.ResourceModel == nil || *analytics.ResourceModel != "account" {
		t.Fatalf("analytics resource model = %v, want account", analytics.ResourceModel)
	}
	if !slices.Equal(analytics.AcceptedScopes, []string{"https://www.googleapis.com/auth/analytics.readonly"}) {
		t.Fatalf("analytics scopes = %v", analytics.AcceptedScopes)
	}
	if !analytics.HTTPMethodsKnown || !slices.Equal(analytics.HTTPMethods, []string{"GET"}) {
		t.Fatalf("analytics methods = %v known = %v", analytics.HTTPMethods, analytics.HTTPMethodsKnown)
	}

	sheets := matrix.Rows[1]
	if !sheets.Write || !sheets.DestructiveBinding {
		t.Fatalf("sheets write = %v destructive = %v", sheets.Write, sheets.DestructiveBinding)
	}
	if !slices.Equal(sheets.AcceptedScopes, []string{"https://www.googleapis.com/auth/drive.file", "https://www.googleapis.com/auth/spreadsheets"}) {
		t.Fatalf("sheets scopes not sorted: %v", sheets.AcceptedScopes)
	}
	if sheets.ResourceModel == nil || *sheets.ResourceModel != "file" || sheets.ResourceArgument == nil || *sheets.ResourceArgument != "spreadsheet_id" {
		t.Fatalf("sheets resource model/argument = %v/%v", sheets.ResourceModel, sheets.ResourceArgument)
	}
	if sheets.HTTPMethodsKnown || len(sheets.HTTPMethods) != 0 {
		t.Fatalf("sheets methods should be explicit unknown: methods = %v known = %v", sheets.HTTPMethods, sheets.HTTPMethodsKnown)
	}

	payload, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "\"source_sha\": null") {
		t.Fatal("nil source_sha not preserved as null")
	}
	if !strings.Contains(text, "5e1c30f9a1b2c3d4e5f60718293a4b5c6d7e8f90") {
		t.Fatal("non-nil source_sha not preserved")
	}

	again, err := BuildLiveMatrix(tools, bindings, evidence)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	againPayload, err := json.MarshalIndent(again, "", "  ")
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if !bytes.Equal(payload, againPayload) {
		t.Fatal("live matrix output is not deterministic")
	}
}

func TestBuildLiveMatrixCatalogConsistencyFailures(t *testing.T) {
	tools, bindings := fakeLiveTools(), fakeLiveBindings()

	duplicateCatalog := append(slices.Clone(tools), tools[0])
	if _, err := BuildLiveMatrix(duplicateCatalog, bindings, nil); err == nil || !strings.Contains(err.Error(), "duplicate catalogue row") {
		t.Fatalf("duplicate catalogue row: err = %v", err)
	}

	if _, err := BuildLiveMatrix(tools, bindings[:1], nil); err == nil || !strings.Contains(err.Error(), "no binding metadata") {
		t.Fatalf("missing binding: err = %v", err)
	}

	mismatched := slices.Clone(bindings)
	mismatched[0].Service = "sheets"
	if _, err := BuildLiveMatrix(tools, mismatched, nil); err == nil || !strings.Contains(err.Error(), "declares service") {
		t.Fatalf("service mismatch: err = %v", err)
	}
}

func TestBuildLiveMatrixRejectsUnknownAndDuplicateEvidence(t *testing.T) {
	tools, bindings := fakeLiveTools(), fakeLiveBindings()

	future := fakeLiveEvidence()[0]
	future.Tool = "analytics_future_tool"
	if _, err := BuildLiveMatrix(tools, bindings, []liveEvidenceRecord{future}); err == nil || !strings.Contains(err.Error(), "not in the active hosted catalogue") {
		t.Fatalf("future tool: err = %v", err)
	}

	deferred := fakeLiveEvidence()[0]
	deferred.Tool = "calendar_list_calendars"
	if _, err := BuildLiveMatrix(tools, bindings, []liveEvidenceRecord{deferred}); err == nil || !strings.Contains(err.Error(), "not in the active hosted catalogue") {
		t.Fatalf("deferred tool: err = %v", err)
	}

	duplicated := append(slices.Clone(fakeLiveEvidence()), fakeLiveEvidence()[0])
	if _, err := BuildLiveMatrix(tools, bindings, duplicated); err == nil || !strings.Contains(err.Error(), "duplicate evidence record") {
		t.Fatalf("duplicate evidence: err = %v", err)
	}
}

func TestNewLiveMatrixToolCannotInheritAnotherToolsEvidence(t *testing.T) {
	tools := append(fakeLiveTools(), runner.NativeToolDescription{Name: "analytics_report_future", Service: "analytics"})
	bindings := append(fakeLiveBindings(), runner.BindingMetadata{Name: "analytics_report_future", Service: "analytics", Scopes: []string{"analytics.readonly"}})
	matrix, err := BuildLiveMatrix(tools, bindings, fakeLiveEvidence())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range matrix.Rows {
		if row.Tool == "analytics_report_future" {
			if row.HistoricalEvidence != nil || row.CandidateLiveState != candidateLiveStateUnverified {
				t.Fatal("new tool inherited unrelated live proof")
			}
			if row.ResourceModel == nil || *row.ResourceModel != "account" {
				t.Fatal("account-level authority was omitted from inventory")
			}
			return
		}
	}
	t.Fatal("new canonical tool was missing from the inventory")
}

func TestBuildLiveMatrixRejectsMalformedEvidence(t *testing.T) {
	tools, bindings := fakeLiveTools(), fakeLiveBindings()
	badSHA := "zz"
	shortSHA := "abc123"
	mutations := map[string]func(*liveEvidenceRecord){
		"bad date":             func(r *liveEvidenceRecord) { r.Date = "2026-13-45" },
		"date with time":       func(r *liveEvidenceRecord) { r.Date = "2026-10-18T00:00:00Z" },
		"bad uuid":             func(r *liveEvidenceRecord) { r.DeploymentVersion = "not-a-uuid" },
		"non-hex sha":          func(r *liveEvidenceRecord) { r.SourceSHA = &badSHA },
		"short sha":            func(r *liveEvidenceRecord) { r.SourceSHA = &shortSHA },
		"http allow evidence":  func(r *liveEvidenceRecord) { r.AllowEvidence = "http://example.com/proof" },
		"empty allow evidence": func(r *liveEvidenceRecord) { r.AllowEvidence = "  " },
		"empty deny evidence":  func(r *liveEvidenceRecord) { r.DenyEvidence = "" },
		"missing variant":      func(r *liveEvidenceRecord) { r.Variant = "" },
		"missing deny variant": func(r *liveEvidenceRecord) { r.DenyVariant = "" },
		"unexpected state":     func(r *liveEvidenceRecord) { r.State = "verified_live" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			record := fakeLiveEvidence()[0]
			mutate(&record)
			if _, err := BuildLiveMatrix(tools, bindings, []liveEvidenceRecord{record}); err == nil {
				t.Fatalf("%s: expected rejection", name)
			}
		})
	}
}

func TestParseLiveEvidenceStrict(t *testing.T) {
	validRecord, err := json.Marshal(fakeLiveEvidence()[0])
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	valid := "[" + string(validRecord) + "]"

	records, err := parseLiveEvidence([]byte(valid + "\n"))
	if err != nil {
		t.Fatalf("valid evidence: %v", err)
	}
	if len(records) != 1 || records[0].Tool != "analytics_report" {
		t.Fatalf("records = %+v", records)
	}

	unknownField := strings.Replace(valid, "}]", ",\"extra\":1}]", 1)
	if _, err := parseLiveEvidence([]byte(unknownField)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field: err = %v", err)
	}

	for name, payload := range map[string]string{
		"trailing garbage": valid + "\ntrailing",
		"trailing value":   valid + "\n[]",
		"null document":    "null",
	} {
		if _, err := parseLiveEvidence([]byte(payload)); err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
	}
}

func TestLiveMatrixRealCatalogCoverage(t *testing.T) {
	tools := runner.NativeToolCatalog(cmd.HostedExistingToolCatalog())
	bindings := runner.NativeBindingMetadata()

	activeCount := 0
	activeNames := map[string]bool{}
	for _, tool := range tools {
		if !activeService(tool.Service) {
			continue
		}
		if activeNames[tool.Name] {
			t.Fatalf("duplicate catalogue row for active tool %q", tool.Name)
		}
		activeNames[tool.Name] = true
		activeCount++
	}

	matrix, err := BuildLiveMatrix(tools, bindings, nil)
	if err != nil {
		t.Fatalf("BuildLiveMatrix: %v", err)
	}
	if matrix.TotalCatalogCount != len(tools) {
		t.Fatalf("TotalCatalogCount = %d, want %d", matrix.TotalCatalogCount, len(tools))
	}
	if matrix.ActiveToolCount != activeCount || len(matrix.Rows) != activeCount {
		t.Fatalf("active count = %d rows = %d, want %d", matrix.ActiveToolCount, len(matrix.Rows), activeCount)
	}
	if matrix.HistoricalEvidenceRows != 0 {
		t.Fatalf("HistoricalEvidenceRows = %d, want 0", matrix.HistoricalEvidenceRows)
	}
	for index, row := range matrix.Rows {
		if !activeNames[row.Tool] {
			t.Fatalf("row %q is not an active catalogue tool", row.Tool)
		}
		if row.CandidateLiveState != candidateLiveStateUnverified {
			t.Fatalf("row %q candidate_live_state = %q", row.Tool, row.CandidateLiveState)
		}
		if row.HistoricalEvidence != nil {
			t.Fatalf("row %q carries evidence with empty evidence input", row.Tool)
		}
		if !row.HTTPMethodsKnown && len(row.HTTPMethods) != 0 {
			t.Fatalf("row %q unknown methods but methods = %v", row.Tool, row.HTTPMethods)
		}
		if index > 0 {
			prev := matrix.Rows[index-1]
			if prev.Service > row.Service || (prev.Service == row.Service && prev.Tool >= row.Tool) {
				t.Fatalf("rows not sorted at index %d: %q/%q then %q/%q", index, prev.Service, prev.Tool, row.Service, row.Tool)
			}
		}
	}

	matrixAgain, err := BuildLiveMatrix(tools, bindings, nil)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	first, err := json.Marshal(matrix)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := json.Marshal(matrixAgain)
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("live matrix output is not deterministic")
	}
}

func TestLiveMatrixRealCatalogWithCuratedEvidenceFile(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "docs", "hosted", "live-tool-evidence.json"))
	if err != nil {
		t.Fatalf("read curated evidence: %v", err)
	}
	records, err := parseLiveEvidence(payload)
	if err != nil {
		t.Fatalf("parse curated evidence: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("curated evidence records = %d, want 2", len(records))
	}
	tools := runner.NativeToolCatalog(cmd.HostedExistingToolCatalog())
	bindings := runner.NativeBindingMetadata()
	matrix, err := BuildLiveMatrix(tools, bindings, records)
	if err != nil {
		t.Fatalf("BuildLiveMatrix: %v", err)
	}
	historical := 0
	for _, row := range matrix.Rows {
		if row.CandidateLiveState != candidateLiveStateUnverified {
			t.Fatalf("row %q candidate_live_state = %q", row.Tool, row.CandidateLiveState)
		}
		if row.HistoricalEvidence != nil {
			historical++
			if row.Service != "analytics" {
				t.Fatalf("curated evidence attached to non-analytics row %q", row.Tool)
			}
			if row.HistoricalEvidence.State != expectedEvidenceState {
				t.Fatalf("row %q evidence state = %q", row.Tool, row.HistoricalEvidence.State)
			}
		}
	}
	if historical != 2 || matrix.HistoricalEvidenceRows != 2 {
		t.Fatalf("historical rows = %d/%d, want 2", historical, matrix.HistoricalEvidenceRows)
	}
}

func TestRunLiveMatrixCheckAndStdout(t *testing.T) {
	evidencePath := filepath.Join("..", "..", "docs", "hosted", "live-tool-evidence.json")
	payload, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("read curated evidence: %v", err)
	}
	records, err := parseLiveEvidence(payload)
	if err != nil {
		t.Fatalf("parse curated evidence: %v", err)
	}
	matrix, err := BuildLiveMatrix(runner.NativeToolCatalog(cmd.HostedExistingToolCatalog()), runner.NativeBindingMetadata(), records)
	if err != nil {
		t.Fatalf("BuildLiveMatrix: %v", err)
	}
	data, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data = append(data, '\n')

	dir := t.TempDir()
	checkPath := filepath.Join(dir, "live-matrix.json")

	if err := os.WriteFile(checkPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	if err := runLiveMatrix(evidencePath, checkPath); !errors.Is(err, errStaleLiveMatrix) {
		t.Fatalf("drift check: err = %v, want errStaleLiveMatrix", err)
	}

	if err := os.WriteFile(checkPath, data, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := runLiveMatrix(evidencePath, checkPath); err != nil {
		t.Fatalf("matching check: %v", err)
	}

	stdout := captureLiveStdout(t, func() error { return runLiveMatrix(evidencePath, "") })
	if !bytes.Equal(stdout, data) {
		t.Fatal("stdout output differs from expected deterministic bytes")
	}
}

func captureLiveStdout(t *testing.T, run func() error) []byte {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer

	var output []byte
	var readErr error
	readDone := make(chan struct{})
	go func() {
		output, readErr = io.ReadAll(reader)
		close(readDone)
	}()

	runErr := run()
	closeErr := writer.Close()
	<-readDone
	os.Stdout = original

	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	if closeErr != nil {
		t.Fatalf("close pipe writer: %v", closeErr)
	}
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	return output
}

func TestCheckedInLiveMatrixSkipsNativeSchemaLoading(t *testing.T) {
	t.Chdir("../..")
	if err := run([]string{"--live-matrix", "--schema-json", "/nonexistent-native-schema.json", "--evidence", "docs/hosted/live-tool-evidence.json", "--check", "docs/hosted/live-tool-matrix.json"}); err != nil {
		t.Fatalf("live inventory must use the canonical tool catalogue without CLI schema I/O: %v", err)
	}
}
