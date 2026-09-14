import type { TargetSummary } from '../../lib/api';
import { formatBps, formatMs, formatRelative } from '../../lib/format';
import { MiniSparkline } from './MiniSparkline';

const STATUS_DOT: Record<string, string> = {
  ok: 'bg-ok',
  degraded: 'bg-warn',
  failed: 'bg-bad',
};

/** TargetCard is the dashboard's latest-per-target summary: the newest
 * result's download/upload/ping, a 24h download sparkline, and a Run now
 * action. A target with no result yet renders a "never run" placeholder
 * instead of readouts. */
export function TargetCard({
  summary, spark, onRun, running,
}: {
  summary: TargetSummary;
  spark: number[];
  onRun: (id: number) => void;
  running: boolean;
}) {
  const { latest } = summary;
  return (
    <div className="rounded border border-line bg-surface p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {latest && (
            <span
              role="img"
              aria-label={`Status: ${latest.status}`}
              className={`inline-block h-2.5 w-2.5 rounded-full ${STATUS_DOT[latest.status] ?? 'bg-muted'}`}
            />
          )}
          <h3 className="font-medium text-fg">{summary.target_name}</h3>
        </div>
        <button
          type="button"
          onClick={() => onRun(summary.target_id)}
          disabled={running}
          className="rounded border border-line px-2 py-1 text-xs text-muted hover:text-fg disabled:opacity-50"
        >
          {running ? 'Running…' : 'Run now'}
        </button>
      </div>

      {!latest ? (
        <p className="mt-3 text-sm text-muted">Never run.</p>
      ) : (
        <>
          <div className="mt-3 grid grid-cols-3 gap-2 text-sm">
            <div>
              <p className="text-xs text-muted">Download</p>
              <p className="tabular-nums text-fg">{formatBps(latest.download_bps)}</p>
            </div>
            <div>
              <p className="text-xs text-muted">Upload</p>
              <p className="tabular-nums text-fg">{formatBps(latest.upload_bps)}</p>
            </div>
            <div>
              <p className="text-xs text-muted">Ping</p>
              <p className="tabular-nums text-fg">{formatMs(latest.ping_ms)}</p>
            </div>
          </div>
          <p className="mt-1 text-xs text-muted">{formatRelative(latest.started_at)}</p>
          <div className="mt-2">
            <MiniSparkline samples={spark} />
          </div>
        </>
      )}
    </div>
  );
}
