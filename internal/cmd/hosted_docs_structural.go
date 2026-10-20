package cmd

import (
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/openclaw/gogcli/internal/docsedit"
)

// Structural adapters accept inline content only. Persisted batches, local files,
// stdin and cross-file lookups are deliberately not exposed.
func hostedDocsStructuralTools() []mcpToolSpec {
	specs := make([]mcpToolSpec, 0, 11)
	for _, command := range []string{"insert-footnote", "insert-horizontal-rule", "insert-page-break", "insert-section-break", "insert-table", "insert-person", "insert-date-chip", "header create", "footer create", "header delete", "footer delete"} {
		options := []mcp.ToolOption{mcp.WithString("document_id", mcp.Required()), mcp.WithString("tab")}
		deleting := strings.HasSuffix(command, " delete")
		if deleting {
			options = append(options, mcp.WithString("segment_id", mcp.Required()))
		} else {
			options = append(options, mcp.WithInteger("index", mcp.Min(1), mcp.Max(2147483647)), mcp.WithBoolean("at_end"))
			switch command {
			case "insert-footnote", "header create", "footer create":
				options = append(options, mcp.WithString("text", mcp.Required()))
			case "insert-section-break":
				options = append(options, mcp.WithString("type", mcp.Enum("next-page", "continuous")))
			case "insert-table":
				options = append(options, mcp.WithInteger("rows", mcp.Required(), mcp.Min(1), mcp.Max(100)), mcp.WithInteger("cols", mcp.Required(), mcp.Min(1), mcp.Max(100)))
			case "insert-person":
				options = append(options, mcp.WithString("email", mcp.Required()))
			case "insert-date-chip":
				options = append(options, mcp.WithString("date", mcp.Required()), mcp.WithString("format", mcp.Enum("abbreviated", "full", "iso")))
			}
		}
		specs = append(specs, mcpToolSpec{Name: "docs_" + strings.NewReplacer("-", "_", " ", "_").Replace(command), Service: "docs", Risk: mcpRiskWrite, Description: "Edit one explicitly granted document via native " + command + ". Inline values only; exact write-tool/file authority and one-use chat approval. Segment deletion also requires destructive authority. Native revision and placement checks remain enforced; no local files, persisted batches or automatic retries.", Options: options, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) { return hostedDocsStructuralArgs(command, req) }})
	}
	return specs
}

func hostedDocsStructuralArgs(command string, req mcp.CallToolRequest) ([]string, error) {
	id, err := hostedDocsNamedEditID(req)
	if err != nil {
		return nil, err
	}
	args := append([]string{"docs"}, strings.Fields(command)...)
	args, err = hostedDocsNamedEditSelection(args, req, "tab")
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(command, " delete") {
		segment, segmentErr := hostedDocsNamedEditString(req, "segment_id", 4096)
		if segmentErr != nil {
			return nil, segmentErr
		}
		if !hostedSlidesObjectIDPattern.MatchString(segment) {
			return nil, errHostedDocsNamedEditArgument
		}
		return append(args, "--", id, segment), nil
	}
	index, err := hostedDocsNamedEditIndex(req, "index")
	if err != nil {
		return nil, err
	}
	atEnd, err := hostedDocsNamedEditBool(req, "at_end", false)
	if err != nil {
		return nil, err
	}
	if _, err = docsedit.PlanEndInsertPlacement(docsedit.EndInsertPlacementOptions{Index: index, AtEnd: atEnd}); err != nil {
		return nil, err
	}
	if index != nil {
		args = append(args, "--index="+strconv.FormatInt(*index, 10))
	}
	if atEnd {
		args = append(args, "--at-end")
	}
	switch command {
	case "insert-footnote", "header create", "footer create":
		value, e := hostedDocsNamedEditString(req, "text", 262144)
		if e != nil {
			return nil, e
		}
		if value == "" {
			return nil, errHostedDocsNamedEditArgument
		}
		args = append(args, "--text="+value)
	case "insert-section-break":
		if _, ok := req.GetArguments()["type"]; ok {
			value, e := hostedDocsNamedEditString(req, "type", 32)
			if e != nil {
				return nil, e
			}
			if value != "next-page" && value != "continuous" {
				return nil, errHostedDocsNamedEditArgument
			}
			args = append(args, "--type="+value)
		}
	case "insert-table":
		for _, key := range []string{"rows", "cols"} {
			value, e := hostedDocsNamedEditIndex(req, key)
			if e != nil {
				return nil, e
			}
			if value == nil || *value < 1 || *value > 100 {
				return nil, errHostedDocsNamedEditArgument
			}
			args = append(args, "--"+key+"="+strconv.FormatInt(*value, 10))
		}
	case "insert-person":
		value, e := hostedDocsNamedEditString(req, "email", 320)
		if e != nil {
			return nil, e
		}
		if strings.TrimSpace(value) != value || !strings.Contains(value, "@") || strings.ContainsAny(value, "\r\n\x00") {
			return nil, errHostedDocsNamedEditArgument
		}
		args = append(args, "--email="+value)
	case "insert-date-chip":
		value, e := hostedDocsNamedEditString(req, "date", 10)
		if e != nil {
			return nil, e
		}
		if _, e = time.Parse("2006-01-02", value); e != nil {
			return nil, e
		}
		args = append(args, "--date="+value)
		if _, ok := req.GetArguments()["format"]; ok {
			value, e := hostedDocsNamedEditString(req, "format", 32)
			if e != nil {
				return nil, e
			}
			if _, e = normalizeDocsDateChipFormat(value); e != nil {
				return nil, e
			}
			args = append(args, "--format="+value)
		}
	}
	return append(args, "--", id), nil
}
