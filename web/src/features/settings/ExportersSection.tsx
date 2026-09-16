import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { KeyValueInput, SettingsCard, SettingsRow, TestButton, useSettingsRowField } from '@/components/settings';
import { ExportAuthFields } from './ExportAuthFields';
import { useSettingsSection } from './useSettingsSection';

function UrlField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} value={value} onChange={(e) => onChange(e.target.value)} />;
}

const READ_ONLY_REASON = 'Read-only: admin group required';

export function ExportersSection() {
  const {
    integrations, setIntegrations, testPending, vmResult, vlResult, runTest, readOnly, locked,
  } = useSettingsSection('exporters');
  const isLocked = (key: string) => locked.includes(`integrations.${key}`);

  return (
    <div className="grid gap-4">
      <SettingsCard
        title="VictoriaMetrics"
        description="Push every result as metrics."
        headerAction={
          <Switch
            id="vm-enabled" aria-label="Enable VictoriaMetrics"
            checked={integrations.vm_enabled}
            disabled={readOnly || isLocked('vm_enabled')}
            onCheckedChange={(checked) => setIntegrations({ ...integrations, vm_enabled: checked })}
          />
        }
        footer={
          <TestButton
            label="Test VictoriaMetrics"
            pending={testPending}
            result={vmResult}
            disabled={readOnly}
            disabledReason={READ_ONLY_REASON}
            onTest={() => runTest('vm', integrations.vm_url, {
              type: integrations.vm_auth_type,
              username: integrations.vm_auth_username,
              password: integrations.vm_auth_password,
              token: integrations.vm_auth_token,
              header_name: integrations.vm_auth_header_name,
              header_value: integrations.vm_auth_header_value,
            })}
          />
        }
      >
        <SettingsRow label="VictoriaMetrics URL" htmlFor="vm-url" lockKey="integrations.vm_url">
          <UrlField value={integrations.vm_url} onChange={(v) => setIntegrations({ ...integrations, vm_url: v })} />
        </SettingsRow>
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
        <div className="px-4 py-3">
          <KeyValueInput
            label="VictoriaMetrics extra labels"
            value={integrations.vm_extra_labels}
            onChange={(v) => setIntegrations({ ...integrations, vm_extra_labels: v })}
          />
        </div>
      </SettingsCard>

      <SettingsCard
        title="VictoriaLogs"
        description="Push every result as logs."
        headerAction={
          <Switch
            id="vl-enabled" aria-label="Enable VictoriaLogs"
            checked={integrations.vl_enabled}
            disabled={readOnly || isLocked('vl_enabled')}
            onCheckedChange={(checked) => setIntegrations({ ...integrations, vl_enabled: checked })}
          />
        }
        footer={
          <TestButton
            label="Test VictoriaLogs"
            pending={testPending}
            result={vlResult}
            disabled={readOnly}
            disabledReason={READ_ONLY_REASON}
            onTest={() => runTest('vl', integrations.vl_url, {
              type: integrations.vl_auth_type,
              username: integrations.vl_auth_username,
              password: integrations.vl_auth_password,
              token: integrations.vl_auth_token,
              header_name: integrations.vl_auth_header_name,
              header_value: integrations.vl_auth_header_value,
            })}
          />
        }
      >
        <SettingsRow label="VictoriaLogs URL" htmlFor="vl-url" lockKey="integrations.vl_url">
          <UrlField value={integrations.vl_url} onChange={(v) => setIntegrations({ ...integrations, vl_url: v })} />
        </SettingsRow>
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
        <div className="px-4 py-3">
          <KeyValueInput
            label="Extra stream fields"
            value={integrations.vl_stream_fields}
            onChange={(v) => setIntegrations({ ...integrations, vl_stream_fields: v })}
          />
        </div>
      </SettingsCard>

      <SettingsCard
        title="Prometheus endpoint"
        description="/metrics answers 404 while disabled."
        headerAction={
          <Switch
            id="metrics-enabled" aria-label="Enable /metrics endpoint"
            checked={integrations.metrics_enabled}
            disabled={readOnly || isLocked('metrics_enabled')}
            onCheckedChange={(checked) => setIntegrations({ ...integrations, metrics_enabled: checked })}
          />
        }
      >
        <div className="px-4 py-3 text-sm text-faint">
          Exposes speedtest results in Prometheus exposition format at /metrics.
        </div>
      </SettingsCard>
    </div>
  );
}
