export const ENGINES = ['ookla', 'cloudflare', 'iperf3'] as const;

export interface Target {
  id: number;
  name: string;
  engine: string;
  enabled: boolean;
  queue_id: number;
  queue_name: string;
  options: Record<string, unknown>;
  thresholds: Record<string, unknown>;
  // rotation_index is read-only: the host/server-id rotation cursor the
  // runner advances on each run. Optional here since it's only set by the
  // live API response, not by test fixtures that build a Target by hand.
  rotation_index?: number;
  created_at: string;
  updated_at: string;
}

export interface Queue {
  id: number;
  name: string;
  created_at: string;
}

export interface Result {
  id: number;
  run_id: number | null;
  target_id: number | null;
  target_name: string;
  engine: string;
  options_snapshot: Record<string, unknown>;
  status: 'ok' | 'failed' | 'degraded';
  error?: string;
  started_at: string;
  duration_ms: number;
  download_bps: number;
  upload_bps: number;
  ping_ms: number;
  jitter_ms: number;
  packet_loss_pct: number;
  server_name: string;
  server_host: string;
  isp: string;
  result_url: string;
  tags: string[];
}

export interface Run {
  id: number;
  schedule_id: number | null;
  trigger: string;
  status: 'queued' | 'running' | 'done' | 'failed' | 'canceled' | 'skipped';
  started_at: string | null;
  finished_at: string | null;
  error: string | null;
}

export interface OoklaServer {
  id: string;
  name: string;
  location: string;
  country: string;
  host: string;
  sponsor?: string;
  lat?: number;
  lon?: number;
  distance_km?: number;
}

export interface Iperf3Server {
  id: number;
  host: string;
  port: number;
  port_end?: number;
  options?: string;
  supports_reverse: boolean;
  supports_udp: boolean;
  supports_ipv6: boolean;
  gbs?: string;
  continent?: string;
  country?: string;
  site?: string;
  provider?: string;
}

/** OoklaSearchResult is the /ookla/servers response shape: the matching
 * servers (already sorted by distance server-side) plus, when the query
 * resolved through geocoding, the resolved place's display name. */
export interface OoklaSearchResult {
  servers: OoklaServer[];
  near: string;
}

export interface Iperf3ServersPage {
  fetched_at: string;
  servers: Iperf3Server[];
  total: number;
}

export const listIperf3Servers = (q: string) =>
  request<Iperf3ServersPage>(`/iperf3/servers${query({ q })}`);
export const refreshIperf3Servers = () =>
  request<{ fetched_at: string; count: number }>('/iperf3/servers/refresh', { method: 'POST' });

export interface ResultsPage {
  results: Result[];
  next_cursor: string;
}

export interface ResultFilters {
  target_id?: number;
  engine?: string;
  status?: string;
  from?: string;
  to?: string;
  tag?: string;
  limit?: number;
}

export type Range = '24h' | '7d' | '30d';

export interface HistoryPoint {
  bucket_start: string;
  count: number;
  fail_count: number;
  avg_download_bps: number;
  min_download_bps: number;
  max_download_bps: number;
  avg_upload_bps: number;
  min_upload_bps: number;
  max_upload_bps: number;
  avg_ping_ms: number;
  min_ping_ms: number;
  max_ping_ms: number;
  avg_jitter_ms: number;
}

export interface History {
  target_id: number;
  from: string;
  to: string;
  bucket_seconds: number;
  points: HistoryPoint[];
}

export interface TargetSummary {
  target_id: number;
  target_name: string;
  engine: string;
  latest: Result | null;
  count: number;
  fail_count: number;
  success_rate: number;
  avg_download_bps: number;
  min_download_bps: number;
  max_download_bps: number;
  avg_upload_bps: number;
  avg_ping_ms: number;
  max_ping_ms: number;
  /** sla_compliance is the fraction (0..1) of this target's successful
   * results in range that met its resolved SLA plan speeds; null when no
   * plan resolves for it (neither a per-target override nor the general
   * plan) or it has no successful results in range. */
  sla_compliance: number | null;
}

export interface SummaryStats {
  from: string;
  to: string;
  targets: TargetSummary[];
  total_results: number;
  total_failures: number;
  success_rate: number;
  /** sla_compliance is the overall fraction (0..1), weighted by each
   * target's own successful-result count; null when it resolves for no
   * target. */
  sla_compliance: number | null;
}

export interface Incident {
  target_id: number | null;
  target_name: string;
  kind: 'result' | 'skipped';
  status: string;
  started_at: string;
  ended_at: string;
  count: number;
  error?: string;
}

