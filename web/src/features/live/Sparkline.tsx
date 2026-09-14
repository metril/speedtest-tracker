/**
 * sparkPath maps samples onto a polyline filling width × height, newest
 * sample at the right. A flat series sits on the baseline rather than
 * dividing by a zero range.
 */
export function sparkPath(samples: number[], width: number, height: number): string {
  if (samples.length < 2) return '';
  const max = Math.max(...samples);
  const min = Math.min(...samples);
  const range = max - min;
  const stepX = width / (samples.length - 1);
  return samples
    .map((v, i) => {
      const x = Math.round(i * stepX * 100) / 100;
      const y = range === 0 ? height : Math.round((height - ((v - min) / range) * height) * 100) / 100;
      return `${i === 0 ? 'M' : 'L'} ${x} ${y}`;
    })
    .join(' ');
}

interface SparklineProps {
  samples: number[];
  width?: number;
  height?: number;
}

/** Sparkline draws the last ~60 instantaneous throughput samples. It is
 * decoration for the gauge, so it is hidden from assistive tech. */
export function Sparkline({ samples, width = 320, height = 48 }: SparklineProps) {
  const d = sparkPath(samples, width, height);
  if (!d) return null;
  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      className="w-full"
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
    >
      <path d={d} fill="none" stroke="var(--color-series-download)" strokeWidth={1.5} strokeLinejoin="round" />
    </svg>
  );
}
