import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useRef, useState } from 'react';
import { formatBps, formatMs } from '../lib/format';
import { useCancelRun } from '../lib/queries';
import { useLiveRun, type LiveRun, type LiveRunEvent } from '../lib/useLiveRun';

/** How long the banner lingers, showing the final phase, after the run
 * reaches a terminal status — long enough to register as "it finished"
 * before the strip disappears. */
const HIDE_DELAY_MS = 1500;

/** LiveRunBanner shows the in-flight test's phase, throughput and a cancel.
 * It also keeps the results/runs/targets query caches fresh: a `result`
 * event means a new row landed, and a run reaching a terminal status means
 * the target/run state settled — both are reasons to refetch. */
export function LiveRunBanner() {
  const qc = useQueryClient();
  const cancel = useCancelRun();
  const [display, setDisplay] = useState<LiveRun | null>(null);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const onEvent = useCallback((event: LiveRunEvent) => {
    if (event.type === 'result') {
      qc.invalidateQueries({ queryKey: ['results'] });
    } else if (event.type === 'run' && event.status) {
      qc.invalidateQueries({ queryKey: ['results'] });
      qc.invalidateQueries({ queryKey: ['runs'] });
      qc.invalidateQueries({ queryKey: ['targets'] });
    }
  }, [qc]);

  const live = useLiveRun({ onEvent });

  useEffect(() => {
    if (live) {
      clearTimeout(hideTimer.current);
      setDisplay(live);
      return undefined;
    }
    hideTimer.current = setTimeout(() => setDisplay(null), HIDE_DELAY_MS);
    return () => clearTimeout(hideTimer.current);
  }, [live]);

  if (!display) return null;

  const pct = Math.round(Math.min(Math.max(display.progress, 0), 1) * 100);
  return (
    <div className="border-b border-sky-900/60 bg-sky-950/40">
      <div className="mx-auto flex max-w-6xl items-center gap-4 px-4 py-2 text-sm">
        <span className="rounded bg-sky-500/20 px-1.5 py-0.5 font-mono text-xs uppercase text-sky-300">
          {display.engine}
        </span>
        <span className="w-24 text-slate-300 capitalize">{display.phase}</span>
        <div className="h-1 flex-1 overflow-hidden rounded bg-slate-800">
          <div className="h-full bg-sky-400 transition-[width] duration-150" style={{ width: `${pct}%` }} />
        </div>
        <span className="w-28 text-right font-mono tabular-nums text-slate-100">
          {formatBps(display.bps)}
        </span>
        <span className="w-20 text-right font-mono tabular-nums text-slate-400">
          {formatMs(display.pingMs)}
        </span>
        <button
          className="rounded border border-slate-700 px-2 py-0.5 text-xs text-slate-300 hover:bg-slate-800 disabled:opacity-50"
          disabled={cancel.isPending || !live}
          onClick={() => live && cancel.mutate(live.runId)}
        >
          Cancel
        </button>
      </div>
    </div>
  );
}