export interface TargetInput {
  name: string;
  engine: string;
  enabled: boolean;
  queue_id: number;
  options: Record<string, unknown>;
  thresholds?: Record<string, unknown>;
}

export interface QueueInput {
  name: string;
}

/** ApiError carries the server's {error:{code,message}} envelope. */
export class ApiError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

const BASE = '/api/v1';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init?.headers } : init?.headers,
  });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const body = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    const envelope = body?.error;
    throw new ApiError(res.status, envelope?.code ?? 'unknown', envelope?.message ?? res.statusText);
  }
  return body as T;
}

function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') q.set(key, String(value));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}

export const listTargets = () => request<Target[]>('/targets');
export const createTarget = (t: TargetInput) =>
  request<Target>('/targets', { method: 'POST', body: JSON.stringify(t) });
export const updateTarget = (id: number, t: TargetInput) =>
  request<Target>(`/targets/${id}`, { method: 'PUT', body: JSON.stringify(t) });
export const deleteTarget = (id: number) =>
  request<void>(`/targets/${id}`, { method: 'DELETE' });
export const runTarget = (id: number) =>
  request<{ run_id: number }>(`/targets/${id}/run`, { method: 'POST' });
export const testTargetOptions = (engine: string, options: Record<string, unknown>) =>
  request<{ ok: boolean }>('/targets/test', {
    method: 'POST',
    body: JSON.stringify({ name: 'validation', engine, enabled: true, options }),
  });

export const listQueues = () => request<Queue[]>('/queues');
export const createQueue = (q: QueueInput) =>
  request<Queue>('/queues', { method: 'POST', body: JSON.stringify(q) });
export const renameQueue = (id: number, q: QueueInput) =>
  request<Queue>(`/queues/${id}`, { method: 'PUT', body: JSON.stringify(q) });
export const deleteQueue = (id: number) =>
  request<void>(`/queues/${id}`, { method: 'DELETE' });
export const listOoklaServers = (q: string, country?: string) =>
  request<OoklaSearchResult>(`/ookla/servers${query({ q, country })}`);

export type RevisionAction = 'create' | 'update' | 'delete' | 'revert' | 'restore';

export interface TargetRevision {
  version: number;
  action: RevisionAction;
  created_at: string;
  snapshot: Target;
  changed: string[];
}

export interface DeletedTarget {
  id: number;
  name: string;
  engine: string;
  queue_name: string;
  deleted_at: string;
  version: number;
}

export const listTargetRevisions = (id: number) =>
  request<TargetRevision[]>(`/targets/${id}/revisions`);
export const revertTargetRevision = (id: number, version: number) =>
  request<Target>(`/targets/${id}/revisions/${version}/revert`, { method: 'POST' });
export const listDeletedTargets = () => request<DeletedTarget[]>('/targets/deleted');
export const restoreTarget = (id: number) =>
  request<Target>(`/targets/deleted/${id}/restore`, { method: 'POST' });

export const listResults = (filters: ResultFilters, cursor?: string) =>
  request<ResultsPage>(`/results${query({ ...filters, cursor, limit: filters.limit ?? 50 })}`);
export const deleteResult = (id: number) =>
  request<void>(`/results/${id}`, { method: 'DELETE' });
export const reexecuteResult = (id: number) =>
  request<{ run_id: number }>(`/results/${id}/reexecute`, { method: 'POST' });
