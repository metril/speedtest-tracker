import { useVirtualizer } from '@tanstack/react-virtual';
import { useRef, useState } from 'react';
import type { Result } from '../../lib/api';
import { formatBps, formatDateTime, formatMs, formatRelative } from '../../lib/format';

interface Props {
  rows: Result[];
  onDelete: (id: number) => void;
  onReexecute: (id: number) => void;
  onTag: (id: number, tags: string[]) => void;
}

const ROW_HEIGHT = 40;

/** ResultsTable renders the result rows through a virtualiser. */
export function ResultsTable({ rows, onDelete, onReexecute, onTag }: Props) {
  const parentRef = useRef<HTMLDivElement>(null);
  const [tagging, setTagging] = useState<number | null>(null);
  const [draft, setDraft] = useState('');

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 12,
  });

  return (
    <div className="rounded border border-slate-800">
      <div className="grid grid-cols-[1fr_5rem_7rem_7rem_5rem_1fr_9rem] gap-2 border-b border-slate-800 px-3 py-2 text-xs uppercase tracking-wide text-slate-500">
        <span>Target</span><span>Engine</span><span>Down</span><span>Up</span>
        <span>Ping</span><span>Tags</span><span className="text-right">When</span>
      </div>
      <div ref={parentRef} className="max-h-[65vh] overflow-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
          {virtualizer.getVirtualItems().map((item) => {
            const r = rows[item.index];
            return (
              <div
                key={r.id}
                className="absolute left-0 flex w-full items-center border-b border-slate-900 px-3 text-sm hover:bg-slate-900/50"
                style={{ height: item.size, transform: `translateY(${item.start}px)` }}
              >
                <div className="grid w-full grid-cols-[1fr_5rem_7rem_7rem_5rem_1fr_9rem] items-center gap-2">
                  <span className="truncate text-slate-100" title={r.error || r.server_name}>
                    {r.status !== 'ok' && <span className="mr-1 text-rose-400">●</span>}
                    {r.target_name}
                  </span>
                  <span className="font-mono text-xs uppercase text-slate-400">{r.engine}</span>
                  <span className="font-mono tabular-nums text-slate-100">{formatBps(r.download_bps)}</span>
                  <span className="font-mono tabular-nums text-slate-300">{formatBps(r.upload_bps)}</span>
                  <span className="font-mono tabular-nums text-slate-400">{formatMs(r.ping_ms)}</span>
                  <span className="flex flex-wrap gap-1">
                    {r.tags.map((t) => (
                      <span key={t} className="rounded bg-slate-800 px-1.5 text-xs text-slate-300">{t}</span>
                    ))}
                    {tagging === r.id ? (
                      <input
                        autoFocus
                        aria-label="Tags"
                        className="w-32 rounded border border-slate-700 bg-slate-900 px-1 text-xs text-slate-100"
                        value={draft}
                        onChange={(e) => setDraft(e.target.value)}
                        onBlur={() => setTagging(null)}
                        onKeyDown={(e) => {
                          if (e.key !== 'Enter') return;
                          onTag(r.id, draft.split(',').map((s) => s.trim()).filter(Boolean));
                          setTagging(null);
                        }}
                      />
                    ) : (
                      <button className="text-xs text-slate-500 hover:text-sky-400"
                        onClick={() => { setTagging(r.id); setDraft(r.tags.join(', ')); }}>
                        + tag
                      </button>
                    )}
                  </span>
                  <span className="flex items-center justify-end gap-2 text-xs text-slate-400">
                    <span title={formatDateTime(r.started_at)}>{formatRelative(r.started_at)}</span>
                    <button className="text-sky-400 hover:text-sky-300" onClick={() => onReexecute(r.id)}>
                      replay
                    </button>
                    <button
                      className="text-rose-400 hover:text-rose-300"
                      onClick={() => {
                        if (window.confirm(`Delete this result for ${r.target_name}?`)) onDelete(r.id);
                      }}
                    >
                      del
                    </button>
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
