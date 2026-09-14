export interface Target {
  id: number;
  name: string;
  engine: string;
  enabled: boolean;
  lane: string;
  options: Record<string, unknown>;
  thresholds: Record<string, unknown>;
  created_at: string;
  updated_at: string;
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
}

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
}

export interface SummaryStats {
  from: string;
  to: string;
  targets: TargetSummary[];
  total_results: number;
  total_failures: number;
  success_rate: number;
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
  lane: string;
  options: Record<string, unknown>;
  thresholds?: Record<string, unknown>;
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
    body: JSON.stringify({ name: 'validation', engine, enabled: true, lane: 'wan', options }),
  });
export const listOoklaServers = (q: string) =>
  request<OoklaServer[]>(`/ookla/servers${query({ q })}`);

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

export const targetHistory = (id: number, range: Range) =>
  request<History>(`/targets/${id}/history${query({ range })}`);
export const statsSummary = (range: Range) =>
  request<SummaryStats>(`/stats/summary${query({ range })}`);
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
}

export interface IntegrationSettings {
  vm_enabled: boolean;
  vm_url: string;
  vm_auth_header: string;
  vm_extra_labels: Record<string, string>;
  vl_enabled: boolean;
  vl_url: string;
  vl_auth_header: string;
  vl_stream_fields: Record<string, string>;
  metrics_enabled: boolean;
}

export interface ThresholdSet {
  download_mbps_min?: number | null;
  upload_mbps_min?: number | null;
  ping_ms_max?: number | null;
  jitter_ms_max?: number | null;
  loss_pct_max?: number | null;
  notify_on_failure?: boolean | null;
}

export type NotifyChannelType = 'webhook' | 'ntfy' | 'apprise';

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

export interface Settings {
  general: GeneralSettings;
  engines: EngineSettings;
  integrations: IntegrationSettings;
  notifications: NotificationSettings;
}

export type SettingsPatch = {
  general?: Partial<GeneralSettings>;
  engines?: Partial<EngineSettings>;
  integrations?: Partial<IntegrationSettings>;
  notifications?: Partial<NotificationSettings>;
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
export const testIntegration = (target: 'vm' | 'vl', body: { url?: string; auth_header?: string }) =>
  request<ConnectionTest>(`/settings/test/${target}`, { method: 'POST', body: JSON.stringify(body) });
export const testNotifyChannel = (channelId: string) =>
  request<ConnectionTest>(`/settings/test/notify/${encodeURIComponent(channelId)}`, { method: 'POST' });
