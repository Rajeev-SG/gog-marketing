package runner

import (
	"testing"

	sheets "google.golang.org/api/sheets/v4"
)

// sheetsGeometryExpectation describes the expected intent of a single
// Sheets SDK request produced from a deterministic fixture. These are
// request-intent assertions against the SDK request object only; no live
// Google API calls or acceptance checks are performed here.
type sheetsGeometryExpectation struct {
	tool      string
	axis      string
	mergeType string
	sheetID   int64
	start     int64
	end       int64
	pixels    int64
	auto      bool
	grid      *sheets.GridRange
}

// checkSheetsGeometrySDKRequest asserts that the SDK request built for a
// sheets geometry operation matches the expected intent.
func checkSheetsGeometrySDKRequest(t *testing.T, request *sheets.Request, want sheetsGeometryExpectation) {
	t.Helper()

	checkGrid := func(actual *sheets.GridRange) {
		if want.grid == nil {
			t.Fatal("missing expected grid range")
		}

		if actual == nil {
			t.Fatalf("expected GridRange %+v, got nil", want.grid)
		}

		if actual.SheetId != want.grid.SheetId {
			t.Errorf("GridRange.SheetId = %d, want %d", actual.SheetId, want.grid.SheetId)
		}

		if actual.StartRowIndex != want.grid.StartRowIndex {
			t.Errorf("GridRange.StartRowIndex = %d, want %d", actual.StartRowIndex, want.grid.StartRowIndex)
		}

		if actual.EndRowIndex != want.grid.EndRowIndex {
			t.Errorf("GridRange.EndRowIndex = %d, want %d", actual.EndRowIndex, want.grid.EndRowIndex)
		}

		if actual.StartColumnIndex != want.grid.StartColumnIndex {
			t.Errorf("GridRange.StartColumnIndex = %d, want %d", actual.StartColumnIndex, want.grid.StartColumnIndex)
		}

		if actual.EndColumnIndex != want.grid.EndColumnIndex {
			t.Errorf("GridRange.EndColumnIndex = %d, want %d", actual.EndColumnIndex, want.grid.EndColumnIndex)
		}
	}

	switch want.tool {
	case "sheets_resize_columns", "sheets_resize_rows":
		if want.auto {
			if request.AutoResizeDimensions == nil {
				t.Fatalf("expected AutoResizeDimensions request for %s, got none", want.tool)
			}

			dim := request.AutoResizeDimensions.Dimensions
			if dim == nil {
				t.Fatalf("AutoResizeDimensions.Dimensions is nil for %s", want.tool)
			}

			if dim.SheetId != want.sheetID {
				t.Errorf("DimensionRange.SheetId = %d, want %d", dim.SheetId, want.sheetID)
			}

			if dim.Dimension != want.axis {
				t.Errorf("DimensionRange.Dimension = %q, want %q", dim.Dimension, want.axis)
			}

			if dim.StartIndex != want.start {
				t.Errorf("DimensionRange.StartIndex = %d, want %d", dim.StartIndex, want.start)
			}

			if dim.EndIndex != want.end {
				t.Errorf("DimensionRange.EndIndex = %d, want %d", dim.EndIndex, want.end)
			}

			return
		}

		if request.UpdateDimensionProperties == nil {
			t.Fatalf("expected UpdateDimensionProperties request for %s, got none", want.tool)
		}

		props := request.UpdateDimensionProperties.Properties
		if props == nil {
			t.Fatalf("UpdateDimensionProperties.Properties is nil for %s", want.tool)
		}

		if request.UpdateDimensionProperties.Fields != "pixelSize" {
			t.Errorf("UpdateDimensionProperties.Fields = %q, want %q", request.UpdateDimensionProperties.Fields, "pixelSize")
		}

		if props.PixelSize != want.pixels {
			t.Errorf("PixelSize = %d, want %d", props.PixelSize, want.pixels)
		}

		dim := request.UpdateDimensionProperties.Range
		if dim == nil {
			t.Fatalf("UpdateDimensionProperties.Range is nil for %s", want.tool)
		}

		if dim.SheetId != want.sheetID {
			t.Errorf("DimensionRange.SheetId = %d, want %d", dim.SheetId, want.sheetID)
		}

		if dim.Dimension != want.axis {
			t.Errorf("DimensionRange.Dimension = %q, want %q", dim.Dimension, want.axis)
		}

		if dim.StartIndex != want.start {
			t.Errorf("DimensionRange.StartIndex = %d, want %d", dim.StartIndex, want.start)
		}

		if dim.EndIndex != want.end {
			t.Errorf("DimensionRange.EndIndex = %d, want %d", dim.EndIndex, want.end)
		}
	case "sheets_merge":
		if request.MergeCells == nil {
			t.Fatal("expected MergeCells request, got none")
		}

		if request.MergeCells.MergeType != want.mergeType {
			t.Errorf("MergeCells.MergeType = %q, want %q", request.MergeCells.MergeType, want.mergeType)
		}

		checkGrid(request.MergeCells.Range)
	case "sheets_unmerge":
		if request.UnmergeCells == nil {
			t.Fatal("expected UnmergeCells request, got none")
		}

		checkGrid(request.UnmergeCells.Range)
	default:
		t.Fatalf("unknown sheets geometry tool %q", want.tool)
	}
}
