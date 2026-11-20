package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/googleapi"
)

func TestExecute_SearchConsoleSitesAdd(t *testing.T) {
	for _, site := range []string{"https://example.com/", "sc-domain:example.com"} {
		t.Run(site, func(t *testing.T) {
			var calls int
			svc := newSearchConsoleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPut {
					t.Errorf("method = %s, want PUT", r.Method)
				}
				if !strings.HasPrefix(r.URL.Path, "/webmasters/v3/sites/") {
					t.Errorf("path = %q", r.URL.Path)
				}
				if r.ContentLength > 0 {
					t.Errorf("sites.add must not send a body")
				}
				if got := r.URL.Query().Get("siteUrl"); got != "" {
					t.Errorf("site must be a path segment, got query %q", got)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			result := executeWithSearchConsoleTestService(t, []string{
				"--json",
				"--account", "a@b.com",
				"searchconsole", "sites", "add", "--", site,
			}, svc)
			if result.err != nil {
				t.Fatalf("Execute: %v", result.err)
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}

			var parsed struct {
				Added   bool   `json:"added"`
				SiteURL string `json:"site_url"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !parsed.Added || parsed.SiteURL != site {
				t.Fatalf("unexpected payload: %#v", parsed)
			}
		})
	}
}

func TestExecute_SearchConsoleSitesAdd_NormalizesDomainCase(t *testing.T) {
	var got string
	svc := newSearchConsoleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	result := executeWithSearchConsoleTestService(t, []string{
		"--json",
		"--account", "a@b.com",
		"searchconsole", "sites", "add", "sc-domain:Example.COM",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.HasSuffix(got, "/webmasters/v3/sites/sc-domain:example.com") {
		t.Fatalf("path = %q, want normalized domain property", got)
	}
}

func TestExecute_SearchConsoleSitesAdd_DryRun(t *testing.T) {
	svc := newSearchConsoleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry run reached Google: %s %s", r.Method, r.URL)
	}))
	result := executeWithSearchConsoleTestService(t, []string{
		"--json",
		"--dry-run",
		"--account", "a@b.com",
		"searchconsole", "sites", "add", "sc-domain:example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var parsed struct {
		DryRun  bool   `json:"dry_run"`
		Op      string `json:"op"`
		Request struct {
			SiteURL string `json:"site_url"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.DryRun || parsed.Op != "searchconsole.sites.add" || parsed.Request.SiteURL != "sc-domain:example.com" {
		t.Fatalf("unexpected dry-run payload: %#v", parsed)
	}
}

func TestExecute_SearchConsoleSitesAdd_ReadOnlyBlocked(t *testing.T) {
	svc := newSearchConsoleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("read-only request reached Google: %s %s", r.Method, r.URL)
	}))
	result := executeWithSearchConsoleTestService(t, []string{
		"--json",
		"--readonly",
		"--account", "a@b.com",
		"searchconsole", "sites", "add", "sc-domain:example.com",
	}, svc)
	if !errors.Is(result.err, googleapi.ErrReadOnly) {
		t.Fatalf("err = %v, want ErrReadOnly", result.err)
	}
}

func TestExecute_SearchConsoleSitesAdd_InvalidSiteIsUsageBeforeDryRun(t *testing.T) {
	for _, site := range []string{
		"example.com",
		"https://example.com/?q=1",
		"https://example.com/#frag",
		"https://user:pass@example.com/",
		"https://example.com/../admin",
		"ftp://example.com/",
		"sc-domain:",
		"sc-domain:*.example.com",
		"sc-domain:not a domain",
		"sc-domain:https://example.com",
	} {
		t.Run(site, func(t *testing.T) {
			result := executeWithSearchConsoleTestService(t, []string{
				"--json",
				"--dry-run",
				"--account", "a@b.com",
				"searchconsole", "sites", "add", "--", site,
			}, nil)
			if result.err == nil || !strings.Contains(result.err.Error(), "invalid siteUrl") {
				t.Fatalf("err = %v, want invalid siteUrl", result.err)
			}
		})
	}
}
