import { useRef } from 'react';
import {
  DndContext, KeyboardSensor, PointerSensor, closestCenter,
  useSensor, useSensors, type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext, arrayMove, sortableKeyboardCoordinates,
  useSortable, verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { Target } from '../../lib/api';

/** Above this many selected targets, drag reordering is disabled in favor
 * of a virtualized list — dnd-kit's sortable strategy measures every item's
 * DOM node, which stops working once rows are windowed. The ↑/↓/× buttons
 * (and keyboard focus) keep working at any size. */
const VIRTUALIZE_THRESHOLD = 50;

interface RowProps {
  id: number;
  label: string;
  index: number;
  count: number;
  draggable: boolean;
  onMove: (index: number, delta: number) => void;
  onRemove: (id: number) => void;
}

function Row({ id, label, index, count, draggable, onMove, onRemove }: RowProps) {
  const sortable = useSortable({ id, disabled: !draggable });
  const style = draggable
    ? { transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }
    : undefined;

  return (
    <li
      ref={draggable ? sortable.setNodeRef : undefined}
      style={style}
      className="flex items-center gap-2 rounded border border-line bg-app px-2 py-1 text-sm"
    >
      {draggable && (
        <span
          {...sortable.attributes} {...sortable.listeners}
          aria-label={`Drag ${label}`}
          className="cursor-grab select-none px-1 text-faint active:cursor-grabbing"
        >
          ⠿
        </span>
      )}
      <span className="w-5 text-right font-mono text-xs text-faint">{index + 1}</span>
      <span className="flex-1 text-fg">{label}</span>
      <button type="button" aria-label={`Move ${label} up`} disabled={index === 0}
        className="rounded border border-line px-1.5 text-xs text-muted disabled:opacity-40"
        onClick={() => onMove(index, -1)}>↑</button>
      <button type="button" aria-label={`Move ${label} down`} disabled={index === count - 1}
        className="rounded border border-line px-1.5 text-xs text-muted disabled:opacity-40"
        onClick={() => onMove(index, 1)}>↓</button>
      <button type="button" aria-label={`Remove ${label}`}
        className="rounded border border-line px-1.5 text-xs text-muted"
        onClick={() => onRemove(id)}>×</button>
    </li>
  );
}

interface Props {
  selected: number[];
  byID: Map<number, Target>;
  onChange: (ids: number[]) => void;
}

/** SortableTargetList shows the selected targets in run order with drag
 * (dnd-kit), explicit ↑/↓/× buttons for keyboard/a11y, and, past 50
 * targets, row virtualization (which disables drag — see VIRTUALIZE_THRESHOLD). */
export function SortableTargetList({ selected, byID, onChange }: Props) {
  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const virtualize = selected.length > VIRTUALIZE_THRESHOLD;
  const scrollRef = useRef<HTMLDivElement>(null);

  const move = (index: number, delta: number) => {
    const to = index + delta;
    if (to < 0 || to >= selected.length) return;
    const next = [...selected];
    [next[index], next[to]] = [next[to], next[index]];
    onChange(next);
  };

  const remove = (id: number) => onChange(selected.filter((x) => x !== id));

  const label = (id: number) => byID.get(id)?.name ?? `#${id}`;

  const handleDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const from = selected.indexOf(Number(active.id));
    const to = selected.indexOf(Number(over.id));
    if (from === -1 || to === -1) return;
    onChange(arrayMove(selected, from, to));
  };

  const virtualizer = useVirtualizer({
    count: selected.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 36,
    overscan: 10,
  });

  if (selected.length === 0) {
    return <p className="text-sm text-faint">No targets selected yet.</p>;
  }

  if (virtualize) {
    const items = virtualizer.getVirtualItems();
    return (
      <div ref={scrollRef} className="max-h-80 overflow-y-auto rounded border border-line p-1">
        <ol className="relative grid" style={{ height: virtualizer.getTotalSize() }}>
          {items.map((vi) => {
            const id = selected[vi.index];
            return (
              <div key={id} style={{ position: 'absolute', top: 0, left: 0, right: 0, transform: `translateY(${vi.start}px)` }}>
                <Row
                  id={id} label={label(id)} index={vi.index} count={selected.length}
                  draggable={false} onMove={move} onRemove={remove}
                />
              </div>
            );
          })}
        </ol>
      </div>
    );
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={selected} strategy={verticalListSortingStrategy}>
        <ol className="grid gap-1">
          {selected.map((id, i) => (
            <Row
              key={id} id={id} label={label(id)} index={i} count={selected.length}
              draggable onMove={move} onRemove={remove}
            />
          ))}
        </ol>
      </SortableContext>
    </DndContext>
  );
}
