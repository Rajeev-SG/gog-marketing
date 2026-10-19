package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/docsedit"
)

func hostedDocsTextEditTools() []mcpToolSpec {
	id := func() mcp.ToolOption { return mcp.WithString("document_id", mcp.Required()) }
	selectors := func() []mcp.ToolOption { return []mcp.ToolOption{mcp.WithString("tab"), mcp.WithString("segment")} }
	anchor := func() []mcp.ToolOption {
		return []mcp.ToolOption{mcp.WithString("at"), mcp.WithInteger("occurrence", mcp.Min(1), mcp.Max(2147483647)), mcp.WithBoolean("match_case")}
	}
	insertOptions := append([]mcp.ToolOption{id(), mcp.WithString("text", mcp.Required()), mcp.WithInteger("index", mcp.Min(0), mcp.Max(2147483647))}, anchor()...)
	insertOptions = append(insertOptions, selectors()...)
	deleteOptions := append([]mcp.ToolOption{id(), mcp.WithInteger("start", mcp.Min(0), mcp.Max(2147483647)), mcp.WithInteger("end", mcp.Min(1), mcp.Max(2147483647))}, anchor()...)
	deleteOptions = append(deleteOptions, selectors()...)
	return []mcpToolSpec{
		{Name: "docs_insert", Service: "docs", Risk: mcpRiskWrite, Description: "Insert nonempty literal plain text at a UTF-16 index, a literal anchor or native append position in one selected document/tab/segment. Header/footer/footnote indexes may start at zero. Preserves native ambiguity and revision checks; exact write-tool/file authority and one-use chat approval. No files/stdin/markdown/persisted batch or automatic retries.", Options: insertOptions, BuildArgs: hostedDocsInsertArgs},
		{Name: "docs_delete", Service: "docs", Risk: mcpRiskWrite, Description: "Delete a UTF-16 start/end range or a literal anchored range in one selected document/tab/segment. Exact write-tool/file and destructive authority required, plus one-use chat approval. Preserves ambiguity/revision checks and native final-newline handling; no file/batch inputs or automatic retries.", Options: deleteOptions, BuildArgs: hostedDocsDeleteTextArgs},
		{Name: "docs_clear", Service: "docs", Risk: mcpRiskWrite, Description: "Clear the native default document body while preserving its final newline and other tabs/headers/footers. No tab selector is exposed because native clear targets the legacy/default body. Exact write-tool/file and destructive authority required; chat uses one-use approval. Single-attempt typed request; no arbitrary sed expressions.", Options: []mcp.ToolOption{id()}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			docID, err := hostedDocsNamedEditID(req)
			if err != nil {
				return nil, err
			}
			return []string{"docs", "clear", "--", docID}, nil
		}},
		{Name: "docs_find_replace", Service: "docs", Risk: mcpRiskWrite, Description: "Replace literal text with literal inline plain text, all matches by default or first only, optionally on a selected tab. Preserves native case/tab behavior; exact write-tool/file grants and chat one-use approval. Empty replacement additionally requires destructive authority. No markdown, files, stdin or automatic retries; native replace-all does not provide a revision precondition, first-only does.", Options: []mcp.ToolOption{id(), mcp.WithString("find", mcp.Required()), mcp.WithString("replacement", mcp.Required()), mcp.WithBoolean("first"), mcp.WithBoolean("match_case"), mcp.WithString("tab")}, BuildArgs: hostedDocsFindReplaceArgs},
	}
}

func hostedDocsTextAnchor(req mcp.CallToolRequest) (docsedit.AnchorOptions, error) {
	var anchor docsedit.AnchorOptions
	if _, present := req.GetArguments()["at"]; present {
		text, err := hostedDocsNamedEditString(req, "at", 262144)
		if err != nil {
			return anchor, err
		}
		anchor.Text = text
		anchor.Provided = true
	}
	occurrence, err := hostedDocsNamedEditIndex(req, "occurrence")
	if err != nil {
		return anchor, err
	}
	if occurrence != nil {
		value := int(*occurrence)
		anchor.Occurrence = &value
	}
	matchCase, err := hostedDocsNamedEditBool(req, "match_case", false)
	if err != nil {
		return anchor, err
	}
	anchor.MatchCase = matchCase
	return anchor, nil
}

