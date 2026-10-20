package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/hosted/policy"
)

func TestCapabilityReviewCoversActualRemainingNativeCommands(t *testing.T) {
	var document capabilityReviewDocument
	if err := json.Unmarshal(capabilityReviewJSON, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || len(document.Entries) != 164 || document.ReviewedSourceSHA == "" {
		t.Fatal("missing reviewed snapshot metadata")
	}
	seen := map[string]bool{}
	methods := map[string]map[string]bool{}
	for _, entry := range document.Entries {
		if seen[entry.Command] {
			t.Fatalf("duplicate native review %s", entry.Command)
		}
		seen[entry.Command] = true
		path := filepath.Join("../..", entry.NativeHandler.File)
		if !strings.HasPrefix(entry.NativeHandler.File, "internal/cmd/") {
			t.Fatalf("invalid source path %s", entry.Command)
		}
		receivers, ok := methods[path]
		if !ok {
			receivers = map[string]bool{}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Name.Name != "Run" {
					continue
				}
				receiver := fn.Recv.List[0].Type
				if star, ok := receiver.(*ast.StarExpr); ok {
					receiver = star.X
				}
				if id, ok := receiver.(*ast.Ident); ok {
					receivers[id.Name] = true
				}
			}
			methods[path] = receivers
		}
		if !receivers[entry.NativeHandler.Receiver] {
			t.Fatalf("%s lacks the cited native Run handler", entry.Command)
		}
		status, _ := classifyReviewedCapability(entry)
		if len(entry.AdditionalScopeRequirements) > 0 {
			if entry.AdditionalScopeEvidence == "" {
				t.Fatalf("%s lacks additional-scope evidence", entry.Command)
			}
			if status == "eligible_missing" || entry.PaidAssessment == "not_required_for_approved_scope_variant" {
				t.Fatalf("%s confuses method-level scopes with complete transitive authorization", entry.Command)
			}
		}
		if status == "eligible_missing" && entry.PaidAssessment != "not_required_for_approved_scope_variant" {
			t.Fatalf("%s was assumed eligible without approved-scope evidence", entry.Command)
		}
		if entry.PaidAssessment == "not_required_for_approved_scope_variant" {
			if err := policy.ValidateScopes(entry.Scopes); err != nil {
				t.Fatalf("%s declared nonrestricted with blocked scope: %v", entry.Command, err)
			}
		}
		if len(entry.GoogleMethods) == 0 {
			t.Fatalf("%s lacks method evidence", entry.Command)
		}
		for _, method := range entry.GoogleMethods {
			evidence, known := document.MethodEvidence[method]
			if !known || evidence.HTTPMethod == "" || evidence.Path == "" {
				t.Fatalf("%s has no official method snapshot for %s", entry.Command, method)
			}
			if entry.PaidAssessment == "not_required_for_approved_scope_variant" {
				if !slices.ContainsFunc(entry.Scopes, func(scope string) bool { return slices.Contains(evidence.Scopes, scope) }) {
					t.Fatalf("%s method %s does not accept declared scopes", entry.Command, method)
				}
			}
		}
	}
	matrix := canonicalMatrix(t)
	for _, leaf := range matrix.Leaves {
		if leaf.Status == "deferred" {
			continue
		}
		key := leaf.Service + " " + leaf.Command
		if leaf.HostedTool == nil && (!seen[key] || leaf.EngineeringReview == nil) {
			t.Errorf("remaining native command escaped review: %s", key)
		}
		delete(seen, key)
	}
	if len(seen) != 0 {
		t.Fatalf("reviews cite nonexistent/non-priority commands: %v", seen)
	}
	if matrix.Classifications["unreviewed_mapping"] != 0 {
		t.Fatal("current remaining native operations are not fully reviewed")
	}
}

func TestFixedRawReadsAreNotArbitraryAPIExclusions(t *testing.T) {
	matrix := canonicalMatrix(t)
	for _, service := range []string{"docs", "slides", "sheets", "drive", "forms"} {
		found := false
		for _, leaf := range matrix.Leaves {
			if leaf.Service != service || leaf.Command != "raw" {
				continue
			}
			found = true
			if service == "forms" {
				if leaf.HostedTool == nil || *leaf.HostedTool != "forms_get" || leaf.ImplementationState != "executable_equivalent_read_variant_live_evidence_separate" {
					t.Fatal("Forms fixed GET alias was not mapped to existing typed tool")
				}
			} else if leaf.Status != "implemented_hosted_unverified" || leaf.HostedTool == nil || *leaf.HostedTool != service+"_raw" || leaf.LiveVerified {
				t.Fatalf("fixed %s raw GET still incorrectly excluded: %s", service, leaf.Status)
			}
			if leaf.EngineeringReview == nil || leaf.EngineeringReview.PaidAssessment != "not_required_for_approved_scope_variant" {
				t.Fatalf("%s raw lost approved-scope evidence", service)
			}
		}
		if !found {
			t.Fatalf("missing actual raw native command %s", service)
		}
	}
}

func TestConnectedSheetsScopeAndBillingAreNotOrdinaryAdapterReadiness(t *testing.T) {
	count := 0
	for command, review := range reviewedCapabilities {
		if !strings.HasPrefix(command, "sheets datasource") {
			continue
		}
		count++
		status, _ := classifyReviewedCapability(review)
		if status != "implementation_design_required" || review.ScopeBoundary != "additional_scope_review_required" || !slices.Contains(review.AdditionalScopeRequirements, "https://www.googleapis.com/auth/bigquery.readonly") {
			t.Fatalf("%s lost additional-scope/cost requirements", command)
		}
		if policy.ValidateScopes(review.AdditionalScopeRequirements) == nil {
			t.Fatalf("%s scope boundary changed: review the recorded requirement before claiming readiness", command)
		}
	}
	if count != 9 {
		t.Fatalf("Connected Sheets source variants reviewed=%d,want9", count)
	}
}

func TestReviewedMultiCallInventoriesRetainRequiredMethods(t *testing.T) {
	for command, required := range map[string][]string{
		"drive sync push": {"drive.files.list", "drive.files.get", "drive.files.create", "drive.files.update"},
		"slides copy":     {"drive.files.get", "drive.files.copy"},
	} {
		review := reviewedCapabilities[command]
		for _, method := range required {
			if !slices.Contains(review.GoogleMethods, method) {
				t.Fatalf("%s omits mandatory native RPC %s", command, method)
			}
		}
	}
}

func TestSlidesCopyEvidenceMatchesActualSharedHelper(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../internal/cmd/drive_copy.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]bool{}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "copyViaDrive" {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			collection, ok := selector.X.(*ast.SelectorExpr)
			if !ok || collection.Sel.Name != "Files" {
				return true
			}
			actual["drive.files."+strings.ToLower(selector.Sel.Name)] = true
			return true
		})
	}
	if !found || len(actual) == 0 {
		t.Fatal("shared native copy helper not inspected")
	}
	recorded := reviewedCapabilities["slides copy"].GoogleMethods
	if len(recorded) != len(actual) {
		t.Fatalf("recorded=%v actual native helper methods=%v", recorded, actual)
	}
	for _, method := range recorded {
		if !actual[method] {
			t.Errorf("inventory invented RPC %s not present in copyViaDrive", method)
		}
	}
}
