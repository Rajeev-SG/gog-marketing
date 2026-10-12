// Package policy holds the hosted-only audit-free consent boundary. Its JSON
// catalogue is shared with the Worker, never inferred from broader CLI scopes.
package policy

import (
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
)

//go:embed services.json
var serviceJSON []byte

//nolint:tagliatelle // Shared Worker consent catalogue preserves the existing camelCase field contract.
type Service struct {
	Label         string   `json:"label"`
	Scopes        []string `json:"scopes"`
	WriteScopes   []string `json:"writeScopes"`
	ResourceModel bool     `json:"resourceModel"`
	Documentation string   `json:"documentation"`
}

var (
	services        = loadServices()
	ErrScopeBlocked = errors.New("scope outside hosted audit-free allowlist")
)

func loadServices() map[string]Service {
	var catalogue map[string]Service
	if err := json.Unmarshal(serviceJSON, &catalogue); err != nil {
		panic("invalid embedded hosted consent catalogue")
	}

	return catalogue
}

func LookupService(name string) (Service, bool) {
	service, ok := services[name]
	service.Scopes = slices.Clone(service.Scopes)
	service.WriteScopes = slices.Clone(service.WriteScopes)

	return service, ok
}

func ValidateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return ErrScopeBlocked
	}
	allowed := map[string]bool{
		"openid": true, "email": true, "profile": true,
		"https://www.googleapis.com/auth/userinfo.email":   true,
		"https://www.googleapis.com/auth/userinfo.profile": true,
	}

	for _, service := range services {
		for _, scope := range service.Scopes {
			allowed[scope] = true
		}

		for _, scope := range service.WriteScopes {
			allowed[scope] = true
		}
	}

	for _, scope := range scopes {
		if !allowed[scope] {
			return ErrScopeBlocked
		}
	}

	return nil
}
