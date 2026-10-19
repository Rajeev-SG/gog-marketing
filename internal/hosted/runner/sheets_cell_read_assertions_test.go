package runner

import (
	"encoding/json"
	"testing"
)

func checkSheetsCellMetadataResult(t *testing.T, kind string, raw json.RawMessage) {
	t.Helper()

	var payload struct {
		Source string `json:"source"`
		Notes  []struct {
			Row   int    `json:"row"`
			Col   int    `json:"col"`
			A1    string `json:"a1"`
			Note  string `json:"note"`
			Value string `json:"value"`
		} `json:"notes"`
		Links []struct {
			Row  int    `json:"row"`
			Col  int    `json:"col"`
			A1   string `json:"a1"`
			Link string `json:"link"`
		} `json:"links"`
		Formats []struct {
			Row    int    `json:"row"`
			Col    int    `json:"col"`
			A1     string `json:"a1"`
			Format *struct {
				NumberFormat *struct {
					Type    string `json:"type"`
					Pattern string `json:"pattern"`
				} `json:"numberFormat"`
			} `json:"format"`
		} `json:"formats"`
		Validations []struct {
			Row  int    `json:"row"`
			Col  int    `json:"col"`
			A1   string `json:"a1"`
			Rule *struct {
				Condition *struct {
					Type   string `json:"type"`
					Values []struct {
						UserEnteredValue string `json:"userEnteredValue"`
					} `json:"values"`
				} `json:"condition"`
			} `json:"rule"`
		} `json:"validations"`
	}

	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode %s payload: %v", kind, err)
	}

	wantA1 := [2]string{"'Revenue Tab'!B3", "'Revenue Tab'!B4"}

	switch kind {
	case "notes":
		if len(payload.Notes) != 1 {
			t.Fatalf("want 1 note, got %d", len(payload.Notes))
		}

		n := payload.Notes[0]
		if n.Row != 3 || n.Col != 2 {
			t.Errorf("note cell want row 3 col 2, got %d/%d", n.Row, n.Col)
		}

		if n.A1 != wantA1[0] {
			t.Errorf("note a1 want %q, got %q", wantA1[0], n.A1)
		}

		if n.Note != "Review" {
			t.Errorf("note text want %q, got %q", "Review", n.Note)
		}

		if n.Value != "$12.00" {
			t.Errorf("note value want %q, got %q", "$12.00", n.Value)
		}
	case "links":
		if len(payload.Links) != 2 {
			t.Fatalf("want 2 links, got %d", len(payload.Links))
		}
		wantLinks := [2]string{"https://foreign.test/direct", "https://foreign.test/rich"}

		for i, l := range payload.Links {
			if l.Row != 3 || l.Col != 2 {
				t.Errorf("link[%d] cell want row 3 col 2, got %d/%d", i, l.Row, l.Col)
			}

			if l.A1 != wantA1[0] {
				t.Errorf("link[%d] a1 want %q, got %q", i, wantA1[0], l.A1)
			}

			if l.Link != wantLinks[i] {
				t.Errorf("link[%d] url want %q, got %q", i, wantLinks[i], l.Link)
			}
		}
	case "format-user", "format-effective":
		if len(payload.Formats) != 1 {
			t.Fatalf("want 1 format, got %d", len(payload.Formats))
		}

		f := payload.Formats[0]
		if f.Row != 3 || f.Col != 2 {
			t.Errorf("%s cell want row 3 col 2, got %d/%d", kind, f.Row, f.Col)
		}

		if f.A1 != wantA1[0] {
			t.Errorf("%s a1 want %q, got %q", kind, wantA1[0], f.A1)
		}

		if f.Format == nil || f.Format.NumberFormat == nil {
			t.Fatalf("%s missing numberFormat", kind)
		}
		nf := f.Format.NumberFormat

		wantType, wantPattern, wantSource := "NUMBER", "0.0", "userEnteredFormat"
		if kind == "format-effective" {
			wantType, wantPattern, wantSource = "CURRENCY", "$0.00", "effectiveFormat"
		}

		if payload.Source != wantSource {
			t.Errorf("format source want %q got %q", wantSource, payload.Source)
		}

		if nf.Type != wantType {
			t.Errorf("%s type want %q, got %q", kind, wantType, nf.Type)
		}

		if nf.Pattern != wantPattern {
			t.Errorf("%s pattern want %q, got %q", kind, wantPattern, nf.Pattern)
		}
	case "validation":
		if len(payload.Validations) != 2 {
			t.Fatalf("want 2 validations, got %d", len(payload.Validations))
		}
		wantRows := [2]int{3, 4}
		wantVals := [2]string{"Alpha", "TableAlpha"}

		for i, v := range payload.Validations {
			if v.Row != wantRows[i] || v.Col != 2 {
				t.Errorf("validation[%d] cell want row %d col 2, got %d/%d", i, wantRows[i], v.Row, v.Col)
			}

			if v.A1 != wantA1[i] {
				t.Errorf("validation[%d] a1 want %q, got %q", i, wantA1[i], v.A1)
			}

			if v.Rule == nil || v.Rule.Condition == nil {
				t.Fatalf("validation[%d] missing condition", i)
			}

			if v.Rule.Condition.Type != "ONE_OF_LIST" {
				t.Errorf("validation[%d] condition want ONE_OF_LIST, got %q", i, v.Rule.Condition.Type)
			}

			if len(v.Rule.Condition.Values) == 0 {
				t.Fatalf("validation[%d] has no values", i)
			}

			if got := v.Rule.Condition.Values[0].UserEnteredValue; got != wantVals[i] {
				t.Errorf("validation[%d] value want %q, got %q", i, wantVals[i], got)
			}
		}
	default:
		t.Fatalf("unknown kind %q", kind)
	}
}
