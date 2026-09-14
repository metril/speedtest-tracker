import { formatBps, formatMs } from '../lib/format';
import { useCancelRun } from '../lib/queries';
import { useLiveRun } from '../lib/useLiveRun';

/** LiveRunBanner shows the in-flight test's phase, throughput and a cancel. */
export function LiveRunBanner() {
  const live = useLiveRun();
  const cancel = useCancelRun();
  if (!live) return null;

  const pct = Math.round(Math.min(Math.max(live.progress, 0), 1) * 100);
  return (
    <div className="border-b border-sky-900/60 bg-sky-950/40">
      <div className="mx-auto flex max-w-6xl items-center gap-4 px-4 py-2 text-sm">
        <span className="rounded bg-sky-500/20 px-1.5 py-0.5 font-mono text-xs uppercase text-sky-300">
          {live.engine}
        </span>
        <span className="w-24 text-slate-300 capitalize">{live.phase}</span>
        <div className="h-1 flex-1 overflow-hidden rounded bg-slate-800">
          <div className="h-full bg-sky-400 transition-[width] duration-150" style={{ width: `${pct}%` }} />
        </div>
        <span className="w-28 text-right font-mono tabular-nums text-slate-100">
          {formatBps(live.bps)}
        </span>
        <span className="w-20 text-right font-mono tabular-nums text-slate-400">
          {formatMs(live.pingMs)}
        </span>
        <button
          className="rounded border border-slate-700 px-2 py-0.5 text-xs text-slate-300 hover:bg-slate-800 disabled:opacity-50"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate(live.runId)}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
