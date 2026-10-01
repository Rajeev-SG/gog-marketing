package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordingResourceReader struct {
	calls []ResourceGrant
	err   error
}

func (r *recordingResourceReader) Read(_ context.Context, _ Connection, _ OAuthToken, grant ResourceGrant) error {
	r.calls = append(r.calls, grant)
	return r.err
}

func TestResourceReadValidatesServiceMappings(t *testing.T) {
	for _, valid := range [][2]string{
		{"analytics", "property"},
		{"analytics", "account"},
		{"tagmanager", "account"},
		{"tagmanager", "container"},
		{"searchconsole", "site"},
		{"googleads", "customer"},
		{"bigquery", "project"},
		{"bigquery", "dataset"},
	} {
		if !validResourceRead(valid[0], valid[1]) {
			t.Fatalf("valid mapping rejected: %v", valid)
		}
	}

	for _, invalid := range [][2]string{
		{"analytics", "dataset"}, {"tagmanager", "site"}, {"unknown", "property"},
	} {
		if validResourceRead(invalid[0], invalid[1]) {
			t.Fatalf("invalid mapping accepted: %v", invalid)
		}
	}
}

func TestReadResourceUsesReaderAfterGrantGate(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	reader := &recordingResourceReader{}
	service.Reader = reader

	connection, err := service.CreateProductConnection(context.Background(), actor, "google", []string{"analytics"})
	if err != nil {
		t.Fatal(err)
	}

	start, err := service.BeginOAuth(context.Background(), actor, connection.ID, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, completeErr := service.CompleteOAuth(context.Background(), start.State, "code"); completeErr != nil {
		t.Fatal(completeErr)
	}

	if _, discoverErr := service.Discover(context.Background(), actor, connection.ID); discoverErr != nil {
		t.Fatal(discoverErr)
	}

	if _, saveErr := store.SetResourceEnabled(context.Background(), actor.OrganizationID, connection.ID, "properties/123", true); saveErr != nil {
		t.Fatal(saveErr)
	}

	read, err := service.ReadResource(context.Background(), actor, connection.ID, "properties/123")
	if err != nil || read.ResourceID != "properties/123" || len(reader.calls) != 1 {
		t.Fatalf("enabled resource read failed: %+v %v calls=%d", read, err, len(reader.calls))
	}

	for _, resource := range []string{"properties/999", "containers/unknown"} {
		if _, readErr := service.ReadResource(context.Background(), actor, connection.ID, resource); !errors.Is(readErr, ErrForbidden) {
			t.Fatalf("ungranted resource reached reader: %s %v", resource, readErr)
		}
	}

	if len(reader.calls) != 1 {
		t.Fatal("denied resources reached the reader")
	}
}

func TestProductResourceReadAPIReturnsFixedSchema(t *testing.T) {
	service, store := productTestService(t)
	actor := ownerActor(t, store)
	reader := &recordingResourceReader{}
	service.Reader = reader
	server, client := newProductTestHandler(t, service, actor)
	cookies := productSessionCookies(t, client, server)
	resp := productGet(t, client, server.URL, "/api/connections/unknown/resource?resource=properties/123", cookies)

	body := readProductBody(t, resp)
	if resp.StatusCode != 403 || !strings.Contains(body, "access_denied") || len(reader.calls) != 0 {
		t.Fatalf("unknown resource was not denied before reader: %d %s", resp.StatusCode, body)
	}
}
