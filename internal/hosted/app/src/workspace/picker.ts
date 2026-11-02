import { accountName, type Connection } from "./model.js";

/** Google Picker (Drive selected files). Loaded only when the user asks for it. */
type PickerResult = Record<string, unknown> & { action: string };
interface GoogleGlobal {
  picker: {
    PickerBuilder: new () => {
      setDeveloperKey(key: string): unknown;
      [method: string]: (...args: never[]) => unknown;
    };
    DocsView: new (view: unknown) => unknown;
    ViewId: { DOCS: unknown };
    Feature: { MULTISELECT_ENABLED: unknown };
    Action: { CANCEL: string; PICKED: string };
    Response: { DOCUMENTS: string };
    Document: { ID: string };
  };
}
type PickerWindow = typeof window & {
  gapi?: { load(name: string, options: { callback: () => void; onerror: () => void }): void };
  google?: GoogleGlobal;
};

let library: Promise<void> | null = null;
function loadLibrary(): Promise<void> {
  library ??= new Promise<void>((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "https://apis.google.com/js/api.js";
    const fail = () => {
      library = null;
      reject(new Error("Google Picker could not load."));
    };
    script.onload = () =>
      (window as PickerWindow).gapi?.load("picker", { callback: resolve, onerror: fail });
    script.onerror = fail;
    document.head.appendChild(script);
  });
  return library;
}

const postJson = (url: string, body: unknown) =>
  fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

export async function openPicker(
  connection: Connection,
  status: (text: string) => void,
  reload: () => Promise<void>,
) {
  const connectionId = connection.id;
  status(`Preparing Google Picker for ${accountName(connection)}…`);
  try {
    await loadLibrary();
    const response = await postJson("/api/google/picker/start", { connectionId });
    const config = (await response.json()) as {
      error?: string;
      apiKey: string;
      appId: string;
      accessToken: string;
      pickerSession: string;
      googleEmail: string;
    };
    if (!response.ok)
      throw new Error(
        config.error ||
          "File picking isn't set up yet. Reconnect this account with selected-file access, or ask the workspace owner.",
      );
    const google = (window as PickerWindow).google!;
    const builder = new google.picker.PickerBuilder() as unknown as Record<
      string,
      (...args: unknown[]) => unknown
    >;
    const chain = (method: string, ...args: unknown[]) => builder[method](...args);
    chain("setDeveloperKey", config.apiKey);
    chain("setAppId", config.appId);
    chain("setOAuthToken", config.accessToken);
    chain("setOrigin", window.location.origin);
    chain("addView", new google.picker.DocsView(google.picker.ViewId.DOCS));
    chain("enableFeature", google.picker.Feature.MULTISELECT_ENABLED);
    chain("setCallback", async (result: PickerResult) => {
      if (result.action === google.picker.Action.CANCEL) {
        config.accessToken = "";
        status("File selection cancelled. Existing access is unchanged.");
        return;
      }
      if (result.action !== google.picker.Action.PICKED) return;
      config.accessToken = "";
      const docs = result[google.picker.Response.DOCUMENTS] as Array<Record<string, string>>;
      const fileIds = docs.map((item) => item[google.picker.Document.ID]);
      try {
        const verified = await postJson("/api/google/picker/complete", {
          connectionId,
          pickerSession: config.pickerSession,
          fileIds,
        });
        const saved = (await verified.json()) as {
          error?: string;
          selectedCount: number;
          rejectedCount: number;
        };
        if (!verified.ok)
          throw new Error(
            saved.error || "Selected files could not be verified. Open Picker again to retry.",
          );
        await reload();
        status(
          `${saved.selectedCount} files verified for ${accountName(connection)}; ${saved.rejectedCount} unavailable. New files start disabled.`,
        );
      } catch (error) {
        status(error instanceof Error ? error.message : "File verification failed.");
      }
    });
    const picker = chain("build") as { setVisible(visible: boolean): void };
    picker.setVisible(true);
    status(`Select files for ${config.googleEmail}. No other account will be changed.`);
  } catch (error) {
    status(error instanceof Error ? error.message : "Could not open Google Picker.");
  }
}
