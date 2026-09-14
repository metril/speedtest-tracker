/** SERIES is the one chart palette, defined as CSS variables in index.css
 * so light and dark swap together and every chart reuses the same hues. */
export const SERIES = {
  download: 'var(--color-series-download)',
  upload: 'var(--color-series-upload)',
  ping: 'var(--color-series-ping)',
  jitter: 'var(--color-series-jitter)',
} as const;

const ONE_DAY_MS = 24 * 60 * 60 * 1000;

/** formatAxisTick renders a bucket_start ISO timestamp for a chart's
 * x-axis. A 24h chart's ticks are all the same day, so a clock time
 * (HH:mm) is what distinguishes them; a 7d/30d chart's ticks span many
 * days at coarse (hour+) resolution, so the calendar date (MMM d) is what
 * distinguishes them instead. spanMs is the plotted range's total width
 * (last bucket minus first), which is what decides which case applies,
 * not any particular bucket's own width. */
export function formatAxisTick(iso: string, spanMs: number): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return spanMs <= ONE_DAY_MS
    ? d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false })
    : d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

/** chartSummary is what a screen reader hears instead of the SVG: the
 * shape of a series reduced to its min, max and latest value. */
export function chartSummary(label: string, values: number[], unit: (n: number) => string): string {
  const clean = values.filter((v) => Number.isFinite(v) && v > 0);
  if (clean.length === 0) return `${label}: no data`;
  const min = Math.min(...clean);
  const max = Math.max(...clean);
  return `${label}: min ${unit(min)}, max ${unit(max)}, latest ${unit(clean[clean.length - 1])}`;
}
