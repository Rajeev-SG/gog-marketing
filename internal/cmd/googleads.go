package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/openclaw/gogcli/internal/googleads"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type GoogleAdsCmd struct {
	Customers GoogleAdsCustomersCmd `cmd:"" help:"List accessible Google Ads customers"`
	Query     GoogleAdsQueryCmd     `cmd:"" help:"Run a read-only GAQL search"`
}

type GoogleAdsAuthFlags struct {
	DeveloperToken  string `name:"developer-token" env:"GOG_GOOGLE_ADS_DEVELOPER_TOKEN" help:"Google Ads developer token (prefer a secret-managed environment variable)"`
	LoginCustomerID string `name:"login-customer-id" aliases:"manager-id" env:"GOG_GOOGLE_ADS_LOGIN_CUSTOMER_ID" help:"Optional manager customer ID for the login-customer-id header"`
	APIVersion      string `name:"api-version" env:"GOG_GOOGLE_ADS_API_VERSION" default:"v25" help:"Official Google Ads REST major version"`
	BaseURL         string `name:"googleads-base-url" hidden:"" env:"GOG_GOOGLE_ADS_BASE_URL"`
}

type GoogleAdsCustomersCmd struct {
	GoogleAdsAuthFlags `embed:""`
	FailEmpty          bool `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no customers"`
}

func (c *GoogleAdsCustomersCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := dryRunExit(ctx, flags, "googleads.customers.list", c.plan()); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := c.client(ctx, account)
	if err != nil {
		return err
	}
	names, err := client.ListAccessibleCustomers(ctx)
	if err != nil {
		return err
	}
	customers := make([]map[string]any, 0, len(names))
	for _, name := range names {
		id := googleAdsCustomerID(name)
		customers = append(customers, map[string]any{"resource_name": name, "customer_id": id})
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"customers": customers}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, len(customers) == 0)
	}
	if len(customers) == 0 {
		u.Err().Println("No accessible Google Ads customers")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CUSTOMER_ID\tRESOURCE_NAME")
	for _, customer := range customers {
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(customer["customer_id"].(string)), sanitizeTab(customer["resource_name"].(string)))
	}
	return nil
}

func (c *GoogleAdsCustomersCmd) plan() map[string]any {
	return map[string]any{"api_version": c.APIVersion, "login_customer_id": normalizeOptionalCustomerID(c.LoginCustomerID)}
}

func (c *GoogleAdsCustomersCmd) client(ctx context.Context, account string) (*googleads.Client, error) {
	return newGoogleAdsClient(ctx, account, c.GoogleAdsAuthFlags)
}

