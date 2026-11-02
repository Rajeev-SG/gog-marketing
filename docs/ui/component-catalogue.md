# Component catalogue — gog-marketing

Reference standard for picking UI components while fixing the 31 October 2026
audit (`FINDINGS.md` IDs `C/U/D/N/P/L`, `DESIGN-REVIEW.md` IDs `DR`). Pair with
[pattern-catalogue.md](pattern-catalogue.md), [DESIGN.md](https://github.com/Rajeev-SG/gog-marketing/blob/main/DESIGN.md) and
[PRODUCT.md](https://github.com/Rajeev-SG/gog-marketing/blob/main/PRODUCT.md).

## How to use this

Before you build or change a control, find it here by name or alias. Check
**Use / Avoid / Instead** to confirm it is the right component, apply the
**Behaviour & a11y** rules, and look at the **Seen in** systems to see what a
conventional version looks like. Each entry uses three layers. **Discovery** is
[The Component Gallery](https://component.gallery/components/), which lists all
60 component types below. **Decision** is NN/g articles and USWDS "When to use /
When to consider something else" guidance. **Application** is Material Design 3,
Apple HIG, WAI-ARIA APG and Mobbin, which show how real products combine
components. The **gog-marketing** line maps the component to screens and to the
finding IDs it resolves. Where a component isn't used, the line says "n/a yet".

> **Decision rule:** prefer the conventional component and its conventional
> behaviour. Deviate only with a written reason in `DESIGN.md` (what, why, and
> which convention you're breaking). "It looks cleaner" is not a reason.

**Stack reminder** (from DESIGN.md): the signed-in workspace (Home, Google data,
Access, Connect AI) and Chat are React apps built with Vite. Interactive
controls come from the component library in `internal/hosted/app/src/ui`, which
wraps [React Aria Components](https://react-spectrum.adobe.com/react-aria/) and
styles them only with `product.css` tokens. Public pages (landing, sign-in,
Help & privacy, 404) stay server-rendered HTML with native elements. Never
hand-roll a control the library already has.

### Implemented components (`src/ui`)

| Catalogue entry | Component | React Aria part | Where it's used |
|---|---|---|---|
| [Button](#button) | `Button` (primary · secondary · danger · quiet · attention) | `Button` | Everywhere; Save bar, dialogs |
| [Link](#link) styled as a button | `LinkButton` | `Link` + `RouterProvider` | Page actions that navigate |
| [Checkbox](#checkbox) | `Checkbox` (with description and indeterminate) | `Checkbox` | Assets, tools, consent form, select-all |
| [Search input](#search-input) | `SearchField` | `SearchField` | Asset and tool search |
| [Segmented control](#segmented-control) | `SegmentedControl` | `ToggleButtonGroup` | Tool filter (All · Reads · Changes) |
| [Dropdown menu](#dropdown-menu) | `ActionMenu` | `MenuTrigger` + `Menu` + `Popover` | Account actions |
| [Modal](#modal) (alert dialog) | `ConfirmProvider` / `useConfirm` | `ModalOverlay` + `Modal` + `Dialog role=alertdialog` | Disconnect, allow changes, allow deletion, discard changes |
| [Accordion](#accordion) | `Disclosure` | `Disclosure` | Tool groups, technical descriptions |
| [Tabs](#tabs) (vertical) | Asset service rail | `Tabs` + `TabList` + `TabPanel` | Google data asset picker |
| [Alert](#alert) | `Notice` | (role=status / alert) | Page outcomes, reconnect banner |
| [Badge](#badge) | `StatusBadge` | — | Abnormal account or service status only |
| Save bar (sticky) | `SaveBar` | — | One save model (D03) |
| [Icon](#icon) | `Icon` | — | Decorative Lucide glyphs, `aria-hidden` |

Rules for adding to the library: wrap the React Aria component, style it only in
`src/ui/ui.css` with tokens and React Aria's `data-*` state attributes, keep its
keyboard and screen-reader behaviour, and add the row above.

### Global behaviour floor (applies to every entry)

- **Touch target ≥ 44×44 CSS px** on coarse pointers. This is WCAG 2.2 SC 2.5.5
  (AAA); the AA floor is 24px under SC 2.5.8. Apple HIG Accessibility also uses
  44×44pt. Sources: [2.5.5](https://www.w3.org/WAI/WCAG22/Understanding/target-size-enhanced.html),
  [2.5.8](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html),
  [HIG Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility). Fixes **L12**.
- **Visible `:focus-visible` ring** (`--focus-ring`), and sticky UI must never
  cover it ([2.4.11](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html)).
- **Never colour alone.** Status always carries a text label
  ([1.4.1](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html)).
- **Status messages are announced without moving focus**, through `role=status`
  or `aria-live` ([4.1.3](https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html)).
- **No horizontal scroll at 320–390px**
  ([1.4.10 Reflow](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html)). Fixes **C02, D04**.
- **Keyboard.** Tab reaches every control, arrow keys move within composite
  widgets, and Escape closes overlays
  ([APG keyboard interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/)).
- **Mobbin** ([mobbin.com](https://mobbin.com), login required) is a place to
  browse real-product screens by component or flow name, for example "Settings",
  "Empty state", "Chat", "Account switcher" or "Bottom sheet". It is cited as
  something to browse, not as fetched content.

### Index (60 Component Gallery types + supplements)

[Accordion](#accordion) · [Alert](#alert) · [Avatar](#avatar) · [Badge](#badge) ·
[Breadcrumbs](#breadcrumbs) · [Button](#button) · [Button group](#button-group) ·
[Card](#card) · [Carousel](#carousel) · [Checkbox](#checkbox) ·
[Color picker](#color-picker) · [Combobox](#combobox) · [Date input](#date-input) ·
[Datepicker](#datepicker) · [Drawer](#drawer) · [Dropdown menu](#dropdown-menu) ·
[Empty state](#empty-state) · [Fieldset](#fieldset) · [File](#file) ·
[File upload](#file-upload) · [Footer](#footer) · [Form](#form) · [Header](#header) ·
[Heading](#heading) · [Hero](#hero) · [Icon](#icon) · [Image](#image) ·
[Label](#label) · [Link](#link) · [List](#list) · [Modal](#modal) ·
[Navigation](#navigation) · [Pagination](#pagination) · [Popover](#popover) ·
[Progress bar](#progress-bar) · [Progress indicator](#progress-indicator) ·
[Quote](#quote) · [Radio button](#radio-button) · [Rating](#rating) ·
[Rich text editor](#rich-text-editor) · [Search input](#search-input) ·
[Segmented control](#segmented-control) · [Select](#select) ·
[Separator](#separator) · [Skeleton](#skeleton) · [Skip link](#skip-link) ·
[Slider](#slider) · [Spinner](#spinner) · [Stack](#stack) · [Stepper](#stepper) ·
[Table](#table) · [Tabs](#tabs) · [Text input](#text-input) ·
[Textarea](#textarea) · [Toast](#toast) · [Toggle](#toggle) ·
[Tooltip](#tooltip) · [Tree view](#tree-view) · [Video](#video) ·
[Visually hidden](#visually-hidden) · [Supplements](#supplements-not-separate-component-gallery-types)

---

### Accordion
*Aliases:* Collapse, Collapsible, Details, Disclosure, Expander · [Gallery](https://component.gallery/components/accordion/)
- **What:** A vertical stack of headings. Each one expands to show its panel or
  collapses to hide it.
- **Use:** When users need only a few specific sections of a page, or when you
  have a lot of content in a small space (especially on mobile).
  **Avoid:** When users need to see most of the content. USWDS: accordions
  "increase cognitive load and interaction cost". Also avoid when there's too
  little content to be worth condensing. **Instead:** well-formatted headings
  and text, [Tabs](#tabs), or in-page links.
- **Behaviour & a11y:** The header is a `<button>` with `aria-expanded` and
  `aria-controls`, or use native `<details>/<summary>`. Enter or Space toggles
  it. Never auto-advance. Use a caret or plus icon, not a right arrow, which
  implies navigation. Allow several panels open at once unless there's a reason
  not to.
- **Seen in:** GOV.UK Accordion · Carbon Accordion · Primer Details · Gestalt Accordion
- **Sources:** [USWDS Accordion](https://designsystem.digital.gov/components/accordion/) ·
  [NN/g Accordions on Desktop](https://www.nngroup.com/articles/accordions-on-desktop/) ·
  [NN/g Accordions on Mobile](https://www.nngroup.com/articles/mobile-accordions/) ·
  [NN/g Accordion icons](https://www.nngroup.com/articles/accordion-icons/) ·
  [APG Accordion](https://www.w3.org/WAI/ARIA/apg/patterns/accordion/) ·
  [HIG Disclosure controls](https://developer.apple.com/design/human-interface-guidelines/disclosure-controls) ·
  [GOV.UK Details](https://design-system.service.gov.uk/components/details/)
- **gog-marketing:** Use for advanced diagnostics (render 09: connection/resource
  IDs). Use for collapsing the per-service tool groups in AI tool permissions so
  339 tools aren't shown at once (**D03**). Use for the chat model details/limits
  disclosure, restyled rather than the native ▸ triangle (**C05, DR06**).

### Alert
*Aliases:* Banner, Callout, Notification, Message, Feedback · [Gallery](https://component.gallery/components/alert/)
- **What:** A prominent inline message about something important: an error, a
  warning, a success or general info.
- **Use:** System status messages, and validation or confirmation after a user
  action. **Avoid:** As the only error signal on long forms (also show errors
  inline). Avoid it for destructive actions; USWDS says to use a confirmation
  modal instead. Avoid putting error text in success styling. **Instead:**
  [Toast](#toast) for transient non-critical confirmations, inline field errors,
  [Modal](#modal) for confirm-before-destroy, and a site alert for service-wide
  notices.
- **Behaviour & a11y:** Use `role=alert` only for urgent errors and
  `role=status` for everything else. The severity icon plus a text title means
  it's never colour-only. Say what happened and what to do next, with one
  action button. Reserve error styling for real errors (NN/g, "Hostile error
  messages").
- **Seen in:** GOV.UK Warning text / Notification banner · Carbon Notification · Polaris Banner · Atlassian Banner / Section message
- **Sources:** [USWDS Alert](https://designsystem.digital.gov/components/alert/) ·
  [USWDS Site alert](https://designsystem.digital.gov/components/site-alert/) ·
  [NN/g Indicators, validations, notifications](https://www.nngroup.com/articles/indicators-validations-notifications/) ·
  [NN/g Error-message guidelines](https://www.nngroup.com/articles/error-message-guidelines/) ·
  [NN/g Hostile error messages](https://www.nngroup.com/articles/hostile-error-messages/) ·
  [HIG Alerts](https://developer.apple.com/design/human-interface-guidelines/alerts) ·
  [GOV.UK Notification banner](https://design-system.service.gov.uk/components/notification-banner/)
- **gog-marketing:** Use one danger alert for the chat limit error, and delete the
  green pill copy (**C03**). Use attention (amber) for "needs reconnect" (render
  05). Use a partial-failure alert that lists the failed services (render 08).
  Shrink "Your next step" to an inline banner that is dismissible and shown only
  while steps are incomplete (**DR03, L01**). Make the chat Google-tools note a
  non-sticky info alert (**C06**). Constrain the chat info banner to the message
  column (**DR13**).

### Avatar
*Aliases:* — · [Gallery](https://component.gallery/components/avatar/)
- **What:** A picture of a user: a photo, an illustration or initials.
- **Use:** To identify the signed-in person or account owner, and as the trigger
  for the account menu. **Avoid:** Treating it as the only way to identify an
  account when several accounts exist; always show the email too. **Instead:**
  a text label, or a name plus email.
- **Behaviour & a11y:** If it's interactive, it's a `<button>` with an accessible
  name ("Account menu for rajeev@…") and a hit area of at least 44px, even when
  the image is 28–32px. Decorative avatars beside a visible name get `alt=""`.
- **Seen in:** Polaris Avatar · Atlassian Avatar · Primer Avatar · Spectrum Avatar
- **Sources:** [M3 Menus](https://m3.material.io/components/menus/overview) ·
  [HIG Managing accounts](https://developer.apple.com/design/human-interface-guidelines/managing-accounts) ·
  [WCAG 2.5.5](https://www.w3.org/WAI/WCAG22/Understanding/target-size-enhanced.html)
  *(no dedicated NN/g or USWDS page)*
- **gog-marketing:** Header avatar (Clerk user menu). It's 28×28 today, so give it a
  44px target (**L12**). Make it the single home for Sign out (**L09**). Use the
  same header on Home and Chat (**DR01**).

### Badge
*Aliases:* Tag, Label, Chip, Lozenge, Pill · [Gallery](https://component.gallery/components/badge/)
- **What:** A small label next to or inside another element. It shows status, a
  property or a count.
- **Use:** To draw attention to state that differs from the norm, and for counts.
  **Avoid:** When it could be confused with buttons nearby. Avoid repeating the
  same badge on every row (USWDS Tag). Don't use badges as icons or
  abbreviations. **Instead:** plain muted text ("Connected"), a status dot plus
  text, or nothing when state is normal.
- **Behaviour & a11y:** Badges aren't interactive. If one is clickable it's a
  button or chip and needs a 44px target. Text carries the meaning and colour is
  only reinforcement. Use the pill radius token only (`999px`).
- **Seen in:** GOV.UK Tag · Carbon Tag · Polaris Badge · Material Badge
- **Sources:** [USWDS Tag](https://designsystem.digital.gov/components/tag/) ·
  [NN/g Indicators, validations, notifications](https://www.nngroup.com/articles/indicators-validations-notifications/) ·
  [M3 Badges](https://m3.material.io/components/badges/overview) ·
  [GOV.UK Tag](https://design-system.service.gov.uk/components/tag/)
- **gog-marketing:** Service status pills. Show them only when state differs: 14
  identical green "Connected" pills hide the one that matters (**DR07**). Show
  "Connected · nothing to choose yet" in neutral styling (**L05**). Remove the
  abbreviation "badges" (BQ, GA, Go…) and use real product marks (**L04**). Never
  put an error in a success pill (**C03**). Pills use the 999px radius only
  (**DR11**).

### Breadcrumbs
*Aliases:* Breadcrumb trail · [Gallery](https://component.gallery/components/breadcrumbs/)
- **What:** A list of links that shows where the current page sits in the
  hierarchy.
- **Use:** In deep hierarchies where users arrive on interior pages from search or
  links. **Avoid:** On simple or shallow sites where the main nav already shows
  context. Avoid them on landing pages and alongside side navigation that
  already shows the hierarchy (USWDS). **Instead:** clear global navigation
  with `aria-current`, page titles, and a [back link](https://design-system.service.gov.uk/components/back-link/).
- **Behaviour & a11y:** `<nav aria-label="Breadcrumb">` containing an `<ol>`. The
  last item is the current page with `aria-current="page"`. Separators are
  decorative and come from CSS.
- **Seen in:** GOV.UK Breadcrumbs · Carbon Breadcrumb · Atlassian Breadcrumbs · Spectrum Breadcrumbs
- **Sources:** [USWDS Breadcrumb](https://designsystem.digital.gov/components/breadcrumb/) ·
  [NN/g Breadcrumbs: 11 guidelines](https://www.nngroup.com/articles/breadcrumbs/) ·
  [APG Breadcrumb](https://www.w3.org/WAI/ARIA/apg/patterns/breadcrumb/) ·
  [HIG Path controls](https://developer.apple.com/design/human-interface-guidelines/path-controls)
- **gog-marketing:** n/a yet. The IA is flat even after the **D05** split
  (Home / Google data / Tools / Connect AI). Use a back link on drill-ins instead,
  for example account → service assets.

### Button
*Aliases:* CTA, Action, FAB (floating variant) · [Gallery](https://component.gallery/components/button/)
- **What:** Triggers an action: submit, open, save or delete.
- **Use:** For important actions. **Avoid:** Use links, not buttons, to move
  between pages. Style less important actions as secondary or tertiary
  (USWDS). **Instead:** [Link](#link) for navigation, and
  [Dropdown menu](#dropdown-menu) for overflow actions.
- **Behaviour & a11y:** Use a native `<button>`. Show all five states: enabled,
  hover, focus, pressed and disabled (NN/g). Disabled must *look* disabled, with
  a neutral fill rather than faded brand colour. During async work set
  `aria-busy` and show a pending label. Use a verb label ("Delete conversation",
  not "OK"). One primary per view. Destructive actions get danger styling and
  sit apart from benign ones.
- **Seen in:** GOV.UK Button · Carbon Button · Polaris Button · Material Button
- **Sources:** [USWDS Button](https://designsystem.digital.gov/components/button/) ·
  [NN/g Button states](https://www.nngroup.com/articles/button-states-communicate-interaction/) ·
  [NN/g OK–Cancel order](https://www.nngroup.com/articles/ok-cancel-or-cancel-ok/) ·
  [NN/g Consequential options close to benign](https://www.nngroup.com/articles/proximity-consequential-options/) ·
  [M3 Buttons](https://m3.material.io/components/buttons/overview) ·
  [M3 Icon buttons](https://m3.material.io/components/icon-buttons/overview) ·
  [HIG Buttons](https://developer.apple.com/design/human-interface-guidelines/buttons) ·
  [APG Button](https://www.w3.org/WAI/ARIA/apg/patterns/button/)
- **gog-marketing:** Define one button set (primary / secondary / ghost / danger)
  on one radius token, and delete the chat-local styles (**DR01**). Disabled Send
  becomes neutral grey (**DR13**). Send must stay in the viewport (**C02**). Give
  Delete danger styling (**L10**). All targets ≥44px (**L12**). One primary per
  mobile viewport (**DR10**). Add 16px above "Connect Google account" (**DR13**).
  Show the in-flight state for discovery (render 07).

### Button group
*Aliases:* Toolbar, Action bar, Button dock · [Gallery](https://component.gallery/components/button-group/)
- **What:** A wrapper around several related buttons.
- **Use:** When actions are related, such as primary plus alternative (Save /
  Cancel), or for linear stepping (Back / Next). **Avoid:** For unrelated
  actions. Avoid mixing destructive and non-destructive actions, which causes
  input mistakes. Don't use it for links to content (USWDS). **Instead:**
  separate placement, or a [Dropdown menu](#dropdown-menu) for overflow.
- **Behaviour & a11y:** Wrap toolbars in `role=toolbar` with arrow-key movement
  only for real toolbars. Otherwise use plain buttons in a flex row. Follow
  platform order consistently (NN/g): this product puts the primary action last,
  on the right.
- **Seen in:** Polaris Button group · Primer Action bar · Spectrum Action group · Gestalt ButtonGroup
- **Sources:** [USWDS Button group](https://designsystem.digital.gov/components/button-group/) ·
  [NN/g OK–Cancel order](https://www.nngroup.com/articles/ok-cancel-or-cancel-ok/) ·
  [M3 Button groups](https://m3.material.io/components/button-groups/overview) ·
  [M3 Toolbars](https://m3.material.io/components/toolbars/overview) ·
  [APG Toolbar](https://www.w3.org/WAI/ARIA/apg/patterns/toolbar/) ·
  [HIG Toolbars](https://developer.apple.com/design/human-interface-guidelines/toolbars)
- **gog-marketing:** The asset-picker footer has a selection summary on the left
  and **Cancel** then **Save** on the right, attached to the panel (**DR09**).
  The delete-confirm dialog uses Cancel then a danger "Delete" (**L10**). The
  chat composer toolbar holds the model menu button and Send (**DR06**).

### Card
*Aliases:* Tile, Panel · [Gallery](https://component.gallery/components/card/)
- **What:** A container for content about one entity.
- **Use:** For a collection of heterogeneous, related summaries that each link to
  more detail. **Avoid:** As a substitute for table rows or tabular data, for
  simple CTAs, or for standalone content (USWDS Card). Don't nest cards.
  **Instead:** [List](#list) rows, [Table](#table), or plain sections with
  headings.
- **Behaviour & a11y:** If the whole card is clickable, make one link that
  stretches over the card, not nested interactive elements. Use a heading inside
  the card. Use the 10px radius token.
- **Seen in:** Material Card · Spectrum Card · Lightning Card · Elastic Panel
- **Sources:** [USWDS Card](https://designsystem.digital.gov/components/card/) ·
  [NN/g Cards: definition](https://www.nngroup.com/articles/cards-component/) ·
  [M3 Cards](https://m3.material.io/components/cards/overview) ·
  [HIG Boxes](https://developer.apple.com/design/human-interface-guidelines/boxes)
- **gog-marketing:** Use cards for account cards (render 04) and KPI tiles only.
  Service and asset rows are list rows, not cards (DESIGN.md). Remove the
  Clerk-inside-page-card nesting on sign-in (**DR08**). Re-compose Home to render
  02 (**DR03**). Use the radius scale 6/10/999 (**DR11**). Fix the right gutter
  on mobile cards (**D04**).

### Carousel
*Aliases:* Content slider · [Gallery](https://component.gallery/components/carousel/)
- **What:** Slides of content that move horizontally, one or more at a time.
- **Use:** Rarely, for browsable peer items when space is tight and seeing every
  item doesn't matter. **Avoid:** For important content, which people
  frequently overlook in carousels. Never auto-advance (NN/g). **Instead:** a
  static list or grid, or one hero.
- **Behaviour & a11y:** Visible previous/next controls of at least 44px, a pause
  control if anything moves, `aria-roledescription="carousel"`, and slide labels
  ("2 of 5").
- **Seen in:** Material Carousel · Lightning Carousel · Ant Design Carousel · Bootstrap Carousel
- **Sources:** [NN/g Carousel usability](https://www.nngroup.com/articles/designing-effective-carousels/) ·
  [NN/g Auto-forwarding carousels](https://www.nngroup.com/articles/auto-forwarding/) ·
  [NN/g Carousels on mobile](https://www.nngroup.com/articles/mobile-carousels/) ·
  [APG Carousel](https://www.w3.org/WAI/ARIA/apg/patterns/carousel/) ·
  [M3 Carousel](https://m3.material.io/components/carousel/overview)
- **gog-marketing:** n/a yet. Don't use one for chat suggestions or KPIs.

### Checkbox
*Aliases:* Checkboxes, Check list · [Gallery](https://component.gallery/components/checkbox/)
- **What:** An input for choosing any number of options, or a single on/off
  acknowledgement.
- **Use:** For multi-select lists, or to acknowledge something such as terms.
  **Avoid:** For single-select (use radios). Avoid it for settings that apply
  immediately, which suit a [Toggle](#toggle) better. **Instead:**
  [Radio button](#radio-button), [Toggle](#toggle), or a
  [Select](#select) with many options.
- **Behaviour & a11y:** Use native `<input type=checkbox>` with a `<label>` that
  wraps the whole row, so the row toggles it and the target is 44px. Mark
  "Select all" as tri-state with `aria-checked=mixed`. Group related boxes in a
  [Fieldset](#fieldset) with a legend.
- **Seen in:** GOV.UK Checkboxes · Carbon Checkbox · Polaris Checkbox · Material Checkbox
- **Sources:** [USWDS Checkbox](https://designsystem.digital.gov/components/checkbox/) ·
  [NN/g Checkboxes vs radio buttons](https://www.nngroup.com/articles/checkboxes-vs-radio-buttons/) ·
  [NN/g Checkboxes: design guidelines](https://www.nngroup.com/articles/checkboxes-design-guidelines/) ·
  [APG Checkbox](https://www.w3.org/WAI/ARIA/apg/patterns/checkbox/) ·
  [M3 Checkbox](https://m3.material.io/components/checkbox/overview) ·
  [HIG Toggles](https://developer.apple.com/design/human-interface-guidelines/toggles)
- **gog-marketing:** Used for asset rows, tool rows, consent Workspace/Marketing
  rows and the "Writes" boxes. Today they are 13–18px and must become whole-row
  44px labels (**L12**). Select all/none in the asset picker (render 06, **DR09**).
  Tool list: default to connected services only (**D03**).

### Color picker
*Aliases:* Color field, Color well, Swatch picker · [Gallery](https://component.gallery/components/color-picker/)
- **What:** An input for choosing a colour.
- **Use:** When users create visual content. **Avoid:** In product settings
  with no visual-authoring job. **Instead:** a fixed palette, or a
  [Radio button](#radio-button) list of named swatches.
- **Behaviour & a11y:** Always give a text value alternative (hex) and a name.
- **Seen in:** Polaris Color field · Spectrum Color area · Lightning Color Picker · Fluent ColorPicker
- **Sources:** [HIG Color wells](https://developer.apple.com/design/human-interface-guidelines/color-wells) ·
  [Gallery](https://component.gallery/components/color-picker/) *(no NN/g or USWDS decision page)*
- **gog-marketing:** n/a. It's out of the product boundary (PRODUCT.md).

### Combobox
*Aliases:* Autocomplete, Autosuggest, Lookup · [Gallery](https://component.gallery/components/combobox/)
- **What:** A text input that filters a list of options as you type.
- **Use:** When there are more than 15 options, or space is limited (USWDS).
  **Avoid:** With few choices, where a select or radios are better.
  **Instead:** [Select](#select) for 7–15 options, [Radio button](#radio-button)
  for fewer than 7.
- **Behaviour & a11y:** Follow APG Combobox: `role=combobox`,
  `aria-expanded`, `aria-activedescendant`, arrow keys through the options,
  Enter to select and Escape to close. Keep the typed value searchable (NN/g
  intranet search).
- **Seen in:** Primer Autocomplete · Spectrum Combo box · Lightning Combobox · Fluent ComboBox
- **Sources:** [USWDS Combo box](https://designsystem.digital.gov/components/combo-box/) ·
  [NN/g Listboxes vs dropdown lists](https://www.nngroup.com/articles/listbox-dropdown/) ·
  [APG Combobox](https://www.w3.org/WAI/ARIA/apg/patterns/combobox/) ·
  [HIG Combo boxes](https://developer.apple.com/design/human-interface-guidelines/combo-boxes)
- **gog-marketing:** n/a yet. Candidate for a tool search in AI tool permissions
  (339 tools, **D03**) and for the account switcher if accounts exceed about 15
  (**D01**).

### Date input
*Aliases:* Memorable date, Date field · [Gallery](https://component.gallery/components/date-input/)
- **What:** Separate day / month / year fields for typing a date.
- **Use:** For dates people know or can look up, such as a birthday.
  **Avoid:** For scheduling, where the day of the week matters.
  **Instead:** [Datepicker](#datepicker).
- **Behaviour & a11y:** `inputmode=numeric`, a fieldset and legend, an
  unambiguous format hint outside the field, and accept typing.
- **Seen in:** GOV.UK Date input · Polaris Date field · Gestalt DateField
- **Sources:** [USWDS Memorable date](https://designsystem.digital.gov/components/memorable-date/) ·
  [NN/g Date-input form fields](https://www.nngroup.com/articles/date-input/)
- **gog-marketing:** n/a yet.

### Datepicker
*Aliases:* Calendar, Datetime picker · [Gallery](https://component.gallery/components/datepicker/)
- **What:** A calendar view for choosing a date or date range.
- **Use:** For scheduling, or when the day of the week matters (USWDS).
  **Avoid:** For familiar dates. **Instead:** [Date input](#date-input),
  plus text entry (always allowed).
- **Behaviour & a11y:** Follow the APG dialog and grid date-picker model, with
  arrow keys through days and a text input fallback.
- **Seen in:** Carbon Date picker · Polaris Date picker · Material Date picker · Atlassian Date time picker
- **Sources:** [USWDS Date picker](https://designsystem.digital.gov/components/date-picker/) ·
  [NN/g Date-input form fields](https://www.nngroup.com/articles/date-input/) ·
  [M3 Date pickers](https://m3.material.io/components/date-pickers/overview) ·
  [HIG Pickers](https://developer.apple.com/design/human-interface-guidelines/pickers)
- **gog-marketing:** n/a yet. Possibly a date-range filter on a future Activity
  log (**N01**).

### Drawer
*Aliases:* Sheet, Side sheet, Bottom sheet, Tray, Flyout, Navigation drawer · [Gallery](https://component.gallery/components/drawer/)
- **What:** A panel that slides in from an edge of the screen.
- **Use:** For secondary or contextual content and lists that don't fit, such
  as a conversation list on mobile or filters on mobile (NN/g "tray"). Use a
  navigation drawer only when there are too many destinations for a bar.
  **Avoid:** Don't duplicate navigation that's already visible, for example in
  a bottom bar. Bottom sheets aren't for information that's always needed
  (NN/g). **Instead:** visible navigation (bar or tabs), a
  [Modal](#modal) for blocking decisions, or inline expansion.
- **Behaviour & a11y:** A modal drawer is a dialog: `role=dialog` with
  `aria-modal=true` (or `<dialog>.showModal()`). Move focus into it on open
  and trap focus. **Escape closes it**, as do a visible Close button and a
  scrim click. Return focus to the trigger. Make the background inert.
  Implement one component and reuse it everywhere.
- **Seen in:** Material Side sheet / Navigation drawer · Atlassian Drawer · Spectrum Tray · Orbit Drawer
- **Sources:** [NN/g Bottom sheets](https://www.nngroup.com/articles/bottom-sheet/) ·
  [NN/g Mobile faceted search with a tray](https://www.nngroup.com/articles/mobile-faceted-search/) ·
  [NN/g Hamburger menus hurt UX metrics](https://www.nngroup.com/articles/hamburger-menus/) ·
  [APG Dialog (modal)](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/) ·
  [M3 Navigation drawer](https://m3.material.io/components/navigation-drawer/overview) ·
  [M3 Side sheets](https://m3.material.io/components/side-sheets/overview) ·
  [M3 Bottom sheets](https://m3.material.io/components/bottom-sheets/overview) ·
  [HIG Sheets](https://developer.apple.com/design/human-interface-guidelines/sheets)
- **gog-marketing:** Build one drawer with a focus trap, Escape, Close × and focus
  return (**N03**). On mobile, drop the duplicate nav drawer. Keep the bottom
  bar, and use a drawer only for the `/chat` conversation list (**L13**). Use a
  bottom sheet for the account ⋮ menu on mobile only with an account-email header
  (**N04**).

### Dropdown menu
*Aliases:* Menu, Overflow menu, Context menu, Kebab/meatball menu, Action menu · [Gallery](https://component.gallery/components/dropdown-menu/)
- **What:** A button that reveals a list of **actions or navigation**. It isn't
  a form input.
- **Use:** For secondary or overflow actions on an object, and for a
  user/account menu. **Avoid:** For choosing a form value. Avoid multi-level
  cascading menus and hiding primary actions in one. Don't put destructive
  items next to benign ones without separation. **Instead:** [Select](#select)
  for values, visible [Buttons](#button) for primary actions.
- **Behaviour & a11y:** Follow the APG Menu Button pattern. The trigger has
  `aria-haspopup=menu` and `aria-expanded`. Arrow keys move between items,
  **Escape closes and returns focus**, and it closes on outside click and on
  route or hash change. Place the ⋮ icon next to the object it acts on, and
  name it ("Actions for rajeev@…"). Put destructive items last, after a
  separator, and confirm them.
- **Seen in:** Carbon Overflow menu · Polaris Menu · Atlassian Dropdown menu · Gestalt Dropdown
- **Sources:** [NN/g Contextual menus: 10 guidelines](https://www.nngroup.com/articles/contextual-menus-guidelines/) ·
  [NN/g Dropdowns: design guidelines](https://www.nngroup.com/articles/drop-down-menus/) ·
  [NN/g Menu-design checklist](https://www.nngroup.com/articles/menu-design/) ·
  [APG Menu button](https://www.w3.org/WAI/ARIA/apg/patterns/menu-button/) ·
  [M3 Menus](https://m3.material.io/components/menus/overview) ·
  [HIG Menus](https://developer.apple.com/design/human-interface-guidelines/menus) ·
  [HIG Pull-down buttons](https://developer.apple.com/design/human-interface-guidelines/pull-down-buttons)
- **gog-marketing:** The account ⋮ menu should close on Escape, outside click and
  hashchange. Give it an account-email header and confirm before Disconnect
  (**N04**). Replace the native `<select>` model picker with a compact menu
  button ("Automatic · Free ▾") in the composer toolbar (**DR06**). The avatar
  menu holds Sign out (**L09**).

### Empty state
*Aliases:* Blankslate, Empty prompt, Zero state, No results · [Gallery](https://component.gallery/components/empty-state/)
- **What:** A message shown when a view has no data, usually with a next action.
- **Use:** For first use (nothing yet), no results from a search or filter, and
  cleared or done states. **Avoid:** Leaving a blank region, or showing a
  generic message that doesn't distinguish "nothing exists" from "nothing
  matches". **Instead:** n/a. This is the convention; design each empty state
  deliberately (NN/g).
- **Behaviour & a11y:** A short heading, one sentence of reason, and one primary
  action (or "Clear filter"). Announce filter-result changes via `role=status`.
  No decorative illustration tiles (DESIGN.md).
- **Seen in:** Atlassian Empty state · Primer Blankslate · Elastic Empty prompt · Orbit Empty state
- **Sources:** [NN/g Designing empty states in complex apps](https://www.nngroup.com/articles/empty-state-interface-design/) ·
  [M3 Lists](https://m3.material.io/components/lists/overview) ·
  [HIG Loading](https://developer.apple.com/design/human-interface-guidelines/loading)
  *(no USWDS page)*
- **gog-marketing:** Distinguish "No assets found yet" from "No assets match
  'x' · Clear search" (**L06**). The empty account must clear the stale header
  counts (**U03**). The new-chat empty state needs room above the composer
  (**C04**). Completed setup becomes "You're set up — open chat" (**L01**). Show
  the "Connected · nothing to choose yet" neutral state (**L05**).

### Fieldset
*Aliases:* Field set, Choice group, Input group · [Gallery](https://component.gallery/components/fieldset/)
- **What:** A wrapper for related form fields, with a legend.
- **Use:** For groups of radios or checkboxes, and for multi-part answers.
  **Avoid:** For a single field. **Instead:** a plain [Label](#label).
- **Behaviour & a11y:** Use `<fieldset><legend>`. The legend is read before each
  control, so keep it short.
- **Seen in:** GOV.UK Fieldset · Polaris Choice list · Gestalt Fieldset · Fluent ChoiceGroup
- **Sources:** [GOV.UK Fieldset](https://design-system.service.gov.uk/components/fieldset/) ·
  [W3C WAI Forms tutorial](https://www.w3.org/WAI/tutorials/forms/) ·
  [NN/g Form white space](https://www.nngroup.com/articles/form-design-white-space/) ·
  [USWDS Form](https://designsystem.digital.gov/components/form/)
- **gog-marketing:** The consent form's Workspace / Marketing groups. AI tool
  groups per service (**D03**). The "Writes" sub-group (**L12**).

### File
*Aliases:* Attachment, Download · [Gallery](https://component.gallery/components/file/)
- **What:** Represents an uploaded or downloadable file: its name, type, size
  and actions.
- **Use:** To show an attachment or download. **Avoid:** Using it for
  non-file resources. **Instead:** a [List](#list) row.
- **Behaviour & a11y:** The link text includes the file name and type/size
  ("Report.pdf, 2 MB").
- **Seen in:** Lightning File *(only Gallery example)*
- **Sources:** [Gallery](https://component.gallery/components/file/) ·
  [HIG File management](https://developer.apple.com/design/human-interface-guidelines/file-management)
  *(no NN/g or USWDS decision page)*
- **gog-marketing:** n/a yet. Possibly chat attachments or exports later.

### File upload
*Aliases:* File input, Dropzone, File uploader · [Gallery](https://component.gallery/components/file-upload/)
- **What:** An input for uploading files from the device.
- **Use:** Only when documents are needed. **Avoid:** Asking for optional
  documents (USWDS). **Instead:** a link to a cloud file picker, such as the
  Google Picker for Drive files.
- **Behaviour & a11y:** Use a native `<input type=file>` with a label. Show
  accepted types and size. The dropzone is an enhancement, not the only way in.
- **Seen in:** GOV.UK File upload · Carbon File uploader · Polaris Drop zone · Lightning File Selector
- **Sources:** [USWDS File input](https://designsystem.digital.gov/components/file-input/) ·
  [GOV.UK File upload](https://design-system.service.gov.uk/components/file-upload/)
- **gog-marketing:** n/a. "Choose Google files" is the Google Picker, not an upload.

### Footer
*Aliases:* Site footer · [Gallery](https://component.gallery/components/footer/)
- **What:** The bottom region of a page, with legal, contact and secondary links.
- **Use:** On public pages, for privacy, terms, contact and about.
  **Avoid:** "Disclaimer bloat" (USWDS). **Instead:** a slim footer with
  five or fewer links.
- **Behaviour & a11y:** Use `<footer>` (contentinfo) once per page.
- **Seen in:** GOV.UK Footer · Duet Footer · USWDS Footer
- **Sources:** [USWDS Footer](https://designsystem.digital.gov/components/footer/) ·
  [NN/g Footers 101](https://www.nngroup.com/articles/footers/)
- **gog-marketing:** Give `/about` the shared public shell and footer (**P02**).
  The footer contact link fixes **P03**.

### Form
*Aliases:* — · [Gallery](https://component.gallery/components/form/)
- **What:** A group of inputs submitted together.
- **Use:** To collect information. **Avoid:** Long single pages with
  conditional logic, and Reset or Clear buttons. **Instead:** a multi-step
  pattern (see [pattern catalogue](pattern-catalogue.md#multi-step-workflows-and-wizards)),
  or [Toggles](#toggle) for instant settings.
- **Behaviour & a11y:** A visible label for every input. Mark required fields.
  Validate on submit or on blur, never while the user is still typing a format
  field. Use an error summary at the top that links to inline errors. Use
  `novalidate` with custom messages instead of native validation bubbles.
- **Seen in:** Carbon Form · Atlassian Form · Ant Design Form · Bootstrap Form
- **Sources:** [USWDS Form](https://designsystem.digital.gov/components/form/) ·
  [NN/g Website forms usability: top 10](https://www.nngroup.com/articles/web-form-design/) ·
  [NN/g Reporting errors in forms](https://www.nngroup.com/articles/errors-forms-design-guidelines/) ·
  [NN/g Required fields](https://www.nngroup.com/articles/required-fields/) ·
  [GOV.UK Error summary](https://design-system.service.gov.uk/components/error-summary/) ·
  [HIG Entering data](https://developer.apple.com/design/human-interface-guidelines/entering-data)
- **gog-marketing:** The consent form, asset and tool permission forms, and
  sign-in. Replace the native validation bubble (**L07**). Use one save model for
  assets and tools, or warn about unsaved changes (**D03**).

### Header
*Aliases:* App bar, Top app bar, Global header, Page header, Topbar · [Gallery](https://component.gallery/components/header/)
- **What:** The bar across the top of every page: product name, main navigation
  and the account.
- **Use:** On every page, identically. **Avoid:** Different headers per area.
  **Instead:** n/a. Use one shared partial.
- **Behaviour & a11y:** `<header>` (banner), the skip link first, the product name
  linking Home, the account menu last. On mobile, collapse secondary actions into
  icon buttons that have accessible names.
- **Seen in:** GOV.UK Header · Material Top app bar · Atlassian Page header · Primer Page header
- **Sources:** [USWDS Header](https://designsystem.digital.gov/components/header/) ·
  [NN/g Menu-design checklist](https://www.nngroup.com/articles/menu-design/) ·
  [M3 App bars](https://m3.material.io/components/app-bars/overview) ·
  [GOV.UK Service navigation](https://design-system.service.gov.uk/components/service-navigation/)
- **gog-marketing:** Use one `AppHeader` on Home and `/chat` (**L09, DR01**). Help
  points at real help (**N02**). On mobile, move "Add Google account" out of the
  title row (**DR10, D04**). Give `/about` the public header (**P02**).

### Heading
*Aliases:* Title, Section header · [Gallery](https://component.gallery/components/heading/)
- **What:** A title that introduces a section.
- **Use:** One `<h1>` per page, then a nested `h2`/`h3` outline. Descriptive,
  with keywords first. **Avoid:** Eyebrows on every card, multiple H1s, and
  forced `<br>` breaks. **Instead:** fewer, clearer headings, and
  `text-wrap: balance`.
- **Behaviour & a11y:** The heading levels form the screen-reader outline. Keep
  visual size and level aligned with one type scale.
- **Seen in:** Polaris Heading · Primer Heading · Spectrum Heading · Gestalt Heading
- **Sources:** [NN/g Layer-cake scanning](https://www.nngroup.com/articles/layer-cake-pattern-scanning/) ·
  [NN/g Text scanning patterns](https://www.nngroup.com/articles/text-scanning-patterns-eyetracking/) ·
  [USWDS Typography](https://designsystem.digital.gov/components/typography/) ·
  [HIG Typography](https://developer.apple.com/design/human-interface-guidelines/typography)
- **gog-marketing:** Use a 6-step type scale with 3 weights (**DR02**). Use one H1
  on sign-in (**L07**). Use about one eyebrow per screen (**DR11**). Remove
  forced breaks (**DR12**).

### Hero
*Aliases:* Jumbotron, Banner · [Gallery](https://component.gallery/components/hero/)
- **What:** A large banner at the top of a landing page.
- **Use:** On a marketing landing page, for one message and one CTA.
  **Avoid:** On task pages such as sign-in or dashboards. **Instead:** a
  plain page title.
- **Behaviour & a11y:** It contains the page H1. Text over images needs
  sufficient contrast.
- **Seen in:** Duet Hero Area *(only Gallery example)*
- **Sources:** [Gallery](https://component.gallery/components/hero/) ·
  [NN/g Carousel usability (hero alternative)](https://www.nngroup.com/articles/designing-effective-carousels/)
- **gog-marketing:** Landing page only. Remove the brief-themed hero from
  sign-in (**L07**). Balance the headings (**DR12**).

### Icon
*Aliases:* Glyph, Symbol · [Gallery](https://component.gallery/components/icon/)
- **What:** A small graphic symbol that hints at purpose.
- **Use:** Paired with text, for conventional meanings. **Avoid:** When the
  meaning is ambiguous, or to compensate for poor hierarchy (USWDS). Avoid
  decorative tiles. **Instead:** a text label. Always use a visible label for
  navigation icons (NN/g).
- **Behaviour & a11y:** Decorative icons get `aria-hidden=true`. Icon-only
  buttons need an accessible name and a tooltip. Use one stroke system (Lucide).
- **Seen in:** Polaris Icon · Atlassian Icon · Primer Icon · Gestalt Icon
- **Sources:** [USWDS Icon](https://designsystem.digital.gov/components/icon/) ·
  [NN/g Icon usability](https://www.nngroup.com/articles/icon-usability/) ·
  [HIG Icons](https://developer.apple.com/design/human-interface-guidelines/icons) ·
  [M3 Icon buttons](https://m3.material.io/components/icon-buttons/overview)
- **gog-marketing:** Replace the text-abbreviation "icons" (BQ, GA, Go…) with
  official product marks (**L04, DR07**). Replace sparkles (✨) for services with
  a plug or service glyph (**DR07**). Remove chevrons from non-interactive rows,
  or make the rows links (**L03**).

### Image
*Aliases:* Picture, Thumbnail, Logo · [Gallery](https://component.gallery/components/image/)
- **What:** An embedded image.
- **Use:** For recognisable product logos and real content. **Avoid:**
  Decorative stock imagery in task flows. **Instead:** nothing.
- **Behaviour & a11y:** Meaningful images get `alt`. Logos next to a visible
  name get `alt=""`. Set explicit width and height to avoid layout shift.
- **Seen in:** Polaris Image / Thumbnail · Gestalt Image · Fluent Image · Elastic Image
- **Sources:** [HIG Images](https://developer.apple.com/design/human-interface-guidelines/images) ·
  [M3 Alt text](https://m3.material.io/foundations/content-design/alt-text) ·
  [Google branding guidelines](https://developers.google.com/identity/branding-guidelines)
- **gog-marketing:** Official Google product marks, only for services the repo
  supports (PRODUCT.md, **L04, DR07**).

### Label
*Aliases:* Form label, Field label · [Gallery](https://component.gallery/components/label/)
- **What:** The text name of a form input.
- **Use:** On every input, always visible and outside the field. **Avoid:**
  Placeholder-as-label (NN/g "Placeholders are harmful"). **Instead:**
  n/a.
- **Behaviour & a11y:** Use `<label for>` or a wrapping label so the label click
  focuses or toggles the control, which also enlarges the target. Use plain
  words with keywords first.
- **Seen in:** Spectrum Field label · Gestalt Label · Fluent Label · Backpack Form label
- **Sources:** [NN/g Placeholders in form fields are harmful](https://www.nngroup.com/articles/form-design-placeholders/) ·
  [NN/g Form white space](https://www.nngroup.com/articles/form-design-white-space/) ·
  [W3C WAI Forms: labels](https://www.w3.org/WAI/tutorials/forms/) ·
  [HIG Labels](https://developer.apple.com/design/human-interface-guidelines/labels)
- **gog-marketing:** Make the whole asset or tool row the label (**L12**). Rename
  "change tools" to "Tools that can edit" (**D03**). Use plain-language copy
  instead of provider IDs (**C05**).

### Link
*Aliases:* Anchor, Hyperlink · [Gallery](https://component.gallery/components/link/)
- **What:** A reference to another page or location.
- **Use:** For navigation. **Avoid:** For actions (use [Button](#button)),
  and for vague text ("Learn more"). **Instead:** descriptive, keyword-first
  link text.
- **Behaviour & a11y:** Coloured and underlined in body text. **The link label
  must match the destination**, because a link is a promise (NN/g). Reserve
  brand blue for links and primary actions.
- **Seen in:** Carbon Link · Polaris Link · Primer Link · Spectrum Link
- **Sources:** [USWDS Link](https://designsystem.digital.gov/components/link/) ·
  [NN/g A link is a promise](https://www.nngroup.com/articles/link-promise/) ·
  [NN/g Writing hyperlinks](https://www.nngroup.com/articles/writing-links/) ·
  [NN/g Visualizing links](https://www.nngroup.com/articles/guidelines-for-visualizing-links/) ·
  [APG Link](https://www.w3.org/WAI/ARIA/apg/patterns/link/)
- **gog-marketing:** Fix the label/destination mismatches: Activity → Access model
  (**N01**), Help → MCP endpoint (**N02**), "Google settings" → Google data
  (**L11**). Turn the request-access dead end into a contact link (**P03**).
  Stop using blue for non-link chat text (**DR04**).

### List
*Aliases:* Description list, Action list, List group, Collection · [Gallery](https://component.gallery/components/list/)
- **What:** A group of related items: ordered, unordered or description lists, or
  interactive rows.
- **Use:** For a homogeneous set of items scanned by name, such as accounts,
  services, assets or conversations. **Avoid:** For long narrative text, or
  when items need comparison across several attributes. **Instead:**
  [Table](#table) for multi-attribute comparison, and cards for heterogeneous
  summaries.
- **Behaviour & a11y:** Use `<ul>/<ol>/<dl>`. In interactive rows, one primary
  target per row. Give clickable rows a hover state and a chevron; non-clickable
  rows get neither.
- **Seen in:** Carbon List · Polaris List · Material List · Primer Action list
- **Sources:** [USWDS List](https://designsystem.digital.gov/components/list/) ·
  [USWDS Collection](https://designsystem.digital.gov/components/collection/) ·
  [M3 Lists](https://m3.material.io/components/lists/overview) ·
  [HIG Lists and tables](https://developer.apple.com/design/human-interface-guidelines/lists-and-tables) ·
  [GOV.UK Summary list](https://design-system.service.gov.uk/components/summary-list/)
- **gog-marketing:** Make service rows about 46px (logo, name, "N enabled",
  status, chevron) as in render 02 (**DR03**). Rows are links or lose their
  chevron (**L03**). Use neutral conversation-list titles with blue only for the
  selected indicator (**DR04**). Merge the duplicate account lists (**D05**).
  Fix the "1 accounts" count and pluralisation (**D02**).

### Modal
*Aliases:* Dialog, Popup, Modal window, Alert dialog · [Gallery](https://component.gallery/components/modal/)
- **What:** A window over the page that blocks the page until the user acts.
- **Use:** To confirm consequential or irreversible actions, and for unsaved-work
  warnings (USWDS). **Avoid:** Use it as a last resort. Avoid it for
  multi-step processes and for routine error, success or warning messages
  (USWDS). **Instead:** inline [Alert](#alert), [Toast](#toast) with undo,
  or a dedicated page.
- **Behaviour & a11y:** Use native `<dialog>` with `showModal()`, or
  `role=dialog`/`alertdialog` with `aria-modal`. Focus moves to the first
  control (or the safest one for destructive dialogs). Trap focus, close on
  **Escape** and return focus. The title names the object, and the buttons are
  specific verbs ("Delete conversation").
- **Seen in:** Carbon Modal · Polaris Modal · Material Dialog · Atlassian Modal dialog
- **Sources:** [USWDS Modal](https://designsystem.digital.gov/components/modal/) ·
  [NN/g Modal & nonmodal dialogs](https://www.nngroup.com/articles/modal-nonmodal-dialog/) ·
  [NN/g Confirmation dialogs](https://www.nngroup.com/articles/confirmation-dialog/) ·
  [NN/g Popups: 10 problematic trends](https://www.nngroup.com/articles/popups/) ·
  [APG Dialog (modal)](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/) ·
  [APG Alert dialog](https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/) ·
  [M3 Dialogs](https://m3.material.io/components/dialogs/overview) ·
  [HIG Alerts](https://developer.apple.com/design/human-interface-guidelines/alerts) ·
  [HIG Modality](https://developer.apple.com/design/human-interface-guidelines/modality)
- **gog-marketing:** Rename and Delete dialogs already work; keep them. Delete
  needs danger styling with Cancel first (**L10**). Add a Disconnect-account
  confirmation that names the account (**N04**).

### Navigation
*Aliases:* Nav, Menu, Side navigation, Navigation bar, Bottom navigation, Tab bar, Navigation rail · [Gallery](https://component.gallery/components/navigation/)
- **What:** A container of links to the app's main destinations.
- **Use:** Visible navigation wherever space allows. On mobile, use a bottom
  bar for 3–5 top-level destinations. **Avoid:** Hidden (hamburger)
  navigation on desktop. Avoid running two navigation systems at once. Avoid
  labels that don't match their destinations. **Instead:** side navigation on
  desktop, a bottom bar on mobile, and tabs for sibling views.
- **Behaviour & a11y:** `<nav aria-label>` with `aria-current="page"` on the
  active item. Icons always have text labels. Targets ≥44px.
- **Seen in:** Material Navigation bar / rail / drawer · Primer Nav list · Spectrum Side navigation · Gestalt SideNavigation
- **Sources:** [USWDS Side navigation](https://designsystem.digital.gov/components/side-navigation/) ·
  [NN/g Basic patterns for mobile navigation](https://www.nngroup.com/articles/mobile-navigation-patterns/) ·
  [NN/g Hamburger menus hurt UX metrics](https://www.nngroup.com/articles/hamburger-menus/) ·
  [NN/g Menu-design checklist](https://www.nngroup.com/articles/menu-design/) ·
  [NN/g Local navigation](https://www.nngroup.com/articles/local-navigation/) ·
  [M3 Navigation bar](https://m3.material.io/components/navigation-bar/overview) ·
  [M3 Navigation rail](https://m3.material.io/components/navigation-rail/overview) ·
  [HIG Tab bars](https://developer.apple.com/design/human-interface-guidelines/tab-bars) ·
  [HIG Sidebars](https://developer.apple.com/design/human-interface-guidelines/sidebars)
- **gog-marketing:** Keep the bottom bar on mobile and drop the nav drawer
  (**L13, N03**). Rename or build Activity (**N01**). Separate Help from
  Settings (**N02**). Split the routes Home / Google data / Tools / Connect AI
  (**D05**). Nav items are 42px today and must be ≥44px (**L12**).

### Pagination
*Aliases:* Pager, Load more · [Gallery](https://component.gallery/components/pagination/)
- **What:** Splits content across pages, with controls to move between them.
- **Use:** For search results and multi-page collections (USWDS). **Avoid:**
  For meaningful groupings or sequential steps. **Instead:** side
  navigation, [Progress indicator](#progress-indicator), "Load more", or
  infinite scroll for browse-only feeds (NN/g).
- **Behaviour & a11y:** `<nav aria-label="Pagination">` with `aria-current` on the
  current page. Keep the position on Back.
- **Seen in:** Carbon Pagination · Atlassian Pagination · Primer Pagination · Elastic Pagination
- **Sources:** [USWDS Pagination](https://designsystem.digital.gov/components/pagination/) ·
  [NN/g Infinite scrolling: when to use, when to avoid](https://www.nngroup.com/articles/infinite-scrolling-tips/) ·
  [NN/g Pagination preferences and "View all"](https://www.nngroup.com/articles/item-list-view-all/) ·
  [GOV.UK Pagination](https://design-system.service.gov.uk/components/pagination/)
- **gog-marketing:** n/a yet. Likely "Load more" for a future Activity log
  (**N01**) and for long asset lists.

### Popover
*Aliases:* Popconfirm, Toggletip, Callout · [Gallery](https://component.gallery/components/popover/)
- **What:** A click-triggered overlay anchored to an element. It can contain
  interactive content.
- **Use:** For brief supplementary detail or a small set of controls next to
  their trigger, such as model limits or "what does this mean". **Avoid:**
  For critical or required information, and for long content. **Instead:**
  inline text, [Accordion](#accordion), or [Drawer](#drawer).
- **Behaviour & a11y:** Use a `<button aria-expanded>` trigger, or the native
  `popover` attribute. Close on Escape and outside click, and return focus. It
  must not trap focus unless it's modal.
- **Seen in:** Carbon Popover / Toggletip · Polaris Popover · Primer Popover · Spectrum Popover
- **Sources:** [NN/g Popups: 10 problematic trends](https://www.nngroup.com/articles/popups/) ·
  [NN/g Tooltip guidelines](https://www.nngroup.com/articles/tooltip-guidelines/) ·
  [HIG Popovers](https://developer.apple.com/design/human-interface-guidelines/popovers) ·
  [APG Disclosure](https://www.w3.org/WAI/ARIA/apg/patterns/disclosure/)
- **gog-marketing:** Fold the sticky chat note into a header info button with a
  popover (**C06**). Put model limits and usage in the model-menu popover
  (**DR06, C05**).

### Progress bar
*Aliases:* Progress, Meter, Usage bar · [Gallery](https://component.gallery/components/progress-bar/)
- **What:** A horizontal bar showing the completion or level of a long task or a
  quota.
- **Use:** For determinate waits over about 10 seconds (NN/g), and as a meter for
  usage against a limit. **Avoid:** Fake progress, and short waits.
  **Instead:** [Spinner](#spinner) for 2–10s waits, a button busy state, or
  text ("12 of 100 used").
- **Behaviour & a11y:** Use `<progress>`/`role=progressbar` with `aria-valuenow`
  and `aria-valuemax` for tasks, and `<meter>`/`role=meter` for quotas. Always
  give a text equivalent.
- **Seen in:** Carbon Progress bar · Atlassian Progress bar · Primer Progress bar · Spectrum Progress bar / Meter
- **Sources:** [NN/g Progress indicators](https://www.nngroup.com/articles/progress-indicators/) ·
  [NN/g Response-time limits](https://www.nngroup.com/articles/response-times-3-important-limits/) ·
  [APG Meter](https://www.w3.org/WAI/ARIA/apg/patterns/meter/) ·
  [M3 Progress indicators](https://m3.material.io/components/progress-indicators/overview) ·
  [HIG Progress indicators](https://developer.apple.com/design/human-interface-guidelines/progress-indicators) ·
  [HIG Gauges](https://developer.apple.com/design/human-interface-guidelines/gauges)
- **gog-marketing:** Use a meter for the chat allowance: "You've used X of Y today
  · resets HH:MM" (**C05, C01**). Discovery uses a busy button, not a bar
  (render 07: no fake progress).

### Progress indicator
*Aliases:* Steps, Stepper, Step indicator, Progress tracker, Task list, Timeline · [Gallery](https://component.gallery/components/progress-indicator/)
- **What:** Shows progress through a series of discrete steps.
- **Use:** For multi-page linear processes with three or more steps (USWDS Step
  indicator), and for a setup checklist whose steps can be done in any order
  (GOV.UK Task list). **Avoid:** For non-linear flows that use a step
  indicator. Avoid it when the step count changes with input, and for very short
  forms (USWDS). **Instead:** a task-list checklist (non-linear), or USWDS
  Process list (static explanation).
- **Behaviour & a11y:** Use an `<ol>`. The current step gets `aria-current="step"`.
  Status is text ("Completed", "Not started"), not colour or ticks alone.
- **Seen in:** Carbon Progress indicator · Atlassian Progress tracker · Lightning Path · GOV.UK Task list
- **Sources:** [USWDS Step indicator](https://designsystem.digital.gov/components/step-indicator/) ·
  [USWDS Process list](https://designsystem.digital.gov/components/process-list/) ·
  [GOV.UK Task list](https://design-system.service.gov.uk/components/task-list/) ·
  [NN/g Wizards](https://www.nngroup.com/articles/wizards/)
- **gog-marketing:** The onboarding checklist computed from real state (render 10).
  The "Your next step" card becomes a compact task list. When every step is done,
  say so and hide it (**L01, DR03**). Keep it global, or label it with the
  account (**D01**).

### Quote
*Aliases:* Blockquote, Pull quote · [Gallery](https://component.gallery/components/quote/)
- **What:** A displayed quotation.
- **Use:** For testimonials or quoted source text. **Avoid:** As decoration.
  **Instead:** plain text.
- **Behaviour & a11y:** `<blockquote>` with `<cite>`.
- **Seen in:** Backpack Blockquote *(only Gallery example)*
- **Sources:** [Gallery](https://component.gallery/components/quote/) *(no NN/g or USWDS decision page)*
- **gog-marketing:** n/a. Possibly markdown blockquotes in chat replies.

### Radio button
*Aliases:* Radio, Radio group, Radio cards · [Gallery](https://component.gallery/components/radio-button/)
- **What:** Chooses exactly one option from a small set of mutually exclusive
  options.
- **Use:** For single-select with fewer than about 7 options, all visible.
  **Avoid:** For multi-select or a single yes/no item, where USWDS says use a
  checkbox. **Instead:** [Select](#select) (7–15),
  [Combobox](#combobox) (15+), [Segmented control](#segmented-control)
  (a view switch).
- **Behaviour & a11y:** Native radios in a `<fieldset>`. Arrow keys move within
  the group. The whole label is the target (≥44px).
- **Seen in:** GOV.UK Radios · Carbon Radio button · Material Radio button · Atlassian Radio
- **Sources:** [USWDS Radio buttons](https://designsystem.digital.gov/components/radio-buttons/) ·
  [NN/g Checkboxes vs radio buttons](https://www.nngroup.com/articles/checkboxes-vs-radio-buttons/) ·
  [APG Radio group](https://www.w3.org/WAI/ARIA/apg/patterns/radio/) ·
  [M3 Radio button](https://m3.material.io/components/radio-button/overview)
- **gog-marketing:** The "Access level" control in the asset picker (render 06), if
  the levels are mutually exclusive (read only / read and write) (**DR09**).

### Rating
*Aliases:* Star rating, Rate · [Gallery](https://component.gallery/components/rating/)
- **What:** Displays or sets a star rating.
- **Use:** For product reviews. **Avoid:** In an access-control product.
  **Instead:** thumbs up/down feedback on chat replies, if needed.
- **Behaviour & a11y:** The input is a radio group with text values.
- **Seen in:** Spectrum Rating · Fluent Rating · Base Web Rating · Ant Design Rate
- **Sources:** [HIG Ratings and reviews](https://developer.apple.com/design/human-interface-guidelines/ratings-and-reviews) ·
  [HIG Rating indicators](https://developer.apple.com/design/human-interface-guidelines/rating-indicators)
  *(no NN/g or USWDS component page)*
- **gog-marketing:** n/a.

### Rich text editor
*Aliases:* RTE, WYSIWYG, Markdown editor · [Gallery](https://component.gallery/components/rich-text-editor/)
- **What:** An editor for formatted text.
- **Use:** When authoring formatted documents. **Avoid:** For chat prompts.
  **Instead:** a plain [Textarea](#textarea), with markdown *rendered* in
  replies.
- **Behaviour & a11y:** The toolbar follows APG Toolbar, and the editor area
  has `role=textbox` with `aria-multiline`.
- **Seen in:** Atlassian Editor · Lightning Rich Text Editor · Elastic Markdown editor
- **Sources:** [Gallery](https://component.gallery/components/rich-text-editor/) ·
  [APG Toolbar](https://www.w3.org/WAI/ARIA/apg/patterns/toolbar/) ·
  [HIG Text views](https://developer.apple.com/design/human-interface-guidelines/text-views)
  *(no NN/g or USWDS decision page)*
- **gog-marketing:** n/a.

### Search input
*Aliases:* Search, Search field, Search bar · [Gallery](https://component.gallery/components/search-input/)
- **What:** An input for finding content by keyword.
- **Use:** As a visible box, not just an icon, on desktop (NN/g). Use one
  search with a clear scope. **Avoid:** On very small sites (USWDS). Avoid
  multiple competing search boxes. **Instead:** a filter field scoped to a
  list.
- **Behaviour & a11y:** `<input type=search>` inside `role=search` with a label,
  at least 27 characters wide (USWDS). Show the active query and the result
  count, and give a clear button.
- **Seen in:** Carbon Search · Polaris Search field · Material Search · Gestalt SearchField
- **Sources:** [USWDS Search](https://designsystem.digital.gov/components/search/) ·
  [NN/g Search: visible and simple](https://www.nngroup.com/articles/search-visible-and-simple/) ·
  [NN/g Intranet-search essentials](https://www.nngroup.com/articles/intranet-search/) ·
  [M3 Search](https://m3.material.io/components/search/overview) ·
  [HIG Search fields](https://developer.apple.com/design/human-interface-guidelines/search-fields)
- **gog-marketing:** Global search scoped to accounts, services and asset metadata
  (PRODUCT.md). Show "1 of 6" counts and an active-filter chip with Clear, and
  don't silently persist the query across reloads (**L06**). It's missing from
  the `/chat` header (**L09**).

### Segmented control
*Aliases:* Toggle button group, Content switcher, Segmented buttons · [Gallery](https://component.gallery/components/segmented-control/)
- **What:** Joined buttons that switch between views or filters of the same data.
- **Use:** For 2–5 mutually exclusive views or filters with short labels.
  **Avoid:** For navigation between pages, and for form values submitted
  later. **Instead:** [Tabs](#tabs) (panels), [Radio button](#radio-button)
  (form value).
- **Behaviour & a11y:** A radiogroup or a set of `aria-pressed` buttons. Each
  segment ≥44px tall. The selected state isn't colour-only.
- **Seen in:** Carbon Content switcher · Primer Segmented control · Gestalt SegmentedControl · Base Web Segmented Control
- **Sources:** [NN/g UI-elements glossary](https://www.nngroup.com/articles/ui-elements-glossary/) ·
  [M3 Segmented buttons](https://m3.material.io/components/segmented-buttons/overview) ·
  [HIG Segmented controls](https://developer.apple.com/design/human-interface-guidelines/segmented-controls)
- **gog-marketing:** The All / Reads / Changes tool filter. It's 30px today and
  needs to be ≥44px (**L12**). Its 0px inner radius must match the group radius
  (**DR11**).

### Select
*Aliases:* Dropdown, Select input, Pop-up button · [Gallery](https://component.gallery/components/select/)
- **What:** A form input that shows the chosen value and opens a list of options.
- **Use:** Sparingly, for about 7–15 options in limited space (USWDS).
  **Avoid:** Fewer than 7 options (use radios) or more than 15 (use a combobox).
  Avoid it for actions. **Instead:** [Radio button](#radio-button),
  [Combobox](#combobox), [Dropdown menu](#dropdown-menu).
- **Behaviour & a11y:** A native `<select>` is the most accessible choice. If you
  style a custom one, follow APG Listbox or Select-only combobox fully.
  Disabled options carry a reason.
- **Seen in:** GOV.UK Select · Carbon Dropdown · Polaris Select · Atlassian Select
- **Sources:** [USWDS Select](https://designsystem.digital.gov/components/select/) ·
  [NN/g Dropdowns: design guidelines](https://www.nngroup.com/articles/drop-down-menus/) ·
  [NN/g Does your form need a dropdown?](https://www.nngroup.com/articles/dropdown-list/) ·
  [APG Listbox](https://www.w3.org/WAI/ARIA/apg/patterns/listbox/) ·
  [HIG Pop-up buttons](https://developer.apple.com/design/human-interface-guidelines/pop-up-buttons)
- **gog-marketing:** The chat model picker is a native `<select>` placed under the
  composer. Move it into the composer toolbar as a styled pop-up or menu button
  (**DR06**). A persistent account switcher (**D01**) can be a select-style
  pop-up button.

### Separator
*Aliases:* Divider, Horizontal rule · [Gallery](https://component.gallery/components/separator/)
- **What:** A line between groups of content.
- **Use:** Between groups inside a menu or a list when spacing alone isn't
  enough. **Avoid:** As a substitute for spacing and headings. **Instead:**
  white space (`--space-*`).
- **Behaviour & a11y:** Use `<hr>` or `role=separator` for semantic breaks, and
  CSS borders for decorative ones.
- **Seen in:** Polaris Divider · Material Divider · Spectrum Divider · Gestalt Divider
- **Sources:** [M3 Divider](https://m3.material.io/components/divider/overview) ·
  [NN/g Form white space](https://www.nngroup.com/articles/form-design-white-space/)
- **gog-marketing:** Separate the destructive items in the account menu (**N04**).

### Skeleton
*Aliases:* Skeleton loader, Shimmer, Placeholder · [Gallery](https://component.gallery/components/skeleton/)
- **What:** Grey placeholder shapes in the layout of content that is still
  loading.
- **Use:** For full-page or region loads of about 1–10s where the layout is known
  (NN/g). **Avoid:** Loads under 1s (it flashes), frame-only skeletons, and
  fake content. **Instead:** nothing for under 1s, a [Spinner](#spinner) for
  an unknown layout, or a [Progress bar](#progress-bar) for long determinate
  waits.
- **Behaviour & a11y:** `aria-busy=true` on the region, one visually hidden
  "Loading…" status, and honour `prefers-reduced-motion` (no shimmer).
- **Seen in:** Fluent Shimmer · Elastic Skeleton · Paste Skeleton loader · Base Web Skeleton
- **Sources:** [NN/g Skeleton screens 101](https://www.nngroup.com/articles/skeleton-screens/) ·
  [M3 Loading indicator](https://m3.material.io/components/loading-indicator/overview) ·
  [HIG Loading](https://developer.apple.com/design/human-interface-guidelines/loading)
- **gog-marketing:** n/a yet, because pages are server-rendered. Only use it if
  the asset or tool panels move to async loading.

### Skip link
*Aliases:* Skip to content · [Gallery](https://component.gallery/components/skip-link/)
- **What:** The first focusable link, which jumps past the header to the main
  content.
- **Use:** On every page. **Avoid:** n/a. **Instead:** n/a.
- **Behaviour & a11y:** Visible on focus. It targets `<main id>`.
- **Seen in:** GOV.UK Skip link · Backpack Skip link · Cauldron SkipLink
- **Sources:** [GOV.UK Skip link](https://design-system.service.gov.uk/components/skip-link/) ·
  [APG Landmarks](https://www.w3.org/WAI/ARIA/apg/patterns/landmarks/)
- **gog-marketing:** It exists on the landing page. It's missing on `/about`
  (**P02**). Confirm it's on `/chat` and the new 404 page (**P01**).

### Slider
*Aliases:* Range input, Range slider · [Gallery](https://component.gallery/components/slider/)
- **What:** Drags a value within a range.
- **Use:** When a relative value matters more than a precise one, with real-time
  feedback (USWDS, NN/g). **Avoid:** For precise numbers, or when the effect is
  delayed. **Instead:** [Text input](#text-input) or [Stepper](#stepper).
- **Behaviour & a11y:** Use `<input type=range>` or APG Slider. Arrow keys change
  the value and the value is announced.
- **Seen in:** Carbon Slider · Material Slider · Atlassian Range · Spectrum Slider
- **Sources:** [USWDS Range slider](https://designsystem.digital.gov/components/range-slider/) ·
  [NN/g Slider design: rules of thumb](https://www.nngroup.com/articles/gui-slider-controls/) ·
  [APG Slider](https://www.w3.org/WAI/ARIA/apg/patterns/slider/) ·
  [M3 Sliders](https://m3.material.io/components/sliders/overview)
- **gog-marketing:** n/a.

### Spinner
*Aliases:* Loader, Loading, Inline loading, Activity indicator · [Gallery](https://component.gallery/components/spinner/)
- **What:** An indeterminate animation showing that work is happening.
- **Use:** For waits of about 2–10s (NN/g), inline in the button or region that
  triggered them. **Avoid:** For waits over 10s, which need a determinate
  indicator or explanation, and for full-page loads with a known layout.
  **Instead:** [Progress bar](#progress-bar), [Skeleton](#skeleton), or
  streaming text.
- **Behaviour & a11y:** Pair it with a text status ("Checking assets…") in a
  live region. Set `aria-busy` on the button. Honour reduced motion.
- **Seen in:** Carbon Inline loading · Polaris Spinner · Atlassian Spinner · Primer Spinner
- **Sources:** [NN/g Progress indicators](https://www.nngroup.com/articles/progress-indicators/) ·
  [NN/g Response-time limits](https://www.nngroup.com/articles/response-times-3-important-limits/) ·
  [M3 Loading indicator](https://m3.material.io/components/loading-indicator/overview) ·
  [HIG Progress indicators](https://developer.apple.com/design/human-interface-guidelines/progress-indicators)
- **gog-marketing:** The discovery POST pending state (render 07). The chat "Gog is
  responding…" text plus Stop button, which already works; keep it.

### Stack
*Aliases:* Layout, Box, Inline · [Gallery](https://component.gallery/components/stack/)
- **What:** A layout wrapper that applies consistent spacing between children.
- **Use:** Everywhere spacing repeats. **Avoid:** Ad-hoc margins.
  **Instead:** n/a.
- **Behaviour & a11y:** It isn't semantic, so the children keep their own roles.
  Use `gap` with `--space-*` tokens and `min-width:0` on flex children (this
  prevents overflow).
- **Seen in:** Polaris Stack · Fluent Stack · Chakra UI Stack
- **Sources:** [M3 Grids and spacing](https://m3.material.io/foundations/layout/grids-spacing/overview) ·
  [HIG Layout](https://developer.apple.com/design/human-interface-guidelines/layout)
  *(no NN/g or USWDS decision page)*
- **gog-marketing:** Add `min-width:0` to the chat flex/grid children (**C02**).
  Keep mobile card gutters symmetric (**D04**). Use the 4px spacing scale
  throughout (**DR11**).

### Stepper
*Aliases:* Number input, Nudger, Quantity, Spin button · [Gallery](https://component.gallery/components/stepper/)
- **What:** A numeric field with − and + buttons.
- **Use:** For small numeric adjustments near a default. **Avoid:** For large
  ranges. **Instead:** [Text input](#text-input) with `inputmode=numeric`.
- **Behaviour & a11y:** APG Spinbutton. Up and Down arrows change the value.
  Each button ≥44px.
- **Seen in:** Carbon Number input · Polaris Number field · Gestalt NumberField · Fluent SpinButton
- **Sources:** [NN/g Listboxes vs dropdowns (control choice)](https://www.nngroup.com/articles/listbox-dropdown/) ·
  [APG Spinbutton](https://www.w3.org/WAI/ARIA/apg/patterns/spinbutton/) ·
  [HIG Steppers](https://developer.apple.com/design/human-interface-guidelines/steppers)
- **gog-marketing:** n/a.

### Table
*Aliases:* Data table, Data grid · [Gallery](https://component.gallery/components/table/)
- **What:** Rows and columns of data, sometimes with sorting and filtering.
- **Use:** For tabular data and directories of similarly structured items, where
  users compare attributes (USWDS, NN/g). **Avoid:** For non-tabular data, or
  as page layout. **Instead:** [List](#list), a description list, or cards.
- **Behaviour & a11y:** `<table>` with `<th scope>` and a `<caption>`. On narrow
  screens, stack rows into labelled pairs rather than dropping columns. Numbers
  are tabular and right-aligned.
- **Seen in:** GOV.UK Table · Carbon Data table · Polaris Index table · Atlassian Dynamic table
- **Sources:** [USWDS Table](https://designsystem.digital.gov/components/table/) ·
  [NN/g Data tables: four major user tasks](https://www.nngroup.com/articles/data-tables/) ·
  [NN/g Mobile tables](https://www.nngroup.com/articles/mobile-tables/) ·
  [APG Table](https://www.w3.org/WAI/ARIA/apg/patterns/table/) ·
  [HIG Lists and tables](https://developer.apple.com/design/human-interface-guidelines/lists-and-tables)
- **gog-marketing:** In the Google services table, fix the "1 accounts" count and
  pluralisation (**D02**). Keep "x enabled" on mobile rather than dropping the
  column (**D04**). Use compact rows (**DR03**). Add column headers to the asset
  picker (**DR09**).

### Tabs
*Aliases:* Tabbed interface, Tab bar (in-page), Tab view · [Gallery](https://component.gallery/components/tabs/)
- **What:** Switch between panels of related content within one view.
- **Use:** For parallel, peer panels where users need one at a time. Put the
  tabs at the top of the panel (NN/g). **Avoid:** For sequential steps, for
  content users need to compare side by side, and as global navigation.
  **Instead:** a [Progress indicator](#progress-indicator), one long page
  with headings, or [Navigation](#navigation).
- **Behaviour & a11y:** APG Tabs: `role=tablist/tab/tabpanel`, arrow keys between
  tabs and Tab into the panel. The selected tab is visually connected to its
  panel. If tabs change the URL, use links with `aria-current` instead.
- **Seen in:** GOV.UK Tabs · Carbon Tabs · Material Tabs · Atlassian Tabs
- **Sources:** [NN/g Tabs, used right](https://www.nngroup.com/articles/tabs-used-right/) ·
  [APG Tabs](https://www.w3.org/WAI/ARIA/apg/patterns/tabs/) ·
  [M3 Tabs](https://m3.material.io/components/tabs/overview) ·
  [HIG Tab views](https://developer.apple.com/design/human-interface-guidelines/tab-views) ·
  [GOV.UK Tabs](https://design-system.service.gov.uk/components/tabs/)
- **gog-marketing:** A mobile Google-data service tab strip (render 12, **DR10**).
  Service tabs in the asset picker rail (**DR09**). Link service rows to their
  asset tab (**L03**).

### Text input
*Aliases:* Text field, Input · [Gallery](https://component.gallery/components/text-input/)
- **What:** A single-line text field.
- **Use:** For unpredictable, free-form answers (USWDS). **Avoid:** Choosing
  from a fixed set, or entering paragraphs. **Instead:** [Select](#select) or
  [Radio button](#radio-button), [Textarea](#textarea).
- **Behaviour & a11y:** A visible label, a hint outside the field, the correct
  `type` and `autocomplete`, and width that matches the expected length. Show
  errors inline and in the summary.
- **Seen in:** GOV.UK Text input · Carbon Text input · Polaris Text field · Material Text field
- **Sources:** [USWDS Text input](https://designsystem.digital.gov/components/text-input/) ·
  [NN/g Placeholders are harmful](https://www.nngroup.com/articles/form-design-placeholders/) ·
  [M3 Text fields](https://m3.material.io/components/text-fields/overview) ·
  [HIG Text fields](https://developer.apple.com/design/human-interface-guidelines/text-fields)
- **gog-marketing:** The sign-in email field: match the app inputs and drop the
  native bubble (**L07, DR08**). The Rename dialog. Search (**L06**).

### Textarea
*Aliases:* Textbox, Text box, Composer · [Gallery](https://component.gallery/components/textarea/)
- **What:** A multi-line text field.
- **Use:** For long free text and chat prompts. **Avoid:** For short answers.
  Avoid character counters unless there's a legal or real limit (USWDS
  Character count). **Instead:** [Text input](#text-input).
- **Behaviour & a11y:** A label (it can be visually hidden in chat). It
  auto-grows to a max height. Enter sends and Shift+Enter adds a newline (chat
  convention), with a visible Send button too. Use one focus ring, on the
  wrapper or the field, not both.
- **Seen in:** GOV.UK Textarea · Polaris Text area · Atlassian Text area · Primer Textarea
- **Sources:** [GOV.UK Textarea](https://design-system.service.gov.uk/components/textarea/) ·
  [USWDS Character count](https://designsystem.digital.gov/components/character-count/) ·
  [M3 Text fields](https://m3.material.io/components/text-fields/overview) ·
  [HIG Text views](https://developer.apple.com/design/human-interface-guidelines/text-views)
- **gog-marketing:** The chat composer. Remove the double focus ring (**DR06**),
  keep it in the viewport on mobile (**C02**), and disable it, with the reason,
  before the user types when the quota is exhausted (**C01**).

### Toast
*Aliases:* Snackbar, Flag, Floating notification · [Gallery](https://component.gallery/components/toast/)
- **What:** A transient, non-modal message in a layer above the content.
- **Use:** For passive confirmations ("Saved", "Already connected") and an undo
  offer. **Avoid:** For errors or anything the user must act on (NN/g), and for
  critical info that disappears. **Instead:** [Alert](#alert) inline,
  [Modal](#modal).
- **Behaviour & a11y:** `role=status`/`aria-live=polite`. It doesn't steal focus.
  Leave it on screen long enough to read, or keep it until dismissed if it has
  an action. It must not cover the focused element or primary controls
  ([2.4.11](https://www.w3.org/WAI/WCAG22/Understanding/focus-not-obscured-minimum.html)).
- **Seen in:** Material Snackbar · Spectrum Toast · Gestalt Toast · Lightning Toast
- **Sources:** [NN/g Indicators, validations, notifications](https://www.nngroup.com/articles/indicators-validations-notifications/) ·
  [NN/g User control & freedom (undo snackbar)](https://www.nngroup.com/articles/user-control-and-freedom/) ·
  [M3 Snackbar](https://m3.material.io/components/snackbar/overview) ·
  [M3 Notifications content](https://m3.material.io/foundations/content-design/notifications)
- **gog-marketing:** The already-connected toast (render 11, `ErrConflict`). "Access
  saved" with Undo after asset or tool changes. Never use it for the limit error
  (**C03**).

### Toggle
*Aliases:* Switch, Lightswitch · [Gallery](https://component.gallery/components/toggle/)
- **What:** An on/off control that **takes effect immediately**.
- **Use:** For system settings or preferences that apply instantly, without Save
  (NN/g). **Avoid:** Inside forms that need Submit, and for multi-option
  choices. **Instead:** [Checkbox](#checkbox) (submitted later),
  [Radio button](#radio-button).
- **Behaviour & a11y:** `<button role=switch aria-checked>`, or a checkbox with
  `role=switch`. Use a direct label ("Email notifications"). The on/off state
  shows in position, not just colour. ≥44px.
- **Seen in:** Carbon Toggle · Polaris Switch · Material Switch · Atlassian Toggle
- **Sources:** [NN/g Toggle-switch guidelines](https://www.nngroup.com/articles/toggle-switch-guidelines/) ·
  [NN/g State-switch controls](https://www.nngroup.com/articles/state-switch-buttons/) ·
  [APG Switch](https://www.w3.org/WAI/ARIA/apg/patterns/switch/) ·
  [M3 Switch](https://m3.material.io/components/switch/overview) ·
  [HIG Toggles](https://developer.apple.com/design/human-interface-guidelines/toggles)
- **gog-marketing:** Decide between two models for access grants. Either use
  toggles that persist instantly, as PRODUCT.md implies ("persist
  immediately"), or keep checkboxes with one Save bar. Don't mix them. This
  resolves the two-Save-buttons risk (**D03**). Record the choice in DESIGN.md.

### Tooltip
*Aliases:* Toggletip, Hint · [Gallery](https://component.gallery/components/tooltip/)
- **What:** A short description shown on hover or focus (or on tap, as a
  toggletip).
- **Use:** For brief, non-critical help, or to name an icon-only button.
  **Avoid:** For critical, task-required or lengthy information (USWDS, NN/g).
  **Instead:** visible text, [Popover](#popover), an inline hint.
- **Behaviour & a11y:** It shows on hover **and** keyboard focus, is dismissible
  with Escape, and is linked with `aria-describedby`. Don't put interactive
  content in it. Touch users need an alternative.
- **Seen in:** Carbon Tooltip / Toggletip · Polaris Tooltip · Material Tooltip · Atlassian Tooltip
- **Sources:** [USWDS Tooltip](https://designsystem.digital.gov/components/tooltip/) ·
  [NN/g Tooltip guidelines](https://www.nngroup.com/articles/tooltip-guidelines/) ·
  [APG Tooltip](https://www.w3.org/WAI/ARIA/apg/patterns/tooltip/) ·
  [M3 Tooltips](https://m3.material.io/components/tooltips/overview)
- **gog-marketing:** Icon-only buttons (☰, ⋮, Close). Don't hide model limits or
  permission meaning in tooltips; use a popover or inline text instead
  (**C05, C06**).

### Tree view
*Aliases:* Tree, Outline view · [Gallery](https://component.gallery/components/tree-view/)
- **What:** Nested, expandable hierarchical items.
- **Use:** For deep hierarchies that users really browse, such as file trees.
  **Avoid:** For two-level groupings. **Instead:** grouped lists with
  [Fieldset](#fieldset) or [Accordion](#accordion) headers.
- **Behaviour & a11y:** APG Treeview is complex (arrow keys, type-ahead). Only
  adopt it if you implement it fully.
- **Seen in:** Carbon Tree view · Primer Tree view · Spectrum Tree view · Elastic Tree view
- **Sources:** [APG Treeview](https://www.w3.org/WAI/ARIA/apg/patterns/treeview/) ·
  [HIG Outline views](https://developer.apple.com/design/human-interface-guidelines/outline-views)
  *(no NN/g or USWDS component page)*
- **gog-marketing:** n/a. The tool groups (**D03**) use accordion groups, not a tree.

### Video
*Aliases:* Video player · [Gallery](https://component.gallery/components/video/)
- **What:** Embedded video with playback controls.
- **Use:** For demos and tutorials users choose to watch. **Avoid:**
  Autoplay. **Instead:** short annotated images.
- **Behaviour & a11y:** Captions, a transcript, keyboard controls, and no
  autoplay with sound.
- **Seen in:** Gestalt Video *(only Gallery example)*
- **Sources:** [HIG Playing video](https://developer.apple.com/design/human-interface-guidelines/playing-video) ·
  [Gallery](https://component.gallery/components/video/)
- **gog-marketing:** n/a yet. Possibly a help-page walkthrough (**N02**).

### Visually hidden
*Aliases:* Screen-reader only, sr-only, Offscreen · [Gallery](https://component.gallery/components/visually-hidden/)
- **What:** Text that is hidden visually but available to assistive technology.
- **Use:** For accessible names of icon buttons, extra context ("Disconnect
  rajeev@…"), and live-region announcements. **Avoid:** For hiding content
  that sighted users also need. **Instead:** visible text.
- **Behaviour & a11y:** Use the standard clip pattern, not `display:none`, which
  hides it from assistive tech too.
- **Seen in:** Paste Screen Reader Only · Chakra UI Visually Hidden · Duet Visually Hidden · Cauldron Offscreen
- **Sources:** [Gallery](https://component.gallery/components/visually-hidden/) ·
  [WCAG 4.1.3 Status messages](https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html)
- **gog-marketing:** Announce filter counts (**L06**), save results, and chat
  streaming or failure. Give icon buttons account-specific names (**N04**).

---

## Supplements (not separate Component Gallery types)

These components are named in the findings and folded into Gallery types above.
They're listed separately because their decision sources differ.

| Component | Closest Gallery type | Decision / application sources | gog-marketing |
|---|---|---|---|
| Error summary | [Alert](#alert) + [Form](#form) | [GOV.UK Error summary](https://design-system.service.gov.uk/components/error-summary/) · [GOV.UK Error message](https://design-system.service.gov.uk/components/error-message/) · [NN/g Reporting errors in forms](https://www.nngroup.com/articles/errors-forms-design-guidelines/) | Sign-in and consent form errors (**L07**) |
| Notification / site banner | [Alert](#alert) | [USWDS Site alert](https://designsystem.digital.gov/components/site-alert/) · [GOV.UK Notification banner](https://design-system.service.gov.uk/components/notification-banner/) | Service-wide chat outage or capacity notice (**C01**) |
| Summary / description list | [List](#list) | [GOV.UK Summary list](https://design-system.service.gov.uk/components/summary-list/) · [USWDS Summary box](https://designsystem.digital.gov/components/summary-box/) | Account details, diagnostics panel (render 09) |
| Task list | [Progress indicator](#progress-indicator) | [GOV.UK Task list](https://design-system.service.gov.uk/components/task-list/) · [GOV.UK Complete multiple tasks](https://design-system.service.gov.uk/patterns/complete-multiple-tasks/) | Setup checklist (**L01**, render 10) |
| Bottom navigation bar | [Navigation](#navigation) | [M3 Navigation bar](https://m3.material.io/components/navigation-bar/overview) · [HIG Tab bars](https://developer.apple.com/design/human-interface-guidelines/tab-bars) · [NN/g Mobile navigation patterns](https://www.nngroup.com/articles/mobile-navigation-patterns/) | Single mobile nav (**L13**) |
| Bottom sheet | [Drawer](#drawer) | [NN/g Bottom sheets](https://www.nngroup.com/articles/bottom-sheet/) · [M3 Bottom sheets](https://m3.material.io/components/bottom-sheets/overview) · [HIG Action sheets](https://developer.apple.com/design/human-interface-guidelines/action-sheets) | Mobile account menu with account header (**N04**) |
| Chip / filter chip | [Badge](#badge) / [Segmented control](#segmented-control) | [M3 Chips](https://m3.material.io/components/chips/overview) · [HIG Token fields](https://developer.apple.com/design/human-interface-guidelines/token-fields) | Active search filter chip with Clear (**L06**); mobile service chips (render 12) |
| Status indicator (dot + text) | [Badge](#badge) | [NN/g Indicators, validations, notifications](https://www.nngroup.com/articles/indicators-validations-notifications/) · [WCAG 1.4.1](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html) | Health summary that's correct per account (**U03**); green dot when healthy (**DR09**) |
| Back link | [Link](#link) | [GOV.UK Back link](https://design-system.service.gov.uk/components/back-link/) | Drill-in pages after the **D05** split |
| Split button | [Button](#button) | [M3 Split button](https://m3.material.io/components/split-button/overview) · [NN/g UI-elements glossary](https://www.nngroup.com/articles/ui-elements-glossary/) | n/a yet |

## Source notes

- **Component Gallery** list fetched 2026-11-01 from
  https://component.gallery/components/ (60 types). The "Seen in" examples are
  taken from each component's Gallery page. USWDS isn't indexed by the Gallery,
  so it's cited separately.
- **USWDS** component pages were fetched directly, and the "When to use / When
  to consider something else" sections were read. USWDS has no Textarea page
  (it returns 404), so GOV.UK Textarea is cited instead.
- **NN/g** refused direct connections from this machine, so each URL was
  confirmed through search-engine results that show the article's title and
  summary. Claims paraphrase those summaries.
- **Material 3** URLs were checked against `m3.material.io/sitemap.xml`. **HIG**
  URLs were checked against Apple's HIG JSON index. **APG**, **GOV.UK** and
  **WCAG** URLs were checked by HTTP 200.
- **Mobbin** wasn't fetched because it requires a login.
