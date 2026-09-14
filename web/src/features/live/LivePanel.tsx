import { useEffect, useState } from 'react';
import { formatBps, formatLoss, formatMs } from '../../lib/format';
import { useCancelRun } from '../../lib/queries';
import type { LiveRun } from '../../lib/useLiveRun';
import { Gauge } from './Gauge';
import { Sparkline } from './Sparkline';
import { useLivePanel } from './LiveRunProvider';

/** How long the compact bar lingers after a run settles. */
const HIDE_DELAY_MS = 4000;

/** Tile is one small readout beside the gauge. */
function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-slate-800 bg-slate-900/60 px-3 py-2">
      <div className="text-[10px] uppercase tracking-[0.18em] text-slate-500">{label}</div>
      <div className="font-mono text-lg tabular-nums text-slate-100">{value}</div>
    </div>
  );
}

/** Stepper shows which target of a multi-target run is executing. */
function Stepper({ total, done }: { total: number; done: number }) {
  if (total <= 1) return null;
  const current = Math.min(done + 1, total);
  return (
    <div className="flex items-center gap-2 text-xs text-slate-400">
      <span>{`Target ${current} of ${total}`}</span>
      <div className="flex gap-1" aria-hidden="true">
        {Array.from({ length: total }, (_, i) => (
          <span
            key={i}
            className={`h-1.5 w-6 rounded-full ${i < done ? 'bg-sky-400' : i === done ? 'bg-sky-400/50' : 'bg-slate-800'}`}
          />
        ))}
      </div>
    </div>
  );
}

/** LivePanel is the live run view: an expanded slide-over for runs the user
 * started, a compact bar for everything else (scheduled runs). */
export function LivePanel() {
  const { live, expanded, open, close } = useLivePanel();
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

  if (!live || !visible) return null;
  return expanded
    ? <ExpandedPanel live={live} onClose={close} onCancel={() => cancel.mutate(live.runId)} canceling={cancel.isPending} />
    : <CompactBar live={live} onExpand={open} />;
}

function ExpandedPanel({ live, onClose, onCancel, canceling }: {
  live: LiveRun; onClose: () => void; onCancel: () => void; canceling: boolean;
}) {
  return (
    <div className="fixed inset-0 z-40 flex justify-end bg-slate-950/70" onClick={onClose}>
      <aside
        role="dialog"
        aria-modal="true"
        aria-label="Live test"
        className="flex h-full w-full max-w-md flex-col gap-5 overflow-y-auto border-l border-slate-800 bg-slate-950 p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-start justify-between">
          <div>
            <h2 className="text-sm uppercase tracking-[0.2em] text-slate-500">Live test</h2>
            <p className="font-mono text-xs text-slate-400">{live.engine}</p>
          </div>
          <button
            className="rounded border border-slate-800 px-2 py-1 text-xs text-slate-400 hover:bg-slate-900"
            onClick={onClose}
          >
            Close
          </button>
        </header>

        <div className="flex justify-center">
          <Gauge bps={live.bps} phase={live.phase} />
        </div>

        <Sparkline samples={live.samples} />

        <div className="grid grid-cols-3 gap-2">
          <Tile label="Ping" value={formatMs(live.pingMs)} />
          <Tile label="Jitter" value={formatMs(live.jitterMs)} />
          <Tile label="Loss" value={formatLoss(live.lossPct)} />
        </div>

        <p className="text-sm text-slate-400">
          {live.serverName || 'Selecting server…'}
          {live.isp ? <span className="text-slate-500"> · {live.isp}</span> : null}
        </p>

        <Stepper total={live.targetsTotal} done={live.targetsDone} />

        <footer className="mt-auto flex items-center gap-3">
          {live.finished ? (
            <>
              <span className="text-sm capitalize text-slate-300">{live.status}</span>
              {live.resultId > 0 && (
                <a
                  className="rounded border border-sky-700 px-3 py-1.5 text-sm text-sky-300 hover:bg-sky-900/40"
                  href={`/results?result_id=${live.resultId}`}
                >
                  View result
                </a>
              )}
            </>
          ) : (
            <button
              className="rounded border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-900 disabled:opacity-50"
              disabled={canceling}
              onClick={onCancel}
            >
              Cancel run
            </button>
          )}
        </footer>
      </aside>
    </div>
  );
}

function CompactBar({ live, onExpand }: { live: LiveRun; onExpand: () => void }) {
  const pct = Math.round(Math.min(Math.max(live.progress, 0), 1) * 100);
  return (
    <div className="border-b border-sky-900/60 bg-sky-950/40" aria-live="polite">
      <div className="mx-auto flex max-w-6xl items-center gap-4 px-4 py-2 text-sm">
        <span className="rounded bg-sky-500/20 px-1.5 py-0.5 font-mono text-xs uppercase text-sky-300">
          {live.engine}
        </span>
        <span className="w-24 capitalize text-slate-300">{live.phase}</span>
        <div
          className="h-1 flex-1 overflow-hidden rounded bg-slate-800"
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div className="h-full bg-sky-400 transition-[width] duration-150" style={{ width: `${pct}%` }} />
        </div>
        <span className="w-28 text-right font-mono tabular-nums text-slate-100">{formatBps(live.bps)}</span>
        <button
          className="rounded border border-slate-700 px-2 py-0.5 text-xs text-slate-300 hover:bg-slate-800"
          onClick={onExpand}
        >
          Expand live test
        </button>
      </div>
    </div>
  );
}
