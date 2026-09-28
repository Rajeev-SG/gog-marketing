package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthStartInput struct {
	State        string
	CodeVerifier string
	Nonce        string
	RedirectURI  string
	Scopes       []string
}

type OAuthProvider interface {
	AuthorizationURL(input OAuthStartInput) string
	Exchange(ctx context.Context, input OAuthStartInput, code string) (OAuthToken, error)
	Refresh(ctx context.Context, token OAuthToken) (OAuthToken, error)
}

type GoogleOAuthProvider struct {
	Config     oauth2.Config
	HTTPClient *http.Client
	Endpoint   oauth2.Endpoint
}

func NewGoogleOAuthProvider(clientID, clientSecret, redirectURI string) *GoogleOAuthProvider {
	endpoint := google.Endpoint

	return &GoogleOAuthProvider{
		Config: oauth2.Config{
			ClientID:     strings.TrimSpace(clientID),
			ClientSecret: strings.TrimSpace(clientSecret),
			RedirectURL:  strings.TrimSpace(redirectURI),
			Endpoint:     endpoint,
		},
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *GoogleOAuthProvider) AuthorizationURL(input OAuthStartInput) string {
	cfg := p.Config
	cfg.RedirectURL = input.RedirectURI
	cfg.Scopes = input.Scopes

	return cfg.AuthCodeURL(input.State,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("nonce", input.Nonce),
		oauth2.S256ChallengeOption(input.CodeVerifier),
	)
}

func (p *GoogleOAuthProvider) Exchange(ctx context.Context, input OAuthStartInput, code string) (OAuthToken, error) {
	cfg := p.Config
	cfg.RedirectURL = input.RedirectURI
	cfg.Scopes = input.Scopes

	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(input.CodeVerifier))
	if err != nil {
		return OAuthToken{}, fmt.Errorf("exchange Google authorization code: %w", err)
	}
	out := tokenFromOAuth2(token)

	identity, err := p.identity(ctx, token)
	if err != nil {
		return OAuthToken{}, wrapControlPlaneError(err)
	}
	out.Subject = identity.Subject
	out.Email = normalizeEmail(identity.Email)

	out.GrantedScopes = splitScopes(token.Extra("scope"))
	if input.Nonce != "" {
		if nonce := idTokenNonce(token.Extra("id_token")); nonce != "" && nonce != input.Nonce {
			return OAuthToken{}, ErrOIDCNonceMismatch
		}
	}

	return out, nil
}

func (p *GoogleOAuthProvider) Refresh(ctx context.Context, token OAuthToken) (OAuthToken, error) {
	cfg := p.Config
	source := cfg.TokenSource(ctx, &oauth2.Token{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	})

	refreshed, err := source.Token()
	if err != nil {
		return OAuthToken{}, fmt.Errorf("refresh Google token: %w", err)
	}

	if strings.TrimSpace(refreshed.RefreshToken) == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	out := tokenFromOAuth2(refreshed)
	out.Subject = token.Subject
	out.Email = token.Email

	out.GrantedScopes = splitScopes(refreshed.Extra("scope"))
	if out.GrantedScopes == nil {
		out.GrantedScopes = token.GrantedScopes
	}

	return out, nil
}

type googleIdentity struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
}

func (p *GoogleOAuthProvider) identity(ctx context.Context, token *oauth2.Token) (googleIdentity, error) {
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return googleIdentity{}, wrapControlPlaneError(err)
	}

	token.SetAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return googleIdentity{}, fmt.Errorf("read Google identity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleIdentity{}, fmt.Errorf("%w: %d", ErrGoogleIdentityHTTP, resp.StatusCode)
	}

	var identity googleIdentity
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return googleIdentity{}, wrapControlPlaneError(err)
	}

	if identity.Subject == "" {
		return googleIdentity{}, ErrGoogleIdentitySubject
	}

	return identity, nil
}

func tokenFromOAuth2(token *oauth2.Token) OAuthToken {
	return OAuthToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	}
}

func splitScopes(value any) []string {
	raw, _ := value.(string)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && !seen[part] {
			seen[part] = true
			out = append(out, part)
		}
	}

	return out
}

func idTokenNonce(value any) string {
	raw, _ := value.(string)

	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Nonce string `json:"nonce"`
	}
	_ = json.Unmarshal(payload, &claims)

	return claims.Nonce
}

type storedOAuthToken struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token"`
	TokenType     string    `json:"token_type,omitempty"`
	Expiry        time.Time `json:"expiry,omitempty"`
	Subject       string    `json:"subject,omitempty"`
	Email         string    `json:"email,omitempty"`
	GrantedScopes []string  `json:"granted_scopes,omitempty"`
}

func marshalToken(token OAuthToken) ([]byte, error) {
	//nolint:gosec // This JSON is encrypted before persistence and never leaves the SecretStore.
	raw, marshalErr := json.Marshal(storedOAuthToken(token))
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal token payload: %w", marshalErr)
	}

	return raw, nil
}

func unmarshalToken(raw []byte) (OAuthToken, error) {
	var stored storedOAuthToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return OAuthToken{}, wrapControlPlaneError(err)
	}

	return OAuthToken(stored), nil
}

func (p *GoogleOAuthProvider) Revoke(ctx context.Context, token OAuthToken) error {
	value := strings.TrimSpace(token.RefreshToken)
	if value == "" {
		value = strings.TrimSpace(token.AccessToken)
	}

	if value == "" {
		return nil
	}

	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/revoke", strings.NewReader("token="+url.QueryEscape(value)))
	if err != nil {
		return wrapControlPlaneError(err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("revoke Google token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("%w: %d", ErrGoogleRevokeHTTP, resp.StatusCode)
	}

	return nil
}
