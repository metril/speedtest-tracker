import { sparkPath } from '../live/Sparkline';

/** MiniSparkline is the 24h download trend on a target card. It is
 * decoration beside the numeric readout, so it is hidden from assistive
 * tech and the card carries the numbers itself. */
export function MiniSparkline({ samples, width = 160, height = 32, color }: {
  samples: number[]; width?: number; height?: number; color?: string;
}) {
  const d = sparkPath(samples, width, height);
  if (!d) return <div className="h-8" aria-hidden="true" />;
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="w-full" preserveAspectRatio="none"
      aria-hidden="true" focusable="false">
      <path d={d} fill="none" stroke={color ?? 'var(--color-series-download)'} strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  );
}
