package googleapi

import (
	"context"
	"fmt"

	siteverificationapi "google.golang.org/api/siteverification/v1"

	"github.com/openclaw/gogcli/internal/googleauth"
)

// NewSiteVerification reuses the Search Console OAuth grant, which carries the
// least-privilege siteverification.verify_only scope. The full
// `siteverification` scope (owner listing, update, delete, delegation) is never
// requested.
func NewSiteVerification(ctx context.Context, email string) (*siteverificationapi.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceSearchConsole, email); err != nil {
		return nil, fmt.Errorf("siteverification options: %w", err)
	} else if svc, err := siteverificationapi.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create siteverification service: %w", err)
	} else {
		return svc, nil
	}
}
