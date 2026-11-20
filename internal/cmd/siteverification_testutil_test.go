package cmd

import (
	"context"

	siteverificationapi "google.golang.org/api/siteverification/v1"

	"github.com/openclaw/gogcli/internal/app"
)

var siteVerificationTestServices = googleServiceTestSupport[siteverificationapi.Service, app.SiteVerificationServiceFactory]{
	newService: siteverificationapi.NewService,
	wrap: func(factory func(context.Context, string) (*siteverificationapi.Service, error)) app.SiteVerificationServiceFactory {
		return factory
	},
	services: func(factory app.SiteVerificationServiceFactory) app.Services {
		return app.Services{SiteVerification: factory}
	},
}

var (
	newSiteVerificationTestService         = siteVerificationTestServices.new
	executeWithSiteVerificationTestService = siteVerificationTestServices.executeWithService
)
