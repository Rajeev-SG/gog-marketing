package runner

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	gapi "google.golang.org/api/googleapi"
)

var searchDomain = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,251}[A-Za-z0-9]$`)

func validSearchSite(site string) bool {
	if strings.HasPrefix(site, "sc-domain:") {
		return searchDomain.MatchString(strings.TrimPrefix(site, "sc-domain:"))
	}
	parsed, err := url.Parse(site)

	return err == nil && validSearchURL(parsed) && parsed.RawQuery == "" && !strings.Contains(parsed.Path, "/../") && !strings.Contains(parsed.Path, "/./") && !strings.HasSuffix(parsed.Path, "/..") && !strings.HasSuffix(parsed.Path, "/.")
}

func validSearchURL(parsed *url.URL) bool {
	return parsed != nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == "" && !strings.Contains(parsed.Path, "\\")
}

func searchURLWithinSite(site, raw string) bool {
	target, err := url.Parse(raw)
	if err != nil || !validSearchURL(target) || strings.Contains(target.Path, "/../") || strings.Contains(target.Path, "/./") {
		return false
	}

	if strings.HasPrefix(site, "sc-domain:") {
		domain := strings.ToLower(strings.TrimPrefix(site, "sc-domain:"))
		host := strings.ToLower(target.Hostname())

		return host == domain || strings.HasSuffix(host, "."+domain)
	}
	prefix, err := url.Parse(site)

	return err == nil && target.Scheme == prefix.Scheme && strings.EqualFold(target.Host, prefix.Host) && strings.HasPrefix(target.EscapedPath(), prefix.EscapedPath())
}

func nativeSearchConsoleBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 8)

	for _, operation := range []struct {
		name, method, template      string
		account, write, destructive bool
	}{
		{"sites_list", http.MethodGet, "/webmasters/v3/sites", true, false, false},
		{"sites_get", http.MethodGet, "/webmasters/v3/sites/{siteUrl}", false, false, false},
		{"query", http.MethodPost, "/webmasters/v3/sites/{siteUrl}/searchAnalytics/query", false, false, false},
		{"sitemaps_list", http.MethodGet, "/webmasters/v3/sites/{siteUrl}/sitemaps", false, false, false},
		{"sitemaps_get", http.MethodGet, "/webmasters/v3/sites/{siteUrl}/sitemaps/{feedpath}", false, false, false},
		{"sitemaps_submit", http.MethodPut, "/webmasters/v3/sites/{siteUrl}/sitemaps/{feedpath}", false, true, false},
		{"sitemaps_delete", http.MethodDelete, "/webmasters/v3/sites/{siteUrl}/sitemaps/{feedpath}", false, true, true},
		{"inspect", http.MethodPost, "/v1/urlInspection/index:inspect", false, false, false},
	} {
		resourceType, resourceArgument := "site", "site_url"
		if operation.account {
			resourceType, resourceArgument = "", ""
		}

		scopes := []string{"webmasters"}
		if !operation.write {
			scopes = append(scopes, "webmasters.readonly")
		}
		result = append(result, NativeToolBinding{
			Name: "searchconsole_" + operation.name, Service: "searchconsole", ResourceType: resourceType, ResourceArgument: resourceArgument, Write: operation.write,
			hosts: []string{"searchconsole.googleapis.com"}, methods: []string{operation.method}, scopes: scopes, destructive: operation.destructive, escapedPaths: true, paths: exactPaths("", ""),
			argumentPaths: func(args map[string]any) ([]string, error) {
				if operation.account {
					return []string{operation.template}, nil
				}

				site, _ := args["site_url"].(string)
				if !validSearchSite(site) {
					return nil, invalidf("invalid Search Console site")
				}
				values := map[string]string{"siteUrl": site}

				if strings.Contains(operation.template, "{feedpath}") {
					feed, _ := args["feed_path"].(string)
					if !searchURLWithinSite(site, feed) {
						return nil, invalidf("sitemap must belong to selected site")
					}
					values["feedpath"] = feed
				}

				if operation.name == "inspect" {
					inspected, _ := args["url"].(string)
					if !searchURLWithinSite(site, inspected) {
						return nil, invalidf("inspection URL must belong to selected site")
					}
				}
				path := url.URL{Path: operation.template}
				gapi.Expand(&path, values)

				return []string{path.EscapedPath()}, nil
			},
		})
	}

	return result
}
