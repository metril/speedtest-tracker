import {
  CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis,
} from 'recharts';
import { chartSummary, formatAxisTick } from '../../lib/chart';

/** Row is what HistoryChart plots: a bucket_start plus arbitrary numeric
 * series columns. Generic over the exact row shape so both single-target
 * HistoryPoint rows and Dashboard's merged `<key>_<targetId>` rows work
 * without a cast. */
export type Row = Record<string, number | string>;

export interface Series<R extends Row = Row> {
  key: Extract<keyof R, string>;
  label: string;
  color: string;
  unit: (n: number) => string;
}

/** HistoryChart plots pre-downsampled buckets. The server already limited
 * the point count, so this draws exactly what it is given: no client-side
 * aggregation, no dots per point, one gridline axis. */
export function HistoryChart<R extends Row>({ title, points, series, height = 240 }: {
  title: string; points: R[]; series: Series<R>[]; height?: number;
}) {
  if (points.length === 0) {
    return (
      <figure>
        <figcaption className="text-sm font-medium text-fg">{title}</figcaption>
        <p className="py-8 text-center text-sm text-muted">No data in this range.</p>
      </figure>
    );
  }
  const label = `${title}. ${series
    .map((s) => chartSummary(s.label, points.map((p) => Number(p[s.key])), s.unit))
    .join('. ')}.`;
  const first = Date.parse(String(points[0].bucket_start));
  const last = Date.parse(String(points[points.length - 1].bucket_start));
  const spanMs = Number.isNaN(first) || Number.isNaN(last) ? 0 : Math.abs(last - first);
  const tickTime = (iso: string) => formatAxisTick(iso, spanMs);
  const tooltipTime = (iso: string) =>
    new Date(iso).toLocaleString(undefined, { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' });

  return (
    <figure>
      <figcaption className="mb-2 text-sm font-medium text-fg">{title}</figcaption>
      <div role="img" aria-label={label} style={{ height }}>
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={points} margin={{ top: 4, right: 8, bottom: 4, left: 8 }}>
            <CartesianGrid stroke="var(--color-line)" vertical={false} />
            <XAxis dataKey="bucket_start" tickFormatter={tickTime} minTickGap={48}
              stroke="var(--color-faint)" tick={{ fontSize: 11 }} />
            <YAxis tickFormatter={(v) => series[0].unit(Number(v))} width={78}
              stroke="var(--color-faint)" tick={{ fontSize: 11 }} />
            <Tooltip
              contentStyle={{ background: 'var(--color-surface)', border: '1px solid var(--color-line)',
                color: 'var(--color-fg)', fontSize: 12 }}
              labelFormatter={tooltipTime}
              formatter={(value, name) => {
                const s = series.find((x) => x.label === name);
                return [s ? s.unit(Number(value)) : String(value), name];
              }} />
            <Legend wrapperStyle={{ fontSize: 12 }} />
            {series.map((s) => (
              <Line key={String(s.key)} type="monotone" dataKey={String(s.key)} name={s.label}
                stroke={s.color} strokeWidth={2} dot={false} isAnimationActive={false} connectNulls />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </figure>
  );
}
