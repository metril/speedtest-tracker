import { useQueryClient } from '@tanstack/react-query';
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import { queryKeys } from '../../lib/queries';
import { useLiveRun, type LiveRun, type LiveRunEvent } from '../../lib/useLiveRun';

export interface LivePanelValue {
  live: LiveRun | null;
  expanded: boolean;
  open: () => void;
  close: () => void;
}

export const LivePanelContext = createContext<LivePanelValue>({
  live: null, expanded: false, open: () => {}, close: () => {},
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

  const onEvent = useCallback((event: LiveRunEvent) => {
    if (event.type === 'result') {
      if (event.result?.target_id) {
        qc.setQueryData(queryKeys.targetLatest(event.result.target_id), event.result);
      }
      qc.invalidateQueries({ queryKey: ['results'] });
      return;
    }
    if (event.type === 'run' && event.status) {
      qc.invalidateQueries({ queryKey: ['results'] });
      qc.invalidateQueries({ queryKey: queryKeys.runs });
      qc.invalidateQueries({ queryKey: queryKeys.targets });
      qc.invalidateQueries({ queryKey: queryKeys.schedules });
    }
  }, [qc]);

  const live = useLiveRun({ onEvent });
  const value = useMemo<LivePanelValue>(() => ({
    live,
    expanded,
    open: () => setExpanded(true),
    close: () => setExpanded(false),
  }), [live, expanded]);

  return <LivePanelContext.Provider value={value}>{children}</LivePanelContext.Provider>;
}
