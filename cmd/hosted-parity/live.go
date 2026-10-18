package main

// Live matrix generation: reads curated evidence facts and combines them with
// the canonical hosted tool catalogue and static binding metadata. It performs
// no schema subprocess, network, Google or auth calls and never proves live
// behavior beyond the curated representative-variant records.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/openclaw/gogcli/internal/cmd"
	"github.com/openclaw/gogcli/internal/hosted/runner"
)

const (
	liveMatrixSchemaVersion = 1

	// candidateLiveStateUnverified is the only candidate live state this
	// generator emits, even for tools with historical evidence records.
	candidateLiveStateUnverified = "implemented_unverified_live"

	// expectedEvidenceState is the exact evidence state accepted from curated
	// facts; anything else is rejected.
	expectedEvidenceState = "representative_variant_verified_existing_deployment"
)

var errLiveInventoryInput = errors.New("invalid live inventory input")

var errStaleLiveMatrix = errors.New("generated live matrix is stale; regenerate the live matrix JSON artifact")

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	shaPattern  = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
)

// liveEvidenceRecord mirrors the curated evidence facts verbatim. Unknown JSON
// fields are rejected at parse time and a valid record is embedded unchanged
// in the row of the tool it names.
type liveEvidenceRecord struct {
	Tool              string  `json:"tool"`
	Date              string  `json:"date"`
	DeploymentVersion string  `json:"deployment_version"`
	SourceSHA         *string `json:"source_sha"`
	Variant           string  `json:"variant"`
	AllowEvidence     string  `json:"allow_evidence"`
	DenyEvidence      string  `json:"deny_evidence"`
	DenyVariant       string  `json:"deny_variant"`
	State             string  `json:"state"`
}

// liveToolRow is one unique active hosted tool with static admission metadata
// and, when present, its curated historical evidence record.
type liveToolRow struct {
	Service            string              `json:"service"`
	Tool               string              `json:"tool"`
	Write              bool                `json:"write"`
	DestructiveBinding bool                `json:"destructive_binding_flag"`
	ResourceModel      *string             `json:"resource_model,omitempty"`
	ResourceArgument   *string             `json:"resource_argument,omitempty"`
	AcceptedScopes     []string            `json:"accepted_scopes"`
	HTTPMethods        []string            `json:"http_methods"`
	HTTPMethodsKnown   bool                `json:"http_methods_known"`
	CandidateLiveState string              `json:"candidate_live_state"`
	HistoricalEvidence *liveEvidenceRecord `json:"historical_evidence,omitempty"`
}

type liveMatrix struct {
	SchemaVersion          int            `json:"schema_version"`
	Scope                  map[string]any `json:"scope"`
	TotalCatalogCount      int            `json:"total_catalog_count"`
	ActiveToolCount        int            `json:"active_tool_count"`
	HistoricalEvidenceRows int            `json:"historical_evidence_rows"`
	Rows                   []liveToolRow  `json:"rows"`
	LiveVerificationNotice string         `json:"live_verification_notice"`
	Limitations            []string       `json:"limitations"`
}

const liveMatrixNotice = "Every candidate_live_state is implemented_unverified_live: catalogue membership and fixture tests never prove live behavior. Only rows with historical_evidence carry representative_variant_verified_existing_deployment proof, and that proof is limited to the exact variant described on an existing deployment. Static binding metadata (destructive flag, accepted scopes, static HTTP methods) is admission-time description, never conditional approval of a native variant."

var liveMatrixLimitations = []string{
	"Historical evidence covers only representative variants on an existing deployment and never constitutes whole-tool, whole-service or exhaustive candidate acceptance.",
	"Rows without historical_evidence are historically unverified; proof is never inferred from tool name prefix, fixture presence or catalogue membership.",
	"accepted_scopes lists alternatives declared by the runner binding, not a claim that every listed scope is required for every method.",
	"http_methods_known=false marks a binding that declares no static method; no HTTP method is inferred.",
	"destructive_binding_flag is the static admission flag from the runner binding; a false value does not guarantee that every variant is non-destructive.",
}

