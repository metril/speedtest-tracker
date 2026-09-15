import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { SettingsCard, SettingsRow, useSettingsRowField } from '@/components/settings';
import { TimezoneSelect } from '../../components/TimezoneSelect';
import { inputClass } from './styles';
import { useSettingsSection } from './useSettingsSection';

type SLAKey = 'sla_download_mbps' | 'sla_upload_mbps';

function toRaw(v: number | undefined): string {
  return v === undefined ? '' : String(v);
}

function BaseUrlField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} value={value} onChange={(e) => onChange(e.target.value)} />;
}

function TimezoneField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return <TimezoneSelect {...props} value={value} onChange={onChange} />;
}

function UnitsField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return (
    <select {...props} className={inputClass} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="Mbps">Mbps</option>
      <option value="MB/s">MB/s</option>
    </select>
  );
}

function LogLevelField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return (
    <select {...props} className={inputClass} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="debug">debug</option>
      <option value="info">info</option>
      <option value="warn">warn</option>
      <option value="error">error</option>
    </select>
  );
}

function NumberField({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} type="number" min={1} value={value} onChange={(e) => onChange(Number(e.target.value))} />;
}

function SlaField({ value, onChange, onClear }: { value: string; onChange: (v: string) => void; onClear: () => void }) {
  const props = useSettingsRowField();
  return (
    <div className="flex gap-2 min-w-0">
      <Input {...props} type="number" min={0} value={value} onChange={(e) => onChange(e.target.value)} />
      <Button type="button" variant="outline" size="sm" onClick={onClear}>
        Clear
      </Button>
    </div>
  );
}

export function GeneralSection() {
  const { general, setGeneral } = useSettingsSection('general');

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
    <div className="grid gap-4">
      <SettingsCard title="Site">
        <SettingsRow
          label="Base URL" description="Used in notification links" htmlFor="general-base-url"
          lockKey="general.base_url"
        >
          <BaseUrlField value={general.base_url} onChange={(v) => setGeneral({ ...general, base_url: v })} />
        </SettingsRow>
        <SettingsRow label="Timezone" htmlFor="general-timezone" lockKey="general.timezone">
          <TimezoneField value={general.timezone} onChange={(v) => setGeneral({ ...general, timezone: v })} />
        </SettingsRow>
        <SettingsRow label="Units" htmlFor="general-units" lockKey="general.units" size="sm">
          <UnitsField value={general.units} onChange={(v) => setGeneral({ ...general, units: v })} />
        </SettingsRow>
      </SettingsCard>

      <SettingsCard title="Data retention">
        <SettingsRow
          label="Results retention (days)" description="How long individual test results are kept"
          htmlFor="general-retention-results" lockKey="general.retention_days_results" size="sm"
        >
          <NumberField
            value={general.retention_days_results}
            onChange={(v) => setGeneral({ ...general, retention_days_results: v })}
          />
        </SettingsRow>
        <SettingsRow
          label="Runs retention (days)" description="How long scheduled run history is kept"
          htmlFor="general-retention-runs" lockKey="general.retention_days_runs" size="sm"
        >
          <NumberField
            value={general.retention_days_runs}
            onChange={(v) => setGeneral({ ...general, retention_days_runs: v })}
          />
        </SettingsRow>
        <SettingsRow
          label="Prune interval (minutes)" description="How often expired data is cleaned up"
          htmlFor="general-prune-interval" lockKey="general.retention_prune_interval_minutes" size="sm"
        >
          <NumberField
            value={general.retention_prune_interval_minutes}
            onChange={(v) => setGeneral({ ...general, retention_prune_interval_minutes: v })}
          />
        </SettingsRow>
      </SettingsCard>

      <SettingsCard
        title="Plan speeds (SLA)"
        description="Leave blank to disable. Targets can override."
      >
        <SettingsRow
          label="Plan download (Mbps)" htmlFor="general-sla-download" lockKey="general.sla_download_mbps"
        >
          <SlaField
            value={raw.sla_download_mbps}
            onChange={(v) => setSla('sla_download_mbps', v)}
            onClear={() => clearSla('sla_download_mbps')}
          />
        </SettingsRow>
        <SettingsRow
          label="Plan upload (Mbps)" htmlFor="general-sla-upload" lockKey="general.sla_upload_mbps"
        >
          <SlaField
            value={raw.sla_upload_mbps}
            onChange={(v) => setSla('sla_upload_mbps', v)}
            onClear={() => clearSla('sla_upload_mbps')}
          />
        </SettingsRow>
      </SettingsCard>

      <SettingsCard title="Diagnostics">
        <SettingsRow label="Log level" htmlFor="general-log-level" lockKey="general.log_level" size="sm">
          <LogLevelField value={general.log_level} onChange={(v) => setGeneral({ ...general, log_level: v })} />
        </SettingsRow>
      </SettingsCard>
    </div>
  );
}
