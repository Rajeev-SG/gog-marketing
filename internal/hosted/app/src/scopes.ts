/** Hosted-only audit-free consent policy (#79). Eligibility is not executable
 * tool coverage or Google production approval. Never inherit CLI OAuth defaults. */
export const IDENTITY_SCOPES: readonly string[] = [
  "openid",
  "email",
  "https://www.googleapis.com/auth/userinfo.email",
] as const;

export interface HostedServiceInfo {
  label: string;
  scopes: string[];
  /** Additional consent only for explicitly selected write services. */
  writeScopes: string[];
  resourceModel: boolean;
  documentation: string;
}
const scopes = (names: string): string[] =>
  names
    .split(" ")
    .filter(Boolean)
    .map((name) => (name === "profile" ? name : "https://www.googleapis.com/auth/" + name));
export const HOSTED_SERVICES: Readonly<Record<string, HostedServiceInfo>> = {
  calendar: {
    label: "Calendar",
    scopes: scopes("calendar.readonly"),
    writeScopes: scopes("calendar"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/calendar/api/auth",
  },
  chat: {
    label: "Google Chat",
    scopes: scopes(
      "chat.spaces.readonly chat.memberships.readonly chat.messages.reactions.readonly chat.users.readstate.readonly",
    ),
    writeScopes: scopes(
      "chat.spaces chat.memberships chat.messages.create chat.messages.reactions.create",
    ),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/chat/authenticate-authorize",
  },
  classroom: {
    label: "Classroom",
    scopes: scopes(
      "classroom.courses.readonly classroom.rosters.readonly classroom.coursework.students.readonly classroom.coursework.me.readonly classroom.courseworkmaterials.readonly classroom.announcements.readonly classroom.topics.readonly classroom.guardianlinks.students.readonly classroom.profile.emails classroom.profile.photos",
    ),
    writeScopes: scopes(
      "classroom.courses classroom.rosters classroom.coursework.students classroom.coursework.me classroom.courseworkmaterials classroom.announcements classroom.topics classroom.guardianlinks.students",
    ),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/classroom/guides/auth",
  },
  drive: {
    label: "Drive (selected files)",
    scopes: scopes("drive.file"),
    writeScopes: scopes("drive.file"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/drive/api/guides/api-specific-auth",
  },
  drivelabels: {
    label: "Drive label schemas",
    scopes: scopes("drive.labels.readonly"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/drive/labels/guides/authorize",
  },
  docs: {
    label: "Google Docs",
    scopes: scopes("documents.readonly drive.file"),
    writeScopes: scopes("documents"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/docs/api/auth",
  },
  slides: {
    label: "Google Slides",
    scopes: scopes("presentations.readonly drive.file"),
    writeScopes: scopes("presentations"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/slides/api/scopes",
  },
  contacts: {
    label: "Contacts",
    scopes: scopes("contacts.readonly contacts.other.readonly directory.readonly"),
    writeScopes: scopes("contacts"),
    resourceModel: false,
    documentation: "https://developers.google.com/people/v1/how-tos/authorizing",
  },
  tasks: {
    label: "Tasks",
    scopes: scopes("tasks.readonly"),
    writeScopes: scopes("tasks"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/tasks/auth",
  },
  people: {
    label: "People profiles",
    scopes: scopes("profile"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation: "https://developers.google.com/people/v1/how-tos/authorizing",
  },
  sheets: {
    label: "Google Sheets",
    scopes: scopes("spreadsheets.readonly drive.file"),
    writeScopes: scopes("spreadsheets"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/sheets/api/scopes",
  },
  forms: {
    label: "Google Forms",
    scopes: scopes("forms.body.readonly forms.responses.readonly"),
    writeScopes: scopes("forms.body"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/forms/api/guides/apps-script-setup",
  },
  sites: {
    label: "Google Sites (selected-file metadata)",
    scopes: scopes("drive.file"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/drive/api/guides/api-specific-auth",
  },
  meet: {
    label: "Google Meet",
    scopes: scopes("meetings.space.readonly"),
    writeScopes: scopes("meetings.space.created meetings.space.settings"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/meet/api/guides/authenticate-authorize",
  },
  appscript: {
    label: "Apps Script",
    scopes: scopes("script.projects.readonly script.deployments.readonly script.processes"),
    writeScopes: scopes("script.projects script.deployments"),
    resourceModel: false,
    documentation: "https://developers.google.com/apps-script/api/how-tos/execute",
  },
  analytics: {
    label: "Google Analytics 4",
    scopes: scopes("analytics.readonly"),
    writeScopes: scopes("analytics.edit"),
    resourceModel: true,
    documentation: "https://developers.google.com/analytics/devguides/config/admin/v1/rest",
  },
  searchconsole: {
    label: "Search Console",
    scopes: scopes("webmasters.readonly"),
    writeScopes: scopes("webmasters"),
    resourceModel: true,
    documentation: "https://developers.google.com/webmaster-tools/v1/how-tos/authorizing",
  },
  tagmanager: {
    label: "Google Tag Manager",
    scopes: scopes("tagmanager.readonly"),
    writeScopes: scopes(
      "tagmanager.edit.containers tagmanager.edit.containerversions tagmanager.publish tagmanager.delete.containers",
    ),
    resourceModel: true,
    documentation: "https://developers.google.com/tag-platform/tag-manager/api/v2/authorization",
  },
  bigquery: {
    label: "BigQuery",
    scopes: scopes("bigquery.readonly"),
    writeScopes: scopes("bigquery"),
    resourceModel: true,
    documentation: "https://docs.cloud.google.com/bigquery/docs/authentication",
  },
  adsense: {
    label: "AdSense",
    scopes: scopes("adsense.readonly"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation:
      "https://developers.google.com/adsense/management/reference/rest/v2/accounts/list",
  },
  cloudadmin: {
    label: "Google Cloud administration",
    scopes: scopes("cloud-platform"),
    writeScopes: scopes("cloud-platform"),
    resourceModel: false,
    documentation: "https://docs.cloud.google.com/docs/authentication",
  },
  groups: {
    label: "Workspace groups",
    scopes: scopes("cloud-identity.groups.readonly"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation: "https://developers.google.com/identity/protocols/oauth2/scopes",
  },
  keep: {
    label: "Google Keep",
    scopes: scopes("keep"),
    writeScopes: scopes("keep"),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/keep/api/reference/rest",
  },
  admin: {
    label: "Workspace Admin",
    scopes: scopes(
      "admin.directory.user.readonly admin.directory.group.readonly admin.directory.group.member.readonly admin.directory.orgunit.readonly",
    ),
    writeScopes: scopes(
      "admin.directory.user admin.directory.group admin.directory.group.member admin.directory.orgunit",
    ),
    resourceModel: false,
    documentation: "https://developers.google.com/workspace/admin/directory/v1/guides/authorizing",
  },
  youtube: {
    label: "YouTube",
    scopes: scopes("youtube.readonly"),
    writeScopes: scopes("youtube"),
    resourceModel: false,
    documentation: "https://developers.google.com/youtube/v3/guides/auth/server-side-web-apps",
  },
  photos: {
    label: "Google Photos (app-created media)",
    scopes: scopes("photoslibrary.readonly.appcreateddata"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation: "https://developers.google.com/photos/overview/authorization",
  },
  photospicker: {
    label: "Google Photos Picker",
    scopes: scopes("photospicker.mediaitems.readonly"),
    writeScopes: scopes(""),
    resourceModel: false,
    documentation: "https://developers.google.com/photos/picker/guides/get-started-picker",
  },
};

export class UnknownServiceError extends Error {}
export class UnsafeHostedScopeError extends Error {}
const ALLOWED_SCOPES = new Set([
  ...IDENTITY_SCOPES,
  "https://www.googleapis.com/auth/userinfo.profile", // Google's canonical alias for profile.
  ...Object.values(HOSTED_SERVICES).flatMap((info) => [...info.scopes, ...info.writeScopes]),
]);
/** Exact allowlist rejects unknown, restricted and transitive additions. */
export function assertHostedScopes(values: readonly string[]): void {
  if (!values.length || !values.every((scope) => ALLOWED_SCOPES.has(scope))) {
    throw new UnsafeHostedScopeError("Google grant contains scopes outside the hosted allowlist");
  }
}

export function isHostedService(service: string): boolean {
  return Object.hasOwn(HOSTED_SERVICES, service);
}

/** Normalize an optional service list; rejects unknown services. */
export function normalizeServices(raw: unknown): string[] {
  if (raw === undefined || raw === null) return [];
  if (!Array.isArray(raw)) {
    throw new UnknownServiceError("services must be a list of service names");
  }
  const out: string[] = [];
  for (const value of raw) {
    if (typeof value !== "string") throw new UnknownServiceError("invalid service name");
    const service = value.trim().toLowerCase();
    if (!service) continue;
    if (!isHostedService(service)) {
      throw new UnknownServiceError("unsupported service");
    }
    if (!out.includes(service)) out.push(service);
  }
  return out;
}

/**
 * Sorted, de-duplicated scope set for identity + selected services.
 * Mirrors Go's mergeScopes ordering so the URL is stable.
 */
export function scopesForServices(services: string[], writeServices: string[] = []): string[] {
  const set = new Set<string>(IDENTITY_SCOPES);
  for (const service of services) {
    const info = HOSTED_SERVICES[service];
    if (!isHostedService(service)) throw new UnknownServiceError("unsupported service");
    for (const scope of info.scopes) set.add(scope);
  }
  for (const service of writeServices) {
    if (
      !isHostedService(service) ||
      !services.includes(service) ||
      !HOSTED_SERVICES[service]!.writeScopes.length
    ) {
      throw new UnknownServiceError("write service must be selected and support writes");
    }
    for (const scope of HOSTED_SERVICES[service]!.writeScopes) set.add(scope);
  }
  const result = [...set].sort();
  assertHostedScopes(result);
  return result;
}

/** Merge granted scopes into an existing sorted grant list. */
export function mergeScopes(existing: string[], granted: string[]): string[] {
  const set = new Set<string>();
  for (const scope of [...existing, ...granted]) {
    const trimmed = String(scope ?? "").trim();
    if (trimmed) set.add(trimmed);
  }
  const result = [...set].sort();
  assertHostedScopes(result);
  return result;
}
