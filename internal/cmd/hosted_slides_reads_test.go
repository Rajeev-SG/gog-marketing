package cmd

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedSlidesReadRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = maps.Clone(args)
	return req
}

func TestHostedSlidesReadSchemas(t *testing.T) {
	tests := []struct {
		name     string
		fields   []string
		required []string
	}{
		{name: "slides_info", fields: []string{"presentation_id"}, required: []string{"presentation_id"}},
		{name: "slides_read_slide", fields: []string{"presentation_id", "slide_id", "detail"}, required: []string{"presentation_id", "slide_id"}},
		{name: "slides_locate", fields: []string{"presentation_id", "text", "page_id", "match_case", "all", "occurrence", "fail_empty"}, required: []string{"presentation_id", "text"}},
	}

	specs := hostedSlidesReadTools()
	byName := make(map[string]mcpToolSpec, len(specs))
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
		byName[spec.Name] = spec
	}
	if !slices.Equal(names, []string{"slides_info", "slides_read_slide", "slides_locate"}) {
		t.Fatalf("tool names = %v", names)
	}

	for _, test := range tests {
		t.Run(test.name+"/schema", func(t *testing.T) {
			spec := byName[test.name]
			if spec.Service != "slides" || spec.Risk != mcpRiskRead {
				t.Fatalf("service/risk = %q/%q, want slides/read", spec.Service, spec.Risk)
			}

			schema := mcp.NewTool(spec.Name, spec.Options...).InputSchema
			if len(schema.Properties) != len(test.fields) {
				t.Fatalf("properties = %v, want %v", schema.Properties, test.fields)
			}
			for _, field := range test.fields {
				if schema.Properties[field] == nil {
					t.Fatalf("missing property %q", field)
				}
			}
			if !slices.Equal(schema.Required, test.required) {
				t.Fatalf("required = %v, want %v", schema.Required, test.required)
			}
		})
	}
}

func TestHostedSlidesReadBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
		want []string
	}{
		{name: "info", tool: "slides_info", args: map[string]any{"presentation_id": "Drive-File_1"}, want: []string{"slides", "info", "--", "Drive-File_1"}},
		{name: "read default", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, want: []string{"slides", "read-slide", "--", "selected-deck", "p:0"}},
		{name: "read explicit false", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "shape:one", "detail": false}, want: []string{"slides", "read-slide", "--", "selected-deck", "shape:one"}},
		{name: "read detail", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0", "detail": true}, want: []string{"slides", "read-slide", "--detail", "--", "selected-deck", "p:0"}},
		{
			name: "locate all options",
			tool: "slides_locate",
			args: map[string]any{
				"presentation_id": "selected-deck", "text": "launch", "page_id": "p:0",
				"match_case": true, "all": true, "fail_empty": true,
			},
			want: []string{"slides", "locate", "--page=p:0", "--match-case", "--all", "--fail-empty", "--", "selected-deck", "launch"},
		},
		{
			name: "literal whitespace and flag-looking text remain positional",
			tool: "slides_locate",
			args: map[string]any{"presentation_id": "selected-deck", "text": "\n --account=foreign@x \n"},
			want: []string{"slides", "locate", "--", "selected-deck", "\n --account=foreign@x \n"},
		},
	}

	specs := hostedSlidesReadTools()
	byName := make(map[string]mcpToolSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			argv, err := byName[test.tool].BuildArgs(hostedSlidesReadRequest(test.args))
			if err != nil {
				t.Fatalf("BuildArgs() error = %v", err)
			}
			if !slices.Equal(argv, test.want) {
				t.Fatalf("argv = %#v, want %#v", argv, test.want)
			}
		})
	}
}

func TestHostedSlidesReadInvalidInput(t *testing.T) {
	specs := hostedSlidesReadTools()
	byName := make(map[string]mcpToolSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}

	cases := []struct {
		name string
		tool string
		args map[string]any
		want error // nil means any non-nil error is valid for a required argument.
	}{
		{name: "conflicting selectors", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "all": true, "occurrence": float64(2)}, want: errHostedSlidesReadOption},
		{name: "missing presentation", tool: "slides_info", args: map[string]any{}},
		{name: "empty presentation", tool: "slides_info", args: map[string]any{"presentation_id": " "}},
		{name: "url presentation", tool: "slides_info", args: map[string]any{"presentation_id": "https://evil.example/deck"}, want: errHostedSlidesReadID},
		{name: "path presentation", tool: "slides_info", args: map[string]any{"presentation_id": "../foreign"}, want: errHostedSlidesReadID},
		{name: "missing slide", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck"}},
		{name: "empty slide", tool: "slides_read_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": ""}},
		{name: "missing text", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck"}, want: errHostedSlidesText},
		{name: "empty text", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": ""}, want: errHostedSlidesText},
		{name: "zero occurrence", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "occurrence": float64(0)}, want: errHostedSlidesOccurrence},
		{name: "fractional occurrence", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "occurrence": float64(1.5)}, want: errHostedSlidesOccurrence},
		{name: "too-large occurrence", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "occurrence": float64(2147483648)}, want: errHostedSlidesOccurrence},
		{name: "string occurrence", tool: "slides_locate", args: map[string]any{"presentation_id": "selected-deck", "text": "launch", "occurrence": "2"}, want: errHostedSlidesOccurrence},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			argv, err := byName[test.tool].BuildArgs(hostedSlidesReadRequest(test.args))
			if test.want == nil && err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if argv != nil {
				t.Fatalf("argv = %#v, want nil", argv)
			}
		})
	}

	boundary := float64(2147483647)
	argv, err := byName["slides_locate"].BuildArgs(hostedSlidesReadRequest(map[string]any{
		"presentation_id": "selected-deck", "text": "launch", "occurrence": boundary,
	}))
	if err != nil {
		t.Fatalf("maximum occurrence rejected: %v", err)
	}
	if !slices.Equal(argv, []string{"slides", "locate", "--occurrence=2147483647", "--", "selected-deck", "launch"}) {
		t.Fatalf("maximum occurrence argv = %#v", argv)
	}
}
