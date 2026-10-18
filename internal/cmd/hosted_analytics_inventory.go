package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// hostedAnalyticsAccountSelector pins the GA4 properties-list tool to a
// canonical account resource; no other filter input is exposed to the model.
var errHostedAnalyticsInventoryInput = errors.New("invalid analytics inventory input")

var hostedAnalyticsAccountSelector = regexp.MustCompile(`^accounts/[0-9]+$`)

func hostedAnalyticsInventoryTools() []mcpToolSpec {
	pagination := func(req mcp.CallToolRequest) (int, error) {
		pageLimit := req.GetInt("max", 50)
		if pageLimit < 1 || pageLimit > 200 {
			return 0, fmt.Errorf("%w: max must be between 1 and 200", errHostedAnalyticsInventoryInput)
		}
		return pageLimit, nil
	}

	trailing := func(req mcp.CallToolRequest, pageFlag string) []string {
		var args []string
		if page := req.GetString("page", ""); page != "" {
			args = append(args, pageFlag+"="+page)
		}
		for _, key := range []string{"all", "fail_empty"} {
			if req.GetBool(key, false) {
				flag := key
				if key == "fail_empty" {
					flag = "fail-empty"
				}
				args = append(args, "--"+flag)
			}
		}
		return args
	}

	return []mcpToolSpec{
		{
			Name:        "analytics_accounts",
			Service:     "analytics",
			Risk:        mcpRiskRead,
			Description: "List GA4 account summaries in the authorised Google account via the native Analytics Admin API, under an account-level exact tool grant. No property asset grant is implied. No arbitrary Admin API filter, write or credential access is exposed.",
			Options: []mcp.ToolOption{
				mcp.WithInteger("max", mcp.Min(1), mcp.Max(200)),
				mcp.WithString("page"),
				mcp.WithBoolean("all"),
				mcp.WithBoolean("fail_empty"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				pageLimit, err := pagination(req)
				if err != nil {
					return nil, err
				}
				args := make([]string, 0, 8)
				args = append(args, "analytics", "accounts", "--max="+strconv.Itoa(pageLimit))
				return append(args, trailing(req, "--page")...), nil
			},
		},
		{
			Name:        "analytics_properties_list",
			Service:     "analytics",
			Risk:        mcpRiskRead,
			Description: "List GA4 properties under one explicitly granted account via the native Analytics Admin API, using a fixed parent:accounts/<id> filter. No arbitrary filter, ancestor or global account-summary fallback is exposed, and no property asset grant is implied.",
			Options: []mcp.ToolOption{
				mcp.WithString("account", mcp.Required()),
				mcp.WithInteger("max", mcp.Min(1), mcp.Max(200)),
				mcp.WithString("page"),
				mcp.WithBoolean("all"),
				mcp.WithBoolean("fail_empty"),
			},
			BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
				if _, exists := req.GetArguments()["filter"]; exists {
					return nil, fmt.Errorf("%w: filter is not configurable", errHostedAnalyticsInventoryInput)
				}
				account, err := requireMCPString(req, "account")
				if err != nil {
					return nil, err
				}
				account = strings.TrimSpace(account)
				if !hostedAnalyticsAccountSelector.MatchString(account) {
					return nil, fmt.Errorf("%w: invalid account selector", errHostedAnalyticsInventoryInput)
				}
				pageLimit, err := pagination(req)
				if err != nil {
					return nil, err
				}
				args := make([]string, 0, 9)
				args = append(args,
					"analytics", "properties", "list",
					"--page-size="+strconv.Itoa(pageLimit),
					"--filter=parent:"+account,
				)
				return append(args, trailing(req, "--page-token")...), nil
			},
		},
	}
}
