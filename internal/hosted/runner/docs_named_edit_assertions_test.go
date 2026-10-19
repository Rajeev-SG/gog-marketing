package runner

import (
	"slices"
	"testing"

	docs "google.golang.org/api/docs/v1"
)

type docsNamedEditExpectation struct {
	tool     string
	name     string
	id       string
	tab      string
	text     string
	revision string
	start    int64
	end      int64
}

func checkDocsNamedEditSDKRequest(t *testing.T, batch *docs.BatchUpdateDocumentRequest, want docsNamedEditExpectation) {
	t.Helper()

	checkTabs := func(got *docs.TabsCriteria) {
		if want.tab == "" {
			if got != nil {
				t.Errorf("TabsCriteria = %+v, want nil (no tab expected)", got)
			}

			return
		}

		if got == nil {
			t.Fatalf("TabsCriteria = nil, want TabIds [%s]", want.tab)
		}

		if !slices.Equal(got.TabIds, []string{want.tab}) {
			t.Errorf("TabIds = %v, want exactly [%s]", got.TabIds, want.tab)
		}
	}

	if batch == nil {
		t.Fatal("BatchUpdateDocumentRequest = nil, want non-nil")
	}

	if len(batch.Requests) != 1 {
		t.Fatalf("len(Requests) = %d, want 1", len(batch.Requests))
	}

	req := batch.Requests[0]
	if req == nil {
		t.Fatal("Requests[0] = nil, want non-nil")
	}

	if batch.WriteControl == nil {
		t.Fatal("WriteControl = nil, want non-nil")
	}

	if batch.WriteControl.RequiredRevisionId != want.revision {
		t.Errorf("RequiredRevisionId = %q, want %q", batch.WriteControl.RequiredRevisionId, want.revision)
	}

	switch want.tool {
	case "docs_named_range_create":
		c := req.CreateNamedRange
		if c == nil {
			t.Fatal("CreateNamedRange = nil, want non-nil")
		}

		if c.Name != want.name {
			t.Errorf("Name = %q, want %q", c.Name, want.name)
		}

		if c.Range == nil {
			t.Fatal("Range = nil, want non-nil")
		}

		if c.Range.StartIndex != want.start || c.Range.EndIndex != want.end || c.Range.TabId != want.tab || c.Range.SegmentId != "" {
			t.Errorf("Range = %+v, want start=%d end=%d tab=%q segmentId=\"\"", c.Range, want.start, want.end, want.tab)
		}
	case "docs_named_range_delete":
		d := req.DeleteNamedRange
		if d == nil {
			t.Fatal("DeleteNamedRange = nil, want non-nil")
		}

		if d.NamedRangeId != want.id {
			t.Errorf("NamedRangeId = %q, want %q", d.NamedRangeId, want.id)
		}

		checkTabs(d.TabsCriteria)
	case "docs_named_range_replace":
		r := req.ReplaceNamedRangeContent
		if r == nil {
			t.Fatal("ReplaceNamedRangeContent = nil, want non-nil")
		}

		if r.NamedRangeId != want.id {
			t.Errorf("NamedRangeId = %q, want %q", r.NamedRangeId, want.id)
		}

		if r.Text != want.text {
			t.Errorf("Text = %q, want %q (exact match incl. blank/multiline)", r.Text, want.text)
		}

		checkTabs(r.TabsCriteria)
	default:
		t.Fatalf("unknown tool %q", want.tool)
	}
}
