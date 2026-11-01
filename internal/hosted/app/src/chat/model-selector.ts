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
  usage?: {
    tenantRequestsToday: number;
    tenantTokensToday: number;
    globalRequestsToday: number;
    resetsAt: string;
  };
};

/** Daily allowances reset at 00:00 UTC; show that moment in the viewer's own time. */
export function resetLabel(resetsAt: string | undefined): string {
  const date = resetsAt ? new Date(resetsAt) : null;
  if (!date || Number.isNaN(date.getTime())) return "Daily at 00:00 UTC";
  return (
    "Resets at " +
    date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) +
    " your time (00:00 UTC)"
  );
}

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
                // Keep option text short: a native select is as wide as its longest option.
                (!model.enabled ? " · Unavailable" : ""),
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
        {
          className:
            "model-status" +
            (settings && connected && !loading && !busy && !running ? " model-status-idle" : ""),
          id: "chat-model-status",
          role: "status",
        },
        status,
      ),
      settings &&
        createElement(
          "details",
          { className: "model-more" },
          createElement("summary", null, "Model details and today's usage"),
          createElement(
            "dl",
            { className: "model-details" },
            createElement("dt", null, "Model"),
            createElement("dd", null, selected?.label ?? "Unavailable"),
            ...(settings.lastModelId
              ? [
                  createElement("dt", { key: "last-dt" }, "Last reply from"),
                  createElement("dd", { key: "last-dd" }, settings.lastModelId),
                ]
              : []),
            createElement("dt", null, "Your usage today"),
            createElement(
              "dd",
              null,
              settings.usage
                ? "You've used " +
                    settings.usage.tenantRequestsToday +
                    " of " +
                    settings.limits.tenantRequestsPerDay +
                    " model requests"
                : "Up to " + settings.limits.tenantRequestsPerDay + " model requests a day",
            ),
            createElement("dt", null, "Shared free capacity"),
            createElement(
              "dd",
              null,
              settings.usage
                ? settings.usage.globalRequestsToday +
                    " of " +
                    settings.limits.globalRequestsPerDay +
                    " requests used across all users"
                : "Up to " +
                    settings.limits.globalRequestsPerDay +
                    " requests a day across all users",
            ),
            createElement("dt", null, "Resets"),
            createElement("dd", null, resetLabel(settings.usage?.resetsAt)),
            ...settings.models
              .filter((model) => !model.enabled)
              .flatMap((model) => [
                createElement("dt", { key: model.key + "-dt" }, model.label),
                createElement(
                  "dd",
                  { key: model.key + "-dd" },
                  "Unavailable: " + (model.disabledReason ?? "temporarily unavailable"),
                ),
              ]),
          ),
          selected &&
            createElement(
              "p",
              { className: "model-description", id: "chat-model-description" },
              selected.description,
            ),
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
