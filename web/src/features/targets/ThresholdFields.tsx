import { useState } from 'react';
import type { ThresholdSet } from '../../lib/api';

interface Props {
  value: ThresholdSet;
  onChange: (next: ThresholdSet) => void;
}

const field = 'w-full rounded border border-line bg-surface px-2 py-1 text-sm text-fg focus:border-accent focus:outline-none';
const label = 'block text-xs font-medium uppercase tracking-wide text-muted';

type NumericKey = 'download_mbps_min' | 'upload_mbps_min' | 'ping_ms_max' | 'jitter_ms_max' | 'loss_pct_max';

const NUMERIC_FIELDS: { key: NumericKey; id: string; text: string }[] = [
  { key: 'download_mbps_min', id: 'threshold-download-min', text: 'Min download (Mbps)' },
  { key: 'upload_mbps_min', id: 'threshold-upload-min', text: 'Min upload (Mbps)' },
  { key: 'ping_ms_max', id: 'threshold-ping-max', text: 'Max ping (ms)' },
  { key: 'jitter_ms_max', id: 'threshold-jitter-max', text: 'Max jitter (ms)' },
  { key: 'loss_pct_max', id: 'threshold-loss-max', text: 'Max packet loss (%)' },
];

/** validateThresholds enforces the same rules as the server (Task 6):
 * every value is zero or more, and packet loss tops out at 100%. */
export function validateThresholds(t: ThresholdSet): string | undefined {
  const numeric = [t.download_mbps_min, t.upload_mbps_min, t.ping_ms_max, t.jitter_ms_max, t.loss_pct_max];
  if (numeric.some((v) => typeof v === 'number' && v < 0)) return 'Thresholds must be zero or more';
  if (typeof t.loss_pct_max === 'number' && t.loss_pct_max > 100) return 'Packet loss must be 0-100';
  return undefined;
}

function toRaw(v: number | null | undefined): string {
  return v === undefined || v === null ? '' : String(v);
}

/** ThresholdFields edits one target's per-target notification thresholds.
 * A blank field means "inherit the global default from Settings ->
 * Notifications" and is omitted from the value entirely. */
export function ThresholdFields({ value, onChange }: Props) {
  const [raw, setRaw] = useState<Record<NumericKey, string>>(() => ({
    download_mbps_min: toRaw(value.download_mbps_min),
    upload_mbps_min: toRaw(value.upload_mbps_min),
    ping_ms_max: toRaw(value.ping_ms_max),
    jitter_ms_max: toRaw(value.jitter_ms_max),
    loss_pct_max: toRaw(value.loss_pct_max),
  }));

  function setNumeric(key: NumericKey, text: string) {
    setRaw((r) => ({ ...r, [key]: text }));
    const next: ThresholdSet = { ...value };
    if (text.trim() === '') {
      delete next[key];
    } else {
      const n = Number(text);
      if (Number.isFinite(n)) next[key] = n; else delete next[key];
    }
    onChange(next);
  }

  return (
    <div className="grid gap-3">
      <p className="text-xs text-faint">
        Blank fields use the global defaults from Settings &rarr; Notifications.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        {NUMERIC_FIELDS.map(({ key, id, text }) => (
          <div key={key}>
            <label className={label} htmlFor={id}>{text}</label>
            <input
              id={id}
              type="number"
              className={field}
              value={raw[key]}
              onChange={(e) => setNumeric(key, e.target.value)}
            />
          </div>
        ))}
      </div>
      <label className="flex w-fit items-center gap-2 text-sm text-muted" htmlFor="threshold-notify-on-failure">
        <input
          id="threshold-notify-on-failure"
          type="checkbox"
          checked={value.notify_on_failure === true}
          onChange={(e) => {
            const next: ThresholdSet = { ...value };
            if (e.target.checked) next.notify_on_failure = true;
            else delete next.notify_on_failure;
            onChange(next);
          }}
        />
        Notify on failed test
      </label>
    </div>
  );
}
