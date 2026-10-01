package controlplane

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openclaw/gogcli/internal/googleauth"
)

const (
	ProductCategoryWorkspace = "Workspace"
	ProductCategoryMarketing = "Marketing"
)

type ProductService struct {
	Service       string
	Name          string
	Category      string
	ResourceModel bool
	Tool          string
}

var productToolByService = map[string]string{
	"gmail":    "gmail_search",
	"calendar": "calendar_events",
	"drive":    "drive_search",
}

var productResourceServices = map[string]bool{
	"analytics": true, "googleads": true, "tagmanager": true,
	"searchconsole": true, "bigquery": true,
}

func ProductServices() []ProductService {
	infos := googleauth.ServicesInfo()

	out := make([]ProductService, 0, len(infos))
	for _, info := range infos {
		service := string(info.Service)

		category := ProductCategoryWorkspace
		if service == "analytics" || service == "googleads" || service == "tagmanager" || service == "searchconsole" || service == "bigquery" || service == "adsense" || service == "cloudadmin" {
			category = ProductCategoryMarketing
		}

		name := service
		if len(info.APIs) > 0 {
			name = info.APIs[0]
		}

		out = append(out, ProductService{
			Service: service, Name: name, Category: category,
			ResourceModel: productResourceServices[service], Tool: productToolByService[service],
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}

		return out[i].Name < out[j].Name
	})

	return out
}

func productService(service string) (ProductService, error) {
	for _, candidate := range ProductServices() {
		if candidate.Service == strings.ToLower(strings.TrimSpace(service)) {
			return candidate, nil
		}
	}

	return ProductService{}, fmt.Errorf("%w: unsupported service %q", ErrInvalid, service)
}

func toolGrantKey(service, tool string) string {
	return strings.ToLower(strings.TrimSpace(service)) + "/" + strings.ToLower(strings.TrimSpace(tool))
}

func productToolSelector(service ProductService) string {
	if service.Tool != "" {
		return service.Tool
	}

	return "*"
}

func validProductTool(service ProductService, tool string) bool {
	return strings.TrimSpace(tool) == productToolSelector(service)
}
