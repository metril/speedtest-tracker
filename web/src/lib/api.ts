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
  limit?: number;
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
