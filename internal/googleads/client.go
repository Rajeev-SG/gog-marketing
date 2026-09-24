package googleads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const DefaultVersion = "v25"

var (
	ErrDeveloperTokenRequired = errors.New("google ads developer token is required")
	ErrCustomerIDRequired     = errors.New("google ads customer ID is required")
	ErrInvalidCustomerID      = errors.New("invalid google ads customer ID")
	ErrInvalidDeveloperToken  = errors.New("invalid google ads developer token")
	ErrGAQLRequired           = errors.New("GAQL query is required")
	ErrInvalidPageSize        = errors.New("page size must be between 1 and 10000")
	ErrHTTPClientRequired     = errors.New("google ads HTTP client is required")
	ErrEmptyResponse          = errors.New("empty response")
)

type Client struct {
	HTTP            *http.Client
	BaseURL         string
	Version         string
	DeveloperToken  string
	LoginCustomerID string
}

type SearchRequest struct {
	Query     string
	PageSize  int32
	PageToken string
}

func (r SearchRequest) MarshalJSON() ([]byte, error) {
	payload := map[string]any{"query": r.Query}
	if r.PageSize > 0 {
		payload["pageSize"] = r.PageSize
	}

	if r.PageToken != "" {
		payload["pageToken"] = r.PageToken
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal google ads search request: %w", err)
	}

	return raw, nil
}

type SearchResponse struct {
	Results       []json.RawMessage
	NextPageToken string
	FieldMask     string
	RequestID     string `json:"-"`
	CustomerID    string `json:"-"`
	Query         string `json:"-"`
}

func (r *SearchResponse) UnmarshalJSON(raw []byte) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode google ads search response object: %w", err)
	}

	if err := decodeWireValue(payload["results"], &r.Results); err != nil {
		return err
	}

	if err := decodeWireValue(payload["nextPageToken"], &r.NextPageToken); err != nil {
		return err
	}

	if err := decodeWireValue(payload["fieldMask"], &r.FieldMask); err != nil {
		return err
	}

	return nil
}

type APIError struct {
	Code      int
	Status    string
	Message   string
	RequestID string
}

func ValidateDeveloperToken(raw string) error {
	token := strings.TrimSpace(raw)
	if len(token) < 10 || len(token) > 64 {
		return ErrInvalidDeveloperToken
	}

	for _, r := range token {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}

		return ErrInvalidDeveloperToken
	}

	return nil
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("Google Ads API error (%d %s): %s [request-id %s]", e.Code, e.Status, e.Message, e.RequestID)
	}

	return fmt.Sprintf("Google Ads API error (%d %s): %s", e.Code, e.Status, e.Message)
}

func NormalizeCustomerID(raw string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}

		if r == '-' || r == ' ' {
			return -1
		}

		return r
	}, raw)
	if digits == "" {
		return "", ErrCustomerIDRequired
	}

	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("%w %q", ErrInvalidCustomerID, raw)
		}
	}

	if len(digits) < 3 || len(digits) > 20 {
		return "", fmt.Errorf("%w %q", ErrInvalidCustomerID, raw)
	}

	return digits, nil
}

func (c *Client) ListAccessibleCustomers(ctx context.Context) ([]string, error) {
	if validateErr := c.validate(); validateErr != nil {
		return nil, validateErr
	}

	body, requestID, err := c.do(ctx, http.MethodGet, c.path("customers:listAccessibleCustomers"), nil)
	if err != nil {
		return nil, err
	}

	var payload map[string]json.RawMessage
	if err := decodeJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("decode google ads accessible customers (request-id %s): %w", requestID, err)
	}

	var names []string
	if err := decodeWireValue(payload["resourceNames"], &names); err != nil {
		return nil, fmt.Errorf("decode google ads resource names (request-id %s): %w", requestID, err)
	}

	return names, nil
}

func (c *Client) Search(ctx context.Context, customerID string, request SearchRequest) (*SearchResponse, error) {
	normalized, err := NormalizeCustomerID(customerID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(request.Query) == "" {
		return nil, ErrGAQLRequired
	}

	if validateErr := c.validate(); validateErr != nil {
		return nil, validateErr
	}

	if request.PageSize < 0 || request.PageSize > 10000 {
		return nil, ErrInvalidPageSize
	}

	body, requestID, err := c.do(ctx, http.MethodPost, c.path("customers/"+normalized+"/googleAds:search"), request)
	if err != nil {
		return nil, err
	}

	response := &SearchResponse{RequestID: requestID, CustomerID: normalized, Query: request.Query}
	if err := decodeJSON(body, response); err != nil {
		return nil, fmt.Errorf("decode google ads search response (request-id %s): %w", requestID, err)
	}
	response.RequestID = requestID

	return response, nil
}

func (c *Client) validate() error {
	if c == nil || c.HTTP == nil {
		return ErrHTTPClientRequired
	}

	if strings.TrimSpace(c.DeveloperToken) == "" {
		return ErrDeveloperTokenRequired
	}

	return ValidateDeveloperToken(c.DeveloperToken)
}

func (c *Client) path(suffix string) string {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://googleads.googleapis.com"
	}

	version := strings.TrimSpace(c.Version)
	if version == "" {
		version = DefaultVersion
	}

	return base + "/" + url.PathEscape(version) + "/" + suffix
}

func (c *Client) do(ctx context.Context, method, endpoint string, payload any) ([]byte, string, error) {
	var reader io.Reader

	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, "", fmt.Errorf("marshal google ads request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, "", fmt.Errorf("create google ads request: %w", err)
	}

	request.Header.Set("developer-token", strings.TrimSpace(c.DeveloperToken))

	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	if login := normalizeOptionalCustomerID(c.LoginCustomerID); login != "" {
		request.Header.Set("login-customer-id", login)
	}

	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("send google ads request: %w", err)
	}

	defer func() { _ = response.Body.Close() }()
	requestID := strings.TrimSpace(response.Header.Get("x-request-id"))

	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, requestID, fmt.Errorf("read google ads response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, requestID, decodeAPIError(response.StatusCode, response.Status, requestID, body)
	}

	return body, requestID, nil
}

func normalizeOptionalCustomerID(raw string) string {
	normalized, err := NormalizeCustomerID(raw)
	if err != nil {
		return ""
	}

	return normalized
}

func decodeJSON(raw []byte, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ErrEmptyResponse
	}

	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode google ads JSON: %w", err)
	}

	return nil
}

func decodeWireValue(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode google ads field: %w", err)
	}

	return nil
}

func decodeAPIError(code int, status, requestID string, body []byte) error {
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(body, &payload)

	var errorPayload struct {
		Message string
		Status  string
	}
	if raw := payload["error"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &errorPayload)
	}

	message := strings.TrimSpace(errorPayload.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}

	if message == "" {
		message = "request failed"
	}

	statusLabel := strings.TrimSpace(errorPayload.Status)
	if statusLabel == "" {
		statusLabel = strings.TrimSpace(status)
	}

	return &APIError{Code: code, Status: statusLabel, Message: message, RequestID: requestID}
}

func FormatPageToken(token string) string {
	return strings.TrimSpace(token)
}
