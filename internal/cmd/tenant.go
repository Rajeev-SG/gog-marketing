package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/tenants"
	"github.com/openclaw/gogcli/internal/ui"
)

type TenantCmd struct {
	Add    TenantAddCmd    `cmd:"" help:"Define an isolated hosted tenant"`
	List   TenantListCmd   `cmd:"" name:"list" aliases:"ls" help:"List tenants"`
	Get    TenantGetCmd    `cmd:"" name:"get" help:"Show one tenant"`
	Remove TenantRemoveCmd `cmd:"" name:"remove" aliases:"rm,delete" help:"Remove a tenant from the registry"`
	Audit  TenantAuditCmd  `cmd:"" help:"Show a tenant's hosted-access audit log"`
	Serve  TenantServeCmd  `cmd:"" help:"Run the hosted tenant API (tenant-isolated gog access)"`
}

func commandTenantStore(ctx context.Context) (*tenants.Store, error) {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return nil, err
	}
	return tenants.NewStore(store.Layout().ConfigDir), nil
}

type TenantAddCmd struct {
	Name       string `arg:"" name:"name" help:"Tenant name (lowercase slug, used as the isolated home directory)"`
	Account    string `arg:"" name:"account" help:"Pinned account email or alias for every hosted call"`
	Client     string `name:"client" help:"OAuth client name for the tenant (optional)"`
	ReadOnly   bool   `name:"readonly" help:"Run every hosted call with --readonly (default for writes protection)"`
	AllowTools string `name:"allow-tools" help:"Comma-separated MCP tool names; empty allows read-risk tools only"`
	Notes      string `help:"Operational notes"`
}

func (c *TenantAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	tenant := tenants.Tenant{
		Name:       c.Name,
		Account:    c.Account,
		Client:     c.Client,
		ReadOnly:   c.ReadOnly,
		AllowTools: splitCommaList(c.AllowTools),
		Notes:      c.Notes,
	}
	if err := dryRunExit(ctx, flags, "tenant.add", map[string]any{"tenant": tenant}); err != nil {
		return err
	}
	store, err := commandTenantStore(ctx)
	if err != nil {
		return err
	}
	if setErr := store.Set(tenant); setErr != nil {
		return setErr
	}
	saved, ok, err := store.Get(c.Name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("tenant %q not found after save", c.Name)
	}
	home, err := store.Home(saved.Name)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tenant": saved, "home": home})
	}
	return writeResult(ctx, u,
		kv("tenant", saved.Name),
		kv("account", saved.Account),
		kv("home", home),
		kv("readonly", saved.ReadOnly),
		kv("allow_tools", strings.Join(saved.AllowTools, ",")),
	)
}

type TenantListCmd struct{}

func (c *TenantListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandTenantStore(ctx)
	if err != nil {
		return err
	}
	tenantsList, err := store.List()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tenants": tenantsList})
	}
	if len(tenantsList) == 0 {
		u.Err().Println("No tenants; add one with: gog tenant add <name> <account>")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tACCOUNT\tREADONLY\tALLOW_TOOLS\tNOTES")
	for _, tenant := range tenantsList {
		readonly := ""
		if tenant.ReadOnly {
			readonly = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sanitizeTab(tenant.Name), sanitizeTab(tenant.Account), readonly,
			sanitizeTab(strings.Join(tenant.AllowTools, ",")), sanitizeTab(tenant.Notes))
	}
	return nil
}

type TenantGetCmd struct {
	Name string `arg:"" name:"name" help:"Tenant name"`
}

func (c *TenantGetCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandTenantStore(ctx)
	if err != nil {
		return err
	}
	tenant, ok, err := store.Get(c.Name)
	if err != nil {
		return err
	}
	if !ok {
		return usage(fmt.Sprintf("tenant %q not found", strings.TrimSpace(c.Name)))
	}
	home, err := store.Home(tenant.Name)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tenant": tenant, "home": home})
	}
	return writeResult(ctx, u,
		kv("tenant", tenant.Name),
		kv("account", tenant.Account),
		kv("client", tenant.Client),
		kv("readonly", tenant.ReadOnly),
		kv("allow_tools", strings.Join(tenant.AllowTools, ",")),
		kv("home", home),
		kv("notes", tenant.Notes),
	)
}

type TenantRemoveCmd struct {
	Name     string `arg:"" name:"name" help:"Tenant name"`
	KeepHome bool   `name:"keep-home" help:"Do not touch the tenant's isolated home directory"`
}

func (c *TenantRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if err := dryRunAndConfirmDestructive(ctx, flags, "tenant.remove", map[string]any{
		"tenant":    strings.TrimSpace(c.Name),
		"keep_home": c.KeepHome,
	}, fmt.Sprintf("Remove tenant %s from the registry (home data is%s removed)", strings.TrimSpace(c.Name), map[bool]string{true: " not", false: " also"}[c.KeepHome])); err != nil {
		return err
	}
	store, err := commandTenantStore(ctx)
	if err != nil {
		return err
	}
	tenant, ok, err := store.Get(c.Name)
	if err != nil {
		return err
	}
	if !ok {
		return usage(fmt.Sprintf("tenant %q not found", strings.TrimSpace(c.Name)))
	}
	if _, delErr := store.Delete(c.Name); delErr != nil {
		return delErr
	}
	home, err := store.Home(tenant.Name)
	if err != nil {
		return err
	}
	if !c.KeepHome {
		if err := os.RemoveAll(home); err != nil {
			return fmt.Errorf("remove tenant home %s: %w", home, err)
		}
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("tenant", tenant.Name),
		kv("home_removed", !c.KeepHome),
		kv("home", home),
	)
}

type TenantAuditCmd struct {
	Name string `arg:"" name:"name" help:"Tenant name"`
	Max  int    `name:"max" help:"Maximum recent entries" default:"50"`
}

func (c *TenantAuditCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandTenantStore(ctx)
	if err != nil {
		return err
	}
	tenant, ok, err := store.Get(c.Name)
	if err != nil {
		return err
	}
	if !ok {
		return usage(fmt.Sprintf("tenant %q not found", strings.TrimSpace(c.Name)))
	}
	tenantHome, err := store.Home(tenant.Name)
	if err != nil {
		return err
	}
	auditLog := tenants.NewAuditLog(tenantHome)
	entries, err := auditLog.Tail(c.Max)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"tenant": tenant.Name, "audit": entries})
	}
	if len(entries) == 0 {
		u.Err().Println("No audit entries")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TIME\tACTION\tDECISION\tEXIT\tDETAIL")
	for _, entry := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			sanitizeTab(entry.Time), sanitizeTab(entry.Action), sanitizeTab(entry.Decision),
			entry.ExitCode, sanitizeTab(entry.Detail))
	}
	return nil
}
