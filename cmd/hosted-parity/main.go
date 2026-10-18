// Command hosted-parity generates a reproducible native command to hosted tool
// inventory from the live Kong schema and executable hosted tool catalog.
// It performs no Google operations and does not infer live verification.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/openclaw/gogcli/internal/cmd"
	"github.com/openclaw/gogcli/internal/hosted/runner"
)

const matrixSchemaVersion = 2

var (
	errStaleMatrix        = errors.New("generated matrix is stale; regenerate docs/hosted/cli-hosted-matrix.json")
	errMissingServiceTool = errors.New("special command map references missing service/tool")
	errMissingHostedTool  = errors.New("special command map references missing hosted tool")
)

var activeServices = []string{
	"analytics", "appscript", "bigquery", "docs", "drive", "forms",
	"searchconsole", "sheets", "slides", "tagmanager", "tasks",
}

// commandToHosted contains only reviewed naming bridges that are not produced
// by the exact-name rule below. The canonical catalog remains authoritative.
var commandToHosted = map[string]string{
	"docs create":                         "docs_create_blank",
	"slides create":                       "slides_create_blank",
	"drive delete":                        "drive_trash",
	"sheets create":                       "sheets_create_blank",
	"sheets add-tab":                      "sheets_add_tab",
	"sheets rename-tab":                   "sheets_rename_tab",
	"sheets delete-tab":                   "sheets_delete_tab",
	"sheets number-format":                "sheets_number_format_range",
	"docs cat":                            "docs_get",
	"slides list-slides":                  "slides_list_slides",
	"slides update-notes":                 "slides_update_notes",
	"tasks lists list":                    "tasks_lists",
	"sheets get":                          "sheets_read_range",
	"sheets update":                       "sheets_update_range",
	"sheets append":                       "sheets_append_range",
	"sheets clear":                        "sheets_clear_range",
	"sheets metadata":                     "sheets_metadata",
	"bigquery datasets get":               "bigquery_dataset_get",
	"bigquery tables get":                 "bigquery_table_get",
	"bigquery tables rows":                "bigquery_table_rows",
	"bigquery tables schema":              "bigquery_table_schema",
	"searchconsole searchanalytics query": "searchconsole_query",
}

// Only these exact missing commands have attributable method/scope/product
// review. Service-level OAuth eligibility alone never authorizes an adapter.
var reviewedEligibleMissing = map[string]string{
	"sheets add-tab":        "Reviewed selected-spreadsheet POST batchUpdate/addSheet; spreadsheets or drive.file and exact file/tool write grants",
	"sheets rename-tab":     "Reviewed selected-spreadsheet GET plus POST batchUpdate/updateSheetProperties; spreadsheets or drive.file and file/tool write grants",
	"sheets delete-tab":     "Reviewed selected-spreadsheet GET plus POST batchUpdate/deleteSheet; spreadsheets or drive.file and destructive file/tool permission",
	"sheets number-format":  "Reviewed selected-spreadsheet GET plus fixed number-format POST batchUpdate; spreadsheets or drive.file and file/tool write grants",
	"appscript deployments": "Reviewed GET projects/{scriptId}/deployments; script.deployments.readonly or script.deployments and an account-level exact tool grant",
	"appscript versions":    "Reviewed GET projects/{scriptId}/versions; script.projects.readonly or script.projects and an account-level exact tool grant",
}

func activeService(name string) bool {
	for _, service := range activeServices {
		if service == name {
			return true
		}
	}
	return false
}

type schemaNode struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Path        string        `json:"path"`
	Subcommands []*schemaNode `json:"subcommands,omitempty"`
}

type schemaDocument struct {
	Command *schemaNode `json:"command"`
}

type bindingMetadata struct {
	AcceptedScopes   []string `json:"accepted_scopes"`
	HTTPMethods      []string `json:"http_methods"`
	HTTPMethodsKnown bool     `json:"http_methods_known"`
	Destructive      bool     `json:"destructive"`
}

type parityLeaf struct {
	Service              string           `json:"service"`
	Command              string           `json:"command"`
	NativeCommand        string           `json:"native_command"`
	HostedTool           *string          `json:"hosted_tool"`
	Status               string           `json:"status"`
	ImplementationState  string           `json:"implementation_state,omitempty"`
	LiveVerified         bool             `json:"live_verified"`
	ClassificationReason string           `json:"classification_reason,omitempty"`
	ResourceModel        *string          `json:"resource_model,omitempty"`
	ResourceArgument     *string          `json:"resource_argument,omitempty"`
	PolicyBinding        *bindingMetadata `json:"policy_binding,omitempty"`
	BindingEvidence      string           `json:"binding_evidence,omitempty"`
}

