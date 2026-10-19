package cmd

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func hostedSlidesEditRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = maps.Clone(args)
	return req
}

func TestHostedSlidesEditSchemas(t *testing.T) {
	tests := []struct {
		name     string
		fields   []string
		required []string
	}{
		{name: "slides_skip_slide", fields: []string{"presentation_id", "slide_id"}, required: []string{"presentation_id", "slide_id"}},
		{name: "slides_unskip_slide", fields: []string{"presentation_id", "slide_id"}, required: []string{"presentation_id", "slide_id"}},
		{name: "slides_replace_text", fields: []string{"presentation_id", "find", "replacement", "match_case", "all", "page_ids", "object_id"}, required: []string{"presentation_id", "find", "replacement"}},
	}

	specs := hostedSlidesEditTools()
	byName := make(map[string]mcpToolSpec, len(specs))
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
		byName[spec.Name] = spec
	}
	if !slices.Equal(names, []string{"slides_skip_slide", "slides_unskip_slide", "slides_replace_text"}) {
		t.Fatalf("tool names = %v", names)
	}

	for _, test := range tests {
		t.Run(test.name+"/schema", func(t *testing.T) {
			spec := byName[test.name]
			if spec.Service != "slides" || spec.Risk != mcpRiskWrite {
				t.Fatalf("service/risk = %q/%q, want slides/write", spec.Service, spec.Risk)
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

func TestHostedSlidesEditBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
		want []string
	}{
		{name: "skip", tool: "slides_skip_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "p:0"}, want: []string{"slides", "skip-slide", "--", "selected-deck", "p:0"}},
		{name: "unskip", tool: "slides_unskip_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "shape:one"}, want: []string{"slides", "unskip-slide", "--", "selected-deck", "shape:one"}},
		{name: "literal whitespace find", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": " ", "replacement": "", "all": true}, want: []string{"slides", "replace-text", "--all", "--", "selected-deck", " ", ""}},
		{name: "replace all", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "all": true}, want: []string{"slides", "replace-text", "--all", "--", "selected-deck", "foo", "bar"}},
		{name: "replace all match_case", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "match_case": true, "all": true}, want: []string{"slides", "replace-text", "--match-case", "--all", "--", "selected-deck", "foo", "bar"}},
		{name: "replace page_ids any", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": []any{"p:0", "p:1"}}, want: []string{"slides", "replace-text", "--page=p:0", "--page=p:1", "--", "selected-deck", "foo", "bar"}},
		{name: "replace page_ids string", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": []string{"p:0", "p:1"}}, want: []string{"slides", "replace-text", "--page=p:0", "--page=p:1", "--", "selected-deck", "foo", "bar"}},
		{name: "replace object", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "object_id": "shape:one"}, want: []string{"slides", "replace-text", "--object=shape:one", "--", "selected-deck", "foo", "bar"}},
		{name: "replace empty replacement", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "", "all": true}, want: []string{"slides", "replace-text", "--all", "--", "selected-deck", "foo", ""}},
		{
			name: "literal whitespace, Unicode, and flag-looking strings remain positional",
			tool: "slides_replace_text",
			args: map[string]any{"presentation_id": "selected-deck", "find": "\n --account=foreign@x 🚀 \n", "replacement": " 🧪 \n --all", "all": true},
			want: []string{"slides", "replace-text", "--all", "--", "selected-deck", "\n --account=foreign@x 🚀 \n", " 🧪 \n --all"},
		},
		{name: "find spaces are valid", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "   foo bar  ", "replacement": " baz ", "all": true}, want: []string{"slides", "replace-text", "--all", "--", "selected-deck", "   foo bar  ", " baz "}},
	}

	specs := hostedSlidesEditTools()
	byName := make(map[string]mcpToolSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			argv, err := byName[test.tool].BuildArgs(hostedSlidesEditRequest(test.args))
			if err != nil {
				t.Fatalf("BuildArgs() error = %v", err)
			}
			if !slices.Equal(argv, test.want) {
				t.Fatalf("argv = %#v, want %#v", argv, test.want)
			}
		})
	}
}

