package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
)

func TestConnectionCRUD_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")

	store := defaultConfigStoreForTest(t)
	runtime := &app.Runtime{Config: store}

	res := executeWithTestRuntime(t, []string{
		"--json", "connection", "add", "personal", "rajeev.sgill@gmail.com",
		"--oauth-client", "default",
		"--services", "analytics,bigquery",
		"--bigquery-project", "bq-proj",
		"--connection-quota-project", "quota-proj",
		"--description", "personal dogfood",
	}, runtime)
	if res.err != nil {
		t.Fatalf("add: %v stderr=%s", res.err, res.stderr)
	}

	var addResp struct {
		Connection config.Connection `json:"connection"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &addResp); err != nil {
		t.Fatalf("add json: %v out=%s", err, res.stdout)
	}
	if addResp.Connection.Name != "personal" || addResp.Connection.BigQueryProject != "bq-proj" {
		t.Fatalf("unexpected add response: %#v", addResp.Connection)
	}

	res = executeWithTestRuntime(t, []string{"--json", "connection", "list"}, runtime)
	if res.err != nil {
		t.Fatalf("list: %v", res.err)
	}
	var listResp struct {
		Connections []config.Connection `json:"connections"`
		Default     string              `json:"default"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &listResp); err != nil {
		t.Fatalf("list json: %v out=%s", err, res.stdout)
	}
	if len(listResp.Connections) != 1 {
		t.Fatalf("unexpected list: %#v", listResp)
	}

	// bigquery service without explicit project is rejected.
	res = executeWithTestRuntime(t, []string{
		"connection", "add", "bad", "x@y.com", "--services", "bigquery",
	}, runtime)
	if res.err == nil || !strings.Contains(res.err.Error(), "--bigquery-project") {
		t.Fatalf("expected bigquery-project usage error, got %v", res.err)
	}

	res = executeWithTestRuntime(t, []string{"--json", "connection", "use", "personal"}, runtime)
	if res.err != nil {
		t.Fatalf("use: %v", res.err)
	}
	defaultName, err := store.DefaultConnectionName()
	if err != nil || defaultName != "personal" {
		t.Fatalf("unexpected default after use: %q err=%v", defaultName, err)
	}

	res = executeWithTestRuntime(t, []string{"--json", "connection", "get", "personal"}, runtime)
	if res.err != nil {
		t.Fatalf("get: %v", res.err)
	}

	res = executeWithTestRuntime(t, []string{"--json", "--force", "connection", "remove", "personal"}, runtime)
	if res.err != nil {
		t.Fatalf("remove: %v", res.err)
	}
	defaultName, err = store.DefaultConnectionName()
	if err != nil {
		t.Fatalf("default after remove: %v", err)
	}
	if defaultName != "" {
		t.Fatalf("expected default cleared after remove, got %q", defaultName)
	}
}

func TestConnectionDefaultsAppliedThroughRootFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("GOG_ACCOUNT", "")

	store := defaultConfigStoreForTest(t)
	if err := store.SetConnection(config.Connection{
		Name:            "singulyr",
		Account:         "rajeev@singulyr.com",
		Client:          "singulyr-client",
		QuotaProject:    "quota-proj",
		BigQueryProject: "bq-proj",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	if err := store.SetDefaultConnection("singulyr"); err != nil {
		t.Fatalf("set default: %v", err)
	}

	flags := &RootFlags{configStoreResolver: func() (*config.ConfigStore, error) { return store, nil }}
	if err := applyConnectionDefaults(flags); err != nil {
		t.Fatalf("apply connection defaults: %v", err)
	}
	if flags.Account != "rajeev@singulyr.com" {
		t.Fatalf("expected account from connection, got %q", flags.Account)
	}
	if flags.Client != "singulyr-client" {
		t.Fatalf("expected client from connection, got %q", flags.Client)
	}
	if flags.QuotaProject != "quota-proj" {
		t.Fatalf("expected quota project from connection, got %q", flags.QuotaProject)
	}
	if flags.connection == nil || flags.connection.BigQueryProject != "bq-proj" {
		t.Fatalf("expected bigquery project on resolved connection: %#v", flags.connection)
	}

	// Explicit flags win over connection defaults.
	flags2 := &RootFlags{
		Account:             "override@gmail.com",
		Client:              "other",
		QuotaProject:        "explicit-quota",
		configStoreResolver: func() (*config.ConfigStore, error) { return store, nil },
	}
	if err := applyConnectionDefaults(flags2); err != nil {
		t.Fatalf("apply connection defaults 2: %v", err)
	}
	if flags2.Account != "override@gmail.com" || flags2.Client != "other" || flags2.QuotaProject != "explicit-quota" {
		t.Fatalf("explicit flags should win: %#v", flags2)
	}
}
