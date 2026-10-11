package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openclaw/gogcli/internal/googleapi"
)

var (
	errCalendarDiscoveryDenied = errors.New("calendar discovery request denied")
	errCalendarDiscoveryLoop   = errors.New("calendar discovery pagination repeated")
)

func (d EngineDiscoverer) calendars(ctx context.Context, connection Connection, token OAuthToken) ([]ResourceGrant, error) {
	ctx = googleapi.WithRequestGuard(ctx, func(request *http.Request) error {
		if request.URL == nil || request.URL.Scheme != "https" || request.URL.Host != "www.googleapis.com" || request.URL.User != nil || request.Method != http.MethodGet || request.URL.Path != "/calendar/v3/users/me/calendarList" {
			return errCalendarDiscoveryDenied
		}

		return nil
	})

	factory := d.CalendarFactory
	if factory == nil {
		factory = googleapi.NewCalendar
	}

	service, err := factory(ctx, connection.GoogleEmail)
	if err != nil {
		return nil, fmt.Errorf("create Calendar client: %w", err)
	}

	items := []ResourceGrant{}
	seenIDs := map[string]bool{}
	seenPages := map[string]bool{}

	page := ""
	for {
		if seenPages[page] {
			return nil, errCalendarDiscoveryLoop
		}
		seenPages[page] = true

		call := service.CalendarList.List().MaxResults(250).Context(ctx)
		if page != "" {
			call = call.PageToken(page)
		}

		response, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("discover Calendar page: %w", err)
		}

		for _, item := range response.Items {
			if item == nil || item.Id == "" || item.Deleted || seenIDs[item.Id] {
				continue
			}
			seenIDs[item.Id] = true

			label := item.SummaryOverride
			if label == "" {
				label = item.Summary
			}
			items = append(items, grant(connection, "calendar", "calendar", item.Id, label, "", token))
		}

		page = response.NextPageToken
		if page == "" {
			return items, nil
		}
	}
}