type GoogleAdsQueryCmd struct {
	GoogleAdsAuthFlags `embed:""`
	CustomerID         string `arg:"" name:"customer-id" help:"Google Ads customer ID; hyphens accepted"`
	GAQL               string `name:"gaql" help:"GAQL query text"`
	GAQLFile           string `name:"gaql-file" type:"path" help:"Read GAQL from a file"`
	PageSize           int    `name:"page-size" aliases:"max" help:"Maximum rows per page (1-10000)" default:"100"`
	PageToken          string `name:"page-token" aliases:"page" help:"Google Ads page token"`
	All                bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch every result page"`
	FailEmpty          bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *GoogleAdsQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	query, err := c.readQuery()
	if err != nil {
		return err
	}
	customerID, err := googleads.NormalizeCustomerID(c.CustomerID)
	if err != nil {
		return err
	}
	if c.PageSize < 1 || c.PageSize > 10000 {
		return usage("--page-size must be between 1 and 10000")
	}
	request := googleads.SearchRequest{Query: query, PageSize: int32(c.PageSize), PageToken: strings.TrimSpace(c.PageToken)}
	plan := map[string]any{
		"customer_id":       customerID,
		"query":             query,
		"page_size":         c.PageSize,
		"page_token":        request.PageToken,
		"all_pages":         c.All,
		"api_version":       c.APIVersion,
		"login_customer_id": normalizeOptionalCustomerID(c.LoginCustomerID),
	}
	if dryRunErr := dryRunExit(ctx, flags, "googleads.query", plan); dryRunErr != nil {
		return dryRunErr
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := c.client(ctx, account)
	if err != nil {
		return err
	}
	var rows []map[string]any
	nextPageToken := request.PageToken
	for {
		request.PageToken = nextPageToken
		response, searchErr := client.Search(ctx, customerID, request)
		if searchErr != nil {
			return searchErr
		}
		for _, raw := range response.Results {
			var row map[string]any
			if decodeErr := json.Unmarshal(raw, &row); decodeErr != nil {
				return fmt.Errorf("decode Google Ads row: %w", decodeErr)
			}
			rows = append(rows, row)
		}
		nextPageToken = googleads.FormatPageToken(response.NextPageToken)
		if !c.All || nextPageToken == "" {
			break
		}
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"customer_id":   customerID,
			"query":         query,
			"rows":          rows,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, len(rows) == 0)
	}
	if len(rows) == 0 {
		u.Err().Println("No Google Ads rows")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	return writeGoogleAdsRows(ctx, rows)
}

func (c *GoogleAdsQueryCmd) readQuery() (string, error) {
	query := strings.TrimSpace(c.GAQL)
	file := strings.TrimSpace(c.GAQLFile)
	if query != "" && file != "" {
		return "", usage("--gaql and --gaql-file are mutually exclusive")
	}
	if file != "" {
		raw, err := os.ReadFile(file) //nolint:gosec // user-provided GAQL file
		if err != nil {
			return "", fmt.Errorf("read --gaql-file: %w", err)
		}
		query = strings.TrimSpace(string(raw))
	}
	if query == "" {
		return "", usage("provide --gaql or --gaql-file")
	}
	return query, nil
}

func (c *GoogleAdsQueryCmd) client(ctx context.Context, account string) (*googleads.Client, error) {
	return newGoogleAdsClient(ctx, account, c.GoogleAdsAuthFlags)
}

func newGoogleAdsClient(ctx context.Context, account string, auth GoogleAdsAuthFlags) (*googleads.Client, error) {
	token := strings.TrimSpace(auth.DeveloperToken)
	if token == "" {
		token = strings.TrimSpace(firstNonEmpty(os.Getenv("GOG_GOOGLE_ADS_DEVELOPER_TOKEN"), os.Getenv("GOOGLE_ADS_DEVELOPER_TOKEN")))
	}
	if token == "" {
		return nil, googleads.ErrDeveloperTokenRequired
	}
	client, err := googleAdsHTTPClient(ctx, account)
	if err != nil {
		return nil, err
	}
	login := strings.TrimSpace(auth.LoginCustomerID)
	if login == "" {
		login = strings.TrimSpace(os.Getenv("GOG_GOOGLE_ADS_LOGIN_CUSTOMER_ID"))
	}
	return &googleads.Client{
		HTTP:            client,
		BaseURL:         strings.TrimSpace(auth.BaseURL),
		Version:         strings.TrimSpace(auth.APIVersion),
		DeveloperToken:  token,
		LoginCustomerID: login,
	}, nil
}

func googleAdsCustomerID(resourceName string) string {
	parts := strings.Split(strings.TrimSpace(resourceName), "/")
	if len(parts) == 0 {
		return ""
	}
	id := parts[len(parts)-1]
	normalized, err := googleads.NormalizeCustomerID(id)
	if err != nil {
		return id
	}
	return normalized
}

func normalizeOptionalCustomerID(raw string) string {
	normalized, err := googleads.NormalizeCustomerID(raw)
	if err != nil {
		return ""
	}
	return normalized
}

func writeGoogleAdsRows(ctx context.Context, rows []map[string]any) error {
	keys := googleAdsColumnKeys(rows)
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(keys, "\t"))
	for _, row := range rows {
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, sanitizeTab(formatGoogleAdsValue(row[key])))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
	return nil
}

func googleAdsColumnKeys(rows []map[string]any) []string {
	seen := map[string]bool{}
	var keys []string
	for _, row := range rows {
		for key := range row {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func formatGoogleAdsValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(raw)
	}
}

func failEmptyExitIf(enabled, empty bool) error {
	if enabled && empty {
		return failEmptyExit(true)
	}
	return nil
}
