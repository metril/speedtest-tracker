import { useState } from 'react';
import { inputClass } from '@/features/settings/styles';
import { FormField } from '../../components/FormField';
import { SwitchField } from '../../components/SwitchField';
import type { ThresholdSet } from '../../lib/api';

interface Props {
  value: ThresholdSet;
  onChange: (next: ThresholdSet) => void;
  /** allowDisable hides the Off mode for the five numeric fields. The
   * global default editor (Settings -> Notifications) passes false: "off"
   * only means something on a target, where it can suppress a metric that
   * still has a global default. */
  allowDisable?: boolean;
  /** showCustomToggle hides the "Custom notification" switch and behaves
   * as if it were always on. The redesigned settings layout embeds
   * ThresholdFields directly inside its own card/toggle chrome, so it
   * doesn't need this component's own toggle affordance. */
  showCustomToggle?: boolean;
}

const field = 'w-full rounded border border-line bg-surface px-2 py-1 text-sm text-fg focus:border-accent focus:outline-none';
const label = 'block text-xs font-medium uppercase tracking-wide text-muted';

type NumericKey =
  | 'download_mbps_min' | 'upload_mbps_min' | 'ping_ms_max' | 'jitter_ms_max' | 'loss_pct_max'
  | 'sla_download_mbps' | 'sla_upload_mbps';

type Mode = 'inherit' | 'off' | 'custom';

const NUMERIC_FIELDS: { key: NumericKey; id: string; text: string }[] = [
  { key: 'download_mbps_min', id: 'threshold-download-min', text: 'Min download (Mbps)' },
  { key: 'upload_mbps_min', id: 'threshold-upload-min', text: 'Min upload (Mbps)' },
  { key: 'ping_ms_max', id: 'threshold-ping-max', text: 'Max ping (ms)' },
  { key: 'jitter_ms_max', id: 'threshold-jitter-max', text: 'Max jitter (ms)' },
  { key: 'loss_pct_max', id: 'threshold-loss-max', text: 'Max packet loss (%)' },
];

const SLA_FIELDS: { key: NumericKey; id: string; text: string }[] = [
  { key: 'sla_download_mbps', id: 'threshold-sla-download', text: 'SLA plan download override (Mbps)' },
  { key: 'sla_upload_mbps', id: 'threshold-sla-upload', text: 'SLA plan upload override (Mbps)' },
];

const BLANK: Record<NumericKey, string> = {
  download_mbps_min: '',
  upload_mbps_min: '',
  ping_ms_max: '',
  jitter_ms_max: '',
  loss_pct_max: '',
  sla_download_mbps: '',
  sla_upload_mbps: '',
};

const INHERIT_MODES: Record<NumericKey, Mode> = {
  download_mbps_min: 'inherit',
  upload_mbps_min: 'inherit',
  ping_ms_max: 'inherit',
  jitter_ms_max: 'inherit',
  loss_pct_max: 'inherit',
  sla_download_mbps: 'inherit',
  sla_upload_mbps: 'inherit',
};

/** validateThresholds enforces the same rules as the server (Task 6):
 * every value is zero or more, and packet loss tops out at 100%. null
 * (disabled) values are ignored, same as absent ones. */
export function validateThresholds(t: ThresholdSet): string | undefined {
  const numeric = [
    t.download_mbps_min, t.upload_mbps_min, t.ping_ms_max, t.jitter_ms_max, t.loss_pct_max,
    t.sla_download_mbps, t.sla_upload_mbps,
  ];
  if (numeric.some((v) => typeof v === 'number' && v < 0)) return 'Thresholds must be zero or more';
  if (typeof t.loss_pct_max === 'number' && t.loss_pct_max > 100) return 'Packet loss must be 0-100';
  return undefined;
}

function toRaw(v: number | null | undefined): string {
  return v === undefined || v === null ? '' : String(v);
}

/** modeOf derives a field's UI mode from its wire value: undefined
 * (inherit the global default), null (disabled for this target), or a
 * number (custom override). */
function modeOf(v: number | null | undefined): Mode {
  if (v === null) return 'off';
  if (v === undefined) return 'inherit';
  return 'custom';
}

/** ThresholdFields edits one target's per-target notification thresholds.
 * A blank field means "inherit the global default from Settings ->
 * Notifications" and is omitted from the value entirely. Thresholds only
 * apply when "Custom notification" is on; off, the target follows the
 * global defaults untouched (an empty {} is equivalent to "off" on the
 * wire, but the toggle is purely a UI affordance for clearing overrides). */
