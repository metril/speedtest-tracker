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
    <div className="rounded border border-line" role="table" aria-label="Results">
      <div
        role="row"
        className="grid grid-cols-[1fr_5rem_7rem_7rem_5rem_1fr_9rem] gap-2 border-b border-line px-3 py-2 text-xs uppercase tracking-wide text-faint"
      >
        <span role="columnheader">Target</span><span role="columnheader">Engine</span>
        <span role="columnheader">Down</span><span role="columnheader">Up</span>
        <span role="columnheader">Ping</span><span role="columnheader">Tags</span>
        <span role="columnheader" className="text-right">When</span>
      </div>
      <div ref={parentRef} className="max-h-[65vh] overflow-auto">
        <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
          {virtualizer.getVirtualItems().map((item) => {
            const r = rows[item.index];
            return (
              <div
                key={r.id}
                role="row"
                className="absolute left-0 flex w-full items-center border-b border-line px-3 text-sm hover:bg-raised"
                style={{ height: item.size, transform: `translateY(${item.start}px)` }}
              >
                <div className="grid w-full grid-cols-[1fr_5rem_7rem_7rem_5rem_1fr_9rem] items-center gap-2">
                  <span role="cell" className="truncate text-fg" title={r.error || r.server_name}>
                    {r.status !== 'ok' && <span className="mr-1 text-bad">●</span>}
                    {r.target_name}
                  </span>
                  <span role="cell" className="font-mono text-xs uppercase text-muted">{r.engine}</span>
                  <span role="cell" className="font-mono tabular-nums text-fg">{formatBps(r.download_bps)}</span>
                  <span role="cell" className="font-mono tabular-nums text-muted">{formatBps(r.upload_bps)}</span>
                  <span role="cell" className="font-mono tabular-nums text-muted">{formatMs(r.ping_ms)}</span>
                  <span role="cell" className="flex flex-wrap gap-1">
                    {r.tags.map((t) => (
                      <span key={t} className="rounded bg-raised px-1.5 text-xs text-muted">{t}</span>
                    ))}
                    {tagging === r.id ? (
                      <input
                        autoFocus
                        aria-label="Tags"
                        className="w-32 rounded border border-line bg-surface px-1 text-xs text-fg"
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
                      <button className="text-xs text-faint hover:text-accent"
                        onClick={() => { setTagging(r.id); setDraft(r.tags.join(', ')); }}>
                        + tag
                      </button>
                    )}
                  </span>
                  <span role="cell" className="flex items-center justify-end gap-2 text-xs text-muted">
                    <span title={formatDateTime(r.started_at)}>{formatRelative(r.started_at)}</span>
                    <button
                      className="text-accent hover:opacity-80"
                      aria-label={`Replay result for ${r.target_name}`}
                      onClick={() => onReexecute(r.id)}
                    >
                      replay
                    </button>
                    <button
                      className="text-bad hover:opacity-80"
                      aria-label={`Delete result for ${r.target_name}`}
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
