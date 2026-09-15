import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { SettingsCard, SettingsRow, useSettingsRowField } from '../../components/settings';
import { Iperf3ServerListSection } from './Iperf3ServerListSection';
import { useSettingsSection } from './useSettingsSection';

function TextField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} value={value} onChange={(e) => onChange(e.target.value)} />;
}

function NumberField({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} type="number" min={1} value={value} onChange={(e) => onChange(Number(e.target.value))} />;
}

function SwitchControl({ checked, onCheckedChange }: { checked: boolean; onCheckedChange: (v: boolean) => void }) {
  const props = useSettingsRowField();
  return (
    <div className="flex justify-end">
      <Switch {...props} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}

export function EnginesSection() {
  const { engines, setEngines } = useSettingsSection('engines');

  return (
    <div className="grid gap-6">
      <SettingsCard title="Binaries">
        <SettingsRow label="Speedtest binary path" htmlFor="engines-speedtest-bin" lockKey="engines.speedtest_bin">
          <TextField value={engines.speedtest_bin} onChange={(v) => setEngines({ ...engines, speedtest_bin: v })} />
        </SettingsRow>
        <SettingsRow label="iperf3 binary path" htmlFor="engines-iperf3-bin" lockKey="engines.iperf3_bin">
          <TextField value={engines.iperf3_bin} onChange={(v) => setEngines({ ...engines, iperf3_bin: v })} />
        </SettingsRow>
      </SettingsCard>

      <SettingsCard title="Ookla terms">
        <SettingsRow
          label="Accept Ookla license" description="Required to run speedtest.net tests."
          htmlFor="engines-ookla-accept-license" lockKey="engines.ookla_accept_license"
        >
          <SwitchControl
            checked={engines.ookla_accept_license}
            onCheckedChange={(checked) => setEngines({ ...engines, ookla_accept_license: checked })}
          />
        </SettingsRow>
        <SettingsRow
          label="Accept Ookla GDPR terms" description="Required for the Ookla engine in GDPR jurisdictions."
          htmlFor="engines-ookla-accept-gdpr" lockKey="engines.ookla_accept_gdpr"
        >
          <SwitchControl
            checked={engines.ookla_accept_gdpr}
            onCheckedChange={(checked) => setEngines({ ...engines, ookla_accept_gdpr: checked })}
          />
        </SettingsRow>
        <SettingsRow
          label="Server list TTL (seconds)" htmlFor="engines-ttl" lockKey="engines.server_list_ttl_seconds" size="sm"
        >
          <NumberField
            value={engines.server_list_ttl_seconds}
            onChange={(v) => setEngines({ ...engines, server_list_ttl_seconds: v })}
          />
        </SettingsRow>
      </SettingsCard>

      <SettingsCard
        title="iperf3 public server list"
        description="A cached list of public iperf3 servers, refreshed daily, used by the target
          form's &quot;Pick from public list&quot; picker."
        footer={<Iperf3ServerListSection />}
      >
        <SettingsRow
          label="iperf3 server list URL" description="Clear to disable the list."
          htmlFor="engines-iperf3-list-url" lockKey="engines.iperf3_list_url"
        >
          <TextField value={engines.iperf3_list_url} onChange={(v) => setEngines({ ...engines, iperf3_list_url: v })} />
        </SettingsRow>
      </SettingsCard>
    </div>
  );
}
