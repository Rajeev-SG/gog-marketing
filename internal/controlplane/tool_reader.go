package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openclaw/gogcli/internal/googleapi"
)

var ErrToolReaderUnsupported = errors.New("tool reader does not support tool")

type ToolReadResult struct {
	ResultCount int `json:"result_count"`
}

type ToolReader interface {
	Read(context.Context, Connection, OAuthToken, string) (ToolReadResult, error)
}

// EngineToolReader exposes representative read-only tools through the same
// stored-token and typed Google client path as resource reads.
type EngineToolReader struct{}

func (EngineToolReader) Read(ctx context.Context, connection Connection, token OAuthToken, tool string) (ToolReadResult, error) {
	ctx = withGoogleAuth(ctx, token)

	switch tool {
	case "gmail_search":
		service, err := googleapi.NewGmail(ctx, connection.GoogleEmail)
		if err != nil {
			return ToolReadResult{}, fmt.Errorf("create Gmail tool reader: %w", err)
		}

		response, err := service.Users.Messages.List("me").Q("newer_than:1d").MaxResults(1).Context(ctx).Do()
		if err != nil {
			return ToolReadResult{}, fmt.Errorf("read Gmail tool: %w", err)
		}

		return ToolReadResult{ResultCount: len(response.Messages)}, nil
	case "calendar_events":
		service, err := googleapi.NewCalendar(ctx, connection.GoogleEmail)
		if err != nil {
			return ToolReadResult{}, fmt.Errorf("create Calendar tool reader: %w", err)
		}

		response, err := service.Events.List("primary").TimeMin(time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)).MaxResults(1).SingleEvents(true).Context(ctx).Do()
		if err != nil {
			return ToolReadResult{}, fmt.Errorf("read Calendar tool: %w", err)
		}

		return ToolReadResult{ResultCount: len(response.Items)}, nil
	case "drive_search":
		service, err := googleapi.NewDrive(ctx, connection.GoogleEmail)
		if err != nil {
			return ToolReadResult{}, fmt.Errorf("create Drive tool reader: %w", err)
		}

		response, err := service.Files.List().PageSize(1).Context(ctx).Do()
		if err != nil {
			return ToolReadResult{}, fmt.Errorf("read Drive tool: %w", err)
		}

		return ToolReadResult{ResultCount: len(response.Files)}, nil
	default:
		return ToolReadResult{}, fmt.Errorf("%w: %s", ErrToolReaderUnsupported, tool)
	}
}
