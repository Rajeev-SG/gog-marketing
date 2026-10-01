package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Policy struct {
	Store Store
}

func (p Policy) Allow(ctx context.Context, actor Actor, connectionID, service, resourceID string) error {
	connection, err := p.Store.GetConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	if connection.OrganizationID != actor.OrganizationID {
		return ErrForbidden
	}

	grant, err := p.Store.GetResourceGrant(ctx, actor.OrganizationID, connectionID, resourceID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
			return fmt.Errorf("%w: resource is not exposed by this connection", ErrForbidden)
		}

		return wrapControlPlaneError(err)
	}

	if (service != "" && grant.Service != service) || !grant.Enabled {
		return fmt.Errorf("%w: resource grant is disabled", ErrForbidden)
	}

	return nil
}

// AllowTool applies service/tool policy where a stable resource picker does not
// exist. Resource-backed services must use Allow and their ResourceGrant rows.
func (p Policy) AllowTool(ctx context.Context, actor Actor, connectionID, service, tool string) error {
	connection, err := p.Store.GetConnection(ctx, actor.OrganizationID, connectionID)
	if err != nil {
		return wrapControlPlaneError(err)
	}

	if connection.OrganizationID != actor.OrganizationID || !containsString(connection.Services, service) {
		return ErrForbidden
	}

	definition, err := productService(service)
	if err != nil || definition.ResourceModel || !validProductTool(definition, tool) {
		return ErrForbidden
	}

	grant, ok := connection.ToolGrants[toolGrantKey(service, tool)]
	if !ok {
		grant, ok = connection.ToolGrants[toolGrantKey(service, "*")]
	}

	if !ok || !grant.Enabled || strings.TrimSpace(tool) == "" {
		return fmt.Errorf("%w: tool grant is disabled", ErrForbidden)
	}

	return nil
}

func containsString(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}

	return false
}
