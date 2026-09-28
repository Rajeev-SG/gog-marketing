package cmd

import "testing"

func TestControlPlaneSecurityValidation(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name       string
		listen     string
		backend    string
		project    string
		adminToken string
		wantError  bool
	}{
		{name: "loopback file", listen: "127.0.0.1:8080", backend: "file", adminToken: token},
		{name: "remote file rejected", listen: "0.0.0.0:8080", backend: "file", adminToken: token, wantError: true},
		{name: "remote secret manager", listen: "0.0.0.0:8080", backend: "secret-manager", project: "prod", adminToken: token},
		{name: "secret manager project required", listen: "0.0.0.0:8080", backend: "secret-manager", adminToken: token, wantError: true},
		{name: "backend required", listen: "127.0.0.1:8080", adminToken: token, wantError: true},
		{name: "admin token required", listen: "127.0.0.1:8080", backend: "file", adminToken: "short", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateControlPlaneSecurity(tc.listen, tc.backend, tc.project, tc.adminToken)
			if tc.wantError && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantError && err != nil {
				t.Fatalf("validation failed: %v", err)
			}
		})
	}
}
