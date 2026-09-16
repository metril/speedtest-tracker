import { TrendingDown, TrendingUp } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { MiniSparkline } from './MiniSparkline';

export interface KpiTileProps {
  label: string;
  value: string;
  /** spark is the metric's samples across the selected range, oldest first. */
  spark?: number[];
  sparkColor?: string;
  /** delta is the signed fractional change vs. the previous period
   * (0.12 = +12%). Omit to hide the delta, e.g. when the previous window
   * has no data. */
  delta?: number;
  /** favorable says whether this delta's direction counts as an
   * improvement -- true for more throughput or a higher success rate,
   * false for more ping or jitter (where a rise is worse). */
  favorable?: boolean;
  /** subtext is a small line shown under the value, e.g. a supporting
   * count ("1,234 tests"). Omit for none. */
  subtext?: string;
  /** title is a native tooltip shown on hover over the whole tile,
   * e.g. explaining how a percentage is computed. Omit for none. */
  title?: string;
}

/** KpiTile is one headline dashboard stat: a big value, an optional
 * previous-period delta (colored by whether the change is favorable, not
 * just by sign), and a small trend sparkline. */
export function KpiTile({ label, value, spark, sparkColor, delta, favorable, subtext, title }: KpiTileProps) {
  const showDelta = delta !== undefined && Number.isFinite(delta) && Math.abs(delta) > 0.0005;
  const isUp = (delta ?? 0) >= 0;
  const isGood = isUp === favorable;
  const deltaColor = isGood ? 'text-ok' : 'text-bad';
  const pct = showDelta ? Math.abs((delta as number) * 100) : 0;
  const pctLabel = pct >= 10 ? Math.round(pct) : pct.toFixed(1);

  return (
    <Card title={title}>
      <CardContent className="p-4">
        <p className="text-sm text-muted">{label}</p>
        <div className="mt-1 flex items-baseline justify-between gap-2">
          <p className="text-2xl font-semibold tabular-nums text-fg">{value}</p>
          {showDelta && (
            <span
              className={`flex items-center gap-0.5 text-xs font-medium tabular-nums ${deltaColor}`}
              aria-label={`${isUp ? 'Up' : 'Down'} ${pctLabel}% from previous period`}
            >
              {isUp ? <TrendingUp className="h-3 w-3" aria-hidden="true" />
                : <TrendingDown className="h-3 w-3" aria-hidden="true" />}
              {pctLabel}%
            </span>
          )}
        </div>
        {subtext && <p className="mt-0.5 text-xs text-muted">{subtext}</p>}
        {spark && spark.length > 1 && (
          <div className="mt-2">
            <MiniSparkline samples={spark} color={sparkColor} height={28} />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
