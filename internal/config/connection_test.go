package config

import "testing"

func TestConnectionCRUD(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	if _, ok, err := store.ResolveConnection("personal"); err != nil {
		t.Fatalf("resolve missing connection: %v", err)
	} else if ok {
		t.Fatal("expected missing connection")
	}

	conn := Connection{
		Name:            "Personal",
		Account:         "rajeev.sgill@gmail.com",
		Client:          "default",
		Services:        []string{"analytics", "bigquery"},
		QuotaProject:    "quota-proj",
		BigQueryProject: "bq-proj",
	}
	if err := store.SetConnection(conn); err != nil {
		t.Fatalf("set connection: %v", err)
	}

	got, ok, err := store.ResolveConnection("PERSONAL")
	if err != nil {
		t.Fatalf("resolve connection: %v", err)
	}

	if !ok || got.Account != "rajeev.sgill@gmail.com" || got.BigQueryProject != "bq-proj" {
		t.Fatalf("unexpected connection: %#v ok=%v", got, ok)
	}

	if setErr := store.SetDefaultConnection("personal"); setErr != nil {
		t.Fatalf("set default connection: %v", setErr)
	}

	defaultName, err := store.DefaultConnectionName()
	if err != nil {
		t.Fatalf("default connection name: %v", err)
	}

	if defaultName != "personal" {
		t.Fatalf("unexpected default connection: %q", defaultName)
	}

	connections, err := store.ListConnections()
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}

	if len(connections) != 1 || connections[0].Name != "personal" {
		t.Fatalf("unexpected connections: %#v", connections)
	}

	deleted, err := store.DeleteConnection("personal")
	if err != nil {
		t.Fatalf("delete connection: %v", err)
	}

	if !deleted {
		t.Fatal("expected delete")
	}

	defaultName, err = store.DefaultConnectionName()
	if err != nil {
		t.Fatalf("default after delete: %v", err)
	}

	if defaultName != "" {
		t.Fatalf("expected default cleared after delete, got %q", defaultName)
	}
}

func TestConnectionValidation(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	if err := store.SetConnection(Connection{Name: "bad name", Account: "a@b.com"}); err == nil {
		t.Fatal("expected invalid connection name")
	}

	if err := store.SetConnection(Connection{Name: "ok", Account: ""}); err == nil {
		t.Fatal("expected missing account")
	}

	if _, err := NormalizeConnectionName("auto"); err == nil {
		t.Fatal("expected reserved connection name")
	}

	if err := store.SetDefaultConnection("missing"); err == nil {
		t.Fatal("expected missing default connection")
	}
}
