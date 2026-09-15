import { useQueryClient } from '@tanstack/react-query';
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { queryKeys } from '../../lib/queries';
import { useLiveRun, type LiveRun, type LiveRunEvent } from '../../lib/useLiveRun';
import { HIDE_DELAY_MS } from './constants';

export interface LivePanelValue {
  live: LiveRun | null;
  expanded: boolean;
  /** hidden is true once the user has closed a run they started themselves
   * (via `open(runId)`); LivePanel renders nothing for that run at all,
   * rather than collapsing it into the compact bar. */
  hidden: boolean;
  /** Pass the run's id when opening for a run the caller just started (a
   * "Run now" click) so `close()` can dismiss it entirely. Call with no
   * argument to merely expand an already-visible run (e.g. the compact
   * bar's "Expand live test" button) without marking it as user-started. */
  open: (runId?: number) => void;
  close: () => void;
}

export const LivePanelContext = createContext<LivePanelValue>({
  live: null, expanded: false, hidden: false, open: () => {}, close: () => {},
});

/** useLivePanel gives pages the live run and the slide-over controls, so a
 * "Run now" button can open the panel it just started. */
export function useLivePanel(): LivePanelValue {
  return useContext(LivePanelContext);
}

/**
 * LiveRunProvider owns the single EventSource subscription for the app and
 * keeps the query caches in step with it: a `result` event patches that
 * target's `latest` cache entry directly (no refetch), and a run reaching a
 * terminal status settles the list queries.
 */
export function LiveRunProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const [expanded, setExpanded] = useState(false);
  // The run id the user themselves started (via open(runId)), and the run
  // id they subsequently closed — tracked separately from `expanded` so a
  // dismissed manual run can disappear entirely instead of collapsing into
  // the compact bar the way a scheduled run does.
  const [startedRunId, setStartedRunId] = useState<number | null>(null);
  const [hiddenRunId, setHiddenRunId] = useState<number | null>(null);

  const onEvent = useCallback((event: LiveRunEvent) => {
    if (event.type === 'result') {
      qc.invalidateQueries({ queryKey: ['results'] });
      qc.invalidateQueries({ queryKey: ['summary'] });
      qc.invalidateQueries({ queryKey: ['history'] });
      qc.invalidateQueries({ queryKey: ['outages'] });
      return;
    }
    if (event.type === 'run' && event.status) {
      qc.invalidateQueries({ queryKey: ['results'] });
      qc.invalidateQueries({ queryKey: queryKeys.runs });
      qc.invalidateQueries({ queryKey: queryKeys.targets });
      qc.invalidateQueries({ queryKey: queryKeys.schedules });
      qc.invalidateQueries({ queryKey: ['summary'] });
      qc.invalidateQueries({ queryKey: ['history'] });
      qc.invalidateQueries({ queryKey: ['outages'] });
    }
  }, [qc]);

  const live = useLiveRun({ onEvent });

  // Collapse back to nothing once a run settles, same delay as the compact
  // bar's own fade so the panel and bar drop together. Without this, a run
  // the user expanded (or a stale one from a prior session) would stay
  // expanded forever, and the *next* run — however it starts — would
  // inherit that expanded state despite the user never opening it.
  useEffect(() => {
    if (!live?.finished) return undefined;
    const timer = setTimeout(() => setExpanded(false), HIDE_DELAY_MS);
    return () => clearTimeout(timer);
  }, [live?.finished, live?.runId]);

  // Once a new run id appears, any prior started/hidden bookkeeping no
  // longer applies to it unless open(runId) already claimed this very run
  // (it runs synchronously in the mutation's onSuccess, ahead of the SSE
  // event that updates live.runId).
  useEffect(() => {
    const id = live?.runId;
    if (id === undefined) return;
    setStartedRunId((prev) => (prev === id ? prev : null));
    setHiddenRunId((prev) => (prev === id ? prev : null));
  }, [live?.runId]);

  const open = useCallback((runId?: number) => {
    if (runId !== undefined) setStartedRunId(runId);
    setExpanded(true);
  }, []);

  const close = useCallback(() => {
    setExpanded(false);
    setHiddenRunId((prev) => {
      if (live && live.runId === startedRunId) return live.runId;
      return prev;
    });
  }, [live, startedRunId]);

  const hidden = live !== null && live.runId === hiddenRunId;

  const value = useMemo<LivePanelValue>(() => ({
    live,
    expanded,
    hidden,
    open,
    close,
  }), [live, expanded, hidden, open, close]);

  return <LivePanelContext.Provider value={value}>{children}</LivePanelContext.Provider>;
}
