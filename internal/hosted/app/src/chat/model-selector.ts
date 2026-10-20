import { createElement, useState } from "react";

export type ChatModelOption = {
  key: string;
  label: string;
  description: string;
  free: boolean;
  enabled: boolean;
  disabledReason?: string;
};

export type ChatModelSettings = {
  selectedKey: string;
  model: string;
  models: ChatModelOption[];
  lastModelId?: string;
  limits: {
    globalRequestsPerDay: number;
    tenantRequestsPerDay: number;
    tenantTokensPerDay: number;
  };
};

export function ModelSelector({
  settings,
  connected,
  running,
  busy,
  loading,
  error,
  onSelect,
  onRetry,
}: {
  settings: ChatModelSettings | null;
  connected: boolean;
  running: boolean;
  busy: boolean;
  loading: boolean;
  error: string;
  onSelect: (key: string) => void | Promise<void>;
  onRetry?: () => void;
}) {
  const [paidChoice, setPaidChoice] = useState<ChatModelOption | null>(null);
  const selectionDisabled = !settings || !connected || running || busy || loading;
  const selected = settings?.models.find((model) => model.key === settings.selectedKey);
  const status = loading
    ? "Loading saved model settings…"
    : !connected
      ? "Reconnecting… Model selection is paused."
      : running
        ? "Model selection is paused while Gog responds."
        : busy
          ? "Saving model choice…"
          : settings
            ? "Model settings loaded."
            : "Waiting for model settings…";

  return createElement(
    "section",
    {
      className: "model-selector",
      "aria-label": "Chat model selection",
      "aria-busy": loading || busy,
    },
    createElement(
      "div",
      { className: "model-selector-main" },
      createElement(
        "label",
        { className: "model-select-label", htmlFor: "chat-model-select" },
        "Chat model",
      ),
      createElement(
        "div",
        { className: "model-select-field" },
        createElement(
          "select",
          {
            id: "chat-model-select",
            value: settings?.selectedKey ?? "",
            disabled: selectionDisabled,
            "aria-describedby": "chat-model-status chat-model-description",
            onChange: (event) => {
              const value = (event.currentTarget as { value?: string }).value ?? "";
              const choice = settings?.models.find((model) => model.key === value);
              if (!choice || !choice.enabled) return;
              if (choice.free) {
                setPaidChoice(null);
                void onSelect(choice.key);
                return;
              }
              setPaidChoice(choice);
            },
          },
          (settings?.models ?? []).map((model) =>
            createElement(
              "option",
              { key: model.key, value: model.key, disabled: !model.enabled },
              model.label +
                (model.label.endsWith("· Free") || model.label.endsWith("· Paid")
                  ? ""
                  : " · " + (model.free ? "Free" : "Paid")) +
                (!model.enabled
                  ? " · Unavailable: " + (model.disabledReason ?? "temporarily unavailable")
                  : ""),
            ),
          ),
        ),
        createElement(
          "span",
          {
            className: "model-badge " + (selected?.free ? "model-free" : "model-paid"),
            "data-testid": "model-price-badge",
          },
          selected ? (selected.free ? "Free" : "Paid") : settings ? "Unavailable" : "Loading",
        ),
      ),
      createElement(
        "p",
        { className: "model-status", id: "chat-model-status", role: "status" },
        status,
      ),
      settings &&
        createElement(
          "p",
          { className: "model-details" },
          createElement("span", null, "Current model: " + settings.model),
          settings.lastModelId &&
            createElement("span", null, "Last routed model: " + settings.lastModelId),
          createElement(
            "span",
            null,
            "Request limits: " +
              settings.limits.tenantRequestsPerDay +
              " requests/day · " +
              settings.limits.tenantTokensPerDay.toLocaleString() +
              " tokens/day · service-wide " +
              settings.limits.globalRequestsPerDay +
              " requests/day",
          ),
        ),
      selected &&
        createElement(
          "p",
          { className: "model-description", id: "chat-model-description" },
          selected.description,
        ),
      error && createElement("p", { className: "model-error", role: "alert" }, error),
      error &&
        !settings &&
        onRetry &&
        createElement(
          "button",
          { type: "button", disabled: !connected || busy || loading, onClick: onRetry },
          "Retry model settings",
        ),
    ),
    paidChoice &&
      createElement(
        "section",
        { className: "model-confirm", "aria-label": "Confirm paid model" },
        createElement(
          "p",
          null,
          createElement(
            "strong",
            null,
            paidChoice.label +
              " is a paid model. Requests can use your organization's paid model budget.",
          ),
        ),
        createElement("p", null, paidChoice.description),
        createElement(
          "div",
          { className: "approval-actions" },
          createElement(
            "button",
            {
              type: "button",
              disabled: selectionDisabled,
              onClick: () => {
                setPaidChoice(null);
                void onSelect(paidChoice.key);
              },
            },
            "Confirm paid model",
          ),
          createElement(
            "button",
            {
              type: "button",
              disabled: busy,
              onClick: () => setPaidChoice(null),
            },
            "Keep current model",
          ),
        ),
      ),
  );
}