func appendDocsTextAnchorFlags(args []string, anchor docsedit.AnchorOptions) []string {
	if anchor.Provided {
		args = append(args, "--at="+anchor.Text)
	}
	if anchor.Occurrence != nil {
		args = append(args, "--occurrence="+strconv.Itoa(*anchor.Occurrence))
	}
	if anchor.MatchCase {
		args = append(args, "--match-case")
	}
	return args
}

func hostedDocsInsertArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	text, err := hostedDocsNamedEditString(req, "text", 262144)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, usage("insert text cannot be empty")
	}
	index, err := hostedDocsNamedEditIndex(req, "index")
	if err != nil {
		return nil, err
	}
	anchor, err := hostedDocsTextAnchor(req)
	if err != nil {
		return nil, err
	}
	if _, planErr := docsedit.PlanInsertPlacement(docsedit.InsertPlacementOptions{Index: index, AllowZero: strings.TrimSpace(req.GetString("segment", "")) != "", Anchor: anchor}); planErr != nil {
		return nil, planErr
	}
	args := []string{"docs", "insert"}
	if index != nil {
		args = append(args, "--index="+strconv.FormatInt(*index, 10))
	}
	args = appendDocsTextAnchorFlags(args, anchor)
	args, err = hostedDocsNamedEditSelection(args, req, "tab", "segment")
	if err != nil {
		return nil, err
	}
	return append(args, "--", id, text), nil
}

func hostedDocsDeleteTextArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	start, err := hostedDocsNamedEditIndex(req, "start")
	if err != nil {
		return nil, err
	}
	end, err := hostedDocsNamedEditIndex(req, "end")
	if err != nil {
		return nil, err
	}
	anchor, err := hostedDocsTextAnchor(req)
	if err != nil {
		return nil, err
	}
	if _, planErr := docsedit.PlanRangePlacement(docsedit.RangePlacementOptions{Start: start, End: end, AllowZero: strings.TrimSpace(req.GetString("segment", "")) != "", Anchor: anchor}); planErr != nil {
		return nil, planErr
	}
	args := []string{"docs", "delete"}
	if start != nil {
		args = append(args, "--start="+strconv.FormatInt(*start, 10))
	}
	if end != nil {
		args = append(args, "--end="+strconv.FormatInt(*end, 10))
	}
	args = appendDocsTextAnchorFlags(args, anchor)
	args, err = hostedDocsNamedEditSelection(args, req, "tab", "segment")
	if err != nil {
		return nil, err
	}
	return append(args, "--", id), nil
}

// hostedDocsFindReplaceArgs builds argv for `gog docs find-replace` from the
// MCP tool arguments, validating the doc ID, the literal find/replacement
// strings, and the optional boolean flags.
func hostedDocsFindReplaceArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	find, err := hostedDocsNamedEditString(req, "find", 262144)
	if err != nil {
		return nil, err
	}
	if find == "" {
		return nil, usage("docs find-replace: \"find\" must be a non-empty string")
	}
	replacement, err := hostedDocsNamedEditString(req, "replacement", 262144)
	if err != nil {
		return nil, err
	}
	args := []string{"docs", "find-replace", "--format=plain"}
	if b, flagErr := hostedDocsNamedEditBool(req, "first", false); flagErr != nil {
		return nil, flagErr
	} else if b {
		args = append(args, "--first")
	}
	if b, flagErr := hostedDocsNamedEditBool(req, "match_case", false); flagErr != nil {
		return nil, flagErr
	} else if b {
		args = append(args, "--match-case")
	}
	args, err = hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}
	return append(args, "--", id, find, replacement), nil
}
