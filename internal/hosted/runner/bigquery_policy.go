package runner

import (
	"net/http"
	"regexp"
)

var (
	bigQueryProjectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,61}[a-z0-9]$`)
	bigQueryDatasetID = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	bigQueryTableID   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func nativeBigQueryReadBindings() []NativeToolBinding {
	result := make([]NativeToolBinding, 0, 6)
	for _, name := range []string{"bigquery_datasets_list", "bigquery_dataset_get", "bigquery_tables_list", "bigquery_table_get", "bigquery_table_schema", "bigquery_table_rows"} {
		result = append(result, NativeToolBinding{Name: name, Service: "bigquery", ResourceType: "project", ResourceArgument: "project", hosts: []string{"bigquery.googleapis.com"}, methods: []string{http.MethodGet}, scopes: []string{"bigquery"}, paths: exactPaths("", ""), argumentPaths: func(args map[string]any) ([]string, error) {
			project, _ := args["project"].(string)
			if !bigQueryProjectID.MatchString(project) || project == "gog-marketing-prod" {
				return nil, invalidf("invalid or platform-owned BigQuery project")
			}
			path := "/bigquery/v2/projects/" + project + "/datasets"
			if name == "bigquery_datasets_list" {
				return []string{path}, nil
			}
			dataset, _ := args["dataset"].(string)
			if len(dataset) > 1024 || !bigQueryDatasetID.MatchString(dataset) {
				return nil, invalidf("invalid BigQuery dataset")
			}
			path += "/" + dataset
			if name == "bigquery_dataset_get" {
				return []string{path}, nil
			}
			path += "/tables"
			if name == "bigquery_tables_list" {
				return []string{path}, nil
			}
			table, _ := args["table"].(string)
			if len(table) > 1024 || !bigQueryTableID.MatchString(table) {
				return nil, invalidf("invalid BigQuery table")
			}
			path += "/" + table
			if name == "bigquery_table_rows" {
				return []string{path, path + "/data"}, nil
			}
			return []string{path}, nil
		}})
	}
	return result
}
