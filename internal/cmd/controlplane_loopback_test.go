package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlPlaneLoopbackCallbackReturnsToProductOrigin(t *testing.T) {
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusNoContent) })
	handler, err := controlPlaneLoopbackBridge(next, "http://localhost:8092", "https://pilot.gog-marketing.localhost:1355")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8092/oauth/google/callback?state=state&code=code", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://pilot.gog-marketing.localhost:1355/oauth/google/callback?state=state&code=code" || calls != 0 {
		t.Fatal("callback was not bridged safely")
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("callback privacy headers missing")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://pilot.gog-marketing.localhost:1355/oauth/google/callback?state=state&code=code", nil))
	if w.Code != http.StatusNoContent || calls != 1 {
		t.Fatal("named-origin callback did not reach existing auth handler")
	}
}

func TestControlPlaneLoopbackBridgeRejectsNonLocalOrigins(t *testing.T) {
	for _, pair := range [][2]string{
		{"https://localhost:8092", "https://pilot.localhost"},
		{"http://example.com", "https://pilot.localhost"},
		{"http://localhost:8092", "https://example.com"},
		{"http://localhost:8092", "https://pilot.localhost/other"},
		{"http://localhost:8092", "https://pilot.localhost?secret=value"},
	} {
		if _, err := controlPlaneLoopbackBridge(http.NotFoundHandler(), pair[0], pair[1]); err == nil {
			t.Fatal("unsafe origin accepted", pair)
		}
	}
}