export function ThresholdFields({ value, onChange, allowDisable = true, showCustomToggle = true }: Props) {
  const [custom, setCustom] = useState(() => Object.keys(value).length > 0);
  const effectiveCustom = showCustomToggle ? custom : true;
  const [raw, setRaw] = useState<Record<NumericKey, string>>(() => ({
    ...BLANK,
    download_mbps_min: toRaw(value.download_mbps_min),
    upload_mbps_min: toRaw(value.upload_mbps_min),
    ping_ms_max: toRaw(value.ping_ms_max),
    jitter_ms_max: toRaw(value.jitter_ms_max),
    loss_pct_max: toRaw(value.loss_pct_max),
    sla_download_mbps: toRaw(value.sla_download_mbps),
    sla_upload_mbps: toRaw(value.sla_upload_mbps),
  }));
  const [modes, setModes] = useState<Record<NumericKey, Mode>>(() => ({
    ...INHERIT_MODES,
    download_mbps_min: modeOf(value.download_mbps_min),
    upload_mbps_min: modeOf(value.upload_mbps_min),
    ping_ms_max: modeOf(value.ping_ms_max),
    jitter_ms_max: modeOf(value.jitter_ms_max),
    loss_pct_max: modeOf(value.loss_pct_max),
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

  function setMode(key: NumericKey, mode: Mode) {
    setModes((m) => ({ ...m, [key]: mode }));
    const next: ThresholdSet = { ...value };
    if (mode === 'inherit') {
      delete next[key];
      setRaw((r) => ({ ...r, [key]: '' }));
    } else if (mode === 'off') {
      next[key] = null;
      setRaw((r) => ({ ...r, [key]: '' }));
    } else {
      const text = raw[key];
      if (text.trim() === '') {
        delete next[key];
      } else {
        const n = Number(text);
        if (Number.isFinite(n)) next[key] = n; else delete next[key];
      }
    }
    onChange(next);
  }

  function toggleCustom(checked: boolean) {
    setCustom(checked);
    setRaw(BLANK);
    setModes(INHERIT_MODES);
    onChange({});
  }

  return (
    <div className="grid gap-3">
      {showCustomToggle && (
        <SwitchField
          id="threshold-custom" label="Custom notification" checked={custom} onCheckedChange={toggleCustom}
          hint="Off: this target uses the global notification defaults from Settings → Notifications."
        />
      )}

      {effectiveCustom && (
        <>
          {Object.keys(value).length === 0 && (
            <p className="text-xs text-faint">
              Nothing overridden yet — this target still follows the global defaults.
            </p>
          )}
          <p className="text-xs text-faint">
            {allowDisable
              ? 'Inherit uses the global defaults from Settings → Notifications; Off disables the check for this target.'
              : 'Unset means no default for that metric.'}
          </p>
          <div className="grid gap-3 sm:grid-cols-2">
            {NUMERIC_FIELDS.map(({ key, id, text }) => {
              const mode = modes[key];
              return (
                <div key={key} className="grid gap-1">
                  <FormField id={`${id}-mode`} label={`${text} mode`}>
                    <select
                      id={`${id}-mode`}
                      className={inputClass}
                      value={mode}
                      onChange={(e) => setMode(key, e.target.value as Mode)}
                    >
                      <option value="inherit">{allowDisable ? 'Inherit' : 'Unset'}</option>
                      {allowDisable && <option value="off">Off</option>}
                      <option value="custom">Custom</option>
                    </select>
                  </FormField>
                  {mode === 'custom' && (
                    <div>
                      <label className={label} htmlFor={id}>{text}</label>
                      <input
                        id={id}
                        type="number"
                        className={field}
                        value={raw[key]}
                        onChange={(e) => setNumeric(key, e.target.value)}
                      />
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <div className="grid gap-3 border-t border-line pt-3">
            <p className="text-xs text-faint">
              Blank fields inherit the plan speeds from Settings &rarr; General.
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              {SLA_FIELDS.map(({ key, id, text }) => (
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
          </div>

          <FormField id="threshold-notify-on-failure" label="Notify on failed test">
            <select
              id="threshold-notify-on-failure"
              className={inputClass}
              value={value.notify_on_failure === undefined ? 'inherit' : String(value.notify_on_failure)}
              onChange={(e) => {
                const next: ThresholdSet = { ...value };
                if (e.target.value === 'inherit') delete next.notify_on_failure;
                else next.notify_on_failure = e.target.value === 'true';
                onChange(next);
              }}
            >
              <option value="inherit">Inherit</option>
              <option value="true">On</option>
              <option value="false">Off</option>
            </select>
          </FormField>
        </>
      )}
    </div>
  );
}
