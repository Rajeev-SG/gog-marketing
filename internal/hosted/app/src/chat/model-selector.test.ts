import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { ModelSelector, type ChatModelSettings } from "./model-selector.js";

type ModelSelectorProps = Parameters<typeof ModelSelector>[0];

const settings: ChatModelSettings = {
  selectedKey: "free-fast",
  model: "openrouter/free",
  models: [
    {
      key: "free-fast",
      label: "Fast and free",
      description: "Everyday replies without paid model budget.",
      free: true,
      enabled: true,
    },
    {
      key: "paid-capable",
      label: "Capable paid",
      description: "Uses paid model budget for difficult work.",
      free: false,
      enabled: true,
    },
    {
      key: "paid-over-budget",
      label: "Unavailable paid",
      description: "Paid model budget has been exhausted.",
      free: false,
      enabled: false,
      disabledReason: "Your organization's model budget is exhausted.",
    },
  ],
  lastModelId: "minimax/minimax-m3.1-flash:free",
  limits: {
    globalRequestsPerDay: 500,
    tenantRequestsPerDay: 40,
    tenantTokensPerDay: 15000,
  },
};

const baseProps: ModelSelectorProps = {
  settings,
  connected: true,
  running: false,
  busy: false,
  loading: false,
  error: "",
  onSelect: () => {},
};

const render = (props: Partial<ModelSelectorProps> = {}) =>
  renderToStaticMarkup(createElement(ModelSelector, { ...baseProps, ...props }));

it("renders model labels, the selected value, limits and the last routed model", () => {
  const markup = render();
  expect(markup).toContain("Fast and free · Free");
  expect(markup).toContain("Capable paid · Paid");
  // Option text stays short (a native select is as wide as its longest option);
  // the reason is disclosed in the details list instead (C02).
  expect(markup).toContain("Unavailable paid · Paid · Unavailable</option>");
  expect(markup).toContain(
    "<dt>Unavailable paid</dt><dd>Unavailable: Your organization&#x27;s model budget is exhausted.</dd>",
  );
  expect(markup).toContain('value="free-fast"');
  // Plain-language usage, no provider IDs in the primary fields (C05).
  expect(markup).toContain("<dt>Model</dt><dd>Fast and free</dd>");
  expect(markup).toContain("<dd>minimax/minimax-m3.1-flash:free</dd>");
  expect(markup).toContain("Up to 40 model requests a day");
  expect(markup).toContain("Up to 500 requests a day across all users");
  expect(markup).toContain("Daily at 00:00 UTC");
});

it("disables model selection while busy, running, disconnected or loading", () => {
  expect(render({ busy: true })).toContain("Saving model choice…");
  expect(render({ busy: true })).toContain('disabled=""');
  expect(render({ running: true })).toContain("Model selection is paused while Gog responds.");
  expect(render({ running: true })).toContain('disabled=""');
  expect(render({ connected: false })).toContain("Reconnecting… Model selection is paused.");
  expect(render({ connected: false })).toContain('disabled=""');
  expect(render({ settings: null, loading: true })).toContain("Loading saved model settings…");
  expect(render({ settings: null, loading: true })).toContain('disabled=""');
});

it("renders unavailable paid options and model selection errors accessibly", () => {
  const markup = render({
    error: "Could not save your model choice. Reconnect and try again.",
  });

  expect(markup).toContain("Could not save your model choice. Reconnect and try again.");
  expect(markup).toContain('role="alert"');
  expect(markup).toContain('aria-busy="false"');
  expect(markup).toContain('aria-label="Chat model selection"');
  expect(markup).toContain('<option value="paid-over-budget" disabled=""');
  expect(markup).toContain("Unavailable: Your organization&#x27;s model budget is exhausted.");
});

it("shows today's usage and the reset time when usage is available (C05)", () => {
  const settings = {
    ...baseProps.settings!,
    usage: {
      tenantRequestsToday: 12,
      tenantTokensToday: 900,
      globalRequestsToday: 77,
      resetsAt: "2026-11-02T00:00:00.000Z",
    },
  };
  const markup = render({ settings });
  expect(markup).toContain("You&#x27;ve used 12 of 40 model requests");
  expect(markup).toContain("77 of 500 requests used across all users");
  expect(markup).toContain("(00:00 UTC)");
});
