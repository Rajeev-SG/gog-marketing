package runner

import (
	"testing"

	sheets "google.golang.org/api/sheets/v4"
)

type sheetsRangeNoteExpectation struct {
	tool   string
	id     string
	name   string
	fields string
	note   string
	grid   *sheets.GridRange
}

func checkSheetsRangeNoteSDKRequest(t *testing.T, request *sheets.Request, want sheetsRangeNoteExpectation) {
	t.Helper()

	if request == nil {
		t.Fatal("expected request, got nil")
	}

	switch want.tool {
	case "sheets_named_ranges_add":
		if request.AddNamedRange == nil || request.AddNamedRange.NamedRange == nil {
			t.Fatalf("expected AddNamedRange with NamedRange, got %+v", request)
		}

		added := request.AddNamedRange.NamedRange
		if added.NamedRangeId != "" {
			t.Fatalf("NamedRangeId must be empty for new provider ID, got %q", added.NamedRangeId)
		}

		if added.Name != want.name {
			t.Fatalf("NamedRange name = %q, want %q", added.Name, want.name)
		}

		checkSheetsGridRange(t, added.Range, want.grid)
	case "sheets_named_ranges_update":
		if request.UpdateNamedRange == nil || request.UpdateNamedRange.NamedRange == nil {
			t.Fatalf("expected UpdateNamedRange with NamedRange, got %+v", request)
		}

		updated := request.UpdateNamedRange.NamedRange
		if updated.NamedRangeId != want.id {
			t.Fatalf("NamedRangeId = %q, want %q", updated.NamedRangeId, want.id)
		}

		if updated.Name != want.name {
			t.Fatalf("NamedRange name = %q, want %q", updated.Name, want.name)
		}

		if request.UpdateNamedRange.Fields != want.fields {
			t.Fatalf("update fields = %q, want %q", request.UpdateNamedRange.Fields, want.fields)
		}

		checkSheetsGridRange(t, updated.Range, want.grid)
	case "sheets_named_ranges_delete":
		if request.DeleteNamedRange == nil {
			t.Fatal("expected DeleteNamedRange, got nil")
		}

		if request.DeleteNamedRange.NamedRangeId != want.id {
			t.Fatalf("NamedRangeId = %q, want %q", request.DeleteNamedRange.NamedRangeId, want.id)
		}
	case "sheets_update_note":
		if request.RepeatCell == nil || request.RepeatCell.Cell == nil {
			t.Fatalf("expected RepeatCell with Cell, got %+v", request)
		}

		if request.RepeatCell.Fields != "note" {
			t.Fatalf("RepeatCell fields = %q, want %q", request.RepeatCell.Fields, "note")
		}

		if request.RepeatCell.Cell.Note != want.note {
			t.Fatalf("Cell.Note = %q, want %q", request.RepeatCell.Cell.Note, want.note)
		}

		checkSheetsGridRange(t, request.RepeatCell.Range, want.grid)
	default:
		t.Fatalf("unexpected tool %q", want.tool)
	}
}

func checkSheetsGridRange(t *testing.T, actual *sheets.GridRange, want *sheets.GridRange) {
	t.Helper()

	if want == nil {
		if actual != nil {
			t.Fatalf("expected nil grid range, got %+v", actual)
		}

		return
	}

	if actual == nil {
		t.Fatalf("expected grid range %+v, got nil", want)
	}

	if actual.SheetId != want.SheetId {
		t.Fatalf("SheetId = %d, want %d", actual.SheetId, want.SheetId)
	}

	if actual.StartRowIndex != want.StartRowIndex {
		t.Fatalf("StartRowIndex = %d, want %d", actual.StartRowIndex, want.StartRowIndex)
	}

	if actual.EndRowIndex != want.EndRowIndex {
		t.Fatalf("EndRowIndex = %d, want %d", actual.EndRowIndex, want.EndRowIndex)
	}

	if actual.StartColumnIndex != want.StartColumnIndex {
		t.Fatalf("StartColumnIndex = %d, want %d", actual.StartColumnIndex, want.StartColumnIndex)
	}

	if actual.EndColumnIndex != want.EndColumnIndex {
		t.Fatalf("EndColumnIndex = %d, want %d", actual.EndColumnIndex, want.EndColumnIndex)
	}
}
