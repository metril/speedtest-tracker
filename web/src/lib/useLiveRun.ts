import { useEffect, useRef, useState } from 'react';
import type { Result } from './api';

/** How many instantaneous throughput samples the sparkline keeps. */
const MAX_SAMPLES = 60;

export interface LiveRun {
  runId: number;
  targetId: number;
  resultId: number;
  engine: string;
  phase: string;
  progress: number;
  bps: number;
  pingMs: number;
  jitterMs: number;
  lossPct: number;
  serverName: string;
  isp: string;
  status: string;
  targetsTotal: number;
  targetsDone: number;
  samples: number[];
  /** finished is true once the run reached a terminal status; the panel
   * keeps showing the settled values instead of blanking. */
  finished: boolean;
}

export interface LiveRunEvent {
  type: 'result' | 'run';
  status?: string;
  result?: Result;
}

interface UseLiveRunOptions {
  onEvent?: (event: LiveRunEvent) => void;
}

interface ProgressPayload {
  run_id: number;
  result_id?: number;
  target_id: number;
  engine: string;
  phase: string;
  progress: number;
  bps: number;
  ping_ms: number;
  jitter_ms?: number;
  loss_pct?: number;
  server_name?: string;
}

interface RunPayload {
  run_id: number;
  status: string;
  targets_total?: number;
  targets_done?: number;
}

const TERMINAL = new Set(['done', 'failed', 'canceled', 'skipped']);

/**
 * useLiveRun subscribes to /api/v1/events and exposes the current (or most
 * recently finished) run. The browser reconnects an EventSource on its own.
 * Deliberately provider-less so it stays trivial to unit test; pass
 * `onEvent` to react to result rows and terminal runs from a component that
 * does sit under a QueryClientProvider.
 */
export function useLiveRun(options?: UseLiveRunOptions): LiveRun | null {
  const [live, setLive] = useState<LiveRun | null>(null);
  const onEventRef = useRef(options?.onEvent);
  onEventRef.current = options?.onEvent;

  useEffect(() => {
    const source = new EventSource('/api/v1/events');

    const onProgress = (e: MessageEvent) => {
      const p = JSON.parse(e.data) as ProgressPayload;
      setLive((prev) => {
        const sameRun = prev && prev.runId === p.run_id;
        const samePhase = sameRun && prev!.phase === p.phase && prev!.targetId === p.target_id;
        const samples = samePhase ? prev!.samples : [];
        const next = p.bps > 0 ? [...samples, p.bps].slice(-MAX_SAMPLES) : samples;
        return {
          runId: p.run_id,
          targetId: p.target_id,
          resultId: p.result_id ?? 0,
          engine: p.engine,
          phase: p.phase,
          progress: p.progress ?? 0,
          bps: p.bps ?? 0,
          pingMs: p.ping_ms ?? 0,
          jitterMs: p.jitter_ms ?? 0,
          lossPct: p.loss_pct ?? 0,
          serverName: p.server_name ?? (sameRun ? prev!.serverName : ''),
          isp: sameRun ? prev!.isp : '',
          status: 'running',
          targetsTotal: sameRun ? prev!.targetsTotal : 1,
          targetsDone: sameRun ? prev!.targetsDone : 0,
          samples: next,
          finished: false,
        };
      });
    };

    const onResult = (e: MessageEvent) => {
      const result = JSON.parse(e.data) as Result;
      onEventRef.current?.({ type: 'result', result });
      setLive((prev) => {
        if (!prev || prev.targetId !== result.target_id) return prev;
        return {
          ...prev,
          resultId: result.id,
          isp: result.isp ?? prev.isp,
          serverName: result.server_name || prev.serverName,
        };
      });
    };

    const onRun = (e: MessageEvent) => {
      const r = JSON.parse(e.data) as RunPayload;
      if (TERMINAL.has(r.status)) {
        onEventRef.current?.({ type: 'run', status: r.status });
      }
      setLive((prev) => {
        if (!prev || prev.runId !== r.run_id) {
          // A `run` event can arrive before the first `progress` event (or
          // for a different run than the one previously tracked); create
          // state rather than dropping it so the stepper counts are kept.
          return {
            runId: r.run_id,
            targetId: 0,
            resultId: 0,
            engine: '',
            phase: 'connecting',
            progress: 0,
            bps: 0,
            pingMs: 0,
            jitterMs: 0,
            lossPct: 0,
            serverName: '',
            isp: '',
            status: r.status,
            targetsTotal: r.targets_total ?? 1,
            targetsDone: r.targets_done ?? 0,
            samples: [],
            finished: TERMINAL.has(r.status),
          };
        }
        return {
          ...prev,
          status: r.status,
          finished: TERMINAL.has(r.status),
          targetsTotal: r.targets_total ?? prev.targetsTotal,
          targetsDone: r.targets_done ?? prev.targetsDone,
        };
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
