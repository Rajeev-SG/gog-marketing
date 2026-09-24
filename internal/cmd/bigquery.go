package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type BigQueryCmd struct {
	Datasets BigQueryDatasetsCmd `cmd:"" help:"Inspect BigQuery datasets"`
	Tables   BigQueryTablesCmd   `cmd:"" help:"Inspect BigQuery tables"`
	Query    BigQueryQueryCmd    `cmd:"" help:"Run Standard SQL with explicit execution project"`
}

type BigQueryProjectFlags struct {
	Project string `name:"billing-project" aliases:"execution-project,project-id" env:"GOG_BIGQUERY_PROJECT" help:"BigQuery execution and billing project (required)"`
}

type BigQueryDatasetsCmd struct {
	List BigQueryDatasetsListCmd `cmd:"" default:"withargs" aliases:"ls" help:"List datasets"`
	Get  BigQueryDatasetGetCmd   `cmd:"" name:"get" aliases:"info,show" help:"Get dataset metadata"`
}

type BigQueryDatasetsListCmd struct {
	BigQueryProjectFlags `embed:""`
	FailEmpty            bool `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no datasets"`
}

func (c *BigQueryDatasetsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	project, err := resolveBigQueryProject(c.Project)
	if err != nil {
		return err
	}
	client, err := openBigQuery(ctx, flags, project)
	if err != nil {
		return err
	}
	defer closeBigQuery(client)
	items, err := client.ListDatasets(ctx)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "datasets": items}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, len(items) == 0)
	}
	if len(items) == 0 {
		u.Err().Println("No BigQuery datasets")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PROJECT\tDATASET\tFULL_ID")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\n", sanitizeTab(item.ProjectID), sanitizeTab(item.DatasetID), sanitizeTab(item.FullID))
	}
	return nil
}

type BigQueryDatasetGetCmd struct {
	BigQueryProjectFlags `embed:""`
	Dataset              string `arg:"" name:"dataset" help:"BigQuery dataset ID"`
}

func (c *BigQueryDatasetGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	project, err := resolveBigQueryProject(c.Project)
	if err != nil {
		return err
	}
	client, err := openBigQuery(ctx, flags, project)
	if err != nil {
		return err
	}
	defer closeBigQuery(client)
	item, err := client.GetDataset(ctx, strings.TrimSpace(c.Dataset))
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "dataset": item})
	}
	return writeResult(ctx, ui.FromContext(ctx),
		kv("project", item.ProjectID),
		kv("dataset", item.DatasetID),
		kv("full_id", item.FullID),
		kv("name", item.Name),
		kv("description", item.Description),
		kv("location", item.Location),
	)
}

type BigQueryTablesCmd struct {
	List   BigQueryTablesListCmd  `cmd:"" default:"withargs" aliases:"ls" help:"List tables in a dataset"`
	Get    BigQueryTableGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get table metadata"`
	Schema BigQueryTableSchemaCmd `cmd:"" name:"schema" help:"Get table schema"`
	Rows   BigQueryTableRowsCmd   `cmd:"" name:"rows" help:"Read a bounded number of table rows"`
}

type BigQueryTablesListCmd struct {
	BigQueryProjectFlags `embed:""`
	Dataset              string `arg:"" name:"dataset" help:"BigQuery dataset ID"`
	FailEmpty            bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no tables"`
}

func (c *BigQueryTablesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	project, err := resolveBigQueryProject(c.Project)
	if err != nil {
		return err
	}
	client, err := openBigQuery(ctx, flags, project)
	if err != nil {
		return err
	}
	defer closeBigQuery(client)
	items, err := client.ListTables(ctx, strings.TrimSpace(c.Dataset))
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "dataset": c.Dataset, "tables": items}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, len(items) == 0)
	}
	if len(items) == 0 {
		ui.FromContext(ctx).Err().Println("No BigQuery tables")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "PROJECT\tDATASET\tTABLE\tFULL_ID")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sanitizeTab(item.ProjectID), sanitizeTab(item.DatasetID), sanitizeTab(item.TableID), sanitizeTab(item.FullID))
	}
	return nil
}

type BigQueryTableGetCmd struct {
	BigQueryProjectFlags `embed:""`
	Dataset              string `arg:"" name:"dataset" help:"BigQuery dataset ID"`
	Table                string `arg:"" name:"table" help:"BigQuery table ID"`
}

func (c *BigQueryTableGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	item, err := bigQueryTable(ctx, flags, c.Project, c.Dataset, c.Table)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"table": item})
	}
	return writeResult(ctx, ui.FromContext(ctx),
		kv("project", item.ProjectID),
		kv("dataset", item.DatasetID),
		kv("table", item.TableID),
		kv("full_id", item.FullID),
		kv("type", item.Type),
		kv("rows", item.NumRows),
		kv("bytes", item.NumBytes),
	)
}

type BigQueryTableSchemaCmd struct {
	BigQueryProjectFlags `embed:""`
	Dataset              string `arg:"" name:"dataset" help:"BigQuery dataset ID"`
	Table                string `arg:"" name:"table" help:"BigQuery table ID"`
}

func (c *BigQueryTableSchemaCmd) Run(ctx context.Context, flags *RootFlags) error {
	item, err := bigQueryTable(ctx, flags, c.Project, c.Dataset, c.Table)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": item.ProjectID, "dataset": item.DatasetID, "table": item.TableID, "schema": item.Schema})
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTYPE\tMODE\tDESCRIPTION")
	writeBigQuerySchemaRows(w, item.Schema, "")
	return nil
}

