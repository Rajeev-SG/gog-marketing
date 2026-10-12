package policy

import (
	"errors"
	"testing"
)

func TestHostedScopesRejectRestrictedAndUnknownTransitiveGrants(t *testing.T) {
	for _, scope := range []string{"drive", "drive.readonly", "drive.metadata.readonly", "drive.activity.readonly", "chat.messages", "chat.messages.readonly", "adwords", "gmail.send", "gmail.readonly", "unknown.future.scope", "cloud-platform", "keep", "admin.directory.user", "photospicker.mediaitems.readonly"} {
		if err := ValidateScopes([]string{"openid", "https://www.googleapis.com/auth/" + scope}); !errors.Is(err, ErrScopeBlocked) {
			t.Errorf("unsafe scope accepted: %s %v", scope, err)
		}
	}

	if ValidateScopes(nil) == nil || ValidateScopes([]string{"https://mail.google.com/"}) == nil {
		t.Fatal("unknown grant accepted")
	}

	for _, service := range []string{"gmail", "googleads", "driveactivity", "admin", "cloudadmin", "keep", "photospicker"} {
		if _, ok := LookupService(service); ok {
			t.Errorf("excluded service accepted: %s", service)
		}
	}
}

func TestHostedConsentCatalogueIsAuditFreeAndImmutable(t *testing.T) {
	if len(services) != 23 {
		t.Fatalf("eligible services=%d, want 23", len(services))
	}

	for name, service := range services {
		scopes := append([]string{"openid"}, service.Scopes...)

		scopes = append(scopes, service.WriteScopes...)
		if err := ValidateScopes(scopes); err != nil {
			t.Errorf("approved service %s rejected: %v", name, err)
		}
	}
	serviceCopy, _ := LookupService("drive")
	serviceCopy.Scopes[0] = "https://www.googleapis.com/auth/drive"

	original, _ := LookupService("drive")
	if original.Scopes[0] != "https://www.googleapis.com/auth/drive.file" {
		t.Fatal("caller changed security catalogue")
	}
}
