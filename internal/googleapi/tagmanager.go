package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/tagmanager/v2"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewTagManager(ctx context.Context, email string) (*tagmanager.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceTagManager, email)
	if err != nil {
		return nil, fmt.Errorf("tagmanager options: %w", err)
	}

	svc, err := tagmanager.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create tagmanager service: %w", err)
	}

	return svc, nil
}
