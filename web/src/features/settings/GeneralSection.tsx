import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { TimezoneSelect } from '../../components/TimezoneSelect';
import { inputClass } from './styles';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

type SLAKey = 'sla_download_mbps' | 'sla_upload_mbps';

function toRaw(v: number | undefined): string {
  return v === undefined ? '' : String(v);
}

export function GeneralSection() {
  const { general, setGeneral, saving, error, saved, save, readOnly } = useSettingsSection('general');

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
      error={error} saved={saved} readOnly={readOnly}
      onSave={() => save('general', { general })}
    >
      <FormField id="general-base-url" label="Base URL">
        <Input id="general-base-url" value={general.base_url}
          onChange={(e) => setGeneral({ ...general, base_url: e.target.value })} />
      </FormField>
      <FormField id="general-timezone" label="Timezone">
        <TimezoneSelect id="general-timezone" value={general.timezone}
          onChange={(tz) => setGeneral({ ...general, timezone: tz })} />
      </FormField>
      <FormField id="general-units" label="Units">
        <select id="general-units" className={inputClass} value={general.units}
          onChange={(e) => setGeneral({ ...general, units: e.target.value })}>
          <option value="Mbps">Mbps</option>
          <option value="MB/s">MB/s</option>
        </select>
      </FormField>
      <FormField id="general-log-level" label="Log level">
        <select id="general-log-level" className={inputClass} value={general.log_level}
          onChange={(e) => setGeneral({ ...general, log_level: e.target.value })}>
          <option value="debug">debug</option>
          <option value="info">info</option>
          <option value="warn">warn</option>
          <option value="error">error</option>
        </select>
      </FormField>
      <FormField id="general-retention-results" label="Results retention (days)">
        <Input id="general-retention-results" type="number" min={1}
          value={general.retention_days_results}
          onChange={(e) => setGeneral({ ...general, retention_days_results: Number(e.target.value) })} />
      </FormField>
      <FormField id="general-retention-runs" label="Runs retention (days)">
        <Input id="general-retention-runs" type="number" min={1}
          value={general.retention_days_runs}
          onChange={(e) => setGeneral({ ...general, retention_days_runs: Number(e.target.value) })} />
      </FormField>
      <FormField id="general-prune-interval" label="Prune interval (minutes)">
        <Input id="general-prune-interval" type="number" min={1}
          value={general.retention_prune_interval_minutes}
          onChange={(e) => setGeneral({ ...general, retention_prune_interval_minutes: Number(e.target.value) })} />
      </FormField>

      <div className="col-span-full grid gap-3 border-t border-line pt-4">
        <div>
          <h3 className="text-sm font-semibold text-fg">Plan speeds (SLA)</h3>
          <p className="text-xs text-faint">
            Your ISP-advertised plan speeds, used to track compliance on the dashboard.
            Leave blank (or 0) to disable.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <FormField id="general-sla-download" label="Plan download (Mbps)">
            <div className="flex gap-2">
              <Input id="general-sla-download" type="number" min={0}
                value={raw.sla_download_mbps}
                onChange={(e) => setSla('sla_download_mbps', e.target.value)} />
              <Button type="button" variant="outline" size="sm"
                onClick={() => clearSla('sla_download_mbps')}>
                Clear
              </Button>
            </div>
          </FormField>
          <FormField id="general-sla-upload" label="Plan upload (Mbps)">
            <div className="flex gap-2">
              <Input id="general-sla-upload" type="number" min={0}
                value={raw.sla_upload_mbps}
                onChange={(e) => setSla('sla_upload_mbps', e.target.value)} />
              <Button type="button" variant="outline" size="sm"
                onClick={() => clearSla('sla_upload_mbps')}>
                Clear
              </Button>
            </div>
          </FormField>
        </div>
      </div>
    </Section>
  );
}