func TestHostedSlidesEditInvalidInput(t *testing.T) {
	specs := hostedSlidesEditTools()
	byName := make(map[string]mcpToolSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}

	cases := []struct {
		name string
		tool string
		args map[string]any
		want error
	}{
		{name: "missing pid skip", tool: "slides_skip_slide", args: map[string]any{"slide_id": "p:0"}},
		{name: "missing sid skip", tool: "slides_skip_slide", args: map[string]any{"presentation_id": "selected-deck"}},
		{name: "empty sid skip", tool: "slides_skip_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": ""}},
		{name: "bad pid skip", tool: "slides_skip_slide", args: map[string]any{"presentation_id": "../foreign", "slide_id": "p:0"}, want: errHostedSlidesReadID},
		{name: "bad sid skip", tool: "slides_skip_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "bad/id"}, want: errHostedSlidesBadObjectID},
		{name: "missing pid unskip", tool: "slides_unskip_slide", args: map[string]any{"slide_id": "p:0"}},
		{name: "missing sid unskip", tool: "slides_unskip_slide", args: map[string]any{"presentation_id": "selected-deck"}},
		{name: "bad pid unskip", tool: "slides_unskip_slide", args: map[string]any{"presentation_id": "https://evil.example/deck", "slide_id": "p:0"}, want: errHostedSlidesReadID},
		{name: "bad sid unskip", tool: "slides_unskip_slide", args: map[string]any{"presentation_id": "selected-deck", "slide_id": "  "}},
		{name: "missing pid replace", tool: "slides_replace_text", args: map[string]any{"find": "foo", "replacement": "bar", "all": true}},
		{name: "bad pid replace", tool: "slides_replace_text", args: map[string]any{"presentation_id": "https://evil.example/deck", "find": "foo", "replacement": "bar", "all": true}, want: errHostedSlidesReadID},
		{name: "missing find", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "replacement": "bar", "all": true}},
		{name: "empty find", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "", "replacement": "bar", "all": true}, want: errHostedSlidesEmptyFind},
		{name: "missing replacement", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "all": true}},
		{name: "no mode", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar"}, want: errHostedSlidesModeConflict},
		{name: "overlapping modes", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "all": true, "object_id": "shape:one"}, want: errHostedSlidesModeConflict},
		{name: "bad object id", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "object_id": "bad/id"}, want: errHostedSlidesBadObjectID},
		{name: "wrong type match_case", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "match_case": "true", "all": true}, want: errHostedSlidesArgType},
		{name: "wrong type all", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "all": "true"}, want: errHostedSlidesArgType},
		{name: "nil object_id", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "object_id": nil}, want: errHostedSlidesNilArg},
		{name: "wrong type object_id", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "object_id": 123}, want: errHostedSlidesArgType},
		{name: "nil page_ids", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": nil}, want: errHostedSlidesNilArg},
		{name: "wrong type page_ids", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": "p:0"}, want: errHostedSlidesArgType},
		{name: "bad element type page_ids", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": []any{"p:0", 123}}, want: errHostedSlidesArgType},
		{name: "nil element page_ids", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": []any{"p:0", nil}}, want: errHostedSlidesNilArg},
		{name: "bad element pid page_ids", tool: "slides_replace_text", args: map[string]any{"presentation_id": "selected-deck", "find": "foo", "replacement": "bar", "page_ids": []string{"p:0", "bad/id"}}, want: errHostedSlidesBadObjectID},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			argv, err := byName[test.tool].BuildArgs(hostedSlidesEditRequest(test.args))
			if test.want == nil && err == nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if argv != nil {
				t.Fatalf("argv = %#v, want nil", argv)
			}
		})
	}
}
