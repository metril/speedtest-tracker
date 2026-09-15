import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { LabelsEditor } from '../../components/LabelsEditor';
import { SwitchField } from '../../components/SwitchField';
import { ExportAuthFields } from './ExportAuthFields';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

export function IntegrationsSection() {
  const {
    integrations, setIntegrations, saving, error, saved, save, testPending, vmResult, vlResult, runTest, readOnly,
    locked,
  } = useSettingsSection('integrations');
  const isLocked = (key: string) => locked.includes(`integrations.${key}`);

  return (
    <Section
      id="integrations-heading" title="Integrations" saving={saving}
      error={error} saved={saved} readOnly={readOnly}
      onSave={() => save('integrations', { integrations })}
    >
      <SwitchField
        id="vm-enabled" label="Enable VictoriaMetrics"
        checked={integrations.vm_enabled}
        onCheckedChange={(checked) => setIntegrations({ ...integrations, vm_enabled: checked })}
      />
      <FormField id="vm-url" label="VictoriaMetrics URL">
        <Input id="vm-url" value={integrations.vm_url}
          onChange={(e) => setIntegrations({ ...integrations, vm_url: e.target.value })} />
      </FormField>
      <ExportAuthFields
        idPrefix="vm" label="VictoriaMetrics auth"
        locked={isLocked}
        readOnly={readOnly}
        value={{
          type: integrations.vm_auth_type,
          username: integrations.vm_auth_username,
          password: integrations.vm_auth_password,
          token: integrations.vm_auth_token,
          header_name: integrations.vm_auth_header_name,
          header_value: integrations.vm_auth_header_value,
        }}
        onChange={(next) => setIntegrations({
          ...integrations,
          vm_auth_type: next.type,
          vm_auth_username: next.username ?? '',
          vm_auth_password: next.password ?? '',
          vm_auth_token: next.token ?? '',
          vm_auth_header_name: next.header_name ?? '',
          vm_auth_header_value: next.header_value ?? '',
        })}
      />
      <LabelsEditor
        label="VictoriaMetrics extra labels"
        value={integrations.vm_extra_labels}
        onChange={(v) => setIntegrations({ ...integrations, vm_extra_labels: v })}
      />
      <div className="flex items-center gap-3">
        <Button type="button" variant="outline" disabled={testPending || readOnly}
          onClick={() => runTest('vm', integrations.vm_url, {
            type: integrations.vm_auth_type,
            username: integrations.vm_auth_username,
            password: integrations.vm_auth_password,
            token: integrations.vm_auth_token,
            header_name: integrations.vm_auth_header_name,
            header_value: integrations.vm_auth_header_value,
          })}>
          Test VictoriaMetrics
        </Button>
        {readOnly && <p className="text-sm text-faint">Read-only: admin group required</p>}
        {vmResult && (
          <p className={vmResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{vmResult.message}</p>
        )}
      </div>

      <SwitchField
        id="vl-enabled" label="Enable VictoriaLogs"
        checked={integrations.vl_enabled}
        onCheckedChange={(checked) => setIntegrations({ ...integrations, vl_enabled: checked })}
      />
      <FormField id="vl-url" label="VictoriaLogs URL">
        <Input id="vl-url" value={integrations.vl_url}
          onChange={(e) => setIntegrations({ ...integrations, vl_url: e.target.value })} />
      </FormField>
      <ExportAuthFields
        idPrefix="vl" label="VictoriaLogs auth"
        locked={isLocked}
        readOnly={readOnly}
        value={{
          type: integrations.vl_auth_type,
          username: integrations.vl_auth_username,
          password: integrations.vl_auth_password,
          token: integrations.vl_auth_token,
          header_name: integrations.vl_auth_header_name,
          header_value: integrations.vl_auth_header_value,
        }}
        onChange={(next) => setIntegrations({
          ...integrations,
          vl_auth_type: next.type,
          vl_auth_username: next.username ?? '',
          vl_auth_password: next.password ?? '',
          vl_auth_token: next.token ?? '',
          vl_auth_header_name: next.header_name ?? '',
          vl_auth_header_value: next.header_value ?? '',
        })}
      />
      <LabelsEditor
        label="Extra stream fields"
        value={integrations.vl_stream_fields}
        onChange={(v) => setIntegrations({ ...integrations, vl_stream_fields: v })}
      />
      <div className="flex items-center gap-3">
        <Button type="button" variant="outline" disabled={testPending || readOnly}
          onClick={() => runTest('vl', integrations.vl_url, {
            type: integrations.vl_auth_type,
            username: integrations.vl_auth_username,
            password: integrations.vl_auth_password,
            token: integrations.vl_auth_token,
            header_name: integrations.vl_auth_header_name,
            header_value: integrations.vl_auth_header_value,
          })}>
          Test VictoriaLogs
        </Button>
        {readOnly && <p className="text-sm text-faint">Read-only: admin group required</p>}
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
