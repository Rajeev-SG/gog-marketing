package cmd

import (
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

func mcpMarketingTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpAnalyticsAccountsTool(),
		mcpAnalyticsReportTool(),
		mcpAnalyticsPropertiesListTool(),
		mcpAnalyticsPropertiesGetTool(),
		mcpAnalyticsDataStreamsListTool(),
		mcpTagManagerAccountsTool(),
		mcpTagManagerContainersTool(),
		mcpTagManagerWorkspacesTool(),
		mcpTagManagerResourceListTool("tagmanager_tags_list", "tags"),
		mcpTagManagerResourceListTool("tagmanager_triggers_list", "triggers"),
		mcpTagManagerResourceListTool("tagmanager_variables_list", "variables"),
		mcpTagManagerVersionsListTool(),
		mcpTagManagerPublishTool(),
		mcpGoogleAdsCustomersTool(),
		mcpGoogleAdsQueryTool(),
		mcpSearchConsoleSitesTool(),
		mcpSearchConsoleQueryTool(),
		mcpSearchConsoleSitemapsTool(),
		mcpSearchConsoleSitemapSubmitTool(),
		mcpSearchConsoleSitemapDeleteTool(),
		mcpSearchConsoleInspectTool(),
		mcpBigQueryDatasetsTool(),
		mcpBigQueryTablesTool(),
		mcpBigQueryTableGetTool(),
		mcpBigQueryTableSchemaTool(),
		mcpBigQueryTableRowsTool(),
		mcpBigQueryDryRunTool(),
		mcpBigQueryQueryTool(),
	}
}

func mcpAnalyticsAccountsTool() mcpToolSpec {
	return mcpToolSpec{Name: "analytics_accounts", Service: "analytics", Risk: mcpRiskRead, Description: "List GA4 account summaries.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.Description("Maximum account summaries"), mcp.DefaultNumber(50), mcp.Min(1), mcp.Max(200))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		return []string{"analytics", "accounts", "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 50), 1, 200))}, nil
	}}
}

