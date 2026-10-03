# DESIGN.md — gog-marketing design system

Aesthetic direction and the component/token contract for the gog-marketing
UI. The approved renders in `docs/ui/renders/` are source material; this
system is the implementation standard. Pair with [PRODUCT.md](PRODUCT.md).

## Aesthetic direction

**Calm enterprise control plane.** Premium, precise B2B software — visually
quiet when healthy, informative when attention is needed. Not an analytics
dashboard, not a generic SaaS template.

- Neutral tinted-gray surfaces on white cards with 1px borders; no
  glassmorphism, no decorative gradients, no neon, no bento grids.
- One action colour (cobalt) plus semantic status colours reserved for
  health. Nothing else is chromatic.
- Typography is a tuned system stack: tight, confident headings; readable
  body; numerals tabular where they line up.
- Dense, list-like rows where users scan many assets; generous breathing
  room on Home and onboarding.
- Subtle depth only where it communicates state (sticky save bar, focus
  rings, toasts). Motion respects `prefers-reduced-motion`.

## Render → implementation map (issue #50)

| Render | Implementation |
| --- | --- |
| 01 application shell | Persistent shell partial: product name, Home, account, sign out. Workspace/Marketing grouping lives in the asset-picker service rail and the services page. |
| 02 home access overview | `home`: overview tiles, account cards, health summary. |
| 03 service card | Reusable service block (name, status, grant meta) in the picker rail and services page. |
| 04 connected account card | Account card on Home with email, state badge, manage/ disconnect. |
| 05 needs attention | Alert treatment on the account card and the assets page with prominent Reconnect. |
| 06 asset picker | Dense service rail + asset rows with search, select all/none, save bar. |
| 07 discovery loading | In-flight button state (`aria-busy`, pending label) while the synchronous discovery POST runs. No fake progress. |
| 08 partial service failure | Alert listing failed services with per-service detail; healthy groups stay usable. |
| 09 advanced diagnostics | Progressive-disclosure panels (connection ID, resource IDs) on the assets page. |
| 10 onboarding completion | `/onboarding/{id}` step checklist computed from real connection state after Google consent. |
| 11 already-connected toast | `ErrConflict` renders as a non-blocking informational toast, not an error. |
| 12 mobile Google data | Responsive picker: rail becomes horizontal status chips, rows stack, save bar sticky. |

## Tokens

Defined once in `internal/controlplane/ui/product.css` as CSS custom
properties. No screen invents its own colour, radius, shadow, or focus style.

- Surfaces: `--surface-page` (tinted neutral), `--surface-card` (white).
- Text: `--text`, `--text-muted`, `--text-faint`.
- Border: `--border`, `--border-strong`.
- Action: `--action` (cobalt), `--action-hover`, `--action-active`.
- Status: `--status-ok`, `--status-warn`, `--status-danger` with soft
  background variants for badges/alerts.
- Radius: `--radius-s` (6px controls), `--radius` (10px cards), `--radius-l`
  (14px panels). Shadow: `--shadow` single subtle elevation.
- Spacing: 4px scale (`--space-1..8`). Focus: `--focus-ring` cobalt ring.

## Component rules

- Buttons: primary (cobalt, one per view), default (bordered), attention
  (reconnect). Minimum 44px touch target on coarse pointers.
- Status is never colour alone: badges carry text labels.
- Cards only where semantically justified; no nested cards. Asset rows are
  bordered list rows, not cards.
- Icons: inline Lucide SVG via the `icon` template function, one stroke
  system, `aria-hidden` by default, never decorative tiles above headings.
- Forms: labels for every input, obvious focus, `:focus-visible` ring that
  sticky UI never covers.
- Toasts: `aria-live="polite"`, informational by default, auto-styled.
- Alerts: `error` (red), `attention` (amber), `info` (neutral/blue).
- Empty and loading states are real components, not blank space.

## Accessibility floor (WCAG 2.2 AA)

Semantic headings/landmarks/forms first; full keyboard operation; visible
focus; contrast checked; status not colour-only; no horizontal overflow at
390px; long account/asset names wrap or truncate with the full value
discoverable; live regions for toasts and in-flight changes.

## Anti-patterns (detector list)

Gratuitous gradients, glass, nested cards, excessive pills/badges, glow
shadows, giant KPI numerals, rounded-square icon tiles, neon/dark-cyberpunk,
card grids where a list belongs, more than one primary action per view,
Tailwind class soup in Go string literals.

## Stack constraints

- Server-rendered Go templates. No React/SPA runtime.
- One static stylesheet embedded with `go:embed`, served at
  `/static/product.css`. No third-party CDN assets in production.
- Icons are inline SVG served from Go (Lucide paths), not an icon font.
- Every material UI batch is verified in a real browser (desktop 1440/1280,
  mobile 390) with the state matrix in `docs/ui/screenshots/`.
