package cmd

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestHostedExecutionMarkerIsNotAClientCLIFlag(t *testing.T) {
	parser, _, err := newParserWithWriters("test", io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"--hosted-execution", "calendar", "events"}); err == nil {
		t.Fatal("trusted hosted marker exposed as a CLI flag")
	}
}

func TestHostedCalendarRejectsAllCalendarSelectorsBeforeRuntimeOrGoogle(t *testing.T) {
	command := CalendarEventsCmd{Max: 10, All: true}
	if err := command.Run(context.Background(), &RootFlags{Account: "owner@example.test", hostedExecution: true}); !errors.Is(err, ErrHostedToolDenied) {
		t.Fatalf("unadmitted all-calendar selector accepted or accessed runtime: %v", err)
	}
}
