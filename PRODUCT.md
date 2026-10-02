# PRODUCT.md — gog-marketing product brief

Durable product truth for anyone building or reviewing the gog-marketing
interface. Pair with [DESIGN.md](DESIGN.md) for the aesthetic system.

## Audience

Marketers and marketing-operations people who manage Google accounts for a
brand or team. They are not developers. They read English product language,
not API terminology, and they care about three questions at all times:

1. What is connected, and is it healthy?
2. What is the agent allowed to see?
3. What changed, and does anything need my attention?

## Job to be done

gog-marketing is the access and permissions layer between an organization's
Google accounts and AI agents. It lets a marketer connect Google accounts,
see which services and assets exist, grant or revoke per-resource access, and
keep that access healthy — without OAuth, scopes, or API detail leaking into
the normal journey.

## Product boundary

- **Superset of standard gog.** The engine keeps the full gog command surface
  (Gmail, Calendar, Drive, Docs, Sheets, Slides, Chat, Contacts, Tasks,
  People, Forms, Meet, Classroom, Apps Script, Workspace administration and
  everything else already in the fork). Marketing access (Google Analytics,
  Google Ads, Google Tag Manager, Search Console, BigQuery) is added on top.
- The UI organizes services into **Workspace** and **Marketing** groups. It
  never presents a five-service allowlist as the whole product.
- Never remove or hide existing standard gog functionality.
- Never reimplement upstream commands; the product UI is a layer over the
  typed gog engine.
- Request/enable only the services and scopes a user actually chooses.
- Use resource-level grants where a safe resource model exists (GA4
  properties, GTM containers, Search Console sites, Ads customers, BigQuery
  projects); otherwise represent access at service/tool level.
- **Not an analytics dashboard.** Product screens show connections, services,
  permissions, health and access configuration — never marketing KPIs or
  customer performance data.
- No invented integrations. Logos and names only for services the repository
  actually supports.
- Product search is scoped to connected accounts, services and asset
  metadata. It never implies search over the underlying marketing data.

## Approved behaviours

- Multiple Google accounts connect independently; credentials and grants
  never mix across accounts.
- Healthy accounts are visually quiet. Reconnect appears only when required.
- Partial service failures never block healthy services.
- Duplicate-account attempts are informational feedback, not errors.
- Access changes persist immediately and are auditable; audit detail stays
  out of the marketer journey unless it is explicitly redacted product copy.
- Advanced technical detail (connection IDs, resource IDs, scopes, JSON) sits
  behind progressive disclosure, never in the primary flow.

## Anti-features (never build these)

- Marketing performance dashboards, KPI numerals, campaign metrics.
- Implying a standard gog service is gone because a render focuses on
  marketing access.
- Fake activity, fake search, fake progress: any shown UI must be backed by
  real product behaviour.
- Showing raw tokens, scopes or internal diagnostics in normal screens.
- Removing existing capabilities from the product architecture.