// runLiveMatrix reads the curated evidence file, builds the live matrix from
// the canonical catalogue and binding metadata, and either prints the
// deterministic JSON document (plus trailing newline) to stdout or, when
// checkPath is set, verifies the artifact at checkPath byte-for-byte.
func runLiveMatrix(evidencePath, checkPath string) error {
	payload, err := os.ReadFile(evidencePath) //nolint:gosec // Explicit local CLI input, never a hosted/request-controlled file path.
	if err != nil {
		return fmt.Errorf("read live evidence: %w", err)
	}
	records, err := parseLiveEvidence(payload)
	if err != nil {
		return fmt.Errorf("parse live evidence %s: %w", evidencePath, err)
	}
	matrix, err := BuildLiveMatrix(runner.NativeToolCatalog(cmd.HostedExistingToolCatalog()), runner.NativeBindingMetadata(), records)
	if err != nil {
		return fmt.Errorf("build live matrix: %w", err)
	}
	data, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return fmt.Errorf("encode live matrix: %w", err)
	}
	data = append(data, '\n')

	if checkPath == "" {
		if _, writeErr := os.Stdout.Write(data); writeErr != nil {
			return fmt.Errorf("write live matrix: %w", writeErr)
		}
		return nil
	}
	current, err := os.ReadFile(checkPath) //nolint:gosec // Explicit local CLI artifact selected by the operator.
	if err != nil {
		return fmt.Errorf("read live matrix for check: %w", err)
	}
	if !bytes.Equal(data, current) {
		return errStaleLiveMatrix
	}
	return nil
}

// BuildLiveMatrix is the pure builder behind runLiveMatrix: it combines the
// canonical tool catalogue, canonical binding metadata and parsed evidence
// records into the deterministic live-matrix document without any I/O.
func BuildLiveMatrix(tools []runner.NativeToolDescription, bindings []runner.BindingMetadata, evidence []liveEvidenceRecord) (liveMatrix, error) {
	activeTools := make(map[string]runner.NativeToolDescription, len(tools))
	for _, tool := range tools {
		if !activeService(tool.Service) {
			continue
		}
		if _, duplicate := activeTools[tool.Name]; duplicate {
			return liveMatrix{}, fmt.Errorf("%w: duplicate catalogue row for active tool %q", errLiveInventoryInput, tool.Name)
		}
		activeTools[tool.Name] = tool
	}

	bindingByName := make(map[string]runner.BindingMetadata, len(bindings))
	for _, binding := range bindings {
		if _, duplicate := bindingByName[binding.Name]; duplicate {
			return liveMatrix{}, fmt.Errorf("%w: duplicate binding metadata for tool %q", errLiveInventoryInput, binding.Name)
		}
		bindingByName[binding.Name] = binding
	}

	rows := make([]liveToolRow, 0, len(activeTools))
	for _, tool := range activeTools {
		binding, found := bindingByName[tool.Name]
		if !found {
			return liveMatrix{}, fmt.Errorf("%w: no binding metadata for active tool %q", errLiveInventoryInput, tool.Name)
		}
		if binding.Service != tool.Service {
			return liveMatrix{}, fmt.Errorf("%w: binding metadata for tool %q declares service %q but catalogue declares %q", errLiveInventoryInput, tool.Name, binding.Service, tool.Service)
		}
		scopes := append([]string(nil), binding.Scopes...)
		methods := append([]string(nil), binding.Methods...)
		sort.Strings(scopes)
		sort.Strings(methods)
		row := liveToolRow{
			Service:            tool.Service,
			Tool:               tool.Name,
			Write:              tool.Write,
			DestructiveBinding: binding.Destructive,
			AcceptedScopes:     nonNilStrings(scopes),
			HTTPMethods:        nonNilStrings(methods),
			HTTPMethodsKnown:   len(binding.Methods) > 0,
			CandidateLiveState: candidateLiveStateUnverified,
		}
		model := tool.ResourceType
		if model == "" {
			model = "account"
		}
		row.ResourceModel = &model
		if tool.ResourceArgument != "" {
			argument := tool.ResourceArgument
			row.ResourceArgument = &argument
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Service != rows[j].Service {
			return rows[i].Service < rows[j].Service
		}
		return rows[i].Tool < rows[j].Tool
	})

	evidenceByTool := make(map[string]liveEvidenceRecord, len(evidence))
	for _, record := range evidence {
		if _, duplicate := evidenceByTool[record.Tool]; duplicate {
			return liveMatrix{}, fmt.Errorf("%w: duplicate evidence record for tool %q", errLiveInventoryInput, record.Tool)
		}
		if _, known := activeTools[record.Tool]; !known {
			return liveMatrix{}, fmt.Errorf("%w: evidence references tool %q that is not in the active hosted catalogue (unknown or deferred)", errLiveInventoryInput, record.Tool)
		}
		if err := validateLiveEvidenceRecord(record); err != nil {
			return liveMatrix{}, fmt.Errorf("invalid evidence record for tool %q: %w", record.Tool, err)
		}
		evidenceByTool[record.Tool] = record
	}
	historicalRows := 0
	for index := range rows {
		if record, found := evidenceByTool[rows[index].Tool]; found {
			evidenceCopy := record
			rows[index].HistoricalEvidence = &evidenceCopy
			historicalRows++
		}
	}

	return liveMatrix{
		SchemaVersion: liveMatrixSchemaVersion,
		Scope: map[string]any{
			"active_services":         activeServices,
			"deferred_services":       []string{"calendar", "drivelabels"},
			"candidate_live_state":    candidateLiveStateUnverified,
			"expected_evidence_state": expectedEvidenceState,
		},
		TotalCatalogCount:      len(tools),
		ActiveToolCount:        len(activeTools),
		HistoricalEvidenceRows: historicalRows,
		Rows:                   rows,
		LiveVerificationNotice: liveMatrixNotice,
		Limitations:            liveMatrixLimitations,
	}, nil
}

