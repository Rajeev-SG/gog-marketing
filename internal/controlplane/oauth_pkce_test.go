package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGoogleOAuthProviderUsesPKCEAndNonce(t *testing.T) {
	const verifier = "pkce-verifier-0123456789abcdef"
	const nonce = "nonce-0123456789abcdef"
	var receivedVerifier string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}

			receivedVerifier = r.Form.Get("code_verifier")
			if receivedVerifier != verifier {
				http.Error(w, "invalid verifier", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer",
				"id_token": jwtWithNonce(nonce),
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"sub":"subject","email":"owner@example.com"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "openidconnect.googleapis.com" {
			request = request.Clone(request.Context())
			request.URL.Scheme = "http"
			request.URL.Host = strings.TrimPrefix(server.URL, "http://")
			request.URL.Path = "/userinfo"
		}

		return server.Client().Transport.RoundTrip(request)
	})
	client := &http.Client{Transport: transport}
	provider := NewGoogleOAuthProvider("client", "secret", "http://example.test/oauth/google/callback")
	provider.Config.Endpoint = oauth2.Endpoint{TokenURL: server.URL + "/token"}
	provider.HTTPClient = client

	input := OAuthStartInput{
		State: "state", CodeVerifier: verifier, Nonce: nonce,
		RedirectURI: "http://example.test/oauth/google/callback",
		Scopes:      []string{"openid", "email"},
	}
	authURL := provider.AuthorizationURL(input)

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}

	query := parsed.Query()
	if query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL is missing PKCE: %s", authURL)
	}

	if query.Get("nonce") != nonce || query.Get("state") != input.State {
		t.Fatalf("authorization URL is missing state/nonce: %s", authURL)
	}

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	if _, err := provider.Exchange(ctx, input, "code"); err != nil {
		t.Fatalf("exchange with matching verifier failed: %v", err)
	}

	if receivedVerifier != verifier {
		t.Fatalf("token exchange did not send the PKCE verifier: %q", receivedVerifier)
	}

	tampered := input

	tampered.CodeVerifier = verifier + "-tampered"
	if _, err := provider.Exchange(ctx, tampered, "code"); err == nil {
		t.Fatal("exchange with tampered PKCE verifier succeeded")
	}

	wrongNonce := input

	wrongNonce.Nonce = "different-nonce"
	if _, err := provider.Exchange(ctx, wrongNonce, "code"); err == nil {
		t.Fatal("exchange with mismatched OIDC nonce succeeded")
	}
}

func jwtWithNonce(nonce string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]string{"nonce": nonce})

	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
