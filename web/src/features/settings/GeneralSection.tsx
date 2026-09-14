import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { TimezoneSelect } from '../../components/TimezoneSelect';
import { fieldClass, inputClass, labelClass } from './styles';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

type SLAKey = 'sla_download_mbps' | 'sla_upload_mbps';

function toRaw(v: number | undefined): string {
  return v === undefined ? '' : String(v);
}

export function GeneralSection() {
  const { general, setGeneral, saving, error, saved, save } = useSettingsSection('general');

  // Local raw-string mirror of the two SLA fields, same pattern as
  // ThresholdFields: `general.sla_*_mbps` alone can't represent "the user
  // is mid-way through clearing this field" (it's a plain number), so a
  // blank input would otherwise snap back to showing "0" the instant it's
  // emptied. Seeded once; blank means 0 ("no plan") on submit.
  const [raw, setRaw] = useState<Record<SLAKey, string>>(() => ({
    sla_download_mbps: toRaw(general.sla_download_mbps),
    sla_upload_mbps: toRaw(general.sla_upload_mbps),
  }));

  function setSla(key: SLAKey, text: string) {
    setRaw((r) => ({ ...r, [key]: text }));
    const n = text.trim() === '' ? 0 : Number(text);
    if (Number.isFinite(n)) setGeneral({ ...general, [key]: n });
  }

  function clearSla(key: SLAKey) {
    setRaw((r) => ({ ...r, [key]: '' }));
    setGeneral({ ...general, [key]: 0 });
  }

  return (
    <Section
      id="general-heading" title="General" saving={saving}
      error={error} saved={saved}
      onSave={() => save('general', { general })}
    >
      <div className={fieldClass}>
        <label htmlFor="general-base-url" className={labelClass}>Base URL</label>
        <Input id="general-base-url" value={general.base_url}
          onChange={(e) => setGeneral({ ...general, base_url: e.target.value })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="general-timezone" className={labelClass}>Timezone</label>
        <TimezoneSelect id="general-timezone" value={general.timezone}
          onChange={(tz) => setGeneral({ ...general, timezone: tz })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="general-units" className={labelClass}>Units</label>
        <select id="general-units" className={inputClass} value={general.units}
          onChange={(e) => setGeneral({ ...general, units: e.target.value })}>
          <option value="Mbps">Mbps</option>
          <option value="MB/s">MB/s</option>
        </select>
      </div>
      <div className={fieldClass}>
        <label htmlFor="general-log-level" className={labelClass}>Log level</label>
        <select id="general-log-level" className={inputClass} value={general.log_level}
          onChange={(e) => setGeneral({ ...general, log_level: e.target.value })}>
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </select>
      </div>
      <div className={fieldClass}>
        <label htmlFor="general-retention-results" className={labelClass}>Results retention (days)</label>
        <Input id="general-retention-results" type="number" min={1}
          value={general.retention_days_results}
          onChange={(e) => setGeneral({ ...general, retention_days_results: Number(e.target.value) })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="general-retention-runs" className={labelClass}>Runs retention (days)</label>
        <Input id="general-retention-runs" type="number" min={1}
          value={general.retention_days_runs}
          onChange={(e) => setGeneral({ ...general, retention_days_runs: Number(e.target.value) })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="general-prune-interval" className={labelClass}>Prune interval (minutes)</label>
        <Input id="general-prune-interval" type="number" min={1}
          value={general.retention_prune_interval_minutes}
          onChange={(e) => setGeneral({ ...general, retention_prune_interval_minutes: Number(e.target.value) })} />
      </div>

      <div className="col-span-full grid gap-3 border-t border-line pt-4">
        <div>
          <h3 className="text-sm font-semibold text-fg">Plan speeds (SLA)</h3>
          <p className="text-xs text-faint">
            Your ISP-advertised plan speeds, used to track compliance on the dashboard.
            Leave blank (or 0) to disable.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className={fieldClass}>
            <label htmlFor="general-sla-download" className={labelClass}>Plan download (Mbps)</label>
            <div className="flex gap-2">
              <Input id="general-sla-download" type="number" min={0}
                value={raw.sla_download_mbps}
                onChange={(e) => setSla('sla_download_mbps', e.target.value)} />
              <Button type="button" variant="outline" size="sm"
                onClick={() => clearSla('sla_download_mbps')}>
                Clear
              </Button>
            </div>
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-sla-upload" className={labelClass}>Plan upload (Mbps)</label>
            <div className="flex gap-2">
              <Input id="general-sla-upload" type="number" min={0}
                value={raw.sla_upload_mbps}
                onChange={(e) => setSla('sla_upload_mbps', e.target.value)} />
              <Button type="button" variant="outline" size="sm"
                onClick={() => clearSla('sla_upload_mbps')}>
                Clear
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Section>
  );
}
