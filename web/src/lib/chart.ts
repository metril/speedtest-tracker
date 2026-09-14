/** SERIES is the one chart palette, defined as CSS variables in index.css
 * so light and dark swap together and every chart reuses the same hues. */
export const SERIES = {
  download: 'var(--color-series-download)',
  upload: 'var(--color-series-upload)',
  ping: 'var(--color-series-ping)',
  jitter: 'var(--color-series-jitter)',
} as const;

/** chartSummary is what a screen reader hears instead of the SVG: the
 * shape of a series reduced to its min, max and latest value. */
export function chartSummary(label: string, values: number[], unit: (n: number) => string): string {
  const clean = values.filter((v) => Number.isFinite(v) && v > 0);
  if (clean.length === 0) return `${label}: no data`;
  const min = Math.min(...clean);
  const max = Math.max(...clean);
  return `${label}: min ${unit(min)}, max ${unit(max)}, latest ${unit(clean[clean.length - 1])}`;
}
