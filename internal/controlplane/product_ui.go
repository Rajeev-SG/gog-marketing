package controlplane

import (
	_ "embed"
	"html/template"
	"net/http"
	"strings"
)

const (
	fieldDisplayName = "DisplayName"
	fieldPageTitle   = "PageTitle"
	fieldCSRF        = "CSRF"
)

// The product UI ships as a single embedded stylesheet and template file so a
// clean checkout renders the same interface without a frontend build step.
// Icons are inline Lucide SVG paths; brand marks are intentionally absent
// until a service is actually supported.

//go:embed ui/product.css
var productCSS string

//go:embed ui/product.tmpl
var productTemplateSource string

// productTemplates keeps the historical test-facing name; it now sources the
// embedded template file.
var productTemplates = productTemplateSource

var productIconPaths = map[string]string{
	"info":    "<circle cx='12' cy='12' r='10'/><path d='M12 16v-4'/><path d='M12 8h.01'/>",
	"shield":  "<path d='M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z'/>",
	"check":   "<path d='M20 6 9 17l-5-5'/>",
	"refresh": "<path d='M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8'/><path d='M21 3v5h-5'/><path d='M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16'/><path d='M8 16H3v5'/>",
	"home":    "<path d='m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z'/><polyline points='9 22 9 12 15 12 15 22'/>",
	"search":  "<circle cx='11' cy='11' r='8'/><path d='m21 21-4.3-4.3'/>",
	"globe":   "<circle cx='12' cy='12' r='10'/><path d='M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20'/><path d='M2 12h20'/>",
	"plus":    "<path d='M5 12h14'/><path d='M12 5v14'/>",
}

func productIcon(name string) template.HTML {
	path, ok := productIconPaths[name]
	if !ok {
		return ""
	}

	//nolint:gosec // SVG paths come only from the fixed internal icon allowlist above.
	return template.HTML(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` + path + `</svg>`)
}

func templateDict(values ...any) map[string]any {
	out := make(map[string]any, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		key, _ := values[i].(string)
		out[key] = values[i+1]
	}

	return out
}

func (h *ProductHandler) staticProductCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(productCSS))
}

func (h *ProductHandler) onboarding(w http.ResponseWriter, r *http.Request) {
	actor, session, ok := h.actor(w, r)
	if !ok {
		return
	}

	connection, err := h.productConnectionByID(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	grants, listErr := h.config.Service.ListResources(r.Context(), actor, connection.ID)
	if listErr != nil {
		grants = nil
	}

	enabled := 0

	for _, grant := range grants {
		if grant.Enabled {
			enabled++
		}
	}

	state := productConnectionState(connection)
	h.render(w, "onboarding", map[string]any{
		"Connection":       connection,
		"State":            state,
		"StateLabel":       productStateLabel(state),
		"EnabledCount":     enabled,
		fieldDisplayName:   h.config.DisplayName,
		fieldPageTitle:     "Onboarding",
		"Message":          r.URL.Query().Get("message"),
		templateErrorField: r.URL.Query().Get("error"),
		fieldCSRF:          session.CSRF,
	})
}

type productRailItem struct {
	Key        string
	Name       string
	State      string
	StateLabel string
}

type productRailGroup struct {
	Name  string
	Items []productRailItem
}

func productRailGroups(connection Connection) []productRailGroup {
	seen := make(map[string]bool)
	workspace := productRailGroup{Name: ProductCategoryWorkspace, Items: []productRailItem{}}
	marketing := productRailGroup{Name: ProductCategoryMarketing, Items: []productRailItem{}}

	for _, raw := range connection.Services {
		service := strings.ToLower(strings.TrimSpace(raw))
		if service == "" || seen[service] {
			continue
		}
		seen[service] = true

		item := productRailItem{Key: service, Name: serviceLabel(service)}
		if status, ok := connection.DiscoveryStatus[service]; ok {
			item.State, item.StateLabel = productRailStatus(status)
		} else {
			item.State, item.StateLabel = "disconnected", "Not checked"
		}

		category := ProductCategoryWorkspace
		if catalogService, catalogErr := productService(service); catalogErr == nil {
			category = catalogService.Category
		}

		if category == ProductCategoryMarketing {
			marketing.Items = append(marketing.Items, item)
		} else {
			workspace.Items = append(workspace.Items, item)
		}
	}

	return []productRailGroup{workspace, marketing}
}

func productRailStatus(status DiscoveryServiceStatus) (string, string) {
	switch status.State {
	case DiscoveryServiceOK:
		return "", "Ready"
	case DiscoveryServiceError:
		return "needs_attention", "Needs attention"
	case DiscoveryServiceUnavailable:
		return "unavailable", "Unavailable"
	case DiscoveryServiceUnsupported:
		return "disconnected", "Unsupported"
	default:
		return "disconnected", "Not checked"
	}
}

func productFailedServices(connection Connection) []productServiceStatus {
	out := []productServiceStatus{}

	for _, status := range productDiscoveryStatuses(connection) {
		if status.Detail == "Ready" || status.Detail == "Not checked" {
			continue
		}
		out = append(out, status)
	}

	return out
}

// productHomeNotice maps structured notice codes to marketer-safe copy.
func productHomeNotice(notice string) string {
	if notice == "duplicate_google_account" {
		return "This Google account is already connected."
	}

	return ""
}
