import { Input } from '@/components/ui/input';
import { fieldClass, inputClass, labelClass } from './styles';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

export function GeneralSection() {
  const { general, setGeneral, saving, error, saved, save } = useSettingsSection('general');

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
        <Input id="general-timezone" value={general.timezone}
          onChange={(e) => setGeneral({ ...general, timezone: e.target.value })} />
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
    </Section>
  );
}
