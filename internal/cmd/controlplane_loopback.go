package cmd

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Portless keeps the browser on its named origin while installed Google clients
// return to a supported localhost callback. Only the callback is bridged; the
// existing product handler still verifies its signed cookie, state and PKCE.
func controlPlaneLoopbackBridge(next http.Handler, callbackBase, browserBase string) (http.Handler, error) {
	if strings.TrimSpace(browserBase) == "" || browserBase == callbackBase {
		return next, nil
	}
	callback, err := url.Parse(callbackBase)
	if err != nil {
		return nil, errors.New("invalid loopback OAuth callback origin")
	}
	browser, err := url.Parse(browserBase)
	if err != nil {
		return nil, errors.New("invalid Portless browser origin")
	}
	local := func(host string) bool {
		ip := net.ParseIP(host)
		return host == "localhost" || (ip != nil && ip.IsLoopback())
	}
	if callback.Scheme != "http" || !local(callback.Hostname()) || callback.User != nil || callback.RawQuery != "" || callback.Fragment != "" || (callback.Path != "" && callback.Path != "/") {
		return nil, errors.New("portless OAuth callback must use an HTTP loopback origin")
	}
	if (browser.Scheme != "http" && browser.Scheme != "https") || !strings.HasSuffix(browser.Hostname(), ".localhost") || browser.User != nil || browser.RawQuery != "" || browser.Fragment != "" || (browser.Path != "" && browser.Path != "/") {
		return nil, errors.New("portless browser origin must be a named .localhost origin")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/oauth/google/callback" && strings.EqualFold(r.Host, callback.Host) {
			destination := *browser
			destination.Path = "/oauth/google/callback"
			destination.RawQuery = r.URL.RawQuery
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, r, destination.String(), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}
