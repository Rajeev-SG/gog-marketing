package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
	sheets "google.golang.org/api/sheets/v4"
)

func TestSheetsRangeNotePostsUseExecutionContext(t *testing.T) {
	note := "Inline"
	for _, test := range []struct {
		name    string
		command interface {
			Run(context.Context, *RootFlags) error
		}
	}{
		{"add", &SheetsNamedRangesAddCmd{SpreadsheetID: "selected-sheet", Name: "Created", Range: "First!A1:B2"}},
		{"update", &SheetsNamedRangesUpdateCmd{SpreadsheetID: "selected-sheet", NameOrID: "range-1", NewName: "Renamed"}},
		{"delete", &SheetsNamedRangesDeleteCmd{SpreadsheetID: "selected-sheet", NameOrID: "range-1"}},
		{"note", &SheetsUpdateNoteCmd{SpreadsheetID: "selected-sheet", Range: "First!A1:B2", Note: &note}},
	} {
		t.Run(test.name, func(t *testing.T) {
			type contextKey struct{}
			ctx, cancel := context.WithDeadline(context.WithValue(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), contextKey{}, "execution"), time.Now().Add(time.Minute))
			defer cancel()
			posts, gets := 0, 0
			service, err := sheets.NewService(context.Background(), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method == http.MethodGet {
					gets++
					body := `{"sheets":[{"properties":{"sheetId":0,"title":"First","gridProperties":{"rowCount":10,"columnCount":10}}}],"namedRanges":[{"namedRangeId":"range-1","name":"Existing","range":{"sheetId":0,"startRowIndex":0,"endRowIndex":2,"startColumnIndex":0,"endColumnIndex":2}}]}`
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
				}
				posts++
				if request.Context().Value(contextKey{}) != "execution" {
					t.Error("range/note POST lost execution context")
				}
				if _, ok := request.Context().Deadline(); !ok {
					t.Error("range/note POST lost deadline")
				}
				cancel()
				if !errors.Is(request.Context().Err(), context.Canceled) {
					t.Error("range/note POST did not observe cancellation")
				}
				return nil, context.Canceled
			})}))
			if err != nil {
				t.Fatal(err)
			}
			err = test.command.Run(withSheetsTestService(ctx, service), &RootFlags{Account: "owner@example.com"})
			if gets != 1 || posts != 1 || !errors.Is(err, context.Canceled) {
				t.Fatalf("POST cancellation: gets=%d posts=%d err=%v", gets, posts, err)
			}
		})
	}
}
