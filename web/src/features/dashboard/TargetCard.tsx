import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import type { Result, ThresholdSet, TargetSummary } from '../../lib/api';
import { formatBps, formatMs, formatRelative } from '../../lib/format';
import { MiniSparkline } from './MiniSparkline';

const STATUS_DOT: Record<string, string> = {
  ok: 'bg-ok',
  degraded: 'bg-warn',
  failed: 'bg-bad',
};

export type ThresholdStatus = 'ok' | 'bad' | 'no-data';

/** thresholdStatus compares a target's latest result against its
 * thresholds. No latest result is "no data"; a latest result with no
 * configured thresholds is a neutral "ok" (nothing to violate); otherwise
 * any exceeded threshold makes it "bad". */
export function thresholdStatus(latest: Result | null, thresholds?: ThresholdSet): ThresholdStatus {
  if (!latest) return 'no-data';
  if (!thresholds) return 'ok';
  const mbps = (bps: number) => bps / 1e6;
  const breached = (
    (thresholds.download_mbps_min != null && mbps(latest.download_bps) < thresholds.download_mbps_min)
    || (thresholds.upload_mbps_min != null && mbps(latest.upload_bps) < thresholds.upload_mbps_min)
    || (thresholds.ping_ms_max != null && latest.ping_ms > thresholds.ping_ms_max)
    || (thresholds.jitter_ms_max != null && latest.jitter_ms > thresholds.jitter_ms_max)
    || (thresholds.loss_pct_max != null && latest.packet_loss_pct > thresholds.loss_pct_max)
  );
  return breached ? 'bad' : 'ok';
}

const THRESHOLD_BADGE: Record<ThresholdStatus, { label: string; variant: 'secondary' | 'destructive' | 'outline' }> = {
  ok: { label: 'Within thresholds', variant: 'secondary' },
  bad: { label: 'Threshold breached', variant: 'destructive' },
  'no-data': { label: 'No data', variant: 'outline' },
};

/** TargetCard is the dashboard's latest-per-target summary: the newest
 * result's download/upload/ping, a 24h download sparkline, a threshold
 * Badge, and a Run now action. A target with no result yet renders a
 * "never run" placeholder instead of readouts. */
export function TargetCard({
  summary, spark, onRun, running, thresholds,
}: {
  summary: TargetSummary;
  spark: number[];
  onRun: (id: number) => void;
  running: boolean;
  thresholds?: ThresholdSet;
}) {
  const { latest } = summary;
  const status = thresholdStatus(latest, thresholds);
  const badge = THRESHOLD_BADGE[status];

  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex items-center justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2">
            {latest && (
              <span
                role="img"
                aria-label={`Status: ${latest.status}`}
                className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${STATUS_DOT[latest.status] ?? 'bg-muted'}`}
              />
            )}
            <h3 className="truncate font-medium text-fg">{summary.target_name}</h3>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={() => onRun(summary.target_id)} disabled={running}>
            {running ? 'Running…' : 'Run now'}
          </Button>
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <Badge variant={badge.variant}>{badge.label}</Badge>
          {summary.sla_compliance != null && (
            <Badge variant="outline">SLA {Math.round(summary.sla_compliance * 100)}%</Badge>
          )}
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
            {latest.status !== 'ok' && latest.error && (
              <p className="mt-1 truncate text-xs text-bad" title={latest.error}>{latest.error}</p>
            )}
            <div className="mt-2">
              <MiniSparkline samples={spark} />
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
