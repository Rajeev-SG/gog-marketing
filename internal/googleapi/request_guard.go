package googleapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// RequestGuard admits each outbound request before credentials or network I/O.
// It is supplied by trusted hosted authorization code, never by tool arguments.
type RequestGuard func(*http.Request) error

var errInvalidGuardedRequest = errors.New("invalid guarded request")

type requestGuardContextKey struct{}

// WithRequestGuard adds opt-in request admission and binds all requests from
// clients created with ctx to its operation lifetime. This also covers native
// Google handlers that call Do without explicitly passing Context(ctx).
// Ordinary CLI transports are unchanged when no guard is present.
func WithRequestGuard(ctx context.Context, guard RequestGuard) context.Context {
	return context.WithValue(ctx, requestGuardContextKey{}, guard)
}

// GuardedTransport applies the trusted context guard to an existing transport.
// Native service-factory seams can use it without duplicating admission logic.
func GuardedTransport(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	guard, ok := ctx.Value(requestGuardContextKey{}).(RequestGuard)
	if !ok || guard == nil {
		return base
	}

	if base == nil {
		base = http.DefaultTransport
	}

	return &requestGuardTransport{operation: ctx, guard: guard, base: base}
}

//nolint:containedctx // The client intentionally binds SDK requests to its trusted operation lifetime.
type requestGuardTransport struct {
	operation context.Context
	guard     RequestGuard
	base      http.RoundTripper
}

func closeGuardedRequestBody(request *http.Request) {
	if request != nil && request.Body != nil {
		_ = request.Body.Close()
	}
}

func (t *requestGuardTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		closeGuardedRequestBody(request)
		return nil, errInvalidGuardedRequest
	}

	if err := t.operation.Err(); err != nil {
		closeGuardedRequestBody(request)
		return nil, fmt.Errorf("google operation ended: %w", err)
	}

	if err := request.Context().Err(); err != nil {
		closeGuardedRequestBody(request)
		return nil, fmt.Errorf("google request ended: %w", err)
	}

	if err := t.guard(request); err != nil {
		closeGuardedRequestBody(request)
		return nil, fmt.Errorf("google request admission: %w", err)
	}

	// Preserve the request's own cancellation while adding the trusted
	// operation deadline/cancellation. Do not cancel on successful headers:
	// the SDK still has to read the response body.
	ctx, cancel := context.WithCancel(t.operation)
	stop := context.AfterFunc(request.Context(), cancel)
	cleanup := func() { stop(); cancel() }

	response, err := t.base.RoundTrip(request.Clone(ctx)) //nolint:contextcheck // The merged context inherits the trusted operation instead of an SDK background context.
	if err != nil {
		cleanup()

		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}

		if operationErr := t.operation.Err(); operationErr != nil {
			return nil, fmt.Errorf("google operation ended: %w", operationErr)
		}

		return nil, fmt.Errorf("guarded Google request: %w", err)
	}

	if response == nil || response.Body == nil {
		cleanup()
		return response, nil
	}

	response.Body = &guardedResponseBody{ReadCloser: response.Body, cleanup: cleanup}

	return response, nil
}

type guardedResponseBody struct {
	io.ReadCloser
	cleanup func()
	once    sync.Once
}

func (b *guardedResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(b.cleanup)
	}

	return n, err //nolint:wrapcheck // Preserve standard response-body EOF/error semantics.
}

func (b *guardedResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cleanup)

	return err //nolint:wrapcheck // Preserve the underlying response-body close error.
}
