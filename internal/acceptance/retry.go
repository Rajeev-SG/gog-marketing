package acceptance

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
)

type RetryPolicy struct {
	MaxAttempts    int
	BaseDelay      time.Duration
	MaxDelay       time.Duration
	AttemptTimeout time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: 250 * time.Millisecond, MaxDelay: 2 * time.Second, AttemptTimeout: 30 * time.Second}
}

func (p RetryPolicy) Run(ctx context.Context, operation string, fn func(context.Context) error) (attempts int, err error) {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 1
	}

	if p.BaseDelay <= 0 {
		p.BaseDelay = 100 * time.Millisecond
	}

	if p.MaxDelay <= 0 {
		p.MaxDelay = p.BaseDelay
	}

	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		attempts = attempt

		attemptCtx, cancel := context.WithTimeout(ctx, p.AttemptTimeout)
		err = fn(attemptCtx)

		cancel()

		if err == nil || !Retryable(err) || attempt == p.MaxAttempts {
			return attempts, wrapAcceptanceError(err)
		}

		delay := p.BaseDelay << (attempt - 1)
		if delay > p.MaxDelay {
			delay = p.MaxDelay
		}

		var randomBytes [8]byte
		if _, readErr := rand.Read(randomBytes[:]); readErr != nil {
			return attempts, wrapAcceptanceError(readErr)
		}

		jitter := time.Duration(int(randomBytes[0]) % int(delay/2+1))
		select {
		case <-ctx.Done():
			return attempts, wrapAcceptanceError(ctx.Err())
		case <-time.After(delay + jitter):
		}
	}

	return attempts, wrapAcceptanceError(err)
}

func Retryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429 || apiErr.Code >= 500
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	message := strings.ToLower(err.Error())

	return strings.Contains(message, "timeout") ||
		strings.Contains(message, "temporarily") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "http 5") ||
		strings.Contains(message, "http 429")
}
