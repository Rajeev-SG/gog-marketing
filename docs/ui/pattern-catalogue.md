# Pattern catalogue — gog-marketing

Higher-level UX patterns: how components combine to solve a recurring problem.
Use this with [component-catalogue.md](component-catalogue.md) (which component),
[DESIGN.md](../../DESIGN.md) (tokens and visual rules) and
[PRODUCT.md](../../PRODUCT.md) (what the product may and may not do). Finding
IDs refer to the 31 October 2026 audit: `FINDINGS.md` (`C/U/D/N/P/L`) and
`DESIGN-REVIEW.md` (`DR`).

## How to use this

Start from the user problem, not the widget. Find the pattern that matches the
problem, read **Solution** and **Don't use when**, and take its components
from the linked catalogue entries. Before a fix counts as done, check every
finding listed under **Governs**. Sources are ordered by layer. **Decision**
comes first (NN/g, then USWDS and GOV.UK, which state when to use something and
when to choose something else). **Application** comes next (Material 3, Apple
HIG, WAI-ARIA APG). **Mobbin** ([mobbin.com](https://mobbin.com), login needed)
is listed as a place to browse real-product flows by name. It wasn't fetched.
The [Finding → pattern index](#finding--pattern-index) at the end maps every
audit ID to its governing pattern.

> **Decision rule:** prefer the conventional pattern that users already know
> from other products. Deviate only with a written reason in `DESIGN.md`
> (pattern, deviation, why, and how it was validated). Product constraints from
> PRODUCT.md win over convention. For example, there's no fake progress and no
> marketing KPIs, even where a "standard dashboard" would show them.

### Index

Navigation & structure: [Global navigation](#global-navigation-and-app-shell) ·
[Mobile nav: bottom bar vs drawer](#mobile-navigation-bottom-bar-vs-drawer) ·
[Breadcrumbs & wayfinding](#breadcrumbs-and-wayfinding) ·
[IA & splitting long pages](#information-architecture-and-splitting-long-pages) ·
[Consistent component language](#consistent-component-language-across-areas)
— Finding & input: [Search & filtering](#search-and-filtering) ·
[Forms & validation](#forms-and-validation) ·
[Progressive disclosure](#progressive-disclosure) ·
[Multi-step workflows](#multi-step-workflows--wizards)
— First use: [Onboarding & setup checklists](#onboarding-and-setup-checklists) ·
[Empty states](#empty-states)
— Feedback & status: [Loading](#loading-and-skeletons) ·
[Errors](#error-recovery-and-error-messages) ·
[Rate limits & quotas](#rate-limit-and-quota-communication) ·
[Toasts vs alerts vs inline](#toasts-vs-alerts-vs-inline-messages) ·
[Status & health](#status-and-health-indication)
— Safety: [Destructive actions & undo](#destructive-action-confirmation-and-undo) ·
[Unsaved changes](#unsaved-changes-protection) ·
[Permissions & consent](#permission-and-consent-management) ·
[Account switching & context](#account-switching-and-context-indication)
— Overview: [Dashboards](#dashboards-and-at-a-glance-overviews) ·
[Tables vs lists](#data-tables-vs-lists) ·
[Activity & audit history](#activity-and-audit-history) ·
[Settings](#settings-pages)
— Support & recovery: [Help & support](#help-and-support) ·
[404 & dead ends](#404-and-dead-end-recovery) ·
[Plain-language copy](#plain-language-ui-copy)
— Surfaces: [Chat / conversational UI](#chat--conversational-ui) ·
[Authentication & sign-in](#authentication-and-sign-in) ·
[Responsive & mobile](#responsive-and-mobile-adaptation)

---

## Navigation and structure

### Global navigation and app shell
- **Problem:** Users need to know where they are and how to reach the main
  areas. Labels must match the destinations they lead to.
- **Solution:** Use one shared shell on every signed-in route: product name
  linking Home, visible primary navigation with `aria-current`, global search,
  Help, and the account (avatar) menu last. Label each item with its
  destination's page title. Because a link is a promise, "Activity" must open
  activity. Keep Help (support) and Settings (configuration) as separate
  destinations. Mark the active item with more than colour.
- **Don't use when:** You have only one or two pages; a header with the
  product name is enough there. Never hide navigation behind a hamburger on
  desktop.
- **Alternatives:** Side navigation (desktop app) and a top bar (public pages).
  Use a navigation rail at medium widths.
- **Components:** [Header](component-catalogue.md#header) ·
  [Navigation](component-catalogue.md#navigation) ·
  [Link](component-catalogue.md#link) ·
  [Dropdown menu](component-catalogue.md#dropdown-menu) (account menu) ·
  [Avatar](component-catalogue.md#avatar) ·
  [Skip link](component-catalogue.md#skip-link) ·
  [Search input](component-catalogue.md#search-input)
- **Sources:** [NN/g Menu-design checklist](https://www.nngroup.com/articles/menu-design/) ·
  [NN/g A link is a promise](https://www.nngroup.com/articles/link-promise/) ·
  [NN/g Hamburger menus hurt UX metrics](https://www.nngroup.com/articles/hamburger-menus/) ·
  [USWDS Header](https://designsystem.digital.gov/components/header/) ·
  [USWDS Side navigation](https://designsystem.digital.gov/components/side-navigation/) ·
  [GOV.UK Service navigation](https://design-system.service.gov.uk/components/service-navigation/) ·
  [M3 Navigation rail](https://m3.material.io/components/navigation-rail/overview) ·
  [HIG Sidebars](https://developer.apple.com/design/human-interface-guidelines/sidebars) ·
  Mobbin: browse "Sidebar navigation" / "Account menu" in SaaS web apps
- **Governs:** N01, N02, L09, L11, DR01, P02

### Mobile navigation: bottom bar vs drawer
- **Problem:** Small screens can't show side navigation. Two competing mobile
  nav systems confuse users and double the bugs.
- **Solution:** With 3–5 top-level destinations, use **one bottom navigation
  bar** with icon and text label, ≥44px targets and `aria-current`. Use a
  drawer only for secondary, contextual content, such as the `/chat`
  conversation list. Every drawer is the same modal component: focus moves in
  and is trapped, Escape, Close × and scrim click all close it, and focus
  returns to the trigger.
- **Don't use when:** Avoid a bottom bar when there are more than 5 peer
  destinations; use a drawer or a "More" item. Avoid a drawer when the bottom
  bar already shows the same items.
- **Alternatives:** A visible top tab strip (few sections) or a hamburger menu
  (many browse-only categories; NN/g says it's acceptable on mobile only).
- **Components:** [Navigation](component-catalogue.md#navigation) ·
  [Drawer](component-catalogue.md#drawer) ·
  [Tabs](component-catalogue.md#tabs) ·
  [Icon](component-catalogue.md#icon)
- **Sources:** [NN/g Basic patterns for mobile navigation](https://www.nngroup.com/articles/mobile-navigation-patterns/) ·
  [NN/g Hamburger icon recognizability](https://www.nngroup.com/articles/hamburger-menu-icon-recognizability/) ·
  [NN/g Bottom sheets](https://www.nngroup.com/articles/bottom-sheet/) ·
  [M3 Navigation bar](https://m3.material.io/components/navigation-bar/overview) ·
  [M3 Navigation drawer](https://m3.material.io/components/navigation-drawer/overview) ·
  [HIG Tab bars](https://developer.apple.com/design/human-interface-guidelines/tab-bars) ·
  [APG Dialog (modal)](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/) ·
  Mobbin: browse "Tab bar" and "Navigation drawer" on iOS/Android
- **Governs:** L13, N03, L12, N04 (mobile bottom sheet)

### Breadcrumbs and wayfinding
- **Problem:** Users who land on an interior view need to know where it sits and
  how to go up a level.
- **Solution:** Use breadcrumbs **only** in hierarchies three or more levels
  deep. For a flat app, use the page title, `aria-current` in the nav, and a
  back link on drill-in views (account → service → assets). Name the current
  object in the page header.
- **Don't use when:** The site is shallow, the main nav already shows
  context, or on landing pages (USWDS).
- **Alternatives:** Back link, local (section) navigation, tabs.
- **Components:** [Breadcrumbs](component-catalogue.md#breadcrumbs) ·
  [Link](component-catalogue.md#link) ·
  [Heading](component-catalogue.md#heading) ·
  [Navigation](component-catalogue.md#navigation)
- **Sources:** [NN/g Breadcrumbs: 11 guidelines](https://www.nngroup.com/articles/breadcrumbs/) ·
  [NN/g Local navigation](https://www.nngroup.com/articles/local-navigation/) ·
  [USWDS Breadcrumb](https://designsystem.digital.gov/components/breadcrumb/) ·
  [GOV.UK Back link](https://design-system.service.gov.uk/components/back-link/) ·
  [HIG Path controls](https://developer.apple.com/design/human-interface-guidelines/path-controls)
- **Governs:** D05 (post-split drill-ins), D01 (name the object in the header)

### Information architecture and splitting long pages
- **Problem:** One long page that holds every job is hard to orient in.
  Duplicate sections make users wonder whether they differ.
- **Solution:** Group by user task (PRODUCT.md's three questions) into real
  routes: **Home** (status at a glance), **Google data** (accounts, services,
  assets), **Tools** (AI tool permissions), **Connect AI** (MCP setup), plus
  Activity and Help. Give each object one canonical place, so there's a single
  account list. Use in-page navigation only when a page still has three or more
  sections longer than about three viewports (USWDS).
- **Don't use when:** Don't split content that users must compare or
  complete together; keep it on one page with headings.
- **Alternatives:** In-page navigation or table of contents, tabs (peer
  panels), accordions (rarely needed sections).
- **Components:** [Navigation](component-catalogue.md#navigation) ·
  [Tabs](component-catalogue.md#tabs) ·
  [Heading](component-catalogue.md#heading) ·
  [Accordion](component-catalogue.md#accordion)
- **Sources:** [NN/g IA vs navigation](https://www.nngroup.com/articles/ia-vs-navigation/) ·
  [NN/g Flat vs deep hierarchies](https://www.nngroup.com/articles/flat-vs-deep-hierarchy/) ·
  [NN/g Table of contents](https://www.nngroup.com/articles/table-of-contents/) ·
  [USWDS In-page navigation](https://designsystem.digital.gov/components/in-page-navigation/) ·
  [M3 Canonical layouts: list-detail](https://m3.material.io/foundations/layout/canonical-examples/list-detail) ·
  [HIG Layout](https://developer.apple.com/design/human-interface-guidelines/layout)
- **Governs:** D05, D03 (Tools as its own page), N01, N02

### Consistent component language across areas
- **Problem:** Home and Chat use different buttons, headers, radii and type
  scales, so moving between them feels like changing products. That erodes
  trust in a permissions product.
- **Solution:** Use one token set and one implementation per component, shared
  by every route. There's one Button set (primary / secondary / ghost / danger),
  one AppHeader, a 6-step type scale with 3 weights, the 6/10/999 radius scale,
  one eyebrow style used about once per screen, and blue reserved for links and
  primary actions. Native controls are either styled to the system or replaced
  by catalogue components.
- **Don't use when:** Never skip this. A deliberate per-area variant needs a
  DESIGN.md entry.
- **Alternatives:** —
- **Components:** [Button](component-catalogue.md#button) ·
  [Header](component-catalogue.md#header) ·
  [Heading](component-catalogue.md#heading) ·
  [Badge](component-catalogue.md#badge) ·
  [Stack](component-catalogue.md#stack)
- **Sources:** [NN/g 10 usability heuristics (#4 consistency and standards)](https://www.nngroup.com/articles/ten-usability-heuristics/) ·
  [M3 Design tokens](https://m3.material.io/foundations/design-tokens/overview) ·
  [M3 Interaction states](https://m3.material.io/foundations/interaction/states/overview) ·
  [HIG Typography](https://developer.apple.com/design/human-interface-guidelines/typography) ·
  [USWDS Typography](https://designsystem.digital.gov/components/typography/)
- **Governs:** DR01, DR02, DR04, DR06, DR08, DR11, DR13, L09

---

## Finding and input

### Search and filtering
- **Problem:** Users need to narrow many accounts, services, assets or tools,
  and always know whether a filter is on.
- **Solution:** Use one visible search box with a stated scope (accounts,
  services and asset metadata, never marketing data). While filtering, show
  "N of M" counts everywhere the totals appear. Show the active query as a chip
  with **Clear**, and announce result counts with `role=status`. A no-match
  empty state names the query and offers Clear. Use batch filters with an Apply
  button when many filters are combined. Use instant filtering when it's fast
  and single. Don't silently restore a stale query after reload, or if you do,
  show it.
- **Don't use when:** The list is short (about 7 items or fewer), where
  scanning is faster.
- **Alternatives:** Facets (multi-attribute filtering), segmented filter (All /
  Reads / Changes), sort.
- **Components:** [Search input](component-catalogue.md#search-input) ·
  [Segmented control](component-catalogue.md#segmented-control) ·
  [Badge](component-catalogue.md#badge) (filter chip) ·
  [Empty state](component-catalogue.md#empty-state) ·
  [Visually hidden](component-catalogue.md#visually-hidden)
- **Sources:** [NN/g Search: visible and simple](https://www.nngroup.com/articles/search-visible-and-simple/) ·
  [NN/g Filters vs facets](https://www.nngroup.com/articles/filters-vs-facets/) ·
  [NN/g User intent affects filter design](https://www.nngroup.com/articles/applying-filters/) ·
  [NN/g Filter categories and values](https://www.nngroup.com/articles/filter-categories-values/) ·
  [USWDS Search](https://designsystem.digital.gov/components/search/) ·
  [M3 Search](https://m3.material.io/components/search/overview) ·
  [HIG Searching](https://developer.apple.com/design/human-interface-guidelines/searching) ·
  Mobbin: browse "Search results" / "Filter" flows
- **Governs:** L06, D03 (tool search, default filter to connected services), L12 (chip size), L09 (search missing on /chat)

### Forms and validation
- **Problem:** Users must enter or choose data correctly without being scolded
  or losing work.
- **Solution:** Use visible labels outside fields, hints outside fields, and
  mark required fields. Validate on submit, or on blur for format, never while
  the user is still typing. On error, show an **error summary** at the top that
  links to each field, plus an inline message next to the field (red with icon
  and text). Messages say what's wrong and how to fix it. Use `novalidate` with
  custom messages instead of native browser bubbles. Group related choices in
  fieldsets. Make whole-row labels at least 44px. Use one primary submit per
  form.
- **Don't use when:** The setting applies instantly; use a toggle with no
  Submit there. Avoid a form for a single action; use a button.
- **Alternatives:** Toggles (instant settings), wizard (long or branching
  input), inline edit.
- **Components:** [Form](component-catalogue.md#form) ·
  [Label](component-catalogue.md#label) ·
  [Text input](component-catalogue.md#text-input) ·
  [Checkbox](component-catalogue.md#checkbox) ·
  [Radio button](component-catalogue.md#radio-button) ·
  [Fieldset](component-catalogue.md#fieldset) ·
  [Alert](component-catalogue.md#alert) (error summary) ·
  [Button group](component-catalogue.md#button-group)
- **Sources:** [NN/g Website forms usability: top 10](https://www.nngroup.com/articles/web-form-design/) ·
  [NN/g 10 guidelines for reporting errors in forms](https://www.nngroup.com/articles/errors-forms-design-guidelines/) ·
  [NN/g Placeholders are harmful](https://www.nngroup.com/articles/form-design-placeholders/) ·
  [NN/g Hostile error messages](https://www.nngroup.com/articles/hostile-error-messages/) ·
  [NN/g Reduce cognitive load in forms](https://www.nngroup.com/articles/4-principles-reduce-cognitive-load/) ·
  [USWDS Form](https://designsystem.digital.gov/components/form/) ·
  [USWDS Validation](https://designsystem.digital.gov/components/validation/) ·
  [GOV.UK Validation](https://design-system.service.gov.uk/patterns/validation/) ·
  [GOV.UK Error summary](https://design-system.service.gov.uk/components/error-summary/) ·
  [M3 Text fields](https://m3.material.io/components/text-fields/overview) ·
  [HIG Entering data](https://developer.apple.com/design/human-interface-guidelines/entering-data) ·
  [W3C WAI Forms tutorial](https://www.w3.org/WAI/tutorials/forms/)
- **Governs:** L07 (native bubble), L12, D03 (two Save buttons), DR09 (footer Cancel/Save), DR13 (spacing)

### Progressive disclosure
- **Problem:** Advanced or rare details (IDs, scopes, provider names, limits)
  overwhelm the main journey but must stay reachable.
- **Solution:** Show the common path first and put advanced items behind a
  clearly labelled disclosure ("Technical details", "Model limits"). Make the
  existence of the hidden content visible. One level of disclosure is usually
  enough. Default to the subset the user actually has, for example tools for
  connected services only, with "Show all".
- **Don't use when:** Users need the content to complete the task, or most
  users would open it anyway (NN/g, USWDS Accordion).
- **Alternatives:** A separate secondary page (staged disclosure), popover,
  wizard.
- **Components:** [Accordion](component-catalogue.md#accordion) ·
  [Popover](component-catalogue.md#popover) ·
  [Tooltip](component-catalogue.md#tooltip) ·
  [Drawer](component-catalogue.md#drawer)
- **Sources:** [NN/g Progressive disclosure](https://www.nngroup.com/articles/progressive-disclosure/) ·
  [NN/g Accordions on desktop](https://www.nngroup.com/articles/accordions-on-desktop/) ·
  [USWDS Accordion](https://designsystem.digital.gov/components/accordion/) ·
  [GOV.UK Details](https://design-system.service.gov.uk/components/details/) ·
  [HIG Disclosure controls](https://developer.apple.com/design/human-interface-guidelines/disclosure-controls)
- **Governs:** D03, C05, C06, render 09 diagnostics, PRODUCT.md "advanced detail behind disclosure"

### Multi-step workflows / wizards
- **Problem:** An occasional, multi-part task with dependencies, such as
  connecting an account (choose services → Google consent → discover → choose
  assets), can get users lost.
- **Solution:** Use a linear sequence with a step indicator for three or more
  steps, Back / Continue, state preserved on Back, and a review step before
  commit ("Check answers"). Hide branches that don't apply. Finish on a
  confirmation page that says what happened and what's next. External steps
  (the Google OAuth redirect) return to the same step with state intact.
- **Don't use when:** The steps can be done in any order (use a task list).
  Avoid it when the step count varies with input (USWDS), when the form is
  very short, or when expert users repeat the task often.
- **Alternatives:** Task list (non-linear), single form with sections,
  progressive disclosure.
- **Components:** [Progress indicator](component-catalogue.md#progress-indicator) ·
  [Button group](component-catalogue.md#button-group) ·
  [Form](component-catalogue.md#form) ·
  [List](component-catalogue.md#list) (summary)
- **Sources:** [NN/g Wizards: definition and recommendations](https://www.nngroup.com/articles/wizards/) ·
  [USWDS Step indicator](https://designsystem.digital.gov/components/step-indicator/) ·
  [USWDS Complete a complex form](https://designsystem.digital.gov/patterns/complete-a-complex-form/) ·
  [USWDS Progress easily](https://designsystem.digital.gov/patterns/complete-a-complex-form/progress-easily/) ·
  [GOV.UK Question pages](https://design-system.service.gov.uk/patterns/question-pages/) ·
  [GOV.UK Check answers](https://design-system.service.gov.uk/patterns/check-answers/) ·
  [GOV.UK Confirmation pages](https://design-system.service.gov.uk/patterns/confirmation-pages/) ·
  Mobbin: browse "Connect account" / "Integration setup" flows
- **Governs:** Connect Google account flow (render 10). It wasn't tested in the
  audit, so validate it when it's built.

---

## First use

### Onboarding and setup checklists
- **Problem:** New users need to finish setup (connect an account, choose data,
  connect AI) without a tutorial that gets in the way.
- **Solution:** Use a **task-list checklist** computed from real state, with
  each item marked by text status ("Done" / "Not started") and a link to do it.
  Show it until setup is complete, then replace it with a short "You're set up —
  open chat" line or remove it. Keep it compact and secondary to the at-a-glance
  status (it shouldn't outweigh the H1). If it's per-account, name the account.
  Teach in context (pull revelations) rather than with up-front tours.
- **Don't use when:** Setup is a single step (use an empty-state CTA). Never
  show a "set up" message when everything is done.
- **Alternatives:** Empty-state CTAs, contextual tips, wizard (strictly linear
  setup).
- **Components:** [Progress indicator](component-catalogue.md#progress-indicator) (task list) ·
  [Alert](component-catalogue.md#alert) (inline banner) ·
  [Empty state](component-catalogue.md#empty-state) ·
  [Link](component-catalogue.md#link)
- **Sources:** [NN/g Onboarding tutorials vs contextual help](https://www.nngroup.com/articles/onboarding-tutorials/) ·
  [NN/g Mobile-app onboarding components](https://www.nngroup.com/articles/mobile-app-onboarding/) ·
  [NN/g 8 guidelines for complex applications](https://www.nngroup.com/articles/complex-application-design/) ·
  [GOV.UK Task list](https://design-system.service.gov.uk/components/task-list/) ·
  [GOV.UK Complete multiple tasks](https://design-system.service.gov.uk/patterns/complete-multiple-tasks/) ·
  [HIG Onboarding](https://developer.apple.com/design/human-interface-guidelines/onboarding) ·
  Mobbin: browse "Onboarding checklist" / "Getting started" in SaaS apps
- **Governs:** L01, DR03, DR10 (two competing primaries), D01 (account-specific next step)

### Empty states
- **Problem:** A blank region looks broken and doesn't tell users why it's empty
  or what to do.
- **Solution:** Design each empty state on purpose. Use a short heading, one
  sentence of cause, and one action. Distinguish the cases: **first use**
  ("No assets yet · Check assets"), **no match** ("No assets match 'x' · Clear
  search"), **nothing to do** ("Connected · nothing to choose yet", neutral
  styling) and **done**. Every count and status on screen must agree with the
  empty state. Never show stale totals next to "nothing found".
- **Don't use when:** Data is still loading (show loading instead).
- **Alternatives:** Starter content or suggestions (for example chat prompt
  suggestions), sample templates.
- **Components:** [Empty state](component-catalogue.md#empty-state) ·
  [Button](component-catalogue.md#button) ·
  [Badge](component-catalogue.md#badge) (neutral)
- **Sources:** [NN/g Designing empty states in complex applications](https://www.nngroup.com/articles/empty-state-interface-design/) ·
  [NN/g Visibility of system status](https://www.nngroup.com/articles/visibility-system-status/) ·
  [HIG Loading](https://developer.apple.com/design/human-interface-guidelines/loading) ·
  Mobbin: browse "Empty state"
- **Governs:** U03, L06, L05, L01, C04, C07 (don't create empty threads)

---

## Feedback and status

### Loading and skeletons
- **Problem:** During waits, users can't tell whether the system is working, and
  they double-submit.
- **Solution:** Pick the indicator by expected duration (NN/g): under 1s show
  nothing; 1–10s use an in-place spinner or busy button (`aria-busy`, pending
  label "Checking assets…"); a known-layout region load uses a skeleton; over
  10s use a determinate progress bar or step text. Disable re-submit while in
  flight. Show **no fake progress** (PRODUCT.md). Honour reduced motion.
- **Don't use when:** A skeleton is wrong for loads under 1s, and a spinner
  is wrong for unbounded waits over 10s without explanation.
- **Alternatives:** Optimistic UI with undo (for fast, reversible writes), and
  streaming partial results (chat).
- **Components:** [Spinner](component-catalogue.md#spinner) ·
  [Skeleton](component-catalogue.md#skeleton) ·
  [Progress bar](component-catalogue.md#progress-bar) ·
  [Button](component-catalogue.md#button) (busy state)
- **Sources:** [NN/g Response-time limits](https://www.nngroup.com/articles/response-times-3-important-limits/) ·
  [NN/g Progress indicators](https://www.nngroup.com/articles/progress-indicators/) ·
  [NN/g Skeleton screens 101](https://www.nngroup.com/articles/skeleton-screens/) ·
  [M3 Loading indicator](https://m3.material.io/components/loading-indicator/overview) ·
  [M3 Progress indicators](https://m3.material.io/components/progress-indicators/overview) ·
  [HIG Loading](https://developer.apple.com/design/human-interface-guidelines/loading)
- **Governs:** render 07 (discovery in-flight), chat streaming state (keep it; the audit says it works)

### Error recovery and error messages
- **Problem:** When something fails, users need to know what happened, whose
  problem it is, and how to recover.
- **Solution:** Show the error **where it happened** (inline, or as the failed
  chat turn), once, in error styling with an icon and text. Write it in plain
  language: what happened, why if it's known, and the next step (Retry,
  Reconnect, try later at a stated time). Keep the user's input. Partial
  failure lists only what failed, and healthy parts stay usable. Never put error
  text in success styling, and never duplicate the same message.
- **Don't use when:** The condition isn't an error. For example,
  "already connected" is informational (toast), and "nothing to choose" is a
  neutral state.
- **Alternatives:** Prevention (constraints, confirmation, disabled with a
  reason), which NN/g rates above any message.
- **Components:** [Alert](component-catalogue.md#alert) ·
  [Button](component-catalogue.md#button) (Retry) ·
  [Toast](component-catalogue.md#toast) (non-errors only) ·
  [Form](component-catalogue.md#form) (error summary)
- **Sources:** [NN/g Error-message guidelines](https://www.nngroup.com/articles/error-message-guidelines/) ·
  [NN/g Error-messages scoring rubric](https://www.nngroup.com/articles/error-messages-scoring-rubric/) ·
  [NN/g Hostile error messages](https://www.nngroup.com/articles/hostile-error-messages/) ·
  [NN/g Preventing slips](https://www.nngroup.com/articles/slips/) ·
  [NN/g Preventing mistakes](https://www.nngroup.com/articles/user-mistakes/) ·
  [USWDS Alert](https://designsystem.digital.gov/components/alert/) ·
  [GOV.UK Problem with the service pages](https://design-system.service.gov.uk/patterns/problem-with-the-service-pages/) ·
  [HIG Feedback](https://developer.apple.com/design/human-interface-guidelines/feedback)
- **Governs:** C01, C03, render 05 / 08 (needs attention, partial failure), L07 (sign-in errors)

### Rate-limit and quota communication
- **Problem:** Users hit a usage limit with no warning and no idea whose limit it
  is or when it resets. The flagship feature becomes a dead end.
- **Solution:** Make the quota **visible before the limit**: "You've used X of Y
  today · resets at HH:MM" (meter plus text) in the model popover, and warn at
  about 80%. When the limit is reached, **disable Send before typing** and say
  why, whose limit it is (yours or the shared service) and when it resets.
  Offer what's still possible (another available model, or come back at the
  stated time). Render a limit hit mid-conversation as the assistant turn with
  Retry, and never persist empty assistant turns. If the limit is service-wide,
  show a site-level notice. Hide provider IDs (`openrouter/free`).
- **Don't use when:** Limits are effectively unreachable. Don't add
  anxiety-inducing meters for limits that are never hit. USWDS Character
  count's guidance applies: if users keep hitting a backend limit, raise the
  limit rather than decorate it.
- **Alternatives:** A queue with ETA, or degrading to a lighter model with
  disclosure.
- **Components:** [Progress bar](component-catalogue.md#progress-bar) (meter) ·
  [Popover](component-catalogue.md#popover) ·
  [Alert](component-catalogue.md#alert) ·
  [Button](component-catalogue.md#button) (disabled with reason) ·
  [Select](component-catalogue.md#select) / [Dropdown menu](component-catalogue.md#dropdown-menu) (model)
- **Sources:** [NN/g Visibility of system status](https://www.nngroup.com/articles/visibility-system-status/) ·
  [NN/g Error-message guidelines (offer remedies, state when available)](https://www.nngroup.com/articles/error-message-guidelines/) ·
  [NN/g 10 heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/) ·
  [USWDS Character count (backend-limit guidance)](https://designsystem.digital.gov/components/character-count/) ·
  [USWDS Site alert](https://designsystem.digital.gov/components/site-alert/) ·
  [GOV.UK Service unavailable pages](https://design-system.service.gov.uk/patterns/service-unavailable-pages/) ·
  [APG Meter](https://www.w3.org/WAI/ARIA/apg/patterns/meter/) ·
  [HIG Gauges](https://developer.apple.com/design/human-interface-guidelines/gauges) ·
  Mobbin: browse "Usage limit" / "Quota" in AI chat apps
  *(NN/g has no dedicated rate-limit article; the heuristics above apply)*
- **Governs:** C01, C05, C03, DR06

### Toasts vs alerts vs inline messages
- **Problem:** Picking the wrong message surface either interrupts users or lets
  important messages vanish.
- **Solution:** Use this decision table.

  | Message | Surface | Persistence | ARIA |
  |---|---|---|---|
  | Field problem | Inline field message (+ error summary) | Until fixed | linked via `aria-describedby` |
  | Result of the user's action, non-critical ("Saved", "Already connected") | **Toast**, optionally with Undo | Auto-dismiss (kept if it has an action) | `role=status` |
  | Error or attention in a region (reconnect, partial failure, limit reached) | **Inline alert** in that region | Until resolved | `role=alert` (urgent) / `status` |
  | Service-wide condition not caused by the user | **Site alert / notification banner** | Every page until resolved | `status` |
  | Decision needed before continuing (destroy, lose work) | **Modal dialog** | Blocking | `alertdialog` |
  | Ongoing state (healthy, connected) | **Status indicator** (dot + text) | Persistent | — |
- **Don't use when:** Never use toasts for errors (NN/g). Never use modals for
  routine success or errors (USWDS). Never show the same message in two
  surfaces.
- **Alternatives:** —
- **Components:** [Toast](component-catalogue.md#toast) ·
  [Alert](component-catalogue.md#alert) ·
  [Modal](component-catalogue.md#modal) ·
  [Badge](component-catalogue.md#badge)
- **Sources:** [NN/g Indicators, validations, notifications](https://www.nngroup.com/articles/indicators-validations-notifications/) ·
  [NN/g Modal & nonmodal dialogs](https://www.nngroup.com/articles/modal-nonmodal-dialog/) ·
  [USWDS Alert](https://designsystem.digital.gov/components/alert/) ·
  [USWDS Site alert](https://designsystem.digital.gov/components/site-alert/) ·
  [USWDS Modal](https://designsystem.digital.gov/components/modal/) ·
  [GOV.UK Notification banner](https://design-system.service.gov.uk/components/notification-banner/) ·
  [M3 Snackbar](https://m3.material.io/components/snackbar/overview) ·
  [M3 Notifications (content design)](https://m3.material.io/foundations/content-design/notifications) ·
  [HIG Managing notifications](https://developer.apple.com/design/human-interface-guidelines/managing-notifications) ·
  [WCAG 4.1.3](https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html)
- **Governs:** C03, C06, render 11 (already-connected toast), DR13 (banner width)

### Status and health indication
- **Problem:** When every row shows the same green "Connected", the one that
  needs attention is invisible, and numbers that contradict each other destroy
  trust.
- **Solution:** **Quiet when healthy, loud when not** (DESIGN.md). Show status
  text only where it differs from the norm, and use a single summary line
  ("All services healthy") for the rest. Pair every colour with a text label.
  Counts are computed from the same source as the detail, pluralised properly,
  and scoped to the visible account. Reconnect appears only when it's needed.
- **Don't use when:** Status doesn't change (don't badge static facts).
- **Alternatives:** A summary row plus an exceptions list.
- **Components:** [Badge](component-catalogue.md#badge) ·
  [Alert](component-catalogue.md#alert) ·
  [Icon](component-catalogue.md#icon) ·
  [List](component-catalogue.md#list)
- **Sources:** [NN/g Visibility of system status](https://www.nngroup.com/articles/visibility-system-status/) ·
  [NN/g Indicators, validations, notifications](https://www.nngroup.com/articles/indicators-validations-notifications/) ·
  [USWDS Tag (when not to use)](https://designsystem.digital.gov/components/tag/) ·
  [WCAG 1.4.1 Use of color](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html) ·
  [HIG Feedback](https://developer.apple.com/design/human-interface-guidelines/feedback)
- **Governs:** U03, D02, DR07, L05, L02, C03, DR09 (grey vs green dot)

---

## Safety

### Destructive-action confirmation and undo
- **Problem:** Disconnecting an account or deleting a conversation is costly to
  get wrong, especially when it appears out of context.
- **Solution:** Pick one by reversibility. **Reversible** actions happen
  immediately and offer **Undo** in a toast. **Irreversible or high-impact**
  actions get a confirmation dialog that names the object ("Disconnect
  rajeev@singulyr.com?") and states the consequence (what access the AI loses).
  The confirm button is a specific danger-styled verb. Cancel comes first and
  holds the default focus. Keep destructive items away from benign ones (a
  separator, last in the menu). Reserve type-to-confirm for the most serious
  cases only.
- **Don't use when:** Avoid confirmation for routine, reversible actions;
  confirmation fatigue makes users click through (NN/g).
- **Alternatives:** Undo, trash or archive with restore, delayed execution.
- **Components:** [Modal](component-catalogue.md#modal) ·
  [Button](component-catalogue.md#button) (danger) ·
  [Toast](component-catalogue.md#toast) (Undo) ·
  [Dropdown menu](component-catalogue.md#dropdown-menu) ·
  [Separator](component-catalogue.md#separator)
- **Sources:** [NN/g Confirmation dialogs](https://www.nngroup.com/articles/confirmation-dialog/) ·
  [NN/g Consequential options close to benign](https://www.nngroup.com/articles/proximity-consequential-options/) ·
  [NN/g User control and freedom](https://www.nngroup.com/articles/user-control-and-freedom/) ·
  [NN/g Preventing mistakes](https://www.nngroup.com/articles/user-mistakes/) ·
  [USWDS Modal (confirmation)](https://designsystem.digital.gov/components/modal/) ·
  [USWDS Alert (destructive actions → modal)](https://designsystem.digital.gov/components/alert/) ·
  [M3 Dialogs](https://m3.material.io/components/dialogs/overview) ·
  [HIG Undo and redo](https://developer.apple.com/design/human-interface-guidelines/undo-and-redo) ·
  [HIG Action sheets](https://developer.apple.com/design/human-interface-guidelines/action-sheets)
- **Governs:** N04, L10, C07 (Delete enabled on an empty thread)

### Unsaved-changes protection
- **Problem:** Users toggle permissions, then navigate away or use a second Save
  button and lose changes silently.
- **Solution:** Choose **one save model per surface**. Either (a) changes apply
  instantly, with toggles, a "Saved" toast and Undo, or (b) changes are batched,
  with a sticky save bar ("3 changes · Cancel · Save") attached to the panel.
  For (b), warn on in-app navigation and `beforeunload` while dirty, and
  distinguish Close (keep the draft) from Cancel (discard). Never have two Save
  buttons for one mental task. The sticky bar must not cover the focused
  element.
- **Don't use when:** Changes auto-save. Don't add a "you have unsaved
  changes" dialog there.
- **Alternatives:** Auto-save with version history, drafts.
- **Components:** [Button group](component-catalogue.md#button-group) (save bar) ·
  [Toggle](component-catalogue.md#toggle) ·
  [Modal](component-catalogue.md#modal) (leave-page warning) ·
  [Toast](component-catalogue.md#toast)
- **Sources:** [NN/g Cancel vs Close](https://www.nngroup.com/articles/cancel-vs-close/) ·
  [NN/g User control and freedom](https://www.nngroup.com/articles/user-control-and-freedom/) ·
  [NN/g Toggle-switch guidelines (instant effect)](https://www.nngroup.com/articles/toggle-switch-guidelines/) ·
  [USWDS Modal ("Your work isn't saved…")](https://designsystem.digital.gov/components/modal/) ·
  [USWDS Keep a record](https://designsystem.digital.gov/patterns/complete-a-complex-form/keep-a-record/) ·
  [MDN beforeunload](https://developer.mozilla.org/en-US/docs/Web/API/Window/beforeunload_event) ·
  [WCAG 2.4.11 Focus not obscured](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html)
- **Governs:** D03, DR09. The existing dirty-navigation warning (docs/ui/verification.md) must cover the tools panel too.

### Permission and consent management
- **Problem:** Marketers must grant AI agents access per service and resource,
  understand what is granted, and change it safely. OAuth detail must not leak
  into that journey.
- **Solution:** Use **incremental, purpose-explained consent**. The user picks
  services (Workspace / Marketing groups), the app explains why in plain
  language, and then requests only those scopes (Google incremental
  authorization). Afterwards, show a per-account summary of what's granted:
  read vs write separated, and "Tools that can edit" called out. Grant at
  resource level where a safe model exists. Use one taxonomy across services,
  consent and tools, so every tool group maps to a service the user can see.
  Revoke is always available and confirmed. Changes are auditable (see
  Activity). Scopes and IDs sit behind disclosure.
- **Don't use when:** Avoid asking for permissions up front "just in case",
  and avoid dark-pattern pre-checks.
- **Alternatives:** Role presets (Read-only / Editor) that expand to detail.
- **Components:** [Fieldset](component-catalogue.md#fieldset) ·
  [Checkbox](component-catalogue.md#checkbox) /
  [Toggle](component-catalogue.md#toggle) ·
  [Accordion](component-catalogue.md#accordion) ·
  [Segmented control](component-catalogue.md#segmented-control) (All / Reads / Changes) ·
  [Modal](component-catalogue.md#modal) (revoke)
- **Sources:** [NN/g Mobile-app permission requests](https://www.nngroup.com/articles/permission-requests/) ·
  [NN/g Cookie permissions 101](https://www.nngroup.com/articles/cookie-permissions/) ·
  [NN/g Progressive disclosure](https://www.nngroup.com/articles/progressive-disclosure/) ·
  [Google OAuth 2.0 for web server apps (incremental authorization)](https://developers.google.com/identity/protocols/oauth2/web-server) ·
  [HIG Privacy](https://developer.apple.com/design/human-interface-guidelines/privacy) ·
  [GOV.UK Cookie banner](https://design-system.service.gov.uk/components/cookie-banner/) ·
  Mobbin: browse "Permissions" / "Connected apps" / "Integrations" settings
- **Governs:** D03, L12, U03, PRODUCT.md approved behaviours

### Account switching and context indication
- **Problem:** With several Google accounts, panels silently follow "the last
  account clicked". Users may grant data on the wrong account.
- **Solution:** Use one **persistent, visible account context**: a switcher in
  the page or section header showing the current account email, a highlighted
  selected account card, and the account named in every account-scoped panel
  header ("Assets for rajeev@…"). On switch, recompute **all** account-scoped
  numbers, health and empty states together. Global summaries (Home KPIs, setup
  checklist) are labelled "All accounts" or stay global. Account-scoped menus
  and sheets carry the account in their header.
- **Don't use when:** There's only one account. Show the email once, with no
  switcher.
- **Alternatives:** Per-account routes (`/accounts/{id}/…`), which make the
  context part of the URL, plus breadcrumbs or a back link.
- **Components:** [Select](component-catalogue.md#select) /
  [Dropdown menu](component-catalogue.md#dropdown-menu) (switcher) ·
  [Card](component-catalogue.md#card) (selected state) ·
  [Heading](component-catalogue.md#heading) ·
  [Avatar](component-catalogue.md#avatar)
- **Sources:** [NN/g Visibility of system status](https://www.nngroup.com/articles/visibility-system-status/) ·
  [NN/g Menu-design checklist ("Where am I?")](https://www.nngroup.com/articles/menu-design/) ·
  [NN/g Contextual menus (proximity to object)](https://www.nngroup.com/articles/contextual-menus-guidelines/) ·
  [HIG Managing accounts](https://developer.apple.com/design/human-interface-guidelines/managing-accounts) ·
  Mobbin: browse "Account switcher" / "Workspace switcher"
- **Governs:** U03, D01, N04, D02

---

## Overview

### Dashboards and at-a-glance overviews
- **Problem:** Home must answer "what's connected, is it healthy, what changed,
  does anything need me?" in one viewport. It must not become a marketing KPI
  dashboard.
- **Solution:** Follow render 02, in this order: a compact KPI row (counts with
  clear nouns: "2 Google accounts", "18 of 31 assets enabled"), then a compact
  services list (logo, name, "N enabled", status only if abnormal), then
  accounts and recent access changes side by side, then a one-line access-model
  strip. Put attention items first when they exist. Everything links to its
  detail. Setup prompts are secondary. Use no giant numerals and no decorative
  icon tiles (DESIGN.md).
- **Don't use when:** Users need to explore or analyse; give them a detail
  page. Never show customer marketing performance (PRODUCT.md anti-feature).
- **Alternatives:** Exceptions-first list ("Needs attention (1)"), plain summary
  sentence.
- **Components:** [Card](component-catalogue.md#card) (KPI tiles, account cards) ·
  [List](component-catalogue.md#list) ·
  [Badge](component-catalogue.md#badge) ·
  [Alert](component-catalogue.md#alert) ·
  [Image](component-catalogue.md#image) (product marks)
- **Sources:** [NN/g Dashboards: charts and graphs](https://www.nngroup.com/articles/dashboards-preattentive/) ·
  [NN/g Cards: definition](https://www.nngroup.com/articles/cards-component/) ·
  [NN/g Complex-application guidelines](https://www.nngroup.com/articles/complex-application-design/) ·
  [USWDS Summary box](https://designsystem.digital.gov/components/summary-box/) ·
  [HIG Charting data](https://developer.apple.com/design/human-interface-guidelines/charting-data) ·
  [M3 Canonical layouts](https://m3.material.io/foundations/layout/canonical-examples/overview)
- **Governs:** DR03, DR07, L02, L01, D02, D05, DR02 (stat numerals sizing)

### Data tables vs lists
- **Problem:** Choosing rows, cards or tables affects scanability. Mobile tables
  lose columns.
- **Solution:** Use a **list** when users scan by name and act on one item
  (accounts, assets, conversations). Use a **table** when users compare
  several attributes across items (services × accounts × enabled). Never use
  cards for tabular data. Rows have one primary target. Chevron and hover appear
  only if the row navigates. On mobile, stack the key attributes inside the row
  ("Analytics · 3 enabled") rather than dropping them.
- **Don't use when:** A table is wrong for 1–2 attributes. A list is wrong
  when users need to sort or compare columns.
- **Alternatives:** Description or summary list (one object's properties).
- **Components:** [Table](component-catalogue.md#table) ·
  [List](component-catalogue.md#list) ·
  [Checkbox](component-catalogue.md#checkbox) (row selection) ·
  [Pagination](component-catalogue.md#pagination)
- **Sources:** [NN/g Data tables: four major user tasks](https://www.nngroup.com/articles/data-tables/) ·
  [NN/g Mobile tables](https://www.nngroup.com/articles/mobile-tables/) ·
  [NN/g Cards: definition](https://www.nngroup.com/articles/cards-component/) ·
  [USWDS Table](https://designsystem.digital.gov/components/table/) ·
  [USWDS Card (not for tabular data)](https://designsystem.digital.gov/components/card/) ·
  [M3 Lists](https://m3.material.io/components/lists/overview) ·
  [HIG Lists and tables](https://developer.apple.com/design/human-interface-guidelines/lists-and-tables)
- **Governs:** D02, D04, L03, DR03, DR09

### Activity and audit history
- **Problem:** In a permissions product, users ask "what did the AI do, and what
  changed?" A nav item called "Activity" that opens a static explainer breaks
  that promise.
- **Solution:** Use a reverse-chronological list of access changes and agent tool
  use: who or what, which account and resource, read or write, and when. Filter
  by account or type, and use "Load more". Each entry links to the affected
  setting. Write the entries in redacted product copy (PRODUCT.md: audit detail
  stays out of the journey unless explicitly redacted). Until it exists, rename
  the nav item to "How access works".
- **Don't use when:** There's no real data behind it (PRODUCT.md forbids fake
  activity).
- **Alternatives:** "Recent access changes" card on Home (render 02) linking to
  the full log.
- **Components:** [List](component-catalogue.md#list) ·
  [Search input](component-catalogue.md#search-input) /
  [Segmented control](component-catalogue.md#segmented-control) (filters) ·
  [Pagination](component-catalogue.md#pagination) ·
  [Empty state](component-catalogue.md#empty-state)
- **Sources:** [NN/g A link is a promise](https://www.nngroup.com/articles/link-promise/) ·
  [NN/g Visibility of system status](https://www.nngroup.com/articles/visibility-system-status/) ·
  [NN/g Infinite scrolling vs Load more](https://www.nngroup.com/articles/infinite-scrolling-tips/) ·
  [USWDS Collection](https://designsystem.digital.gov/components/collection/) ·
  Mobbin: browse "Activity log" / "Audit log" in admin apps
- **Governs:** N01

### Settings pages
- **Problem:** Users look for configuration under "Settings". Here it opens
  developer MCP setup, which is the same place Help goes.
- **Solution:** Make Settings a real page grouped by topic (Workspace, Accounts,
  Notifications, Connect your AI). Use keyword-first labels ("Email
  notifications"), instant toggles for preferences, and sensible defaults,
  because most users never customise. Developer setup (MCP URL, Copy) lives in
  its own "Connect your AI" section, with technical detail behind disclosure.
  Settings reachable from the avatar menu and the nav go to the same place.
- **Don't use when:** You have only one or two options; put them inline
  where they apply.
- **Alternatives:** In-context settings next to the feature.
- **Components:** [Toggle](component-catalogue.md#toggle) ·
  [List](component-catalogue.md#list) ·
  [Heading](component-catalogue.md#heading) ·
  [Accordion](component-catalogue.md#accordion) ·
  [Button](component-catalogue.md#button) (Copy)
- **Sources:** [NN/g Toggle-switch guidelines (settings labels)](https://www.nngroup.com/articles/toggle-switch-guidelines/) ·
  [NN/g Pagination preferences (users rarely customise)](https://www.nngroup.com/articles/item-list-view-all/) ·
  [HIG Settings](https://developer.apple.com/design/human-interface-guidelines/settings) ·
  [M3 Lists](https://m3.material.io/components/lists/overview) ·
  Mobbin: browse "Settings" in B2B web apps
- **Governs:** N02, D05 (Connect AI route)

---

## Support and recovery

### Help and support
- **Problem:** "Help" that opens a developer endpoint leaves stuck users with
  nowhere to go.
- **Solution:** Help opens a real help page or panel with task-focused FAQ
  ("Why can't the AI see my GA4 property?", "How do I disconnect?"), a
  searchable list, contact or support, and a status link. Add contextual help
  next to complex controls ("What's the difference between read and write?")
  using progressive disclosure. Don't use tours.
- **Don't use when:** You'd repeat documentation that the UI could make
  unnecessary. Fix the UI first.
- **Alternatives:** Inline hints, popovers, an empty-state explanation.
- **Components:** [Link](component-catalogue.md#link) ·
  [Accordion](component-catalogue.md#accordion) (FAQ) ·
  [Popover](component-catalogue.md#popover) ·
  [Search input](component-catalogue.md#search-input)
- **Sources:** [NN/g Help and documentation (heuristic #10)](https://www.nngroup.com/articles/help-and-documentation/) ·
  [NN/g Onboarding tutorials vs contextual help](https://www.nngroup.com/articles/onboarding-tutorials/) ·
  [NN/g Pop-ups and adaptive help](https://www.nngroup.com/articles/pop-up-adaptive-help/) ·
  [HIG Offering help](https://developer.apple.com/design/human-interface-guidelines/offering-help) ·
  [USWDS Accordion](https://designsystem.digital.gov/components/accordion/)
- **Governs:** N02, P03 (who to contact), P02 (plain-language about/privacy)

### 404 and dead-end recovery
- **Problem:** Raw JSON `{"error":"Not found."}` with no title or navigation
  strands users. Natural guesses like `/settings` hit it.
- **Solution:** When `Accept: text/html`, return an HTML 404 in the shared shell.
  It has a clear `<title>`, an H1 ("Page not found"), one sentence, links to
  Home / Sign in (and search when signed in), and keeps the skip link and
  header. Redirect likely guesses (`/settings` → Settings). Every page needs an
  exit; "Request access" pages need a real next step (contact or waitlist).
- **Don't use when:** API clients (`Accept: application/json`) should keep
  getting JSON.
- **Alternatives:** Redirects for known aliases, case-insensitive routes
  (`/CHAT`).
- **Components:** [Header](component-catalogue.md#header) ·
  [Heading](component-catalogue.md#heading) ·
  [Link](component-catalogue.md#link) ·
  [Search input](component-catalogue.md#search-input) ·
  [Footer](component-catalogue.md#footer)
- **Sources:** [NN/g Improving the dreaded 404](https://www.nngroup.com/articles/improving-dreaded-404-error-message/) ·
  [NN/g Error-message guidelines](https://www.nngroup.com/articles/error-message-guidelines/) ·
  [USWDS 404 page template](https://designsystem.digital.gov/templates/404-page/) ·
  [GOV.UK Page not found pages](https://design-system.service.gov.uk/patterns/page-not-found-pages/)
- **Governs:** P01, P03, P02

### Plain-language UI copy
- **Problem:** Engineering language reaches marketers: "openrouter/free",
  "change tools", "hosted-tool availability", "Clerk browser sign-in", "not a
  public-beta readiness claim".
- **Solution:** Write for the audience in PRODUCT.md, who aren't developers.
  Use outcome words ("Tools that can edit", "You've used 40 of 100 today"),
  consistent nouns everywhere (Google data, not "Google settings"), correct
  plurals, and no vendor or provider names in normal screens. Legal or status
  disclaimers become a short plain statement plus a link. Labels lead with the
  keyword.
- **Don't use when:** Developer surfaces (Connect your AI, diagnostics) may
  show technical terms behind disclosure.
- **Alternatives:** —
- **Components:** [Label](component-catalogue.md#label) ·
  [Link](component-catalogue.md#link) ·
  [Alert](component-catalogue.md#alert)
- **Sources:** [NN/g Error-message guidelines (plain language)](https://www.nngroup.com/articles/error-message-guidelines/) ·
  [NN/g Be succinct](https://www.nngroup.com/articles/be-succinct-writing-for-the-web/) ·
  [NN/g Better link labels](https://www.nngroup.com/articles/better-link-labels/) ·
  [M3 UX writing best practices](https://m3.material.io/foundations/content-design/style-guide/ux-writing-best-practices) ·
  [HIG Writing](https://developer.apple.com/design/human-interface-guidelines/writing) ·
  [USWDS Prose](https://designsystem.digital.gov/components/prose/)
- **Governs:** C05, D03, D02, L02, L11, L07, P02, DR11 (eyebrows), cross-cutting "internal language" theme

---

## Surfaces

### Chat / conversational UI
- **Problem:** The flagship flow must make it obvious how to ask, show progress,
  render replies readably, recover from failure, and show which model and
  allowance apply, on desktop and on a 390px phone.
- **Solution:**
  - **Layout:** Put the conversation column at a readable max width (about
    720–800px) with `min-width:0` so it never overflows. User turns are
    right-aligned tinted bubbles (about 70% max width) in primary text colour.
    Assistant turns are full-width prose with a small mark. Drop the uppercase
    "YOU/GOG" labels, or keep them neutral. Render markdown properly.
  - **Composer:** Pin it to the bottom and keep it inside the viewport. The
    textarea auto-grows. Enter sends and Shift+Enter adds a newline, and Send is
    a visible ≥44px button. Use one focus ring. A toolbar row holds the **model
    picker** as a compact menu button ("Automatic · Free ▾") whose popover shows
    plain-language limits and usage. Unavailable models are listed but disabled,
    with the reason.
  - **Streaming:** Append text incrementally, show a Stop button, and pause
    model changes. Announce completion, not every token (`aria-live=polite` on a
    status line). The current "Gog is responding…" behaviour is correct.
  - **Failure and retry:** A failed reply renders **as that assistant turn**
    with the reason and **Retry**. Never persist empty assistant bubbles, and
    keep the user's prompt. A known quota block disables Send up front (see
    [Rate limits](#rate-limit-and-quota-communication)).
  - **Empty state:** Show 3–4 suggestion prompts that are fully visible above
    the composer. Create the thread on **first send**, not on "New chat".
  - **Threads:** Use a conversation list (sidebar on desktop, drawer on
    mobile) with neutral titles and a selected indicator. Rename and Delete go
    in a row menu, and Delete is confirmed and danger-styled.
  - **Trust:** Use one short "AI can make mistakes" footnote. The tool-use note
    is non-sticky or sits in an info popover. Show which Google tools were used,
    with sources, and require approval for write tools.
- **Don't use when:** A structured form or button would do the job faster,
  for example toggling a permission. NN/g: tell users clearly what the bot
  can do.
- **Alternatives:** Command palette, guided forms, suggested-action chips.
- **Components:** [Textarea](component-catalogue.md#textarea) ·
  [Button](component-catalogue.md#button) ·
  [Dropdown menu](component-catalogue.md#dropdown-menu) /
  [Select](component-catalogue.md#select) (model) ·
  [Popover](component-catalogue.md#popover) ·
  [Progress bar](component-catalogue.md#progress-bar) (meter) ·
  [Spinner](component-catalogue.md#spinner) ·
  [Alert](component-catalogue.md#alert) ·
  [Drawer](component-catalogue.md#drawer) ·
  [List](component-catalogue.md#list) ·
  [Modal](component-catalogue.md#modal) ·
  [Empty state](component-catalogue.md#empty-state)
- **Sources:** [NN/g 10 guidelines for AI chatbots](https://www.nngroup.com/articles/ai-chatbots-design-guidelines/) ·
  [NN/g Prompt controls in GenAI chatbots](https://www.nngroup.com/articles/prompt-controls-genai/) ·
  [NN/g The user experience of chatbots](https://www.nngroup.com/articles/chatbots/) ·
  [NN/g Explainable AI in chat interfaces](https://www.nngroup.com/articles/explainable-ai/) ·
  [NN/g 6 types of conversations with GenAI](https://www.nngroup.com/articles/AI-conversation-types/) ·
  [NN/g Response-time limits](https://www.nngroup.com/articles/response-times-3-important-limits/) ·
  [M3 Text fields](https://m3.material.io/components/text-fields/overview) ·
  [M3 Menus](https://m3.material.io/components/menus/overview) ·
  [HIG Feedback](https://developer.apple.com/design/human-interface-guidelines/feedback) ·
  [HIG Loading](https://developer.apple.com/design/human-interface-guidelines/loading) ·
  Mobbin: browse "Chat" / "AI assistant" screens in ChatGPT, Claude, Gemini and Perplexity
  *(USWDS has no chat guidance)*
- **Governs:** C01, C02, C03, C04, C05, C06, C07, L10, L11, DR04, DR05, DR06, DR13

### Authentication and sign-in
- **Problem:** Sign-in is the first impression of "secure". Three headings, a
  dev-mode badge, a nested clipped widget and native validation bubbles
  undermine it.
- **Solution:** Use one card, one H1 ("Sign in to gog-marketing") and one
  primary path (Continue with Google, following Google branding). Style the
  Clerk widget through its `appearance` API to match app tokens, or make it the
  only card. Use production keys (no "Development mode"). Errors appear inline
  in the system style. Don't greet first-time users with "Welcome back". After
  sign-in, return to the requested page (already works for `/chat`). Public
  visitors without access get a real next step (request or contact).
- **Don't use when:** Content could be public; don't put a login wall
  in front of it (NN/g).
- **Alternatives:** Magic link / passkey (only if supported by the auth
  provider).
- **Components:** [Form](component-catalogue.md#form) ·
  [Text input](component-catalogue.md#text-input) ·
  [Button](component-catalogue.md#button) ·
  [Heading](component-catalogue.md#heading) ·
  [Card](component-catalogue.md#card) ·
  [Alert](component-catalogue.md#alert)
- **Sources:** [NN/g Login walls](https://www.nngroup.com/articles/login-walls/) ·
  [NN/g HealthCare.gov account-setup usability](https://www.nngroup.com/articles/affordable_care_act_usability_issues/) ·
  [USWDS Authentication pages template](https://designsystem.digital.gov/templates/authentication-pages/) ·
  [USWDS Create a user profile](https://designsystem.digital.gov/patterns/create-a-user-profile/) ·
  [GOV.UK Create accounts](https://design-system.service.gov.uk/patterns/create-accounts/) ·
  [HIG Managing accounts](https://developer.apple.com/design/human-interface-guidelines/managing-accounts) ·
  [Google Sign-in branding guidelines](https://developers.google.com/identity/branding-guidelines) ·
  [Clerk customization](https://clerk.com/docs/customization/overview) ·
  Mobbin: browse "Login" / "Sign in" in B2B SaaS
- **Governs:** L07, L08, DR08, P03

### Responsive and mobile adaptation
- **Problem:** Mobile was treated as an afterthought: overflowing chat, clipped
  CTAs, dropped columns, sticky notes over content, and duplicate navigation.
- **Solution:** Adapt rather than shrink. Use canonical layouts per breakpoint
  (compact below 600px, medium 600–839px, expanded 840px and up per M3). On
  compact widths: single column, symmetric gutters, header CTAs move to an icon
  button or the section bottom, **one primary per viewport**, tables become
  stacked rows that keep key attributes, the rail becomes a horizontal tab or
  chip strip, the save bar is sticky, and there's no permanent sticky banners.
  Every target is ≥44px. Prove it with an automated `scrollWidth <= innerWidth`
  check at 390px (and 320px) per route.
- **Don't use when:** —
- **Alternatives:** —
- **Components:** [Navigation](component-catalogue.md#navigation) ·
  [Tabs](component-catalogue.md#tabs) ·
  [Drawer](component-catalogue.md#drawer) ·
  [Table](component-catalogue.md#table) ·
  [Stack](component-catalogue.md#stack) ·
  [Button group](component-catalogue.md#button-group) (sticky save bar)
- **Sources:** [NN/g Mobile first is not mobile only](https://www.nngroup.com/articles/mobile-first-not-mobile-only/) ·
  [NN/g Mobile tables](https://www.nngroup.com/articles/mobile-tables/) ·
  [NN/g Mobile faceted search with a tray](https://www.nngroup.com/articles/mobile-faceted-search/) ·
  [M3 Breakpoints](https://m3.material.io/foundations/layout/breakpoints/overview) ·
  [M3 Adaptive design](https://m3.material.io/foundations/layout/layout-overview/adaptive-design) ·
  [HIG Layout](https://developer.apple.com/design/human-interface-guidelines/layout) ·
  [WCAG 1.4.10 Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html) ·
  [WCAG 2.5.5 Target size](https://www.w3.org/WAI/WCAG22/Understanding/target-size-enhanced.html)
- **Governs:** C02, C06, D04, DR10, L12, L13, N03

---

## Finding → pattern index

Each finding has a primary governing pattern, and some have a secondary one.

| ID | Sev | Primary pattern | Also |
|---|---|---|---|
| C01 | High | [Rate-limit & quota](#rate-limit-and-quota-communication) | [Chat](#chat--conversational-ui), [Errors](#error-recovery-and-error-messages) |
| C02 | High | [Responsive & mobile](#responsive-and-mobile-adaptation) | [Chat](#chat--conversational-ui) |
| U03 | High | [Account switching & context](#account-switching-and-context-indication) | [Status & health](#status-and-health-indication), [Empty states](#empty-states) |
| C03 | Med | [Toasts vs alerts vs inline](#toasts-vs-alerts-vs-inline-messages) | [Errors](#error-recovery-and-error-messages) |
| C04 | Med | [Chat](#chat--conversational-ui) | [Empty states](#empty-states) |
| C05 | Med | [Rate-limit & quota](#rate-limit-and-quota-communication) | [Plain-language copy](#plain-language-ui-copy), [Progressive disclosure](#progressive-disclosure) |
| C06 | Med | [Chat](#chat--conversational-ui) | [Toasts vs alerts vs inline](#toasts-vs-alerts-vs-inline-messages), [Responsive](#responsive-and-mobile-adaptation) |
| N01 | Med | [Activity & audit history](#activity-and-audit-history) | [Global navigation](#global-navigation-and-app-shell) |
| N02 | Med | [Help & support](#help-and-support) | [Settings](#settings-pages), [Global navigation](#global-navigation-and-app-shell) |
| N03 | Med | [Mobile nav: bottom bar vs drawer](#mobile-navigation-bottom-bar-vs-drawer) | [Responsive](#responsive-and-mobile-adaptation) |
| N04 | Med | [Destructive actions & undo](#destructive-action-confirmation-and-undo) | [Account switching & context](#account-switching-and-context-indication) |
| D01 | Med | [Account switching & context](#account-switching-and-context-indication) | [Onboarding](#onboarding-and-setup-checklists) |
| D02 | Med | [Status & health](#status-and-health-indication) | [Tables vs lists](#data-tables-vs-lists) |
| D03 | Med | [Permission & consent](#permission-and-consent-management) | [Unsaved changes](#unsaved-changes-protection), [Progressive disclosure](#progressive-disclosure) |
| D04 | Med | [Responsive & mobile](#responsive-and-mobile-adaptation) | [Tables vs lists](#data-tables-vs-lists) |
| D05 | Med | [IA & splitting long pages](#information-architecture-and-splitting-long-pages) | [Dashboards](#dashboards-and-at-a-glance-overviews) |
| P01 | Med | [404 & dead ends](#404-and-dead-end-recovery) | — |
| P02 | Med | [Plain-language copy](#plain-language-ui-copy) | [Global navigation](#global-navigation-and-app-shell), [404 & dead ends](#404-and-dead-end-recovery) |
| C07 | Low | [Chat](#chat--conversational-ui) | [Empty states](#empty-states) |
| P03 | Low | [404 & dead ends](#404-and-dead-end-recovery) | [Authentication](#authentication-and-sign-in), [Help](#help-and-support) |
| L01 | Low | [Onboarding & setup checklists](#onboarding-and-setup-checklists) | [Empty states](#empty-states) |
| L02 | Low | [Dashboards](#dashboards-and-at-a-glance-overviews) | [Plain-language copy](#plain-language-ui-copy) |
| L03 | Low | [Tables vs lists](#data-tables-vs-lists) | — |
| L04 | Low | [Status & health](#status-and-health-indication) | [Dashboards](#dashboards-and-at-a-glance-overviews) |
| L05 | Low | [Empty states](#empty-states) | [Status & health](#status-and-health-indication) |
| L06 | Low | [Search & filtering](#search-and-filtering) | [Empty states](#empty-states) |
| L07 | Low | [Authentication & sign-in](#authentication-and-sign-in) | [Forms & validation](#forms-and-validation) |
| L08 | Low | [Authentication & sign-in](#authentication-and-sign-in) | (also add a favicon; fix CSP and telemetry console noise) |
| L09 | Low | [Global navigation](#global-navigation-and-app-shell) | [Consistent component language](#consistent-component-language-across-areas) |
| L10 | Low | [Destructive actions & undo](#destructive-action-confirmation-and-undo) | [Chat](#chat--conversational-ui) |
| L11 | Low | [Plain-language copy](#plain-language-ui-copy) | [Global navigation](#global-navigation-and-app-shell) |
| L12 | Low | [Responsive & mobile](#responsive-and-mobile-adaptation) | [Forms & validation](#forms-and-validation) |
| L13 | Low | [Mobile nav: bottom bar vs drawer](#mobile-navigation-bottom-bar-vs-drawer) | — |
| DR01 | High | [Consistent component language](#consistent-component-language-across-areas) | [Global navigation](#global-navigation-and-app-shell) |
| DR02 | High | [Consistent component language](#consistent-component-language-across-areas) | [Dashboards](#dashboards-and-at-a-glance-overviews) |
| DR03 | High | [Dashboards](#dashboards-and-at-a-glance-overviews) | [Onboarding](#onboarding-and-setup-checklists) |
| DR04 | Med | [Consistent component language](#consistent-component-language-across-areas) | [Chat](#chat--conversational-ui) |
| DR05 | Med | [Chat](#chat--conversational-ui) | — |
| DR06 | Med | [Chat](#chat--conversational-ui) | [Rate-limit & quota](#rate-limit-and-quota-communication) |
| DR07 | Med | [Status & health](#status-and-health-indication) | [Dashboards](#dashboards-and-at-a-glance-overviews) |
| DR08 | Med | [Authentication & sign-in](#authentication-and-sign-in) | [Consistent component language](#consistent-component-language-across-areas) |
| DR09 | Med | [Unsaved changes](#unsaved-changes-protection) | [Tables vs lists](#data-tables-vs-lists), [Forms](#forms-and-validation) |
| DR10 | Med | [Responsive & mobile](#responsive-and-mobile-adaptation) | [Onboarding](#onboarding-and-setup-checklists) |
| DR11 | Low | [Consistent component language](#consistent-component-language-across-areas) | [Plain-language copy](#plain-language-ui-copy) |
| DR12 | Low | [Consistent component language](#consistent-component-language-across-areas) | (headings: `text-wrap: balance`, no forced `<br>`) |
| DR13 | Low | [Consistent component language](#consistent-component-language-across-areas) | [Toasts vs alerts vs inline](#toasts-vs-alerts-vs-inline-messages) |

## Source notes

- NN/g refused direct connections from this machine. Every NN/g URL was confirmed
  through search results that show the article title and summary, and claims
  paraphrase those summaries. NN/g has no dedicated rate-limit, account-switching,
  settings-page or chat-streaming article, so the nearest heuristics are cited.
- USWDS pages (components, patterns, templates) were fetched directly. USWDS
  publishes few patterns, so GOV.UK Design System patterns (fetched, HTTP 200)
  fill the decision layer for 404, check answers, task lists, validation and
  service unavailable.
- Material 3 URLs were checked against `m3.material.io/sitemap.xml`. Apple HIG
  URLs were checked against the HIG JSON index. M3 and HIG have no chat-specific
  pattern page.
- Mobbin wasn't fetched (login required). The suggested search terms are
  browsing hints, not quoted content.
