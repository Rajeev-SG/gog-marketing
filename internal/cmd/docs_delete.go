package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsedit"
	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DocsDeleteCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Start      *int64 `name:"start" help:"Start index (>= 1; required unless --at is set)"`
	End        *int64 `name:"end" help:"End index (> start; required unless --at is set)"`
	At         string `name:"at" help:"Anchor by literal text and delete that matched range"`
	Occurrence *int   `name:"occurrence" help:"Use the Nth --at match (1-based; required when --at is ambiguous)"`
	MatchCase  bool   `name:"match-case" help:"Use case-sensitive --at matching"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Segment    string `name:"segment" help:"Target an exact header, footer, or footnote segment ID"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
	Batch      string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsDeleteCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	placement, err := docsedit.PlanRangePlacement(docsedit.RangePlacementOptions{
		Start:     c.Start,
		End:       c.End,
		AllowZero: strings.TrimSpace(c.Segment) != "",
		Anchor: docsedit.AnchorOptions{
			Text:       c.At,
			Provided:   flagProvided(kctx, "at"),
			Occurrence: c.Occurrence,
			MatchCase:  c.MatchCase,
		},
	})
	if err != nil {
		return usage(err.Error())
	}
	at := placement.Anchor.Text

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab
	dryRunPayload := map[string]any{
		"document_id": docID,
		"start_index": docsDeleteDryRunStart(c.Start),
		"end_index":   docsDeleteDryRunEnd(c.End),
		"deleted":     docsDeleteDryRunDeleted(c.Start, c.End, at),
		"tab":         c.Tab,
		"segment":     c.Segment,
		"batch":       c.Batch,
	}
	addDocsAtAnchorDryRunPayload(dryRunPayload, docsAtAnchorFlags{At: at, Occurrence: c.Occurrence, MatchCase: c.MatchCase})
	if dryRunErr := dryRunExit(ctx, flags, "docs.delete", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}
	if batchErr := validateDocsBatchTarget(ctx, flags, c.Batch, docID); batchErr != nil {
		return batchErr
	}

	ctx = googleapi.WithoutRetries(ctx)

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	batchRevision, err := captureDocsBatchRevision(ctx, svc, c.Batch, docID)
	if err != nil {
		return err
	}
	resolvedPlacement, err := resolveDocsPlacementTarget(ctx, svc, docID, c.Tab, c.Segment, placement)
	if err != nil {
		return err
	}
	start := resolvedPlacement.Range.Start
	end := resolvedPlacement.Range.End
	c.Tab = resolvedPlacement.TabID
	c.Segment = resolvedPlacement.SegmentID

	batchReq := &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{docsedit.BuildDeleteRequest(docsedit.Range{Start: start, End: end}, c.Tab)},
	}
	applyDocsRequestTarget(batchReq.Requests, docsTargetFromPlacement(resolvedPlacement.ResolvedPlacement))
	batchReq.WriteControl = docsRequiredRevisionWriteControl(resolvedPlacement.RequiredRevisionID)
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.delete", batchRevision, batchReq.Requests, false); queued || queueErr != nil {
		return queueErr
	}
	result, err := svc.Documents.BatchUpdate(docID, batchReq).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("deleting content: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": result.DocumentId,
			"deleted":    end - start,
			"startIndex": start,
			"endIndex":   end,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		if c.Segment != "" {
			payload["segmentId"] = c.Segment
			payload["segmentType"] = resolvedPlacement.SegmentKind
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", result.DocumentId)
	u.Out().Linef("deleted\t%d characters", end-start)
	u.Out().Linef("range\t%d-%d", start, end)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	if c.Segment != "" {
		u.Out().Linef("segmentId\t%s", c.Segment)
		u.Out().Linef("segmentType\t%s", resolvedPlacement.SegmentKind)
	}
	return nil
}

func docsDeleteDryRunStart(start *int64) any {
	if start == nil {
		return nil
	}
	return *start
}

func docsDeleteDryRunEnd(end *int64) any {
	if end == nil {
		return nil
	}
	return *end
}

func docsDeleteDryRunDeleted(start, end *int64, at string) any {
	if at != "" {
		return "at:range"
	}
	if start == nil || end == nil {
		return nil
	}
	return *end - *start
}

type DocsClearCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
}

func (c *DocsClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if err := dryRunExit(ctx, flags, "docs.clear", map[string]any{
		"document_id": docID,
	}); err != nil {
		return err
	}
	// The sed executor has an independent retry loop; clear uses one typed write.
	ctx = googleapi.WithoutRetries(ctx)
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	doc, err := svc.Documents.Get(docID).Context(ctx).Do()
	if err != nil {
		return err
	}
	bodyEnd := int64(2)
	if doc.Body != nil && len(doc.Body.Content) > 0 {
		bodyEnd = doc.Body.Content[len(doc.Body.Content)-1].EndIndex
		if bodyEnd < 2 {
			bodyEnd = 2
		}
	}
	// Preserve native clear's no-op for whitespace-only bodies.
	isEmpty := true
	if doc.Body != nil {
		for _, element := range doc.Body.Content {
			if element.Table != nil {
				isEmpty = false
				break
			}
			if element.Paragraph != nil {
				for _, part := range element.Paragraph.Elements {
					if part.TextRun != nil && strings.TrimSpace(part.TextRun.Content) != "" {
						isEmpty = false
						break
					}
				}
			}
			if !isEmpty {
				break
			}
		}
	}
	if isEmpty {
		return sedOutputOK(ctx, ui.FromContext(ctx), docID, sedOutputKV{"cleared", 0})
	}
	deleteEnd := bodyEnd - 1
	if deleteEnd < 2 {
		return sedOutputOK(ctx, ui.FromContext(ctx), docID, sedOutputKV{"cleared", 0})
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		WriteControl: docsRequiredRevisionWriteControl(doc.RevisionId),
		Requests:     []*docs.Request{{DeleteContentRange: &docs.DeleteContentRangeRequest{Range: &docs.Range{StartIndex: 1, EndIndex: deleteEnd}}}},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("clearing document: %w", err)
	}
	return sedOutputOK(ctx, ui.FromContext(ctx), docID, sedOutputKV{"cleared", deleteEnd - 1})
}
