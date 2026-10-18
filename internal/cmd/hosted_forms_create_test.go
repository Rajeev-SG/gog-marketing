package cmd

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedFormsCreateRequest(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = "forms_create_blank"
	req.Params.Arguments = args
	return req
}

func TestHostedFormsCreateBuildsLiteralTitleArgvOnly(t *testing.T) {
	specs := hostedFormsCreateTools()
	if len(specs) != 1 {
		t.Fatalf("unexpected hosted forms tool count: %d", len(specs))
	}

	spec := specs[0]
	if spec.Name != "forms_create_blank" || spec.Service != "forms" || spec.Risk != mcpRiskWrite {
		t.Fatalf("unexpected hosted forms create spec: %+v", spec)
	}

	argv, err := spec.BuildArgs(hostedFormsCreateRequest(map[string]any{"title": "Quarterly plan"}))
	if err != nil {
		t.Fatalf("valid title rejected: %v", err)
	}

	want := []string{"forms", "create", "--title=Quarterly plan"}
	if len(argv) != len(want) {
		t.Fatalf("unexpected argv: %v", argv)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}

func TestHostedFormsCreateTitleCannotEscapeIntoAccountFlag(t *testing.T) {
	argv, err := hostedFormsCreateTools()[0].BuildArgs(hostedFormsCreateRequest(map[string]any{"title": "Quarterly --account=foreign"}))
	if err != nil {
		t.Fatalf("valid title rejected: %v", err)
	}

	if len(argv) != 3 || argv[2] != "--title=Quarterly --account=foreign" {
		t.Fatalf("title escaped the literal --title flag: %v", argv)
	}

	for _, arg := range argv {
		if strings.HasPrefix(arg, "--account") {
			t.Fatalf("title produced an account flag: %v", argv)
		}
	}
}

func TestHostedFormsCreateRejectsInvalidTitles(t *testing.T) {
	build := hostedFormsCreateTools()[0].BuildArgs

	denials := []struct {
		label string
		title string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"unicode control", "Quarterly\u0085plan"},
		{"bell control", "Quarterly\x07plan"},
		{"oversized", strings.Repeat("a", 513)},
	}
	for _, denial := range denials {
		argv, err := build(hostedFormsCreateRequest(map[string]any{"title": denial.title}))
		if err == nil || argv != nil {
			t.Fatalf("%s was not rejected: argv=%v err=%v", denial.label, argv, err)
		}
	}

	if _, err := build(hostedFormsCreateRequest(nil)); err == nil {
		t.Fatal("missing title accepted")
	}

	argv, err := build(hostedFormsCreateRequest(map[string]any{"title": strings.Repeat("a", 512)}))
	if err != nil || len(argv) != 3 {
		t.Fatalf("512-byte title rejected: argv=%v err=%v", argv, err)
	}
}