// parseLiveEvidence strictly decodes the curated evidence file: the document
// must be exactly one JSON array of evidence records, with no unknown fields
// and no trailing JSON.
func parseLiveEvidence(payload []byte) ([]liveEvidenceRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var records *[]liveEvidenceRecord
	if err := decoder.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode evidence array: %w", err)
	}
	if records == nil {
		return nil, fmt.Errorf("%w: evidence document must be a JSON array of evidence records", errLiveInventoryInput)
	}
	var trailing any
	switch err := decoder.Decode(&trailing); {
	case errors.Is(err, io.EOF):
		return *records, nil
	case err == nil:
		return nil, fmt.Errorf("%w: trailing JSON after evidence array", errLiveInventoryInput)
	default:
		return nil, fmt.Errorf("trailing data after evidence array: %w", err)
	}
}

// validateLiveEvidenceRecord enforces the curated-fact contract: tool
// membership in the active catalogue is checked by the caller; this covers the
// ISO-day date, UUID deployment version, optional 40-hex source SHA, HTTPS
// provenance URLs, the required variant/denial fields and the exact expected
// evidence state.
func validateLiveEvidenceRecord(record liveEvidenceRecord) error {
	if strings.TrimSpace(record.Variant) == "" {
		return fmt.Errorf("%w: missing variant", errLiveInventoryInput)
	}
	if strings.TrimSpace(record.DenyEvidence) == "" {
		return fmt.Errorf("%w: missing deny_evidence", errLiveInventoryInput)
	}
	if strings.TrimSpace(record.DenyVariant) == "" {
		return fmt.Errorf("%w: missing deny_variant", errLiveInventoryInput)
	}
	if record.State != expectedEvidenceState {
		return fmt.Errorf("%w: state %q is not the expected %q", errLiveInventoryInput, record.State, expectedEvidenceState)
	}
	if _, err := time.Parse("2006-01-02", record.Date); err != nil {
		return fmt.Errorf("date %q is not an ISO calendar day: %w", record.Date, err)
	}
	if !uuidPattern.MatchString(record.DeploymentVersion) {
		return fmt.Errorf("%w: deployment_version %q is not a UUID", errLiveInventoryInput, record.DeploymentVersion)
	}
	if record.SourceSHA != nil && !shaPattern.MatchString(*record.SourceSHA) {
		return fmt.Errorf("%w: source_sha %q is not a 40-hex SHA", errLiveInventoryInput, *record.SourceSHA)
	}
	if err := requireHTTPSURL(record.AllowEvidence, "allow_evidence"); err != nil {
		return err
	}
	if err := requireHTTPSURL(record.DenyEvidence, "deny_evidence"); err != nil {
		return err
	}
	return nil
}

func requireHTTPSURL(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is empty", errLiveInventoryInput, field)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s %q is not a valid URL: %w", field, value, err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("%w: %s %q must be an https URL", errLiveInventoryInput, field, value)
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
