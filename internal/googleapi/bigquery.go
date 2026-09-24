package googleapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/googleauth"
)

type BigQuerySchemaField struct {
	Name        string                `json:"name"`
	Type        string                `json:"type"`
	Mode        string                `json:"mode,omitempty"`
	Description string                `json:"description,omitempty"`
	Fields      []BigQuerySchemaField `json:"fields,omitempty"`
}

type BigQueryDataset struct {
	ProjectID   string `json:"project_id"`
	DatasetID   string `json:"dataset_id"`
	FullID      string `json:"full_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	ETag        string `json:"etag,omitempty"`
}

type BigQueryTable struct {
	ProjectID   string                `json:"project_id"`
	DatasetID   string                `json:"dataset_id"`
	TableID     string                `json:"table_id"`
	FullID      string                `json:"full_id,omitempty"`
	Name        string                `json:"name,omitempty"`
	Description string                `json:"description,omitempty"`
	Type        string                `json:"type,omitempty"`
	NumRows     uint64                `json:"num_rows,omitempty"`
	NumBytes    int64                 `json:"num_bytes,omitempty"`
	Schema      []BigQuerySchemaField `json:"schema,omitempty"`
	ETag        string                `json:"etag,omitempty"`
}

type BigQueryQueryResult struct {
	JobID               string                `json:"job_id,omitempty"`
	DryRun              bool                  `json:"dry_run"`
	Schema              []BigQuerySchemaField `json:"schema,omitempty"`
	Rows                []map[string]any      `json:"rows"`
	TotalBytesProcessed int64                 `json:"total_bytes_processed,omitempty"`
	TotalBytesBilled    int64                 `json:"total_bytes_billed,omitempty"`
	CacheHit            bool                  `json:"cache_hit,omitempty"`
}

type BigQueryClient interface {
	Project() string
	ListDatasets(context.Context) ([]BigQueryDataset, error)
	GetDataset(context.Context, string) (*BigQueryDataset, error)
	ListTables(context.Context, string) ([]BigQueryTable, error)
	GetTable(context.Context, string, string) (*BigQueryTable, error)
	ReadRows(context.Context, string, string, int64) ([]map[string]any, []BigQuerySchemaField, error)
	Query(context.Context, string, bool, int64) (*BigQueryQueryResult, error)
	Close() error
}

type BigQueryClientFactory func(context.Context, string, string) (BigQueryClient, error)

var (
	ErrBigQueryProjectRequired        = errors.New("bigquery execution project is required")
	ErrBigQueryMaxBytesBilledRequired = errors.New("bigquery max bytes billed must be greater than zero")
	ErrBigQueryHTTPClientRequired     = errors.New("bigquery HTTP client is required")
	ErrBigQueryDryRunFailed           = errors.New("bigquery dry-run failed")
)

type bigQueryAdapter struct {
	client     *bigquery.Client
	httpClient *http.Client
	projectID  string
	baseURL    string
}

func NewBigQuery(ctx context.Context, account, project string) (BigQueryClient, error) {
	if project == "" {
		return nil, ErrBigQueryProjectRequired
	}

	client, err := NewHTTPClient(ctx, googleauth.ServiceBigQuery, account)
	if err != nil {
		return nil, fmt.Errorf("bigquery auth client: %w", err)
	}

	bq, err := bigquery.NewClient(ctx, project, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create bigquery client: %w", err)
	}

	return &bigQueryAdapter{client: bq, httpClient: client, projectID: project}, nil
}

func (a *bigQueryAdapter) Project() string {
	if a.client != nil {
		return a.client.Project()
	}

	return a.projectID
}

func (a *bigQueryAdapter) ListDatasets(ctx context.Context) ([]BigQueryDataset, error) {
	it := a.client.Datasets(ctx)
	var out []BigQueryDataset

	for {
		dataset, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}

		if err != nil {
			return nil, fmt.Errorf("list BigQuery datasets: %w", err)
		}

		out = append(out, BigQueryDataset{ProjectID: dataset.ProjectID, DatasetID: dataset.DatasetID, FullID: dataset.ProjectID + ":" + dataset.DatasetID})
	}
}

func (a *bigQueryAdapter) GetDataset(ctx context.Context, datasetID string) (*BigQueryDataset, error) {
	md, err := a.client.Dataset(datasetID).Metadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("get BigQuery dataset metadata: %w", err)
	}

	return &BigQueryDataset{ProjectID: a.client.Project(), DatasetID: datasetID, FullID: md.FullID, Name: md.Name, Description: md.Description, Location: md.Location, ETag: md.ETag}, nil
}

func (a *bigQueryAdapter) ListTables(ctx context.Context, datasetID string) ([]BigQueryTable, error) {
	it := a.client.Dataset(datasetID).Tables(ctx)
	var out []BigQueryTable

	for {
		table, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}

		if err != nil {
			return nil, fmt.Errorf("list BigQuery tables: %w", err)
		}

		out = append(out, BigQueryTable{ProjectID: table.ProjectID, DatasetID: table.DatasetID, TableID: table.TableID, FullID: table.ProjectID + ":" + table.DatasetID + "." + table.TableID})
	}
}

func (a *bigQueryAdapter) GetTable(ctx context.Context, datasetID, tableID string) (*BigQueryTable, error) {
	md, err := a.client.Dataset(datasetID).Table(tableID).Metadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("get BigQuery table metadata: %w", err)
	}

	return &BigQueryTable{
		ProjectID:   a.client.Project(),
		DatasetID:   datasetID,
		TableID:     tableID,
		FullID:      md.FullID,
		Name:        md.Name,
		Description: md.Description,
		Type:        string(md.Type),
		NumRows:     md.NumRows,
		NumBytes:    md.NumBytes,
		Schema:      convertBigQuerySchema(md.Schema),
		ETag:        md.ETag,
	}, nil
}

func (a *bigQueryAdapter) ReadRows(ctx context.Context, datasetID, tableID string, limit int64) ([]map[string]any, []BigQuerySchemaField, error) {
	it := a.client.Dataset(datasetID).Table(tableID).Read(ctx)
	var schema []BigQuerySchemaField

	rows := []map[string]any{}
	for limit <= 0 || int64(len(rows)) < limit {
		var values []bigquery.Value

		err := it.Next(&values)
		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			return nil, nil, fmt.Errorf("read BigQuery table rows: %w", err)
		}

		if schema == nil {
			schema = convertBigQuerySchema(it.Schema)
		}

		rows = append(rows, bigQueryRow(it.Schema, values))
	}

	return rows, schema, nil
}

func (a *bigQueryAdapter) Query(ctx context.Context, sql string, dryRun bool, maxBytesBilled int64) (*BigQueryQueryResult, error) {
	if maxBytesBilled <= 0 {
		return nil, ErrBigQueryMaxBytesBilledRequired
	}

	if dryRun {
		return a.queryDryRun(ctx, sql, maxBytesBilled)
	}
	query := a.client.Query(sql)
	query.UseStandardSQL = true
	query.MaxBytesBilled = maxBytesBilled

	it, err := query.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("read BigQuery query result: %w", err)
	}
	var schema []BigQuerySchemaField
	rows := []map[string]any{}

	for {
		var values []bigquery.Value

		nextErr := it.Next(&values)
		if errors.Is(nextErr, iterator.Done) {
			break
		}

		if nextErr != nil {
			return nil, fmt.Errorf("read BigQuery query row: %w", nextErr)
		}

		if schema == nil {
			schema = convertBigQuerySchema(it.Schema)
		}

		rows = append(rows, bigQueryRow(it.Schema, values))
	}

	result := &BigQueryQueryResult{Rows: rows, Schema: schema}
	if it.SourceJob() != nil {
		result.JobID = it.SourceJob().ID()
		if status := it.SourceJob().LastStatus(); status != nil && status.Statistics != nil {
			result.TotalBytesProcessed = status.Statistics.TotalBytesProcessed
			if details, ok := status.Statistics.Details.(*bigquery.QueryStatistics); ok {
				result.TotalBytesBilled = details.TotalBytesBilled
				result.CacheHit = details.CacheHit
			}
		}
	}

	return result, nil
}

func (a *bigQueryAdapter) queryDryRun(ctx context.Context, sql string, maxBytesBilled int64) (*BigQueryQueryResult, error) {
	if a.httpClient == nil {
		return nil, ErrBigQueryHTTPClientRequired
	}

	payload, err := json.Marshal(map[string]any{
		"query":              sql,
		"useLegacySql":       false,
		"dryRun":             true,
		"maximumBytesBilled": maxBytesBilled,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal BigQuery dry-run request: %w", err)
	}

	baseURL := strings.TrimRight(a.baseURL, "/")
	if baseURL == "" {
		baseURL = "https://bigquery.googleapis.com"
	}
	endpoint := baseURL + "/bigquery/v2/projects/" + url.PathEscape(a.projectID) + "/queries"

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create BigQuery dry-run request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := a.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send BigQuery dry-run request: %w", err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read BigQuery dry-run response: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w (%d): %s", ErrBigQueryDryRunFailed, response.StatusCode, strings.TrimSpace(string(body)))
	}

	var payloadMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &payloadMap); err != nil {
		return nil, fmt.Errorf("decode BigQuery dry-run response: %w", err)
	}

	result := &BigQueryQueryResult{DryRun: true, Rows: []map[string]any{}}
	if raw := payloadMap["totalBytesProcessed"]; len(raw) > 0 {
		result.TotalBytesProcessed = parseBigQueryInt(raw)
	}

	if raw := payloadMap["totalBytesBilled"]; len(raw) > 0 {
		result.TotalBytesBilled = parseBigQueryInt(raw)
	}

	if raw := payloadMap["cacheHit"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &result.CacheHit)
	}

	if raw := payloadMap["schema"]; len(raw) > 0 {
		var schemaPayload struct {
			Fields bigquery.Schema `json:"fields"`
		}
		if err := json.Unmarshal(raw, &schemaPayload); err != nil {
			return nil, fmt.Errorf("decode BigQuery dry-run schema: %w", err)
		}
		result.Schema = convertBigQuerySchema(schemaPayload.Fields)
	}

	if raw := payloadMap["jobReference"]; len(raw) > 0 {
		var jobReference map[string]json.RawMessage
		if err := json.Unmarshal(raw, &jobReference); err != nil {
			return nil, fmt.Errorf("decode BigQuery dry-run job reference: %w", err)
		}

		if rawID := jobReference["jobId"]; len(rawID) > 0 {
			_ = json.Unmarshal(rawID, &result.JobID)
		}
	}

	return result, nil
}

func parseBigQueryInt(raw json.RawMessage) int64 {
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		parsed, parseErr := strconv.ParseInt(text, 10, 64)
		if parseErr == nil {
			return parsed
		}
	}

	return 0
}

func (a *bigQueryAdapter) Close() error {
	if err := a.client.Close(); err != nil {
		return fmt.Errorf("close BigQuery client: %w", err)
	}

	return nil
}

func bigQueryRow(schema bigquery.Schema, values []bigquery.Value) map[string]any {
	row := make(map[string]any, len(values))
	for i, value := range values {
		name := fmt.Sprintf("column_%d", i+1)
		if i < len(schema) && schema[i] != nil {
			name = schema[i].Name
		}
		row[name] = normalizeBigQueryValue(value)
	}

	return row
}

func convertBigQuerySchema(schema bigquery.Schema) []BigQuerySchemaField {
	if len(schema) == 0 {
		return nil
	}

	out := make([]BigQuerySchemaField, 0, len(schema))
	for _, field := range schema {
		if field == nil {
			continue
		}

		mode := "NULLABLE"
		if field.Repeated {
			mode = "REPEATED"
		} else if field.Required {
			mode = "REQUIRED"
		}
		out = append(out, BigQuerySchemaField{
			Name:        field.Name,
			Type:        string(field.Type),
			Mode:        mode,
			Description: field.Description,
			Fields:      convertBigQuerySchema(field.Schema),
		})
	}

	return out
}

func normalizeBigQueryValue(value any) any {
	switch v := value.(type) {
	case nil, string, bool, int, int32, int64, float32, float64:
		return v
	default:
		return fmt.Sprint(v)
	}
}
