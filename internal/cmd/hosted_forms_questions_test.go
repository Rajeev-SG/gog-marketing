package cmd

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedFormsQuestionFixedSchemasAndArguments(t *testing.T) {
	specs := hostedFormsQuestionTools()
	if len(specs) != 3 {
		t.Fatalf("tools=%d", len(specs))
	}
	byName := map[string]mcpToolSpec{}
	for _, s := range specs {
		if s.Service != "forms" || s.Risk != mcpRiskWrite {
			t.Fatalf("unexpected service/risk: %+v", s)
		}
		byName[s.Name] = s
	}
	for _, test := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"forms_delete_question", map[string]any{"form_id": "selected-form", "index": 0}, []string{"forms", "questions", "delete", "--", "selected-form", "0"}},
		{"forms_move_question", map[string]any{"form_id": "selected-form", "old_index": 0, "new_index": 2}, []string{"forms", "questions", "move", "--", "selected-form", "0", "2"}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": " --account=foreign ", "type": "radio", "options": []any{"A", " B "}, "index": 0}, []string{"forms", "questions", "add", "--title= --account=foreign ", "--type=radio", "--option=A", "--option= B ", "--index=0", "--", "selected-form"}},
	} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = test.args
		args, err := byName[test.name].BuildArgs(req)
		if err != nil || !slices.Equal(args, test.want) {
			t.Fatalf("%s argv=%q error=%v", test.name, args, err)
		}
	}
}

func TestHostedFormsQuestionRejectsInvalidInput(t *testing.T) {
	byName := map[string]mcpToolSpec{}
	for _, s := range hostedFormsQuestionTools() {
		byName[s.Name] = s
	}
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"forms_delete_question", map[string]any{"form_id": "selected-form", "index": -1}},
		{"forms_delete_question", map[string]any{"form_id": "selected-form", "index": 0.5}},
		{"forms_move_question", map[string]any{"form_id": "selected-form", "old_index": "0", "new_index": 1}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "required": "true"}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "options": []any{nil}, "type": "radio"}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "type": "scale", "scale_low": 2}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "type": "file_upload"}},
		{"forms_add_question", map[string]any{"form_id": "../foreign", "title": "Question"}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "type": nil}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "type": "radio", "options": []string{"A", "B"}, "correct": []string{"B"}, "points": 0}},
		{"forms_add_question", map[string]any{"form_id": "selected-form", "title": "Question", "requests": []any{}}},
	} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = test.args
		if _, err := byName[test.name].BuildArgs(req); err == nil {
			t.Fatalf("%s accepted %v", test.name, test.args)
		}
	}
}
