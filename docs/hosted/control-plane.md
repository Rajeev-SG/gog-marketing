# Hosted product control plane

The hosted v1 control plane is the Clerk-authenticated Cloudflare Worker surface
for the public gog-marketing product. Identity is always server-verified:
Clerk resolves the session and tenant before the Worker reads or writes D1. The
browser never supplies tenant authority, connection ownership, credential
material, or provider configuration.

## Routes

| Route | Method | Purpose |
| --- | --- | --- |
| `/` | `GET` | Clerk sign-in when unauthenticated; product workspace when authenticated. |
| `/api/google/resources` | `GET` | List the tenant's real Google connections and persisted/discovered assets from D1. |
| `/api/google/grants` | `POST` | Atomically save explicit enable/disable choices. |
| `/api/google/connections/:connectionId/discover` | `POST` | Refresh the trusted credential if needed and run the existing #63 discovery path. |

Connection actions and `POST /api/google/connect` remain documented in
[google-connections](./google-connections.md). The MCP route itself is not
implemented here and remains #65.

### Resource list

`GET /api/google/resources` returns a user-safe projection only:

```json
{
  "connections": [
    {
      "id": "connection-handle",
      "email": "account@example.com",
      "status": "active",
      "services": ["analytics"],
      "discovery": {
        "status": "ok | empty | unavailable | error",
        "detail": "safe-code",
        "resourceCount": 6,
        "checkedAt": "2026-10-06T00:00:00.000Z"
      },
      "resources": [
        {
          "connectionId": "connection-handle",
          "service": "analytics",
          "resourceId": "properties/123",
          "resourceType": "property",
          "displayName": "Example property",
          "enabled": false
        }
      ]
    }
  ]
}
```

The response never includes internal tenant UUIDs, credentials, raw provider
errors, database/provider identifiers beyond the user-facing connection and
resource handles, or inventory that is not persisted.

### Grant save

`POST /api/google/grants` uses a same-origin cookie request with
`Content-Type: application/json` and an `Origin` allowed by the configured
authorized parties:

```json
{
  "connectionId": "connection-handle",
  "grants": [
    {
      "service": "analytics",
      "resourceId": "properties/123",
      "enabled": true
    }
  ]
}
```

The Worker first verifies the connection and every requested resource against
the authenticated tenant's persisted grants. Unknown or foreign handles reject
the complete request before any mutation. Valid updates are committed through
the repository's transactional D1 batch. Send only changed choices; unspecified
resources and failed discovery outcomes retain their existing choices.

### Discovery

`POST /api/google/connections/:connectionId/discover` verifies server-side
connection ownership and identity before using the persisted OAuth refresh path
and the existing private Go/WIF discovery contract. Successful discovery may
persist inventories in bounded native-D1 chunks. Added assets start disabled;
conflicts preserve the current stored choice, while unavailable, empty, and
failed runs are distinct and never erase or expand grants. Service consent
remains canonical, read-only, and incremental through the existing Google
connect flow.

## AI panel and canonical MCP URL

The workspace's "Connect to your AI" panel displays:

```
<canonical origin>/mcp
```

where `<canonical origin>` is derived only from the trusted
`GOG_HOSTED_CANONICAL_ORIGIN` HTTPS configuration. If that operator setting is
missing or invalid, the panel fails visibly and does not infer authority from
`Host`, forwarded headers, or browser hints. The panel includes a copy action
and concise future Codex setup guidance, but does not claim that MCP is
working: endpoint authentication remains #65.

## Safety boundaries

- Clerk identity and tenant status are verified server-side before any
  protected route.
- Cookie-bearing JSON mutations require trusted same-origin JSON; malformed or
  cross-origin requests are rejected without state changes.
- Connections, credentials, grants, and audits are tenant-scoped through D1
  compound ownership.
- Credentials stay encrypted at rest and never enter browser output, logs, or
  audit records.
- New discoveries default to disabled; failed refresh or discovery preserves
  existing enable/disable choices.