export const setResultTags = (id: number, tags: string[]) =>
  request<{ id: number; tags: string[] }>(`/results/${id}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tags }),
  });
export const listTags = () => request<{ id: number; name: string }[]>('/tags');
export const cancelRun = (id: number) =>
  request<{ run_id: number }>(`/runs/${id}`, { method: 'DELETE' });

export interface ScheduleRun {
  status: string;
  started_at: string;
}

export interface Schedule {
  id: number;
  name: string;
  cron: string;
  enabled: boolean;
  timezone: string;
  target_ids: number[];
  next_run: string;
  last_run: ScheduleRun | null;
  created_at: string;
  updated_at: string;
}

export interface ScheduleInput {
  name: string;
  cron: string;
  enabled: boolean;
  timezone: string;
  target_ids: number[];
}

/** ScheduleSaved is the create/update response: the saved row plus any
 * overlap warnings the server computed. */
export interface ScheduleSaved {
  schedule: Schedule;
  warnings: string[];
}

export interface RunsPage {
  runs: Run[];
  next_cursor: string;
}

export const listSchedules = async () =>
  (await request<{ schedules: Schedule[] }>('/schedules')).schedules;
export const createSchedule = (s: ScheduleInput) =>
  request<ScheduleSaved>('/schedules', { method: 'POST', body: JSON.stringify(s) });
export const updateSchedule = (id: number, s: ScheduleInput) =>
  request<ScheduleSaved>(`/schedules/${id}`, { method: 'PUT', body: JSON.stringify(s) });
export const deleteSchedule = (id: number) =>
  request<void>(`/schedules/${id}`, { method: 'DELETE' });
export const runSchedule = (id: number) =>
  request<{ run_id: number }>(`/schedules/${id}/run`, { method: 'POST' });
export const scheduleNext = async (id: number) =>
  (await request<{ next: string[] }>(`/schedules/${id}/next`)).next;

/** validateCron asks the server to parse an expression and preview its
 * next fire times; an invalid expression throws an ApiError with the
 * parser's message, which the form shows inline. */
export const validateCron = async (cron: string, timezone: string) =>
  (await request<{ ok: boolean; next: string[] }>('/schedules/validate', {
    method: 'POST',
    body: JSON.stringify({ cron, timezone }),
  })).next;

export const listRuns = (scheduleId?: number) =>
  request<RunsPage>(`/runs${query({ schedule_id: scheduleId, limit: 20 })}`);

export const targetHistory = (id: number, range: Range, offset?: 0 | 1) =>
  request<History>(`/targets/${id}/history${query({ range, offset })}`);
export const statsSummary = (range: Range, offset?: 0 | 1) =>
  request<SummaryStats>(`/stats/summary${query({ range, offset })}`);
export const listOutages = (range: Range) =>
  request<{ from: string; to: string; incidents: Incident[] }>(`/outages${query({ range })}`);

/** resultsCsvUrl builds the export link for the current filters. It is a
 * plain href, not a fetch: the browser streams the download itself. */
export const resultsCsvUrl = (filters: ResultFilters) =>
  `${BASE}/results.csv${query({ ...filters, limit: undefined })}`;

export const renameTag = (id: number, name: string) =>
  request<{ id: number; name: string }>(`/tags/${id}`, { method: 'PUT', body: JSON.stringify({ name }) });
export const deleteTag = (id: number) => request<void>(`/tags/${id}`, { method: 'DELETE' });

/** MASKED_SECRET is what the server sends instead of a stored secret, and
 * what a form sends back to mean "leave the stored value alone". */
export const MASKED_SECRET = '***';

export interface GeneralSettings {
  base_url: string;
  timezone: string;
  units: string;
  log_level: string;
  retention_days_results: number;
  retention_days_runs: number;
  retention_prune_interval_minutes: number;
  /** sla_download_mbps/sla_upload_mbps are the ISP-advertised plan speeds
   * (in Mbps) used to compute the dashboard's SLA compliance tile. The
   * server omits either field entirely from GET /settings when unset;
   * PUT 0 (or omit) to leave/clear it -- 0 or blank means "no plan". */
  sla_download_mbps?: number;
  sla_upload_mbps?: number;
}

export interface EngineSettings {
  speedtest_bin: string;
  iperf3_bin: string;
  ookla_accept_license: boolean;
  ookla_accept_gdpr: boolean;
  server_list_ttl_seconds: number;
  default_ookla_options: Record<string, unknown>;
  default_cloudflare_options: Record<string, unknown>;
  default_iperf3_options: Record<string, unknown>;
  iperf3_list_url: string;
}

export type ExportAuthType = 'none' | 'basic' | 'bearer' | 'custom';

/** ExportAuth is the structured auth payload sent to POST
 * /settings/test/{vm|vl}, mirroring the vm_auth_ and vl_auth_ settings
 * fields for whichever type is selected. The server also still accepts
 * the legacy auth_header field, but the UI no longer sends it. */
export interface ExportAuth {
  type: ExportAuthType;
  username?: string;
  password?: string;
  token?: string;
  header_name?: string;
  header_value?: string;
}

export interface IntegrationSettings {
  vm_enabled: boolean;
  vm_url: string;
  vm_auth_header: string;
  vm_auth_type: ExportAuthType;
  vm_auth_username: string;
  vm_auth_password: string;
  vm_auth_token: string;
  vm_auth_header_name: string;
  vm_auth_header_value: string;
  vm_extra_labels: Record<string, string>;
  vl_enabled: boolean;
  vl_url: string;
  vl_auth_header: string;
  vl_auth_type: ExportAuthType;
  vl_auth_username: string;
  vl_auth_password: string;
  vl_auth_token: string;
  vl_auth_header_name: string;
  vl_auth_header_value: string;
  vl_stream_fields: Record<string, string>;
  metrics_enabled: boolean;
}

export interface ThresholdSet {
  /** download_mbps_min, upload_mbps_min, ping_ms_max, jitter_ms_max and
   * loss_pct_max each have three states on a target: key absent means
   * inherit the global default from Settings -> Notifications; key present
   * with null means this metric is disabled for this target (the backend
   * skips it entirely, regardless of the global default); key present with
   * a number overrides the global default with that value. */
  download_mbps_min?: number | null;
  upload_mbps_min?: number | null;
  ping_ms_max?: number | null;
  jitter_ms_max?: number | null;
  loss_pct_max?: number | null;
  notify_on_failure?: boolean | null;
  /** sla_download_mbps/sla_upload_mbps override the general SLA plan
   * speeds (GeneralSettings.sla_download_mbps/sla_upload_mbps) for this
   * target only, resolved independently per field; omitted (like every
   * other field here) means "inherit the general plan for this field". */
  sla_download_mbps?: number | null;
  sla_upload_mbps?: number | null;
}

export type NotifyChannelType = 'webhook' | 'apprise';

export interface NotifyChannel {
  id: string;
  type: NotifyChannelType;
  name: string;
  enabled: boolean;
  url: string;
  token?: string;
  headers?: Record<string, string>;
  priority?: string;
  tags?: string[];
  urls?: string[];
}

export interface NotificationSettings {
  enabled: boolean;
  channels: NotifyChannel[];
  default_thresholds: ThresholdSet;
  cooldown_minutes: number;
  quiet_hours_start: string;
  quiet_hours_end: string;
  notify_recovery: boolean;
}

export type AuthMode = 'open' | 'forward_auth' | 'token' | 'oidc';

export interface AuthSettings {
  mode: AuthMode;
  user_header: string;
  groups_header: string;
  groups_separator: string;
  trusted_proxies: string[];
  admin_group: string;
  allow_tokens: boolean;
  oidc_issuer: string;
  oidc_client_id: string;
  oidc_client_secret: string;
  oidc_redirect_base_url: string;
  oidc_scopes: string[];
  oidc_groups_claim: string;
  oidc_allowed_groups: string[];
  oidc_allowed_emails: string[];
  session_ttl_hours: number;
}

export interface Settings {
  general: GeneralSettings;
  engines: EngineSettings;
  integrations: IntegrationSettings;
  notifications: NotificationSettings;
  auth: AuthSettings;
  locked: string[];
}

export type SettingsPatch = {
  general?: Partial<GeneralSettings>;
  engines?: Partial<EngineSettings>;
  integrations?: Partial<IntegrationSettings>;
  notifications?: Partial<NotificationSettings>;
  auth?: Partial<AuthSettings>;
};

export interface ConnectionTest {
  ok: boolean;
  status?: number;
  latency_ms?: number;
  error?: string;
}

export const getSettings = () => request<Settings>('/settings');
export const updateSettings = (patch: SettingsPatch) =>
  request<Settings>('/settings', { method: 'PUT', body: JSON.stringify(patch) });
export const testIntegration = (target: 'vm' | 'vl', body: { url?: string; auth?: ExportAuth }) =>
  request<ConnectionTest>(`/settings/test/${target}`, { method: 'POST', body: JSON.stringify(body) });
export const testOIDC = (body: { issuer: string; client_id: string; client_secret: string }) =>
  request<ConnectionTest>('/settings/test/oidc', { method: 'POST', body: JSON.stringify(body) });
export const testNotifyChannel = (channelId: string) =>
  request<ConnectionTest>(`/settings/test/notify/${encodeURIComponent(channelId)}`, { method: 'POST' });

export interface Me {
  mode: AuthMode;
  user: string;
  groups: string[];
  is_admin: boolean;
  source?: string;
  email?: string;
  name?: string;
}

export const getMe = () => request<Me>('/me');

/** logout ends the current session. Unlike every other call here it hits
 * /auth/logout, not an /api/v1 path, so it bypasses request()/BASE and
 * calls fetch directly; the server answers 204 either way. */
export const logout = async () => {
  await fetch('/auth/logout', { method: 'POST' });
};

/** ApiTokenInfo never carries the token itself -- only CreatedToken does,
 * and only in the response to the request that created it. */
export interface ApiTokenInfo {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at?: string;
}

export interface CreatedToken extends ApiTokenInfo {
  token: string;
}

export const listTokens = async () =>
  (await request<{ tokens: ApiTokenInfo[] }>('/settings/tokens')).tokens;
export const createToken = (name: string) =>
  request<CreatedToken>('/settings/tokens', { method: 'POST', body: JSON.stringify({ name }) });
export const deleteToken = (id: number) =>
  request<void>(`/settings/tokens/${id}`, { method: 'DELETE' });
