const DASH = '—';

/** Renders bits per second as Kbps / Mbps / Gbps. */
export function formatBps(bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return DASH;
  const mbps = bps / 1_000_000;
  if (mbps < 1) return `${Math.round(bps / 1000)} Kbps`;
  if (mbps >= 1000) return `${(mbps / 1000).toFixed(2)} Gbps`;
  return `${mbps.toFixed(1)} Mbps`;
}

/** Renders milliseconds, keeping a decimal only where it matters. */
export function formatMs(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return DASH;
  return ms < 100 ? `${ms.toFixed(1)} ms` : `${Math.round(ms)} ms`;
}

/** Renders a packet-loss percentage. */
export function formatLoss(pct: number): string {
  if (!Number.isFinite(pct) || pct <= 0) return '0%';
  return `${pct.toFixed(1)}%`;
}

/** Renders an ISO timestamp as a compact "time ago" label. */
export function formatRelative(iso: string, now: Date = new Date()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return DASH;
  const seconds = Math.round((now.getTime() - then) / 1000);
  if (seconds < 5) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

/** Renders a 0..1 fraction as a percentage, one decimal at most. */
export function formatPercent(fraction: number): string {
  if (!Number.isFinite(fraction)) return DASH;
  const pct = fraction * 100;
  const rounded = Math.round(pct * 10) / 10;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(1)}%`;
}

/** Renders a byte count as B / KB / MB / GB (binary units). */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return DASH;
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) { value /= 1024; i += 1; }
  return i === 0 ? `${Math.round(value)} B` : `${value.toFixed(1)} ${units[i]}`;
}

/** Renders an ISO timestamp in the viewer's locale, seconds included. */
export function formatDateTime(iso: string): string {
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return DASH;
  return new Date(ms).toLocaleString(undefined, {
    year: 'numeric', month: 'short', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  });
}