type serviceSummary struct {
	Service            string         `json:"service"`
	NativeLeaves       int            `json:"native_leaves"`
	HostedTools        int            `json:"hosted_tools"`
	MappedNativeLeaves int            `json:"mapped_native_leaves"`
	Classifications    map[string]int `json:"classifications"`
}

type parityMatrix struct {
	SchemaVersion           int              `json:"schema_version"`
	Scope                   map[string]any   `json:"scope"`
	NativeLeafCount         int              `json:"native_leaf_count"`
	HostedToolCount         int              `json:"hosted_tool_count"`
	TotalHostedCatalogCount int              `json:"total_hosted_catalog_count"`
	ExcludedHostedToolCount int              `json:"excluded_hosted_tool_count"`
	MappedNativeLeafCount   int              `json:"mapped_native_leaf_count"`
	MappedHostedToolCount   int              `json:"mapped_hosted_tool_count"`
	Classifications         map[string]int   `json:"classifications"`
	ByService               []serviceSummary `json:"by_service"`
	Leaves                  []parityLeaf     `json:"leaves"`
	UnmappedHostedTools     []string         `json:"unmapped_hosted_tools"`
	LiveVerificationNotice  string           `json:"live_verification_notice"`
	Limitations             []string         `json:"limitations"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "hosted-parity: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("hosted-parity", flag.ContinueOnError)
	schemaPath := flags.String("schema-json", "", "read the canonical `gog schema --json` document from a file instead of running gog")
	checkPath := flags.String("check", "", "compare generated output with `path` instead of printing it")
	liveMode := flags.Bool("live-matrix", false, "generate per-tool live evidence inventory without a CLI schema subprocess")
	evidencePath := flags.String("evidence", "docs/hosted/live-tool-evidence.json", "curated existing-deployment evidence JSON for live inventory mode")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if *liveMode {
		return runLiveMatrix(*evidencePath, *checkPath)
	}

	var schema schemaDocument
	if *schemaPath == "" {
		payload, err := exec.CommandContext(context.Background(), "go", "run", "./cmd/gog", "schema", "--json").Output()
		if err != nil {
			return fmt.Errorf("run native schema: %w", err)
		}
		if err := json.Unmarshal(payload, &schema); err != nil {
			return fmt.Errorf("decode native schema: %w", err)
		}
	} else {
		payload, err := os.ReadFile(*schemaPath)
		if err != nil {
			return fmt.Errorf("read native schema: %w", err)
		}
		if err := json.Unmarshal(payload, &schema); err != nil {
			return fmt.Errorf("decode native schema: %w", err)
		}
	}

	bindings := map[string]bindingMetadata{}
	for _, binding := range runner.NativeBindingMetadata() {
		scopes := append([]string(nil), binding.Scopes...)
		methods := append([]string(nil), binding.Methods...)
		sort.Strings(scopes)
		sort.Strings(methods)
		bindings[binding.Name] = bindingMetadata{
			AcceptedScopes:   scopes,
			HTTPMethods:      methods,
			HTTPMethodsKnown: len(methods) > 0,
			Destructive:      binding.Destructive,
		}
	}
	matrix, err := BuildMatrix(schema.Command, runner.NativeToolCatalog(cmd.HostedExistingToolCatalog()), bindings)
	if err != nil {
		return fmt.Errorf("build matrix: %w", err)
	}
	payload, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return fmt.Errorf("encode matrix: %w", err)
	}
	payload = append(payload, '\n')

	if *checkPath == "" {
		if _, writeErr := os.Stdout.Write(payload); writeErr != nil {
			return fmt.Errorf("write matrix: %w", writeErr)
		}
		return nil
	}
	current, err := os.ReadFile(*checkPath)
	if err != nil {
		return fmt.Errorf("read matrix for check: %w", err)
	}
	if !bytes.Equal(payload, current) {
		return errStaleMatrix
	}
	return nil
}

// BuildMatrix accepts canonical inputs explicitly so tests can exercise exact
// naming, duplicate native aliases and unknown mappings without process forks.
func BuildMatrix(root *schemaNode, hosted []runner.NativeToolDescription, bindings map[string]bindingMetadata) (parityMatrix, error) {
	leaves := nativeLeaves(root)
	hostedByService := map[string]map[string]bool{}
	for _, tool := range hosted {
		if !activeService(tool.Service) {
			continue
		}
		if hostedByService[tool.Service] == nil {
			hostedByService[tool.Service] = map[string]bool{}
		}
		hostedByService[tool.Service][tool.Name] = true
	}

	for command, tool := range commandToHosted {
		service, _, found := strings.Cut(command, " ")
		if !found {
			return parityMatrix{}, fmt.Errorf("%w: %s -> %s", errMissingServiceTool, command, tool)
		}
		if tools, present := hostedByService[service]; present && !tools[tool] {
			return parityMatrix{}, fmt.Errorf("%w: %s -> %s", errMissingHostedTool, command, tool)
		}
	}

	out := make([]parityLeaf, 0, len(leaves))
	matchedTools := map[string]bool{}
	mappedNativeLeafCount := 0
	classifications := map[string]int{}
	for _, leaf := range leaves {
		entry := parityLeaf{
			Service:       leaf.service,
			Command:       leaf.command,
			NativeCommand: "gog " + leaf.path,
			LiveVerified:  false,
		}
		if tool, mapped := hostedMapping(leaf.service, leaf.command, hostedByService); mapped {
			entry.HostedTool = &tool
			entry.Status = "implemented_hosted_unverified"
			entry.ImplementationState = "executable_fixed_schema_live_evidence_separate"
			entry.ClassificationReason = "exact canonical hosted tool invokes this native handler path"
			matchedTools[tool] = true
			mappedNativeLeafCount++
			if metadata, found := bindings[tool]; found {
				entry.PolicyBinding = &metadata
				entry.BindingEvidence = "runner binding metadata"
				if !metadata.HTTPMethodsKnown {
					entry.BindingEvidence += "; HTTP method unknown because binding omits static methods"
				}
			} else {
				entry.BindingEvidence = "unknown binding metadata"
			}
			if description := hostedToolDescription(hosted, tool); description != nil {
				model, argument := description.ResourceType, description.ResourceArgument
				if model != "" {
					entry.ResourceModel = &model
				}
				if argument != "" {
					entry.ResourceArgument = &argument
				}
			}
		} else {
			entry.Status, entry.ClassificationReason = classifyMissing(leaf.service, leaf.command)
		}
		classifications[entry.Status]++
		out = append(out, entry)
	}

	unmapped := make([]string, 0)
	for _, tool := range hosted {
		if activeService(tool.Service) && !matchedTools[tool.Name] {
			unmapped = append(unmapped, tool.Name)
		}
	}
	sort.Strings(unmapped)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Command < out[j].Command
	})

	summaries := make([]serviceSummary, 0, len(activeServices))
	activeHostedToolCount := 0
	for _, service := range activeServices {
		summary := serviceSummary{
			Service:            service,
			Classifications:    map[string]int{},
			HostedTools:        len(hostedByService[service]),
			MappedNativeLeaves: 0,
		}
		activeHostedToolCount += summary.HostedTools
		for _, leaf := range out {
			if leaf.Service == service {
				summary.NativeLeaves++
				summary.Classifications[leaf.Status]++
				if leaf.HostedTool != nil {
					summary.MappedNativeLeaves++
				}
			}
		}
		summaries = append(summaries, summary)
	}

	return parityMatrix{
		SchemaVersion: matrixSchemaVersion,
		Scope: map[string]any{
			"active_services":                     activeServices,
			"deferred_services":                   []string{"calendar", "drivelabels"},
			"prior_deferred_excluded":             true,
			"calendar_drive_labels_in_gap_counts": false,
		},
		NativeLeafCount:         len(out),
		HostedToolCount:         activeHostedToolCount,
		TotalHostedCatalogCount: len(hosted),
		ExcludedHostedToolCount: len(hosted) - activeHostedToolCount,
		MappedNativeLeafCount:   mappedNativeLeafCount,
		MappedHostedToolCount:   len(matchedTools),
		Classifications:         classifications,
		ByService:               summaries,
		Leaves:                  out,
		UnmappedHostedTools:     unmapped,
		LiveVerificationNotice:  "Implemented means an executable fixed-schema hosted tool exists and fixture tests may pass; every live_verified value is false and no Google acceptance is claimed.",
		Limitations: []string{
			"HTTP methods are populated only when the runner binding declares static methods; http_methods_known=false marks omitted methods as unknown.",
			"Accepted scopes are alternatives declared by the runner policy, not a claim that every listed scope is required for every method.",
			"Missing commands without exact method/scope/resource review are missing_requires_audit, not proven eligible.",
			"A mapped native leaf identifies a fixed-schema hosted subset; full CLI option/variant parity is not claimed.",
			"A second Search Console leaf intentionally maps to searchconsole_query; this is command duplication, not duplicate hosted work.",
		},
	}, nil
}

func hostedMapping(service, command string, hostedByService map[string]map[string]bool) (string, bool) {
	if tool, special := commandToHosted[service+" "+command]; special {
		return tool, hostedByService[service][tool]
	}
	tool := service + "_" + strings.ReplaceAll(command, " ", "_")
	return tool, hostedByService[service][tool]
}

type nativeLeaf struct {
	service string
	command string
	path    string
}

func nativeLeaves(root *schemaNode) []nativeLeaf {
	var out []nativeLeaf
	if root == nil {
		return out
	}
	for _, service := range root.Subcommands {
		if service == nil {
			continue
		}
		walkNativeLeaves(service, service.Name, &out)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].service != out[j].service {
			return out[i].service < out[j].service
		}
		return out[i].command < out[j].command
	})
	return out
}

func walkNativeLeaves(node *schemaNode, path string, out *[]nativeLeaf) {
	if len(node.Subcommands) == 0 {
		parts := strings.Split(path, " ")
		if len(parts) < 2 {
			return
		}
		service := parts[0]
		if !activeService(service) {
			return
		}
		*out = append(*out, nativeLeaf{
			service: service,
			command: strings.Join(parts[1:], " "),
			path:    path,
		})
		return
	}
	for _, child := range node.Subcommands {
		if child != nil {
			walkNativeLeaves(child, path+" "+child.Name, out)
		}
	}
}

func classifyMissing(service, command string) (string, string) {
	key := service + " " + command
	if reason, reviewed := reviewedEligibleMissing[key]; reviewed {
		return "eligible_missing", reason
	}

	switch key {
	case "drive labels file apply", "drive labels file list", "drive labels file remove", "drive labels get", "drive labels list":
		return "deferred", "Drive Labels is explicitly deferred and excluded from active gap counts"
	case "drive activity query", "drive audit sharing", "drive audit user",
		"drive changes list", "drive changes poll", "drive changes serve",
		"drive changes start-token", "drive changes stop", "drive changes watch",
		"drive drives", "drive du", "drive inventory", "drive ls", "drive raw",
		"drive search", "drive shortcut create", "drive sync push", "drive tree":
		return "excluded", "global/all-Drive, audit or arbitrary-endpoint behavior is outside selected-file hosted grants"
	case "drive download", "drive bulk remove-public", "drive bulk update-role":
		return "product_limited", "hosted fixed-schema JSON output/runtime or selected-file batching does not currently represent this native behavior"
	case "docs export", "docs raw", "slides export", "slides raw", "slides thumbnail":
		return "product_limited", "hosted fixed-schema JSON output/runtime does not currently represent binary export or raw API behavior"
	case "sheets datasource add", "sheets datasource delete", "sheets datasource describe",
		"sheets datasource list", "sheets datasource refresh", "sheets datasource table describe",
		"sheets datasource table list", "sheets datasource table read", "sheets datasource update",
		"sheets export":
		return "product_limited", "external datasource, billing semantics or local-file export output need a separate verified model"
	case "sheets raw":
		return "excluded", "arbitrary Sheets batch/API passthrough is outside fixed hosted tool policy"
	case "forms raw":
		return "excluded", "arbitrary Forms passthrough is outside fixed hosted tool policy"
	case "appscript pull":
		return "product_limited", "the native command writes a local filesystem tree, which the hosted boundary does not expose"
	case "appscript run":
		return "excluded", "Apps Script execution remains policy-excluded pending transitive-scope audit"
	case "bigquery query":
		return "product_limited", "SQL jobs require a customer billing project, max-bytes-billed and explicit billable approval before hosting"
	}
	return "missing_requires_audit", "no reviewed policy classification exists; audit required before implementation"
}

func hostedToolDescription(tools []runner.NativeToolDescription, name string) *runner.NativeToolDescription {
	for index := range tools {
		if tools[index].Name == name {
			return &tools[index]
		}
	}
	return nil
}
