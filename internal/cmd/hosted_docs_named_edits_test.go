package cmd

import (
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHostedDocsNamedEditArguments(t *testing.T) {
	tools := hostedDocsNamedEditTools()
	if len(tools) != 4 {
		t.Fatalf("Docs edit tools=%d", len(tools))
	}
	specs := map[string]mcpToolSpec{}
	for _, spec := range tools {
		want := mcpRiskWrite
		if spec.Name == "docs_find_range" {
			want = mcpRiskRead
		}
		if spec.Service != "docs" || spec.Risk != want {
			t.Fatalf("unexpected contract%s", spec.Name)
		}
		if _, exists := specs[spec.Name]; exists {
			t.Fatalf("duplicate%s", spec.Name)
		}
		specs[spec.Name] = spec
	}
	build := func(name string, args map[string]any) ([]string, error) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = args
		return specs[name].BuildArgs(req)
	}
	var cases []struct {
		Name string
		Args map[string]any
		Want []string
	}
	if err := json.Unmarshal([]byte(`[{"name":"docs_find_range","args":{"document_id":"selected-doc","text":"anchor"},"want":["docs","find-range","--normalize-whitespace=true","--","selected-doc","anchor"]},{"name":"docs_find_range","args":{"document_id":"selected-doc","text":"anchor","all":true,"match_case":true,"normalize_whitespace":false,"fail_empty":true,"tab":"Work","segment":"header-1"},"want":["docs","find-range","--match-case","--all","--fail-empty","--normalize-whitespace=false","--tab=Work","--segment=header-1","--","selected-doc","anchor"]},{"name":"docs_find_range","args":{"document_id":"selected-doc","text":"anchor","occurrence":2},"want":["docs","find-range","--normalize-whitespace=true","--occurrence=2","--","selected-doc","anchor"]},{"name":"docs_named_range_create","args":{"document_id":"selected-doc","name":"Indexed","start":1,"end":6,"tab":"Work"},"want":["docs","named-range","create","--name=Indexed","--start=1","--end=6","--tab=Work","--","selected-doc"]},{"name":"docs_named_range_create","args":{"document_id":"selected-doc","name":"Anchored","at":"\ud83d\ude00 anchor","occurrence":2,"match_case":true,"tab":"Work"},"want":["docs","named-range","create","--name=Anchored","--at=\ud83d\ude00 anchor","--occurrence=2","--match-case","--tab=Work","--","selected-doc"]},{"name":"docs_named_range_delete","args":{"document_id":"selected-doc","name_or_id":"nr-1","tab":"Work"},"want":["docs","named-range","delete","--tab=Work","--","selected-doc","nr-1"]},{"name":"docs_named_range_replace","args":{"document_id":"selected-doc","name_or_id":"nr-1","text":"X\ud83d\ude00","tab":"Work"},"want":["docs","named-range","replace","--text=X\ud83d\ude00","--tab=Work","--","selected-doc","nr-1"]},{"name":"docs_named_range_replace","args":{"document_id":"selected-doc","name_or_id":"nr-1","text":"","tab":"Work"},"want":["docs","named-range","replace","--text=","--tab=Work","--","selected-doc","nr-1"]},{"name":"docs_named_range_replace","args":{"document_id":"selected-doc","name_or_id":"nr-1","text":"@/tmp/not-a-file"},"want":["docs","named-range","replace","--text=@/tmp/not-a-file","--","selected-doc","nr-1"]}]`), &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		got, err := build(test.Name, test.Args)
		if err != nil || !slices.Equal(got, test.Want) {
			t.Fatalf("%s argv=%q want%q err=%v", test.Name, got, test.Want, err)
		}
	}
	valid := map[string]map[string]any{"docs_find_range": {"document_id": "selected-doc", "text": "anchor"}, "docs_named_range_create": {"document_id": "selected-doc", "name": "Indexed", "start": 1, "end": 6}, "docs_named_range_delete": {"document_id": "selected-doc", "name_or_id": "nr-1"}, "docs_named_range_replace": {"document_id": "selected-doc", "name_or_id": "nr-1", "text": "X😀"}}
	for name, args := range valid {
		for _, bad := range []any{nil, 1, "", " spaced ", "a/b", "https://docs.google.com/doc"} {
			candidate := maps.Clone(args)
			candidate["document_id"] = bad
			if _, err := build(name, candidate); err == nil {
				t.Fatalf("%s accepted invalid document id%v", name, bad)
			}
		}
		for _, key := range []string{"tab"} {
			for _, bad := range []any{nil, 1, strings.Repeat("x", 4097)} {
				candidate := maps.Clone(args)
				candidate[key] = bad
				if _, err := build(name, candidate); err == nil {
					t.Fatalf("%s accepted invalid%s", name, key)
				}
			}
		}
	}
	for _, key := range []string{"match_case", "all", "fail_empty", "normalize_whitespace"} {
		for _, bad := range []any{nil, "true", 1} {
			args := maps.Clone(valid["docs_find_range"])
			args[key] = bad
			if _, err := build("docs_find_range", args); err == nil {
				t.Fatalf("accepted invalid boolean%s", key)
			}
		}
	}
	for _, args := range []map[string]any{{"document_id": "selected-doc", "text": ""}, {"document_id": "selected-doc", "text": "x", "all": true, "occurrence": 1}, {"document_id": "selected-doc", "text": "x", "occurrence": 0}, {"document_id": "selected-doc", "text": "x", "occurrence": nil}, {"document_id": "selected-doc", "text": nil}} {
		if _, err := build("docs_find_range", args); err == nil {
			t.Fatalf("accepted invalid search%v", args)
		}
	}
	for _, args := range []map[string]any{{"document_id": "selected-doc", "name": "x"}, {"document_id": "selected-doc", "name": "x", "at": ""}, {"document_id": "selected-doc", "name": "x", "at": "anchor", "start": 1, "end": 3}, {"document_id": "selected-doc", "name": "x", "start": 0, "end": 3}, {"document_id": "selected-doc", "name": "x", "start": 1}, {"document_id": "selected-doc", "name": "x", "start": 3, "end": 3}, {"document_id": "selected-doc", "name": "x", "start": 1, "end": 3, "occurrence": 1}, {"document_id": "selected-doc", "name": "x", "start": 1, "end": 3, "match_case": true}} {
		if _, err := build("docs_named_range_create", args); err == nil {
			t.Fatalf("accepted invalid placement%v", args)
		}
	}
	for _, bad := range []any{nil, "1", 1.5, -1, math.NaN(), math.Inf(1), int64(math.MaxInt32) + 1} {
		args := maps.Clone(valid["docs_named_range_create"])
		args["start"] = bad
		if _, err := build("docs_named_range_create", args); err == nil {
			t.Fatalf("accepted invalid index%v", bad)
		}
	}
	for _, name := range []string{strings.Repeat("界", 256), strings.Repeat("😀", 128)} {
		args := maps.Clone(valid["docs_named_range_create"])
		args["name"] = name
		if _, err := build("docs_named_range_create", args); err != nil {
			t.Fatalf("rejected valid UTF16 name boundary: %v", err)
		}
	}
	for _, name := range []any{nil, 1, " ", strings.Repeat("界", 257), strings.Repeat("😀", 129)} {
		args := maps.Clone(valid["docs_named_range_create"])
		args["name"] = name
		if _, err := build("docs_named_range_create", args); err == nil {
			t.Fatal("accepted invalid/overlong UTF16 name")
		}
	}
	for _, name := range []string{"docs_named_range_delete", "docs_named_range_replace"} {
		for _, bad := range []any{nil, 1, " ", strings.Repeat("x", 4097)} {
			args := maps.Clone(valid[name])
			args["name_or_id"] = bad
			if _, err := build(name, args); err == nil {
				t.Fatalf("accepted invalid selector%s", name)
			}
		}
	}
	for _, bad := range []any{nil, 1, strings.Repeat("x", 262145)} {
		args := maps.Clone(valid["docs_named_range_replace"])
		args["text"] = bad
		if _, err := build("docs_named_range_replace", args); err == nil {
			t.Fatal("accepted invalid replacement text")
		}
	}
	for _, text := range []string{"", " ", "Line 1\n你好😀", "@/tmp/not-a-file", "--file=/tmp/not-a-file"} {
		args := maps.Clone(valid["docs_named_range_replace"])
		args["text"] = text
		got, err := build("docs_named_range_replace", args)
		if err != nil || !slices.Contains(got, "--text="+text) {
			t.Fatalf("replacement literal not preserved%q err%v", text, err)
		}
	}
}
