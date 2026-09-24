package googleapi

import (
	"context"
	"net/http"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewGoogleAdsHTTPClient(ctx context.Context, account string) (*http.Client, error) {
	return NewHTTPClient(ctx, googleauth.ServiceGoogleAds, account)
}
