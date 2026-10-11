package googleapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/authclient"
)

var (
	errGuardTestDenied  = errors.New("resource denied")
	errGuardTestNetwork = errors.New("unexpected network")
)

func TestRequestGuardAdmissionPrecedesTokenAndNetwork(t *testing.T) {
	denied := errGuardTestDenied
	var tokenCalls atomic.Int64
	var networkCalls atomic.Int64
	ctx := WithRequestGuard(t.Context(), func(*http.Request) error { return denied })
	transport := GuardedTransport(ctx, &oauth2.Transport{
		Source: requestGuardTokenSource(func() (*oauth2.Token, error) {
			tokenCalls.Add(1)
			return &oauth2.Token{AccessToken: "synthetic"}, nil
		}),
		Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			networkCalls.Add(1)
			return nil, errGuardTestNetwork
		}),
	})
	body := &trackedRequestBody{Reader: strings.NewReader("payload")}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://www.googleapis.com/drive/v3/files/unselected", body)
	if err != nil {
		t.Fatal(err)
	}

	response, err := transport.RoundTrip(request)
	if response != nil {
		_ = response.Body.Close()
	}

	if !errors.Is(err, denied) || tokenCalls.Load() != 0 || networkCalls.Load() != 0 || body.closes != 1 {
		t.Fatalf("admission failed: err=%v tokens=%d network=%d closes=%d", err, tokenCalls.Load(), networkCalls.Load(), body.closes)
	}
}

type requestGuardTokenSource func() (*oauth2.Token, error)

func (f requestGuardTokenSource) Token() (*oauth2.Token, error) { return f() }

func TestRequestGuardAddsOperationDeadlineToBackgroundRequest(t *testing.T) {
	started := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(api.Close)

	op, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	ctx := WithRequestGuard(op, func(*http.Request) error { return nil })
	client := &http.Client{Transport: GuardedTransport(ctx, http.DefaultTransport)}

	response, err := client.Get(api.URL) //nolint:noctx // Deliberately exercise SDK-like background requests.
	if response != nil {
		_ = response.Body.Close()
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost operation deadline: %v", err)
	}

	select {
	case <-started:
	default:
		t.Fatal("test never reached the HTTP request")
	}
}

func TestRequestGuardPreservesExplicitRequestCancellation(t *testing.T) {
	ctx := WithRequestGuard(t.Context(), func(*http.Request) error { return nil })
	requestCtx, cancel := context.WithCancel(t.Context())
	cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://www.googleapis.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: GuardedTransport(ctx, http.DefaultTransport)}

	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost request cancellation: %v", err)
	}
}

func TestRequestGuardKeepsResponseAliveUntilBodyReadAndOperationCancellation(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "prefix")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(api.Close)

	op, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := WithRequestGuard(op, func(*http.Request) error { return nil })
	client := &http.Client{Transport: GuardedTransport(ctx, http.DefaultTransport)}

	response, err := client.Get(api.URL) //nolint:noctx // Verify that the transport binds background streaming requests.
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	buffer := make([]byte, len("prefix"))
	if _, err := io.ReadFull(response.Body, buffer); err != nil || string(buffer) != "prefix" {
		t.Fatalf("body canceled at headers: %q %v", buffer, err)
	}

	cancel()

	if _, err := io.ReadAll(response.Body); err == nil {
		t.Fatal("operation cancellation did not terminate the streaming body")
	}
}

func TestRequestGuardAbsentLeavesCLITransportUnchanged(t *testing.T) {
	base := http.DefaultTransport
	if GuardedTransport(t.Context(), base) != base {
		t.Fatal("ordinary CLI transport changed")
	}

	if GuardedTransport(WithRequestGuard(t.Context(), nil), base) != base {
		t.Fatal("nil guard changed transport")
	}
}

func TestRequestGuardPreservesTrustedOperationValues(t *testing.T) {
	ctx := WithRequestGuard(WithoutRetries(t.Context()), func(*http.Request) error { return nil })
	transport := GuardedTransport(ctx, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if !retriesDisabled(request.Context()) {
			t.Fatal("background SDK request lost trusted no-retry policy")
		}

		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: http.Header{}}, nil
	}))

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://www.googleapis.com/", nil)
	if err != nil {
		t.Fatal(err)
	}

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestRequestGuardWiresBothOrdinaryAndServiceAccountClientPaths(t *testing.T) {
	denied := errGuardTestDenied
	var requests atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(w, `{"id":"unselected"}`)
	}))
	t.Cleanup(api.Close)
	ctx := WithRequestGuard(t.Context(), func(*http.Request) error { return denied })
	ctx = WithAuthDependencies(ctx, AuthDependencies{Mode: AuthModeStored})
	ctx = authclient.WithAccessToken(ctx, "synthetic")

	ordinary, err := optionsForAccountScopes(ctx, "drive", "owner@example.com", []string{"https://www.googleapis.com/auth/drive.file"})
	if err != nil {
		t.Fatal(err)
	}

	for _, options := range [][]option.ClientOption{
		ordinary,
		tokenSourceClientOptions(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "synthetic"})),
	} {
		service, err := drive.NewService(ctx, append(options, option.WithEndpoint(api.URL+"/"))...)
		if err != nil {
			t.Fatal(err)
		}

		_, err = service.Files.Get("unselected").Do()
		if !errors.Is(err, denied) {
			t.Fatalf("guard missing from credential path: %v", err)
		}
	}

	if requests.Load() != 0 {
		t.Fatal("rejected credential path reached API")
	}
}
