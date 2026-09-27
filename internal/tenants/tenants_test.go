package tenants

import (
	"bytes"
	"testing"
)

func TestTenantStoreCRUD(t *testing.T) {
	store := NewStore(t.TempDir())

	if _, ok, err := store.Get("personal"); err != nil {
		t.Fatalf("get missing: %v", err)
	} else if ok {
		t.Fatal("expected missing tenant")
	}

	if err := store.Set(Tenant{Name: "Personal", Account: "rajeev.sgill@gmail.com", ReadOnly: true}); err != nil {
		t.Fatalf("set: %v", err)
	}

	tenant, ok, err := store.Get("PERSONAL")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if !ok || tenant.Account != "rajeev.sgill@gmail.com" || !tenant.ReadOnly {
		t.Fatalf("unexpected tenant: %#v ok=%v", tenant, ok)
	}

	tenantsList, err := store.List()
	if err != nil || len(tenantsList) != 1 {
		t.Fatalf("unexpected list: %#v err=%v", tenantsList, err)
	}

	deleted, err := store.Delete("personal")
	if err != nil || !deleted {
		t.Fatalf("delete: deleted=%v err=%v", deleted, err)
	}

	if _, ok, _ := store.Get("personal"); ok {
		t.Fatal("expected deleted tenant")
	}
}

func TestTenantStoreValidation(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Set(Tenant{Name: "Bad Name", Account: "a@b.com"}); err == nil {
		t.Fatal("expected invalid tenant name")
	}

	if err := store.Set(Tenant{Name: "ok", Account: ""}); err == nil {
		t.Fatal("expected missing account")
	}

	if _, err := NormalizeName("has_underscore"); err == nil {
		t.Fatal("expected slug validation failure for underscore")
	}
}

func TestDeriveKeyringPassword(t *testing.T) {
	key, err := DeriveKeyringPassword([]byte("master"), "personal")
	if err != nil || len(key) != 32 {
		t.Fatalf("derive: len=%d err=%v", len(key), err)
	}

	other, err := DeriveKeyringPassword([]byte("master"), "singulyr")
	if err != nil {
		t.Fatalf("derive other: %v", err)
	}

	if bytes.Equal(key, other) {
		t.Fatal("tenant keys must differ")
	}

	same, _ := DeriveKeyringPassword([]byte("master"), "PERSONAL")
	if !bytes.Equal(key, same) {
		t.Fatal("derivation must be deterministic for the same tenant")
	}

	if _, err := DeriveKeyringPassword(nil, "personal"); err == nil {
		t.Fatal("expected missing master secret error")
	}
}

func TestAuditLogAppendAndTail(t *testing.T) {
	auditLog := NewAuditLog(t.TempDir())

	entries, err := auditLog.Tail(10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty tail: %#v err=%v", entries, err)
	}

	if appendErr := auditLog.Append(AuditEntry{Tenant: "personal", Action: "gmail_search", Decision: "execute", ExitCode: 0}); appendErr != nil {
		t.Fatalf("append: %v", appendErr)
	}

	if appendErr2 := auditLog.Append(AuditEntry{Tenant: "personal", Action: "googleads_query", Decision: "deny", Detail: "not allowlisted"}); appendErr2 != nil {
		t.Fatalf("append 2: %v", appendErr2)
	}

	entries, err = auditLog.Tail(10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("tail: %#v err=%v", entries, err)
	}

	if entries[1].Decision != "deny" {
		t.Fatalf("unexpected entry order: %#v", entries)
	}

	bounded, err := auditLog.Tail(1)
	if err != nil || len(bounded) != 1 || bounded[0].Decision != "deny" {
		t.Fatalf("bounded tail: %#v err=%v", bounded, err)
	}
}