func mcpAnalyticsReportTool() mcpToolSpec {
	return mcpToolSpec{Name: "analytics_report", Service: "analytics", Risk: mcpRiskRead, Description: "Run a GA4 Data API report.", Options: []mcp.ToolOption{mcp.WithString("property", mcp.Required()), mcp.WithString("dimensions", mcp.DefaultString("date")), mcp.WithString("metrics", mcp.DefaultString("activeUsers")), mcp.WithString("from", mcp.DefaultString("7daysAgo")), mcp.WithString("to", mcp.DefaultString("today")), mcp.WithInteger("max", mcp.DefaultNumber(100), mcp.Min(1), mcp.Max(250000))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		property, err := requireMCPString(req, "property")
		if err != nil {
			return nil, err
		}
		return []string{"analytics", "report", property, "--dimensions", req.GetString("dimensions", "date"), "--metrics", req.GetString("metrics", "activeUsers"), "--from", req.GetString("from", "7daysAgo"), "--to", req.GetString("to", "today"), "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 100), 1, 250000))}, nil
	}}
}

func mcpAnalyticsPropertiesListTool() mcpToolSpec {
	return mcpToolSpec{Name: "analytics_properties_list", Service: "analytics", Risk: mcpRiskRead, Description: "List GA4 properties.", Options: []mcp.ToolOption{mcp.WithInteger("max", mcp.DefaultNumber(50), mcp.Min(1), mcp.Max(200))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		return []string{"analytics", "properties", "list", "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 50), 1, 200))}, nil
	}}
}

func mcpAnalyticsPropertiesGetTool() mcpToolSpec {
	return mcpToolSpec{Name: "analytics_properties_get", Service: "analytics", Risk: mcpRiskRead, Description: "Get one GA4 property.", Options: []mcp.ToolOption{mcp.WithString("property", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		property, err := requireMCPString(req, "property")
		if err != nil {
			return nil, err
		}
		return []string{"analytics", "properties", "get", property}, nil
	}}
}

func mcpAnalyticsDataStreamsListTool() mcpToolSpec {
	return mcpToolSpec{Name: "analytics_datastreams_list", Service: "analytics", Risk: mcpRiskRead, Description: "List GA4 data streams.", Options: []mcp.ToolOption{mcp.WithString("property", mcp.Required()), mcp.WithInteger("max", mcp.DefaultNumber(50), mcp.Min(1), mcp.Max(200))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		property, err := requireMCPString(req, "property")
		if err != nil {
			return nil, err
		}
		return []string{"analytics", "datastreams", "list", property, "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 50), 1, 200))}, nil
	}}
}

func mcpTagManagerAccountsTool() mcpToolSpec {
	return mcpToolSpec{Name: "tagmanager_accounts_list", Service: "tagmanager", Risk: mcpRiskRead, Description: "List GTM accounts.", BuildArgs: func(mcp.CallToolRequest) ([]string, error) { return []string{"tagmanager", "accounts", "list"}, nil }}
}

func mcpTagManagerContainersTool() mcpToolSpec {
	return mcpToolSpec{Name: "tagmanager_containers_list", Service: "tagmanager", Risk: mcpRiskRead, Description: "List GTM containers for an account.", Options: []mcp.ToolOption{mcp.WithString("account", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		account, err := requireMCPString(req, "account")
		if err != nil {
			return nil, err
		}
		return []string{"tagmanager", "containers", "list", account}, nil
	}}
}

func mcpTagManagerWorkspacesTool() mcpToolSpec {
	return mcpToolSpec{Name: "tagmanager_workspaces_list", Service: "tagmanager", Risk: mcpRiskRead, Description: "List GTM workspaces.", Options: []mcp.ToolOption{mcp.WithString("account", mcp.Required()), mcp.WithString("container", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		account, err := requireMCPString(req, "account")
		if err != nil {
			return nil, err
		}
		container, err := requireMCPString(req, "container")
		if err != nil {
			return nil, err
		}
		return []string{"tagmanager", "workspaces", "list", account, container}, nil
	}}
}

func mcpTagManagerResourceListTool(name, kind string) mcpToolSpec {
	return mcpToolSpec{Name: name, Service: "tagmanager", Risk: mcpRiskRead, Description: "List GTM " + kind + ".", Options: []mcp.ToolOption{mcp.WithString("account", mcp.Required()), mcp.WithString("container", mcp.Required()), mcp.WithString("workspace", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		account, err := requireMCPString(req, "account")
		if err != nil {
			return nil, err
		}
		container, err := requireMCPString(req, "container")
		if err != nil {
			return nil, err
		}
		workspace, err := requireMCPString(req, "workspace")
		if err != nil {
			return nil, err
		}
		return []string{"tagmanager", kind, "list", account, container, workspace}, nil
	}}
}

func mcpTagManagerVersionsListTool() mcpToolSpec {
	return mcpToolSpec{Name: "tagmanager_versions_list", Service: "tagmanager", Risk: mcpRiskRead, Description: "List GTM container version headers.", Options: []mcp.ToolOption{mcp.WithString("account", mcp.Required()), mcp.WithString("container", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		account, err := requireMCPString(req, "account")
		if err != nil {
			return nil, err
		}
		container, err := requireMCPString(req, "container")
		if err != nil {
			return nil, err
		}
		return []string{"tagmanager", "versions", "list", account, container}, nil
	}}
}

func mcpTagManagerPublishTool() mcpToolSpec {
	return mcpToolSpec{Name: "tagmanager_versions_publish", Service: "tagmanager", Risk: mcpRiskWrite, Description: "Publish a GTM container version. Requires --allow-write.", Options: []mcp.ToolOption{mcp.WithString("account", mcp.Required()), mcp.WithString("container", mcp.Required()), mcp.WithString("version", mcp.Required()), mcp.WithString("fingerprint")}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		account, err := requireMCPString(req, "account")
		if err != nil {
			return nil, err
		}
		container, err := requireMCPString(req, "container")
		if err != nil {
			return nil, err
		}
		version, err := requireMCPString(req, "version")
		if err != nil {
			return nil, err
		}
		args := []string{"tagmanager", "versions", "publish", account, container, version}
		if fingerprint := strings.TrimSpace(req.GetString("fingerprint", "")); fingerprint != "" {
			args = append(args, "--fingerprint", fingerprint)
		}
		return args, nil
	}}
}

func mcpGoogleAdsCustomersTool() mcpToolSpec {
	return mcpToolSpec{Name: "googleads_customers_list", Service: "googleads", Risk: mcpRiskRead, Description: "List accessible Google Ads customers.", BuildArgs: func(mcp.CallToolRequest) ([]string, error) { return []string{"googleads", "customers", "list"}, nil }}
}

func mcpGoogleAdsQueryTool() mcpToolSpec {
	return mcpToolSpec{Name: "googleads_query", Service: "googleads", Risk: mcpRiskRead, Description: "Run a bounded Google Ads GAQL search.", Options: []mcp.ToolOption{mcp.WithString("customer_id", mcp.Required()), mcp.WithString("query", mcp.Required()), mcp.WithInteger("page_size", mcp.DefaultNumber(100), mcp.Min(1), mcp.Max(10000)), mcp.WithBoolean("all", mcp.DefaultBool(false))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		customer, err := requireMCPString(req, "customer_id")
		if err != nil {
			return nil, err
		}
		query, err := requireMCPString(req, "query")
		if err != nil {
			return nil, err
		}
		args := []string{"googleads", "query", customer, "--gaql", query, "--page-size", strconv.Itoa(clampMCPInt(req.GetInt("page_size", 100), 1, 10000))}
		if req.GetBool("all", false) {
			args = append(args, "--all")
		}
		return args, nil
	}}
}

func mcpSearchConsoleSitesTool() mcpToolSpec {
	return mcpToolSpec{Name: "searchconsole_sites_list", Service: "searchconsole", Risk: mcpRiskRead, Description: "List Search Console sites.", BuildArgs: func(mcp.CallToolRequest) ([]string, error) { return []string{"searchconsole", "sites", "list"}, nil }}
}

func mcpSearchConsoleQueryTool() mcpToolSpec {
	return mcpToolSpec{Name: "searchconsole_query", Service: "searchconsole", Risk: mcpRiskRead, Description: "Run a Search Console Search Analytics query.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required()), mcp.WithString("from", mcp.Required()), mcp.WithString("to", mcp.Required()), mcp.WithString("dimensions", mcp.DefaultString("QUERY")), mcp.WithInteger("max", mcp.DefaultNumber(1000), mcp.Min(1), mcp.Max(25000))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		site, err := requireMCPString(req, "site_url")
		if err != nil {
			return nil, err
		}
		from, err := requireMCPString(req, "from")
		if err != nil {
			return nil, err
		}
		to, err := requireMCPString(req, "to")
		if err != nil {
			return nil, err
		}
		return []string{"searchconsole", "query", site, "--from", from, "--to", to, "--dimensions", req.GetString("dimensions", "QUERY"), "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 1000), 1, 25000))}, nil
	}}
}

func mcpSearchConsoleSitemapsTool() mcpToolSpec {
	return mcpToolSpec{Name: "searchconsole_sitemaps_list", Service: "searchconsole", Risk: mcpRiskRead, Description: "List Search Console sitemaps.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		site, err := requireMCPString(req, "site_url")
		if err != nil {
			return nil, err
		}
		return []string{"searchconsole", "sitemaps", "list", site}, nil
	}}
}

func mcpSearchConsoleSitemapSubmitTool() mcpToolSpec {
	return mcpToolSpec{Name: "searchconsole_sitemaps_submit", Service: "searchconsole", Risk: mcpRiskWrite, Description: "Submit a Search Console sitemap. Requires --allow-write.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required()), mcp.WithString("feed_path", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		site, err := requireMCPString(req, "site_url")
		if err != nil {
			return nil, err
		}
		feed, err := requireMCPString(req, "feed_path")
		if err != nil {
			return nil, err
		}
		return []string{"searchconsole", "sitemaps", "submit", site, feed}, nil
	}}
}

func mcpSearchConsoleSitemapDeleteTool() mcpToolSpec {
	return mcpToolSpec{Name: "searchconsole_sitemaps_delete", Service: "searchconsole", Risk: mcpRiskWrite, Description: "Delete a Search Console sitemap. Requires --allow-write and destructive confirmation.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required()), mcp.WithString("feed_path", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		site, err := requireMCPString(req, "site_url")
		if err != nil {
			return nil, err
		}
		feed, err := requireMCPString(req, "feed_path")
		if err != nil {
			return nil, err
		}
		return []string{"searchconsole", "sitemaps", "delete", site, feed}, nil
	}}
}

func mcpSearchConsoleInspectTool() mcpToolSpec {
	return mcpToolSpec{Name: "searchconsole_inspect", Service: "searchconsole", Risk: mcpRiskRead, Description: "Inspect Search Console URL indexing.", Options: []mcp.ToolOption{mcp.WithString("site_url", mcp.Required()), mcp.WithString("url", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		site, err := requireMCPString(req, "site_url")
		if err != nil {
			return nil, err
		}
		url, err := requireMCPString(req, "url")
		if err != nil {
			return nil, err
		}
		return []string{"searchconsole", "inspect", site, url}, nil
	}}
}

func mcpBigQueryProjectArgs(req mcp.CallToolRequest) ([]string, error) {
	project, err := requireMCPString(req, "project")
	if err != nil {
		return nil, err
	}
	return []string{"--billing-project", project}, nil
}

func mcpBigQueryDatasetsTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_datasets_list", Service: "bigquery", Risk: mcpRiskRead, Description: "List BigQuery datasets in an explicit execution project.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		return append([]string{"bigquery", "datasets", "list"}, project...), nil
	}}
}

func mcpBigQueryTablesTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_tables_list", Service: "bigquery", Risk: mcpRiskRead, Description: "List BigQuery tables.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("dataset", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		dataset, err := requireMCPString(req, "dataset")
		if err != nil {
			return nil, err
		}
		return append([]string{"bigquery", "tables", "list", dataset}, project...), nil
	}}
}

func mcpBigQueryTableGetTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_table_get", Service: "bigquery", Risk: mcpRiskRead, Description: "Get BigQuery table metadata.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("dataset", mcp.Required()), mcp.WithString("table", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		dataset, err := requireMCPString(req, "dataset")
		if err != nil {
			return nil, err
		}
		table, err := requireMCPString(req, "table")
		if err != nil {
			return nil, err
		}
		return append([]string{"bigquery", "tables", "get", dataset, table}, project...), nil
	}}
}

func mcpBigQueryTableSchemaTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_table_schema", Service: "bigquery", Risk: mcpRiskRead, Description: "Get BigQuery table schema.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("dataset", mcp.Required()), mcp.WithString("table", mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		dataset, err := requireMCPString(req, "dataset")
		if err != nil {
			return nil, err
		}
		table, err := requireMCPString(req, "table")
		if err != nil {
			return nil, err
		}
		return append([]string{"bigquery", "tables", "schema", dataset, table}, project...), nil
	}}
}

func mcpBigQueryTableRowsTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_table_rows", Service: "bigquery", Risk: mcpRiskRead, Description: "Read a bounded number of BigQuery table rows.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("dataset", mcp.Required()), mcp.WithString("table", mcp.Required()), mcp.WithInteger("max", mcp.DefaultNumber(100), mcp.Min(1), mcp.Max(10000))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		dataset, err := requireMCPString(req, "dataset")
		if err != nil {
			return nil, err
		}
		table, err := requireMCPString(req, "table")
		if err != nil {
			return nil, err
		}
		return append([]string{"bigquery", "tables", "rows", dataset, table, "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 100), 1, 10000))}, project...), nil
	}}
}

func mcpBigQueryDryRunTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_query_dry_run", Service: "bigquery", Risk: mcpRiskRead, Description: "Validate BigQuery SQL and report estimated bytes without executing it.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("sql", mcp.Required()), mcp.WithInteger("max_bytes_billed", mcp.Description("Hard byte-billing cap applied to the query"), mcp.DefaultNumber(1073741824), mcp.Min(1), mcp.Max(2000000000))}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		sql, err := requireMCPString(req, "sql")
		if err != nil {
			return nil, err
		}
		maxBytesBilled := strconv.Itoa(clampMCPInt(req.GetInt("max_bytes_billed", 1073741824), 1, 2000000000))
		return append([]string{"bigquery", "query", "--sql", sql, "--dry-run", "--max-bytes-billed", maxBytesBilled}, project...), nil
	}}
}

func mcpBigQueryQueryTool() mcpToolSpec {
	return mcpToolSpec{Name: "bigquery_query", Service: "bigquery", Risk: mcpRiskWrite, Description: "Execute arbitrary BigQuery SQL after explicit cost acknowledgement. Requires --allow-write and an explicit execution project.", Options: []mcp.ToolOption{mcp.WithString("project", mcp.Required()), mcp.WithString("sql", mcp.Required()), mcp.WithInteger("max", mcp.DefaultNumber(100), mcp.Min(1), mcp.Max(10000)), mcp.WithInteger("max_bytes_billed", mcp.Description("Hard byte-billing cap applied to the query"), mcp.DefaultNumber(1073741824), mcp.Min(1), mcp.Max(2000000000)), mcp.WithBoolean("acknowledge_cost", mcp.Description("Confirm execution after reviewing bigquery_query_dry_run"), mcp.Required())}, BuildArgs: func(req mcp.CallToolRequest) ([]string, error) {
		project, err := mcpBigQueryProjectArgs(req)
		if err != nil {
			return nil, err
		}
		sql, err := requireMCPString(req, "sql")
		if err != nil {
			return nil, err
		}
		if !req.GetBool("acknowledge_cost", false) {
			return nil, usage("bigquery_query requires acknowledge_cost=true after a dry run")
		}
		maxBytesBilled := strconv.Itoa(clampMCPInt(req.GetInt("max_bytes_billed", 1073741824), 1, 2000000000))
		args := make([]string, 0, 9+len(project))
		args = append(args, "bigquery", "query", "--sql", sql, "--max", strconv.Itoa(clampMCPInt(req.GetInt("max", 100), 1, 10000)), "--max-bytes-billed", maxBytesBilled, "--acknowledge-cost")
		return append(args, project...), nil
	}}
}
