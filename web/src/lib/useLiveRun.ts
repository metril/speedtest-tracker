import { useEffect, useState } from 'react';

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
 */
export function useLiveRun(): LiveRun | null {
  const [live, setLive] = useState<LiveRun | null>(null);

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
    const onRun = (e: MessageEvent) => {
      const r = JSON.parse(e.data) as RunPayload;
      setLive((prev) => {
        if (TERMINAL.has(r.status)) {
          return prev && prev.runId !== r.run_id ? prev : null;
        }
        if (!prev || prev.runId !== r.run_id) return prev;
        return { ...prev, status: r.status };
      });
    };

    source.addEventListener('progress', onProgress);
    source.addEventListener('run', onRun);
    return () => {
      source.removeEventListener('progress', onProgress);
      source.removeEventListener('run', onRun);
      source.close();
    };
  }, []);

  return live;
}
