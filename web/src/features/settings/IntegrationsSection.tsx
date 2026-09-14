import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { LabelsEditor } from '../../components/LabelsEditor';
import { SwitchField } from '../../components/SwitchField';
import { fieldClass, labelClass } from './styles';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

export function IntegrationsSection() {
  const {
    integrations, setIntegrations, saving, error, saved, save, testPending, vmResult, vlResult, runTest,
  } = useSettingsSection('integrations');

  return (
    <Section
      id="integrations-heading" title="Integrations" saving={saving}
      error={error} saved={saved}
      onSave={() => save('integrations', { integrations })}
    >
      <SwitchField
        id="vm-enabled" label="Enable VictoriaMetrics"
        checked={integrations.vm_enabled}
        onCheckedChange={(checked) => setIntegrations({ ...integrations, vm_enabled: checked })}
      />
      <div className={fieldClass}>
        <label htmlFor="vm-url" className={labelClass}>VictoriaMetrics URL</label>
        <Input id="vm-url" value={integrations.vm_url}
          onChange={(e) => setIntegrations({ ...integrations, vm_url: e.target.value })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="vm-auth" className={labelClass}>VictoriaMetrics auth header</label>
        <Input id="vm-auth" type="password" placeholder="leave unchanged"
          value={integrations.vm_auth_header}
          onChange={(e) => setIntegrations({ ...integrations, vm_auth_header: e.target.value })} />
      </div>
      <LabelsEditor
        label="VictoriaMetrics extra labels"
        value={integrations.vm_extra_labels}
        onChange={(v) => setIntegrations({ ...integrations, vm_extra_labels: v })}
      />
      <div className="flex items-center gap-3">
        <Button type="button" variant="outline" disabled={testPending}
          onClick={() => runTest('vm', integrations.vm_url, integrations.vm_auth_header)}>
          Test VictoriaMetrics
        </Button>
        {vmResult && (
          <p className={vmResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{vmResult.message}</p>
        )}
      </div>

      <SwitchField
        id="vl-enabled" label="Enable VictoriaLogs"
        checked={integrations.vl_enabled}
        onCheckedChange={(checked) => setIntegrations({ ...integrations, vl_enabled: checked })}
      />
      <div className={fieldClass}>
        <label htmlFor="vl-url" className={labelClass}>VictoriaLogs URL</label>
        <Input id="vl-url" value={integrations.vl_url}
          onChange={(e) => setIntegrations({ ...integrations, vl_url: e.target.value })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="vl-auth" className={labelClass}>VictoriaLogs auth header</label>
        <Input id="vl-auth" type="password" placeholder="leave unchanged"
          value={integrations.vl_auth_header}
          onChange={(e) => setIntegrations({ ...integrations, vl_auth_header: e.target.value })} />
      </div>
      <LabelsEditor
        label="Extra stream fields"
        value={integrations.vl_stream_fields}
        onChange={(v) => setIntegrations({ ...integrations, vl_stream_fields: v })}
      />
      <div className="flex items-center gap-3">
        <Button type="button" variant="outline" disabled={testPending}
          onClick={() => runTest('vl', integrations.vl_url, integrations.vl_auth_header)}>
          Test VictoriaLogs
        </Button>
        {vlResult && (
          <p className={vlResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{vlResult.message}</p>
        )}
      </div>

      <SwitchField
        id="metrics-enabled" label="Enable /metrics endpoint"
        hint="/metrics answers 404 while disabled"
        checked={integrations.metrics_enabled}
        onCheckedChange={(checked) => setIntegrations({ ...integrations, metrics_enabled: checked })}
      />
    </Section>
  );
}
