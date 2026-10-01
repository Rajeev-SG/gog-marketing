package controlplane

import (
	"context"
	"errors"
	"fmt"
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
