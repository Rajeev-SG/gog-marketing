package runner

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	capabilityIssuer    = "gog-marketing-worker"
	capabilitySubject   = "gog-marketing-worker"
	capabilityAudience  = "gog-marketing-runner"
	capabilityAlgorithm = "HS256"

	// capabilityMaxWindow is the maximum signed lifetime in seconds.
	capabilityMaxWindow = 60
)

// CapabilityError distinguishes invocation-token rejections from request
// validation and engine failures. Codes are stable wire values; messages are
// always generic so token contents never leak.
type CapabilityError struct {
	Code       string
	HTTPStatus int
}

func (e *CapabilityError) Error() string { return e.Code }

func capabilityReject(code string, status int) error {
	return &CapabilityError{Code: code, HTTPStatus: status}
}

type capabilityHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type capabilityClaims struct {
	Issuer        string `json:"iss"`
	Subject       string `json:"sub"`
	Audience      string `json:"aud"`
	IssuedAt      any    `json:"iat"`
	ExpiresAt     any    `json:"exp"`
	TenantID      string `json:"tenant_id"`
	ConnectionID  string `json:"connection_id"`
	Operation     string `json:"operation"`
	RequestSHA256 string `json:"request_sha256"`
}

// verifyCapability checks the compact HS256 invocation token against the raw
// request body bytes. It validates the exact algorithm, issuer, subject,
// audience, finite <=60s time window, and the SHA-256 body digest signed by
// the worker. The secret is dedicated signing material, never a Google
// credential.
func verifyCapability(secret, token string, body []byte, now time.Time) (capabilityClaims, error) {
	if len(secret) < 32 {
		return capabilityClaims{}, capabilityReject("server_config", 500)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	headerBytes, err := decodeSegment(parts[0])
	if err != nil {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	var header capabilityHeader
	if jsonErr := json.Unmarshal(headerBytes, &header); jsonErr != nil || header.Algorithm != capabilityAlgorithm {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	payloadBytes, payloadErr := decodeSegment(parts[1])
	if payloadErr != nil {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	signature, sigErr := decodeSegment(parts[2])
	if sigErr != nil {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))

	if !hmac.Equal(signature, mac.Sum(nil)) {
		return capabilityClaims{}, capabilityReject("bad_signature", 401)
	}

	claims, claimsErr := decodeClaims(payloadBytes)
	if claimsErr != nil {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	if claims.Issuer != capabilityIssuer || claims.Subject != capabilitySubject ||
		claims.Audience != capabilityAudience {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	issuedAt, iatErr := claimSeconds(claims.IssuedAt)
	expiresAt, expErr := claimSeconds(claims.ExpiresAt)

	if iatErr != nil || expErr != nil {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	if expiresAt <= issuedAt || expiresAt-issuedAt > capabilityMaxWindow {
		return capabilityClaims{}, capabilityReject("token_invalid", 401)
	}

	nowSeconds := now.Unix()

	if nowSeconds < issuedAt {
		return capabilityClaims{}, capabilityReject("token_not_yet_valid", 401)
	}

	if nowSeconds >= expiresAt {
		return capabilityClaims{}, capabilityReject("token_expired", 401)
	}

	digest := sha256.Sum256(body)
	expected := hex.EncodeToString(digest[:])

	if !hmac.Equal([]byte(claims.RequestSHA256), []byte(expected)) {
		return capabilityClaims{}, capabilityReject("request_digest_mismatch", 401)
	}

	return claims, nil
}

func decodeClaims(payload []byte) (capabilityClaims, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))

	var claims capabilityClaims
	if err := decoder.Decode(&claims); err != nil {
		return capabilityClaims{}, fmt.Errorf("decode capability claims: %w", err)
	}

	if err := requireEOF(decoder); err != nil {
		return capabilityClaims{}, fmt.Errorf("capability claims trailing data: %w", err)
	}

	if claims.TenantID == "" || claims.ConnectionID == "" || claims.Operation == "" || claims.RequestSHA256 == "" {
		return capabilityClaims{}, errMissingCapabilityClaims
	}

	return claims, nil
}

func decodeSegment(segment string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return nil, fmt.Errorf("decode token segment: %w", err)
	}

	return data, nil
}

func claimSeconds(raw any) (int64, error) {
	switch value := raw.(type) {
	case float64:
		if value != float64(int64(value)) {
			return 0, errNonIntegerTimeClaim
		}

		return int64(value), nil
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse time claim: %w", err)
		}

		return parsed, nil
	default:
		return 0, errUnsupportedTimeClaim
	}
}