type BigQueryTableRowsCmd struct {
	BigQueryProjectFlags `embed:""`
	Dataset              string `arg:"" name:"dataset" help:"BigQuery dataset ID"`
	Table                string `arg:"" name:"table" help:"BigQuery table ID"`
	Max                  int64  `name:"max" aliases:"limit" help:"Maximum rows (1-10000)" default:"100"`
	FailEmpty            bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *BigQueryTableRowsCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Max < 1 || c.Max > 10000 {
		return usage("--max must be between 1 and 10000")
	}
	project, err := resolveBigQueryProject(c.Project)
	if err != nil {
		return err
	}
	client, err := openBigQuery(ctx, flags, project)
	if err != nil {
		return err
	}
	defer closeBigQuery(client)
	rows, schema, err := client.ReadRows(ctx, strings.TrimSpace(c.Dataset), strings.TrimSpace(c.Table), c.Max)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "dataset": c.Dataset, "table": c.Table, "schema": schema, "rows": rows}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, len(rows) == 0)
	}
	if len(rows) == 0 {
		ui.FromContext(ctx).Err().Println("No BigQuery rows")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	writeBigQueryRows(ctx, rows)
	return nil
}

type BigQueryQueryCmd struct {
	BigQueryProjectFlags `embed:""`
	SQL                  string `name:"sql" help:"Standard SQL query"`
	File                 string `name:"file" type:"path" help:"Read Standard SQL from a file"`
	Max                  int64  `name:"max" aliases:"limit" help:"Maximum rows (1-10000)" default:"100"`
	FailEmpty            bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no rows"`
}

func (c *BigQueryQueryCmd) Run(ctx context.Context, flags *RootFlags) error {
	sql, err := c.readSQL()
	if err != nil {
		return err
	}
	project, err := resolveBigQueryProject(c.Project)
	if err != nil {
		return err
	}
	if c.Max < 1 || c.Max > 10000 {
		return usage("--max must be between 1 and 10000")
	}
	client, err := openBigQuery(ctx, flags, project)
	if err != nil {
		return err
	}
	defer closeBigQuery(client)
	result, err := client.Query(ctx, sql, flags != nil && flags.DryRun)
	if err != nil {
		return err
	}
	if result != nil && !result.DryRun && int64(len(result.Rows)) > c.Max {
		result.Rows = result.Rows[:c.Max]
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"project": project, "sql": sql, "result": result}); err != nil {
			return err
		}
		return failEmptyExitIf(c.FailEmpty, result != nil && !result.DryRun && len(result.Rows) == 0)
	}
	if result != nil && result.DryRun {
		u := ui.FromContext(ctx)
		return writeResult(ctx, u,
			kv("project", project),
			kv("dry_run", true),
			kv("job_id", result.JobID),
			kv("total_bytes_processed", result.TotalBytesProcessed),
			kv("total_bytes_billed", result.TotalBytesBilled),
		)
	}
	if result == nil || len(result.Rows) == 0 {
		ui.FromContext(ctx).Err().Println("No BigQuery rows")
		return failEmptyExitIf(c.FailEmpty, true)
	}
	writeBigQueryRows(ctx, result.Rows)
	return nil
}

func (c *BigQueryQueryCmd) readSQL() (string, error) {
	sql := strings.TrimSpace(c.SQL)
	file := strings.TrimSpace(c.File)
	if sql != "" && file != "" {
		return "", usage("--sql and --file are mutually exclusive")
	}
	if file != "" {
		raw, err := os.ReadFile(file) //nolint:gosec // user-provided SQL file
		if err != nil {
			return "", fmt.Errorf("read --file: %w", err)
		}
		sql = strings.TrimSpace(string(raw))
	}
	if sql == "" {
		return "", usage("provide --sql or --file")
	}
	return sql, nil
}

func resolveBigQueryProject(explicit string) (string, error) {
	project := strings.TrimSpace(explicit)
	if project == "" {
		project = strings.TrimSpace(os.Getenv("GOG_BIGQUERY_PROJECT"))
	}
	if project == "" {
		return "", usage("BigQuery execution/billing project is required; pass --project or set GOG_BIGQUERY_PROJECT")
	}
	return project, nil
}

func openBigQuery(ctx context.Context, flags *RootFlags, project string) (googleapi.BigQueryClient, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, err
	}
	client, err := bigQueryClient(ctx, account, project)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func closeBigQuery(client googleapi.BigQueryClient) {
	if client != nil {
		_ = client.Close()
	}
}

func bigQueryTable(ctx context.Context, flags *RootFlags, project, dataset, table string) (*googleapi.BigQueryTable, error) {
	resolvedProject, err := resolveBigQueryProject(project)
	if err != nil {
		return nil, err
	}
	client, err := openBigQuery(ctx, flags, resolvedProject)
	if err != nil {
		return nil, err
	}
	defer closeBigQuery(client)
	item, err := client.GetTable(ctx, strings.TrimSpace(dataset), strings.TrimSpace(table))
	if err != nil {
		return nil, err
	}
	return item, nil
}

func writeBigQuerySchemaRows(w interface{ Write([]byte) (int, error) }, fields []googleapi.BigQuerySchemaField, prefix string) {
	for _, field := range fields {
		name := field.Name
		if prefix != "" {
			name = prefix + "." + field.Name
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sanitizeTab(name), sanitizeTab(field.Type), sanitizeTab(field.Mode), sanitizeTab(field.Description))
		writeBigQuerySchemaRows(w, field.Fields, name)
	}
}

func writeBigQueryRows(ctx context.Context, rows []map[string]any) {
	keys := map[string]bool{}
	var columns []string
	for _, row := range rows {
		for key := range row {
			if !keys[key] {
				keys[key] = true
				columns = append(columns, key)
			}
		}
	}
	sort.Strings(columns)
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, strings.Join(columns, "\t"))
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, sanitizeTab(formatGoogleAdsValue(row[column])))
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}
}
