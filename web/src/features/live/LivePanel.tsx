import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { Button } from '@/components/ui/button';
import { ErrorDialog } from '@/components/ErrorDialog';
import { formatBps, formatLoss, formatMs } from '../../lib/format';
import { useCancelRun } from '../../lib/queries';
import type { LiveRun } from '../../lib/useLiveRun';
import { Gauge, type GaugePhase } from './Gauge';
import { Sparkline } from './Sparkline';
import { useLivePanel } from './LiveRunProvider';
import { HIDE_DELAY_MS } from './constants';

/** Tile is one small readout beside the gauge. */
function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-raised px-3 py-2">
      <div className="text-[10px] uppercase tracking-[0.18em] text-faint">{label}</div>
      <div className="font-mono text-lg tabular-nums text-fg">{value}</div>
    </div>
  );
}

/** Stepper shows which target of a multi-target run is executing. */
function Stepper({ total, done }: { total: number; done: number }) {
  if (total <= 1) return null;
  const current = Math.min(done + 1, total);
  return (
    <div className="flex items-center gap-2 text-xs text-muted">
      <span>{`Target ${current} of ${total}`}</span>
      <div className="flex gap-1" aria-hidden="true">
        {Array.from({ length: total }, (_, i) => (
          <span
            key={i}
            className={`h-1.5 w-6 rounded-full ${i < done ? 'bg-accent' : i === done ? 'bg-accent/50' : 'bg-raised'}`}
          />
        ))}
      </div>
    </div>
  );
}

/** LivePanel is the live run view: an expanded slide-over for runs the user
 * started, a compact bar for everything else (scheduled runs). */
export function LivePanel() {
  const { live, expanded, hidden, open, close } = useLivePanel();
  const cancel = useCancelRun();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (!live) {
      setVisible(false);
      return undefined;
    }
    setVisible(true);
    if (!live.finished || expanded) return undefined;
    const timer = setTimeout(() => setVisible(false), HIDE_DELAY_MS);
    return () => clearTimeout(timer);
  }, [live, expanded]);

  if (!live || !visible || hidden) return null;
  return expanded
    ? <ExpandedPanel live={live} onClose={close} onCancel={() => cancel.mutate(live.runId)} canceling={cancel.isPending} />
    : <CompactBar live={live} onExpand={() => open()} />;
}

function ExpandedPanel({ live, onClose, onCancel, canceling }: {
  live: LiveRun; onClose: () => void; onCancel: () => void; canceling: boolean;
}) {
  const [errorOpen, setErrorOpen] = useState(false);
  // Escape closes the dialog, and focus returns to whatever had it before
  // the dialog opened (the "Expand live test" / "Run now" button).
  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      previouslyFocused?.focus?.();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-app/70">
      <button type="button" aria-label="Close" className="absolute inset-0 cursor-default" onClick={onClose} />
      <aside
        role="dialog"
        aria-modal="true"
        aria-label="Live test"
        className="relative flex h-full w-full max-w-md flex-col gap-5 overflow-y-auto border-l border-line bg-app p-6"
      >
        <header className="flex items-start justify-between">
          <div>
            <h2 className="text-sm uppercase tracking-[0.2em] text-faint">Live test</h2>
            <p className="font-mono text-xs text-muted">{live.engine}</p>
          </div>
          <button
            className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-surface"
            onClick={onClose}
          >
            Close
          </button>
        </header>

        <div className="flex justify-center">
          <Gauge bps={live.bps} phase={live.phase as GaugePhase} />
        </div>

        <Sparkline samples={live.samples} />

        <div className="grid grid-cols-3 gap-2">
          <Tile label="Ping" value={formatMs(live.pingMs)} />
          <Tile label="Jitter" value={formatMs(live.jitterMs)} />
          <Tile label="Loss" value={formatLoss(live.lossPct)} />
        </div>

        <p className="text-sm text-muted">
          {live.serverName || 'Selecting server…'}
          {live.isp ? <span className="text-faint"> · {live.isp}</span> : null}
        </p>

        <Stepper total={live.targetsTotal} done={live.targetsDone} />

        {live.finished && live.status !== 'done' && live.error && (
          <Button type="button" variant="outline" size="sm" className="w-fit" onClick={() => setErrorOpen(true)}>
            View error
          </Button>
        )}

        <footer className="mt-auto flex items-center gap-3">
          {live.finished ? (
            <>
              <span className="text-sm capitalize text-muted">{live.status}</span>
              {live.resultId > 0 && (
                <Link
                  className="rounded border border-accent px-3 py-1.5 text-sm text-accent hover:bg-accent/20"
                  to={`/results?result_id=${live.resultId}`}
                >
                  View result
                </Link>
              )}
            </>
          ) : (
            <button
              className="rounded border border-line px-3 py-1.5 text-sm text-muted hover:bg-surface disabled:opacity-50"
              disabled={canceling}
              onClick={onCancel}
            >
              Cancel run
            </button>
          )}
        </footer>
      </aside>
      {live.finished && live.status !== 'done' && live.error && (
        <ErrorDialog open={errorOpen} onOpenChange={setErrorOpen} title="Live test failed" error={live.error} />
      )}
    </div>
  );
}

function CompactBar({ live, onExpand }: { live: LiveRun; onExpand: () => void }) {
  const pct = Math.round(Math.min(Math.max(live.progress, 0), 1) * 100);
  return (
    <div className="border-b border-accent/30 bg-accent/10">
      <div className="mx-auto flex max-w-6xl items-center gap-4 px-4 py-2 text-sm">
        <span className="rounded bg-accent/20 px-1.5 py-0.5 font-mono text-xs uppercase text-accent">
          {live.engine}
        </span>
        <span className="w-24 capitalize text-muted" aria-live="polite">{live.phase}</span>
        <div
          className="h-1 flex-1 overflow-hidden rounded bg-raised"
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div className="h-full bg-accent transition-[width] duration-150" style={{ width: `${pct}%` }} />
        </div>
        <span className="w-28 text-right font-mono tabular-nums text-fg">{formatBps(live.bps)}</span>
        <button
          className="rounded border border-line px-2 py-0.5 text-xs text-muted hover:bg-raised"
          onClick={onExpand}
        >
          Expand live test
        </button>
      </div>
    </div>
  );
}
