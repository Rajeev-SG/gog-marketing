package cmd

import (
	"errors"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/docsedit"
)

var errHostedDocsNamedEditArgument = errors.New("invalid Docs named-range/search argument")

func hostedDocsNamedEditID(req mcp.CallToolRequest) (string, error) {
	id, err := requireMCPString(req, "document_id")
	if err != nil {
		return "", err
	}
	if id != req.GetString("document_id", "") {
		return "", errHostedDocsNamedEditArgument
	}
	if validationErr := validateHostedDocsInventoryID(id); validationErr != nil {
		return "", validationErr
	}
	return id, nil
}

func hostedDocsNamedEditString(req mcp.CallToolRequest, key string, limit int) (string, error) {
	value, ok := req.GetArguments()[key].(string)
	if !ok || len(value) > limit {
		return "", errHostedDocsNamedEditArgument
	}
	return value, nil
}

func hostedDocsNamedEditBool(req mcp.CallToolRequest, key string, defaultValue bool) (bool, error) {
	raw, present := req.GetArguments()[key]
	if !present {
		return defaultValue, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, errHostedDocsNamedEditArgument
	}
	return value, nil
}

func hostedDocsNamedEditIndex(req mcp.CallToolRequest, key string) (*int64, error) {
	raw, present := req.GetArguments()[key]
	if !present {
		return nil, nil //nolint:nilnil // A missing optional index is valid; nil preserves omission rather than zero.
	}
	value, err := validateHostedTabIndexArg(raw)
	if err != nil {
		return nil, err
	}
	index := int64(value)
	return &index, nil
}

func hostedDocsNamedEditSelection(args []string, req mcp.CallToolRequest, keys ...string) ([]string, error) {
	for _, key := range keys {
		if _, present := req.GetArguments()[key]; present {
			value, err := hostedDocsNamedEditString(req, key, 4096)
			if err != nil {
				return nil, err
			}
			args = append(args, "--"+key+"="+value)
		}
	}
	return args, nil
}

func hostedDocsNamedEditTools() []mcpToolSpec {
	id := func() mcp.ToolOption { return mcp.WithString("document_id", mcp.Required()) }
	tab := func() mcp.ToolOption { return mcp.WithString("tab") }
	specs := []mcpToolSpec{
		{Name: "docs_find_range", Service: "docs", Risk: mcpRiskRead, Description: "Find native UTF-16 text ranges in one explicitly granted document, optionally targeting a tab or exact header/footer/footnote segment. Defaults to first match with whitespace normalization; supports all or an occurrence, case matching and fail-empty.", Options: []mcp.ToolOption{id(), mcp.WithString("text", mcp.Required()), mcp.WithBoolean("match_case"), mcp.WithBoolean("all"), mcp.WithInteger("occurrence", mcp.Min(1), mcp.Max(2147483647)), mcp.WithBoolean("normalize_whitespace"), mcp.WithBoolean("fail_empty"), tab(), mcp.WithString("segment")}, BuildArgs: hostedDocsFindRangeArgs},
		{Name: "docs_named_range_create", Service: "docs", Risk: mcpRiskWrite, Description: "Create a named range in one explicitly granted document by literal anchor or explicit UTF-16 start/end indexes, optionally on a selected tab. Native ambiguity checks and revision guards remain enforced. Requires exact write-tool/file grants; chat additionally requires one-use exact-argument approval.", Options: []mcp.ToolOption{id(), mcp.WithString("name", mcp.Required()), mcp.WithString("at"), mcp.WithInteger("start", mcp.Min(1), mcp.Max(2147483647)), mcp.WithInteger("end", mcp.Min(2), mcp.Max(2147483647)), mcp.WithInteger("occurrence", mcp.Min(1), mcp.Max(2147483647)), mcp.WithBoolean("match_case"), tab()}, BuildArgs: hostedDocsNamedRangeCreateArgs},
		{Name: "docs_named_range_delete", Service: "docs", Risk: mcpRiskWrite, Description: "Delete a named-range definition by exact ID or unique exact name from one explicitly granted document, optionally scoped to a tab. Requires destructive permission and exact write-tool/file grants; chat additionally requires one-use exact-argument approval; native revision guard remains enforced.", Options: []mcp.ToolOption{id(), mcp.WithString("name_or_id", mcp.Required()), tab()}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			return hostedDocsNamedRangeMutationArgs("docs_named_range_delete", req)
		}},
		{Name: "docs_named_range_replace", Service: "docs", Risk: mcpRiskWrite, Description: "Replace named-range content with literal inline plain text in one explicitly granted document, optionally scoped to a tab. Empty text clears and additionally requires destructive permission. Exact write-tool/file grants, native revision guards and readback remain enforced; chat additionally requires one-use exact-argument approval; no file/stdin/markdown input.", Options: []mcp.ToolOption{id(), mcp.WithString("name_or_id", mcp.Required()), mcp.WithString("text", mcp.Required()), tab()}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
			return hostedDocsNamedRangeMutationArgs("docs_named_range_replace", req)
		}},
	}
	return specs
}

func hostedDocsFindRangeArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	text, err := hostedDocsNamedEditString(req, "text", 262144)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return nil, errHostedDocsNamedEditArgument
	}
	matchCase, err := hostedDocsNamedEditBool(req, "match_case", false)
	if err != nil {
		return nil, err
	}
	all, err := hostedDocsNamedEditBool(req, "all", false)
	if err != nil {
		return nil, err
	}
	failEmpty, err := hostedDocsNamedEditBool(req, "fail_empty", false)
	if err != nil {
		return nil, err
	}
	normalizeWhitespace, err := hostedDocsNamedEditBool(req, "normalize_whitespace", true)
	if err != nil {
		return nil, err
	}
	occurrence, err := hostedDocsNamedEditIndex(req, "occurrence")
	if err != nil {
		return nil, err
	}
	if occurrence != nil && *occurrence <= 0 {
		return nil, errHostedDocsNamedEditArgument
	}
	if occurrence != nil && all {
		return nil, errHostedDocsNamedEditArgument
	}
	args := []string{"docs", "find-range"}
	if matchCase {
		args = append(args, "--match-case")
	}
	if all {
		args = append(args, "--all")
	}
	if failEmpty {
		args = append(args, "--fail-empty")
	}
	args = append(args, "--normalize-whitespace="+strconv.FormatBool(normalizeWhitespace))
	if occurrence != nil {
		args = append(args, "--occurrence="+strconv.FormatInt(*occurrence, 10))
	}
	args, err = hostedDocsNamedEditSelection(args, req, "tab", "segment")
	if err != nil {
		return nil, err
	}
	args = append(args, "--", id, text)
	return args, nil
}

func hostedDocsNamedRangeCreateArgs(req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	name, err := hostedDocsNamedEditString(req, "name", 4096)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf16Len(trimmed) > 256 {
		return nil, errHostedDocsNamedEditArgument
	}
	var at string
	atPresent := false
	if _, ok := req.GetArguments()["at"]; ok {
		atPresent = true
		at, err = hostedDocsNamedEditString(req, "at", 262144)
		if err != nil {
			return nil, err
		}
	}
	start, err := hostedDocsNamedEditIndex(req, "start")
	if err != nil {
		return nil, err
	}
	end, err := hostedDocsNamedEditIndex(req, "end")
	if err != nil {
		return nil, err
	}
	occurrence, err := hostedDocsNamedEditIndex(req, "occurrence")
	if err != nil {
		return nil, err
	}
	var occurrencePtr *int
	if occurrence != nil {
		if *occurrence <= 0 {
			return nil, errHostedDocsNamedEditArgument
		}
		value := int(*occurrence)
		occurrencePtr = &value
	}
	matchCase, err := hostedDocsNamedEditBool(req, "match_case", false)
	if err != nil {
		return nil, err
	}
	if _, planErr := docsedit.PlanRangePlacement(docsedit.RangePlacementOptions{
		Start: start,
		End:   end,
		Anchor: docsedit.AnchorOptions{
			Text:       at,
			Provided:   atPresent,
			Occurrence: occurrencePtr,
			MatchCase:  matchCase,
		},
	}); planErr != nil {
		return nil, planErr
	}
	argv := []string{"docs", "named-range", "create", "--name=" + name}
	if start != nil {
		argv = append(argv, "--start="+strconv.FormatInt(*start, 10))
	}
	if end != nil {
		argv = append(argv, "--end="+strconv.FormatInt(*end, 10))
	}
	if atPresent {
		argv = append(argv, "--at="+at)
	}
	if occurrence != nil {
		argv = append(argv, "--occurrence="+strconv.FormatInt(*occurrence, 10))
	}

	if matchCase {
		argv = append(argv, "--match-case")
	}
	argv, err = hostedDocsNamedEditSelection(argv, req, "tab")
	if err != nil {
		return nil, err
	}
	argv = append(argv, "--", id)
	return argv, nil
}

func hostedDocsNamedRangeMutationArgs(tool string, req mcp.CallToolRequest) ([]string, error) {
	id, idErr := hostedDocsNamedEditID(req)
	if idErr != nil {
		return nil, idErr
	}
	nameOrID, err := hostedDocsNamedEditString(req, "name_or_id", 4096)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(nameOrID) == "" {
		return nil, errHostedDocsNamedEditArgument
	}

	var command string
	switch tool {
	case "docs_named_range_delete":
		command = "delete"
	case "docs_named_range_replace":
		command = "replace"
	default:
		return nil, errHostedDocsNamedEditArgument
	}

	args := []string{"docs", "named-range", command}

	if command == "replace" {
		text, textErr := hostedDocsNamedEditString(req, "text", 262144)
		if textErr != nil {
			return nil, textErr
		}
		args = append(args, "--text="+text)
	}

	args, err = hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}

	args = append(args, "--", id, nameOrID)
	return args, nil
}
