package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	gapi "google.golang.org/api/googleapi"
	siteverificationapi "google.golang.org/api/siteverification/v1"

	"github.com/openclaw/gogcli/internal/errfmt"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

// Verification deliberately supports only the DNS TXT flow for internet
// domains. Site/file/meta/tag-manager methods and owner delegation are not
// exposed: the least-privilege siteverification.verify_only grant cannot list,
// update, delete or delegate ownership, and this surface never sends `owners`.
const siteVerificationDomainType = "INET_DOMAIN"

const siteVerificationDNSMethod = "DNS_TXT"

var siteVerificationDomainLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var (
	siteVerificationIPv4       = regexp.MustCompile(`^[0-9.]+$`)
	siteVerificationNumericTLD = regexp.MustCompile(`^[0-9]+$`)
)

// wrapSiteVerificationError maps the two common provider failures to actionable
// instructions. The Site Verification API is a separate enablement and scope
// from Search Console, so it needs its own message.
func wrapSiteVerificationError(err error) error {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return err
	}

	if apiErr.Code != 403 {
		return err
	}

	message := strings.ToLower(apiErr.Message)
	switch {
	case strings.Contains(message, "accessnotconfigured"), strings.Contains(message, "api has not been used"):
		return errfmt.NewUserFacingError("Site Verification API is not enabled for this OAuth project. Enable it at https://console.cloud.google.com/apis/library/siteverification.googleapis.com", err)
	case strings.Contains(message, "insufficientpermissions"), strings.Contains(message, "insufficient permission"), strings.Contains(message, "insufficient authentication scopes"):
		return errfmt.NewUserFacingError("insufficient permissions for Site Verification. Re-authorize with: gog auth add <email> --services searchconsole", err)
	default:
		return err
	}
}

type SearchConsoleVerificationCmd struct {
	Token  SearchConsoleVerificationTokenCmd  `cmd:"" name:"token" aliases:"get-token,dns-txt" help:"Get the DNS TXT ownership-verification token for an internet domain"`
	Verify SearchConsoleVerificationVerifyCmd `cmd:"" name:"verify" help:"Verify domain ownership after publishing the DNS TXT record"`
}

type SearchConsoleVerificationTokenCmd struct {
	Domain string `arg:"" name:"domain" help:"Bare domain to verify (e.g. example.com)"`
}

func (c *SearchConsoleVerificationTokenCmd) Run(ctx context.Context, flags *RootFlags) error {
	domain, err := normalizeSiteVerificationDomain(c.Domain)
	if err != nil {
		return err
	}

	// Token issuance starts the ownership-verification write flow: honour the
	// same dry-run/read-only contract as the verify step.
	err = marketingDryRunExit(ctx, flags, "searchconsole.verification.token", map[string]any{
		"domain":              domain,
		"site_type":           siteVerificationDomainType,
		"verification_method": siteVerificationDNSMethod,
	})
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := siteVerificationService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.WebResource.GetToken(&siteverificationapi.SiteVerificationWebResourceGettokenRequest{
		Site: &siteverificationapi.SiteVerificationWebResourceGettokenRequestSite{
			Type:       siteVerificationDomainType,
			Identifier: domain,
		},
		VerificationMethod: siteVerificationDNSMethod,
	}).Context(ctx).Do()
	if err != nil {
		return wrapSiteVerificationError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"domain":     domain,
			"site_type":  siteVerificationDomainType,
			"method":     resp.Method,
			"token":      resp.Token,
			"dns_record": "TXT",
			"dns_host":   domain,
			"dns_value":  resp.Token,
			"next_step":  fmt.Sprintf("publish TXT %q at %s, then run: gog searchconsole verification verify %s", resp.Token, domain, domain),
			"state":      "token_issued",
			"verified":   false,
		})
	}

	return writeResult(ctx, ui.FromContext(ctx),
		kv("domain", domain),
		kv("method", resp.Method),
		kv("dns_record", "TXT"),
		kv("dns_host", domain),
		kv("dns_value", resp.Token),
		kv("verified", false),
	)
}

type SearchConsoleVerificationVerifyCmd struct {
	Domain string `arg:"" name:"domain" help:"Bare domain to verify (e.g. example.com)"`
}

func (c *SearchConsoleVerificationVerifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	domain, err := normalizeSiteVerificationDomain(c.Domain)
	if err != nil {
		return err
	}

	err = marketingDryRunExit(ctx, flags, "searchconsole.verification.verify", map[string]any{
		"domain":              domain,
		"site_type":           siteVerificationDomainType,
		"verification_method": siteVerificationDNSMethod,
	})
	if err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := siteVerificationService(ctx, account)
	if err != nil {
		return err
	}
	// Owners are never supplied: verification_only records the authenticated
	// user's own ownership and cannot delegate it to other email addresses.
	resource, err := svc.WebResource.Insert(siteVerificationDNSMethod, &siteverificationapi.SiteVerificationWebResourceResource{
		Site: &siteverificationapi.SiteVerificationWebResourceResourceSite{
			Type:       siteVerificationDomainType,
			Identifier: domain,
		},
	}).Context(ctx).Do()
	if err != nil {
		return wrapSiteVerificationError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"domain":    domain,
			"site_type": siteVerificationDomainType,
			"method":    siteVerificationDNSMethod,
			"verified":  true,
			"resource":  resource,
		})
	}

	return writeResult(ctx, ui.FromContext(ctx),
		kv("domain", domain),
		kv("method", siteVerificationDNSMethod),
		kv("verified", true),
	)
}

// normalizeSearchConsoleSiteProperty canonicalises a property identifier before
// Sites.Add. It accepts the two documented forms: a URL-prefix property
// (http(s) URL, no query/fragment/dot-segments) or an sc-domain property, whose
// domain is lowercased and validated because Search Console stores domain
// properties in canonical form.
func normalizeSearchConsoleSiteProperty(raw string) (string, error) {
	site := strings.TrimSpace(raw)
	if site == "" {
		return "", usage("empty siteUrl")
	}

	if domain, ok := strings.CutPrefix(site, "sc-domain:"); ok {
		normalized, err := normalizeSiteVerificationDomain(domain)
		if err != nil {
			return "", usagef("invalid siteUrl %q (expected sc-domain:<domain>)", raw)
		}

		return "sc-domain:" + normalized, nil
	}

	parsed, err := url.ParseRequestURI(site)
	if err != nil || parsed == nil || parsed.Host == "" {
		return "", usagef("invalid siteUrl %q (expected http(s) URL or sc-domain:<domain>)", raw)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", usagef("invalid siteUrl %q (expected http(s) URL or sc-domain:<domain>)", raw)
	}

	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(site, "?#\\") {
		return "", usagef("invalid siteUrl %q (query, fragment, credentials and backslashes are not allowed)", raw)
	}

	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == ".." || segment == "." {
			return "", usagef("invalid siteUrl %q (dot segments are not allowed)", raw)
		}
	}

	return site, nil
}

// normalizeSiteVerificationDomain accepts only a bare internet domain. It
// rejects schemes, paths, ports, credentials, wildcards, trailing dots, IP
// literals and public suffixes so a caller cannot smuggle a URL or a foreign
// resource identifier into the Site Verification token/insert request.
func normalizeSiteVerificationDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(raw))
	if domain == "" {
		return "", usage("empty domain")
	}
	if len(domain) > 253 {
		return "", usagef("invalid domain %q (too long)", raw)
	}
	if strings.ContainsAny(domain, " \t\r\n/\\?#@:*%") || strings.Contains(domain, "..") {
		return "", usagef("invalid domain %q (expected a bare domain such as example.com)", raw)
	}
	if strings.HasSuffix(domain, ".") {
		return "", usagef("invalid domain %q (trailing dot is not allowed)", raw)
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", usagef("invalid domain %q (expected a registrable domain)", raw)
	}
	for _, label := range labels {
		if !siteVerificationDomainLabel.MatchString(label) {
			return "", usagef("invalid domain %q (expected a bare domain such as example.com)", raw)
		}
	}
	// Reject IP literals: every label of an IPv4 address is numeric, and a
	// registrable domain must not end in a numeric-only TLD.
	if siteVerificationIPv4.MatchString(domain) {
		return "", usagef("invalid domain %q (IP addresses are not verifiable domains)", raw)
	}
	if siteVerificationNumericTLD.MatchString(labels[len(labels)-1]) {
		return "", usagef("invalid domain %q (numeric top-level domain)", raw)
	}

	return domain, nil
}
