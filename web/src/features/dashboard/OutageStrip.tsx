import type { Incident } from '../../lib/api';
import { formatDateTime } from '../../lib/format';

function colorFor(status: string): string {
  if (status === 'failed') return 'bg-bad';
  if (status === 'degraded' || status === 'skipped') return 'bg-warn';
  return 'bg-muted';
}

function pct(iso: string, from: number, span: number): number {
  const t = Date.parse(iso);
  if (span <= 0) return 0;
  return Math.min(100, Math.max(0, ((t - from) / span) * 100));
}

/** OutageStrip lays incidents on a percentage-positioned time axis between
 * `from` and `to`. Each incident is a focusable button, not a decorative
 * div, so keyboard and screen-reader users get the same detail a mouse
 * hover would show via `title`. */
export function OutageStrip({ incidents, from, to }: { incidents: Incident[]; from: string; to: string }) {
  const fromMs = Date.parse(from);
  const toMs = Date.parse(to);
  const span = toMs - fromMs;

  if (incidents.length === 0) {
    return (
      <div className="rounded border border-line bg-surface p-4">
        <p className="text-sm text-muted">No outages in this range.</p>
      </div>
    );
  }

  return (
    <div className="rounded border border-line bg-surface p-4">
      <div className="relative h-8 w-full rounded bg-raised">
        {incidents.map((incident, i) => {
          const left = pct(incident.started_at, fromMs, span);
          const right = pct(incident.ended_at, fromMs, span);
          const width = Math.max(right - left, 0.3);
          const label = `${incident.target_name}: ${incident.status}, ${incident.count} test${
            incident.count === 1 ? '' : 's'
          }, ${formatDateTime(incident.started_at)} to ${formatDateTime(incident.ended_at)}`;
          return (
            <button
              key={`${incident.target_id ?? 'none'}-${incident.started_at}-${i}`}
              type="button"
              aria-label={label}
              title={incident.error}
              style={{ left: `${left}%`, width: `${width}%`, minWidth: '3px' }}
              className={`absolute top-0 h-full rounded ${colorFor(incident.status)}`}
            />
          );
        })}
      </div>
      <div className="mt-1 flex justify-between text-xs text-faint">
        <span>{formatDateTime(from)}</span>
        <span>{formatDateTime(to)}</span>
      </div>
    </div>
  );
}
