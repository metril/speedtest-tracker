import { useEffect, useRef, useState } from 'react';

export interface LiveRun {
  runId: number;
  targetId: number;
  engine: string;
  phase: string;
  progress: number;
  bps: number;
  pingMs: number;
  status: string;
}

/** LiveRunEvent notifies callers of stream activity that doesn't belong in
 * LiveRun state itself (a completed result, a run reaching a terminal
 * status) so they can react to it — e.g. invalidating query caches — without
 * this hook taking a dependency on React Query. */
export interface LiveRunEvent {
  type: 'result' | 'run';
  status?: string;
}

interface UseLiveRunOptions {
  onEvent?: (event: LiveRunEvent) => void;
}

interface ProgressPayload {
  run_id: number;
  target_id: number;
  engine: string;
  phase: string;
  progress: number;
  bps: number;
  ping_ms: number;
}

interface RunPayload {
  run_id: number;
  status: string;
}

const TERMINAL = new Set(['done', 'failed', 'canceled', 'skipped']);

/**
 * useLiveRun subscribes to /api/v1/events and exposes the currently
 * running test, or null when nothing is in flight. The browser reconnects
 * an EventSource on its own; a terminal run event clears the state.
 * Deliberately provider-less (no React Query dependency) so it stays easy to
 * unit test in isolation — pass `onEvent` to react to `result`/terminal
 * `run` activity from a component that does sit under a QueryClientProvider.
 */
export function useLiveRun(options?: UseLiveRunOptions): LiveRun | null {
  const [live, setLive] = useState<LiveRun | null>(null);
  const onEventRef = useRef(options?.onEvent);
  onEventRef.current = options?.onEvent;

  useEffect(() => {
    const source = new EventSource('/api/v1/events');

    const onProgress = (e: MessageEvent) => {
      const p = JSON.parse(e.data) as ProgressPayload;
      setLive({
        runId: p.run_id,
        targetId: p.target_id,
        engine: p.engine,
        phase: p.phase,
        progress: p.progress ?? 0,
        bps: p.bps ?? 0,
        pingMs: p.ping_ms ?? 0,
        status: 'running',
      });
    };
    const onResult = () => {
      onEventRef.current?.({ type: 'result' });
    };
    const onRun = (e: MessageEvent) => {
      const r = JSON.parse(e.data) as RunPayload;
      if (TERMINAL.has(r.status)) {
        onEventRef.current?.({ type: 'run', status: r.status });
      }
      setLive((prev) => {
        if (TERMINAL.has(r.status)) {
          return prev && prev.runId !== r.run_id ? prev : null;
        }
        if (!prev || prev.runId !== r.run_id) return prev;
        return { ...prev, status: r.status };
      });
    };

    source.addEventListener('progress', onProgress);
    source.addEventListener('result', onResult);
    source.addEventListener('run', onRun);
    return () => {
      source.removeEventListener('progress', onProgress);
      source.removeEventListener('result', onResult);
      source.removeEventListener('run', onRun);
      source.close();
    };
  }, []);

  return live;
}
