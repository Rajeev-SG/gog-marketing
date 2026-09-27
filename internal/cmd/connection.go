package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ConnectionCmd struct {
	Add    ConnectionAddCmd    `cmd:"" help:"Define a named multi-account connection"`
	List   ConnectionListCmd   `cmd:"" name:"list" aliases:"ls" help:"List stored connections"`
	Get    ConnectionGetCmd    `cmd:"" name:"get" help:"Show one stored connection"`
	Use    ConnectionUseCmd    `cmd:"" help:"Set the default connection for account routing"`
	Remove ConnectionRemoveCmd `cmd:"" name:"remove" aliases:"rm,delete" help:"Remove a stored connection"`
}

type ConnectionAddCmd struct {
	Name            string `arg:"" name:"name" help:"Connection name (lowercase letters, digits, '-', '_', '.')"`
	Account         string `arg:"" name:"account" help:"Account email or alias this connection routes to"`
	Client          string `name:"oauth-client" help:"OAuth client name holding the credentials (defaults to gog client resolution)"`
	Services        string `help:"Comma-separated services this connection is scoped for (e.g. analytics,tagmanager,bigquery)"`
	QuotaProject    string `name:"connection-quota-project" help:"Google Cloud project billed for API usage (X-Goog-User-Project)"`
	BigQueryProject string `name:"bigquery-project" help:"Explicit BigQuery execution/billing project"`
	Description     string `help:"Human-readable purpose of this connection"`
}

func (c *ConnectionAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	name, err := config.NormalizeConnectionName(c.Name)
	if err != nil {
		return usage(err.Error())
	}
	account, err := config.NormalizeConnectionAccount(c.Account)
	if err != nil {
		return usage(err.Error())
	}
	client, err := config.NormalizeClientNameOrDefault(c.Client)
	if err != nil {
		return usage(err.Error())
	}

	var services []string
	if strings.TrimSpace(c.Services) != "" {
		parsed, parseErr := parseAuthServices(c.Services)
		if parseErr != nil {
			return parseErr
		}
		services = make([]string, 0, len(parsed))
		for _, svc := range parsed {
			services = append(services, string(svc))
		}
	}
	for _, svc := range services {
		if svc == "bigquery" && strings.TrimSpace(c.BigQueryProject) == "" {
			return usage("connection includes bigquery service; --bigquery-project is required to keep execution/billing explicit")
		}
	}

	conn := config.Connection{
		Name:            name,
		Account:         account,
		Client:          client,
		Services:        services,
		QuotaProject:    strings.TrimSpace(c.QuotaProject),
		BigQueryProject: strings.TrimSpace(c.BigQueryProject),
		Description:     strings.TrimSpace(c.Description),
	}

	dryRunErr := dryRunExit(ctx, flags, "connection.add", map[string]any{"connection": conn})
	if dryRunErr != nil {
		return dryRunErr
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	if err := store.SetConnection(conn); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"connection": conn})
	}
	return writeResult(ctx, u,
		kv("connection", conn.Name),
		kv("account", conn.Account),
		kv("client", conn.Client),
		kv("services", strings.Join(conn.Services, ",")),
		kv("quota_project", conn.QuotaProject),
		kv("bigquery_project", conn.BigQueryProject),
	)
}

type ConnectionListCmd struct{}

func (c *ConnectionListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	connections, err := store.ListConnections()
	if err != nil {
		return err
	}
	defaultConn, err := store.DefaultConnectionName()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"connections": connections,
			"default":     defaultConn,
		})
	}
	if len(connections) == 0 {
		u.Err().Println("No connections; add one with: gog connection add <name> --account <email>")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME	ACCOUNT	CLIENT	SERVICES	QUOTA_PROJECT	BIGQUERY_PROJECT	DEFAULT")
	for _, conn := range connections {
		fmt.Fprintf(w, "%s	%s	%s	%s	%s	%s	%s\n",
			sanitizeTab(conn.Name),
			sanitizeTab(conn.Account),
			sanitizeTab(conn.Client),
			sanitizeTab(strings.Join(conn.Services, ",")),
			sanitizeTab(conn.QuotaProject),
			sanitizeTab(conn.BigQueryProject),
			boolMark(conn.Name == defaultConn),
		)
	}
	return nil
}

type ConnectionGetCmd struct {
	Name string `arg:"" name:"name" help:"Connection name"`
}

func (c *ConnectionGetCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	conn, ok, err := store.ResolveConnection(c.Name)
	if err != nil {
		return err
	}
	if !ok {
		return usage(fmt.Sprintf("connection %q not found", strings.TrimSpace(c.Name)))
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"connection": conn})
	}
	sort.Strings(conn.Services)
	return writeResult(ctx, u,
		kv("connection", conn.Name),
		kv("account", conn.Account),
		kv("client", conn.Client),
		kv("services", strings.Join(conn.Services, ",")),
		kv("quota_project", conn.QuotaProject),
		kv("bigquery_project", conn.BigQueryProject),
		kv("description", conn.Description),
	)
}

type ConnectionUseCmd struct {
	Name string `arg:"" name:"name" help:"Connection name"`
}

func (c *ConnectionUseCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	name := strings.TrimSpace(c.Name)
	if err := dryRunExit(ctx, flags, "connection.use", map[string]any{"connection": name}); err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	if err := store.SetDefaultConnection(name); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		normalized, err := config.NormalizeConnectionName(name)
		if err != nil {
			return err
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"default": normalized})
	}
	u.Out().Linef("default connection	%s", name)
	return nil
}

type ConnectionRemoveCmd struct {
	Name string `arg:"" name:"name" help:"Connection name"`
}

func (c *ConnectionRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	name := strings.TrimSpace(c.Name)
	if err := dryRunAndConfirmDestructive(ctx, flags, "connection.remove", map[string]any{"connection": name},
		fmt.Sprintf("Remove connection %s (config only; no Google state changes)", name)); err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	deleted, err := store.DeleteConnection(name)
	if err != nil {
		return err
	}
	if !deleted {
		return usage(fmt.Sprintf("connection %q not found", name))
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("connection", name),
	)
}

func boolMark(ok bool) string {
	if ok {
		return "*"
	}
	return ""
}
