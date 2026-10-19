package main

import (
	_ "embed"
	"encoding/json"
)

//go:embed capability-review.json
var capabilityReviewJSON []byte

type capabilityReview struct {
	AdditionalScopeRequirements []string `json:"additional_scope_requirements,omitempty"`
	AdditionalScopeEvidence     string   `json:"additional_scope_evidence,omitempty"`
	Command                     string   `json:"command"`
	ScopeBoundary               string   `json:"scope_boundary"`
	PaidAssessment              string   `json:"paid_assessment"`
	Hostability                 string   `json:"hostability"`
	GoogleMethods               []string `json:"google_methods"`
	Scopes                      []string `json:"scopes"`
	ResourceModel               string   `json:"resource_model"`
	Constraint                  string   `json:"constraint"`
	SourceEvidence              string   `json:"source_evidence"`
	ExistingTool                string   `json:"existing_tool,omitempty"`
	ControllerNotes             []string `json:"controller_notes,omitempty"`
	NativeHandler               struct {
		File         string `json:"file"`
		LineAtReview int    `json:"line_at_review"`
		Receiver     string `json:"receiver"`
	} `json:"native_handler"`
	Variants []struct {
		Name           string `json:"name"`
		Hostability    string `json:"hostability"`
		NativeEvidence string `json:"native_evidence"`
	} `json:"variants,omitempty"`
}
type capabilityMethodEvidence struct {
	HTTPMethod string   `json:"http_method"`
	Path       string   `json:"path"`
	Scopes     []string `json:"scopes"`
}
type capabilityReviewDocument struct {
	SchemaVersion     int                                 `json:"schema_version"`
	ReviewedDate      string                              `json:"reviewed_date"`
	ReviewedSourceSHA string                              `json:"reviewed_source_sha"`
	MethodEvidence    map[string]capabilityMethodEvidence `json:"method_evidence"`
	Entries           []capabilityReview                  `json:"entries"`
}

var reviewedCapabilities = loadCapabilityReviews()

func loadCapabilityReviews() map[string]capabilityReview {
	var document capabilityReviewDocument
	if err := json.Unmarshal(capabilityReviewJSON, &document); err != nil {
		panic(err)
	}
	result := make(map[string]capabilityReview, len(document.Entries))
	for _, entry := range document.Entries {
		result[entry.Command] = entry
	}
	return result
}

func classifyReviewedCapability(review capabilityReview) (string, string) {
	if review.ScopeBoundary == "outside_current_allowlist" && review.Hostability != "owner_excluded" {
		return "scope_boundary_blocked", review.Constraint
	}
	switch review.Hostability {
	case "ready_for_fixed_adapter", "already_covered_variant":
		return "eligible_missing", review.Constraint
	case "requires_resource_or_transport_design":
		return "implementation_design_required", review.Constraint
	case "owner_excluded":
		return "excluded", review.Constraint
	default:
		return "unreviewed_mapping", "Engineering scope/resource review is incomplete; this does not establish a paid Google security-assessment requirement."
	}
}
