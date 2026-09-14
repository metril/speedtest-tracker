import { Input } from './ui/input';
import { inputClass } from '../features/settings/styles';

/** A curated shortlist of common zones, shown before the full IANA list so
 * the dropdown isn't 400 entries deep by default. */
export const CURATED_TIMEZONES = [
  'UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'Europe/London', 'Europe/Berlin', 'Europe/Zurich', 'Asia/Kolkata', 'Asia/Singapore',
  'Asia/Tokyo', 'Australia/Sydney',
];

/** Not all supported JS environments have `Intl.supportedValuesOf` yet
 * (added later than the rest of Intl), so guard the typing locally
 * instead of widening the global Intl lib type. */
type IntlWithSupportedValuesOf = typeof Intl & {
  supportedValuesOf?: (key: 'timeZone') => string[];
};

function allTimezones(): string[] | undefined {
  const fn = (Intl as IntlWithSupportedValuesOf).supportedValuesOf;
  return typeof fn === 'function' ? fn('timeZone') : undefined;
}

interface Props {
  id: string;
  value: string;
  onChange: (value: string) => void;
  className?: string;
}

/** A timezone picker shared by GeneralSection and ScheduleForm. Prefers a
 * full IANA list (grouped behind a curated shortlist) when the runtime
 * supports `Intl.supportedValuesOf`, and falls back to the curated list
 * plus a free-text input otherwise. */
export function TimezoneSelect({ id, value, onChange, className }: Props) {
  const all = allTimezones();
  const isCustomTimezone = value !== '' && !CURATED_TIMEZONES.includes(value)
    && !(all?.includes(value) ?? false);

  if (all) {
    return (
      <select id={id} className={className ?? inputClass} value={value}
        onChange={(e) => onChange(e.target.value)}>
        {isCustomTimezone && <option value={value}>{value}</option>}
        <optgroup label="Common">
          {CURATED_TIMEZONES.map((tz) => <option key={tz} value={tz}>{tz}</option>)}
        </optgroup>
        <optgroup label="All zones">
          {all.filter((tz) => !CURATED_TIMEZONES.includes(tz)).map((tz) => (
            <option key={tz} value={tz}>{tz}</option>
          ))}
        </optgroup>
      </select>
    );
  }

  return (
    <div className="grid gap-1">
      <select id={id} className={className ?? inputClass}
        value={isCustomTimezone ? '__custom__' : value}
        onChange={(e) => onChange(e.target.value)}>
        {CURATED_TIMEZONES.map((tz) => <option key={tz} value={tz}>{tz}</option>)}
        {isCustomTimezone && <option value="__custom__" disabled>Custom…</option>}
      </select>
      <Input
        aria-label="Custom timezone"
        placeholder="Or type an IANA zone, e.g. Europe/Zurich"
        value={isCustomTimezone ? value : ''}
        onChange={(e) => onChange(e.target.value)}
        className="text-xs"
      />
    </div>
  );
}
