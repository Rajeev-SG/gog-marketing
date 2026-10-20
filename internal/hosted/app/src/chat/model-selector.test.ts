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
  expect(markup).toContain(
    "Unavailable paid · Paid · Unavailable: Your organization&#x27;s model budget is exhausted.",
  );
  expect(markup).toContain('value="free-fast"');
  expect(markup).toContain("Current model: openrouter/free");
  expect(markup).toContain("Last routed model: minimax/minimax-m3.1-flash:free");
  expect(markup).toContain(
    "Request limits: 40 requests/day · 15,000 tokens/day · service-wide 500 requests/day",
  );
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
