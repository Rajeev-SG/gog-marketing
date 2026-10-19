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

func TestSheetsChartGetUsesExecutionContext(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithDeadline(context.WithValue(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), contextKey{}, "execution"), time.Now().Add(time.Minute))
	defer cancel()
	called := false
	service, err := sheets.NewService(context.Background(), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		called = true
		if request.Context().Value(contextKey{}) != "execution" {
			t.Error("chart request lost execution context")
		}
		if _, ok := request.Context().Deadline(); !ok {
			t.Error("chart request lost deadline")
		}
		cancel()
		if !errors.Is(request.Context().Err(), context.Canceled) {
			t.Error("chart request did not observe execution cancellation")
		}
		return nil, context.Canceled
	})}))
	if err != nil {
		t.Fatal(err)
	}
	err = (&SheetsChartGetCmd{SpreadsheetID: "selected-sheet", ChartID: 73}).Run(withSheetsTestService(ctx, service), &RootFlags{Account: "owner@example.com"})
	if !called || !errors.Is(err, context.Canceled) {
		t.Fatalf("chart cancellation: called=%t err=%v", called, err)
	}
}
