package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"
)

func TestSheetsCellReadsUseExecutionContext(t *testing.T) {
	for _, test := range []struct {
		name    string
		command interface {
			Run(context.Context, *RootFlags) error
		}
	}{
		{"notes", &SheetsNotesCmd{SpreadsheetID: "selected-sheet", Range: "Sheet1!B3:B4"}},
		{"links", &SheetsLinksGetCmd{SpreadsheetID: "selected-sheet", Range: "Sheet1!B3:B4"}},
		{"format", &SheetsReadFormatCmd{SpreadsheetID: "selected-sheet", Range: "Sheet1!B3:B4"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			type contextKey struct{}
			ctx, cancel := context.WithDeadline(context.WithValue(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), contextKey{}, "execution"), time.Now().Add(time.Minute))
			defer cancel()
			called := false
			service, err := sheets.NewService(context.Background(), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				called = true
				if request.Context().Value(contextKey{}) != "execution" {
					t.Error("cell request lost execution context")
				}
				if _, ok := request.Context().Deadline(); !ok {
					t.Error("cell request lost deadline")
				}
				cancel()
				if !errors.Is(request.Context().Err(), context.Canceled) {
					t.Error("cell request did not observe execution cancellation")
				}
				return nil, context.Canceled
			})}))
			if err != nil {
				t.Fatal(err)
			}
			err = test.command.Run(withSheetsTestService(ctx, service), &RootFlags{Account: "owner@example.com"})
			if !called || !errors.Is(err, context.Canceled) {
				t.Fatalf("cell cancellation: called=%t err=%v", called, err)
			}
		})
	}
}
