/** Sweep of the dial, in degrees: 240° centred on straight up. */
const START_DEG = -210;
const END_DEG = 30;
const DEFAULT_MAX_MBPS = 1000;

/** The phases a run can report progress for. */
export type GaugePhase = 'connecting' | 'ping' | 'download' | 'upload' | 'done' | 'error';

/** Per-phase accent, as CSS custom properties so the dial stays readable
 * when the theme's palette swaps between light and dark. The dial is the
 * one loud element in the live panel, so the phase is carried by colour
 * rather than another label. */
export const PHASE_STROKE: Record<GaugePhase, string> = {
  connecting: 'var(--color-faint)',
  ping: 'var(--color-series-ping)',
  download: 'var(--color-series-download)',
  upload: 'var(--color-series-upload)',
  done: 'var(--color-ok)',
  error: 'var(--color-bad)',
};

/**
 * gaugeFraction maps bits per second onto 0..1 on a log scale, so a 20 Mbps
 * DSL line and a 900 Mbps fibre line both use most of the dial instead of
 * the slow one hugging zero.
 */
export function gaugeFraction(bps: number, maxMbps = DEFAULT_MAX_MBPS): number {
  if (!Number.isFinite(bps) || bps <= 0) return 0;
  const mbps = bps / 1_000_000;
  const f = Math.log10(1 + mbps) / Math.log10(1 + maxMbps);
  return Math.min(Math.max(f, 0), 1);
}

/** polar converts a dial angle to SVG coordinates. */
function polar(cx: number, cy: number, r: number, deg: number): [number, number] {
  const rad = (deg * Math.PI) / 180;
  return [cx + r * Math.cos(rad), cy + r * Math.sin(rad)];
}

/** arcPath renders one circular arc as an SVG path. */
export function arcPath(cx: number, cy: number, r: number, startDeg: number, endDeg: number): string {
  const [x1, y1] = polar(cx, cy, r, startDeg);
  const [x2, y2] = polar(cx, cy, r, endDeg);
  const large = Math.abs(endDeg - startDeg) > 180 ? 1 : 0;
  const sweep = endDeg > startDeg ? 1 : 0;
  return `M ${x1.toFixed(2)} ${y1.toFixed(2)} A ${r} ${r} 0 ${large} ${sweep} ${x2.toFixed(2)} ${y2.toFixed(2)}`;
}

interface GaugeProps {
  bps: number;
  phase: GaugePhase;
  maxMbps?: number;
}

/** Gauge is the live panel's speed dial: a 240° arc with the current
 * throughput in tabular figures at its centre. */
export function Gauge({ bps, phase, maxMbps = DEFAULT_MAX_MBPS }: GaugeProps) {
  const fraction = gaugeFraction(bps, maxMbps);
  const mbps = Number.isFinite(bps) && bps > 0 ? bps / 1_000_000 : 0;
  const stroke = PHASE_STROKE[phase] ?? PHASE_STROKE.connecting;
  const track = arcPath(110, 110, 88, START_DEG, END_DEG);
  const valueEnd = START_DEG + (END_DEG - START_DEG) * fraction;
  const value = fraction > 0 ? arcPath(110, 110, 88, START_DEG, valueEnd) : '';

  return (
    <div className="relative w-[220px]">
      <svg
        viewBox="0 0 220 200"
        className="w-full"
        role="meter"
        aria-valuemin={0}
        aria-valuemax={maxMbps}
        aria-valuenow={Math.round(mbps)}
        aria-valuetext={`${mbps.toFixed(1)} Mbps`}
        aria-label="current throughput"
      >
        <path d={track} fill="none" stroke="var(--color-line)" strokeWidth={14} strokeLinecap="round" />
        {value && (
          <path
            d={value}
            fill="none"
            stroke={stroke}
            strokeWidth={14}
            strokeLinecap="round"
            className="transition-[d] duration-150 motion-reduce:transition-none"
          />
        )}
      </svg>
      <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center pt-4">
        <span className="font-mono text-5xl tabular-nums tracking-tight text-fg">
          {mbps.toFixed(1)}
        </span>
        <span className="text-xs uppercase tracking-[0.2em] text-faint">Mbps</span>
        <span className="mt-2 text-sm capitalize" style={{ color: stroke }}>
          {phase}
        </span>
      </div>
    </div>
  );
}
