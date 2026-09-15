import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { SwitchField } from '../../components/SwitchField';
import { Iperf3ServerListSection } from './Iperf3ServerListSection';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

export function EnginesSection() {
  const { engines, setEngines, saving, error, saved, save, readOnly } = useSettingsSection('engines');

  return (
    <Section
      id="engines-heading" title="Engines" saving={saving}
      error={error} saved={saved} readOnly={readOnly}
      onSave={() => save('engines', { engines })}
    >
      <FormField id="engines-speedtest-bin" label="Speedtest binary path">
        <Input id="engines-speedtest-bin" value={engines.speedtest_bin}
          onChange={(e) => setEngines({ ...engines, speedtest_bin: e.target.value })} />
      </FormField>
      <FormField id="engines-iperf3-bin" label="iperf3 binary path">
        <Input id="engines-iperf3-bin" value={engines.iperf3_bin}
          onChange={(e) => setEngines({ ...engines, iperf3_bin: e.target.value })} />
      </FormField>
      <SwitchField
        id="engines-ookla-accept-license" label="Accept Ookla license"
        checked={engines.ookla_accept_license}
        onCheckedChange={(checked) => setEngines({ ...engines, ookla_accept_license: checked })}
      />
      <SwitchField
        id="engines-ookla-accept-gdpr" label="Accept Ookla GDPR terms"
        checked={engines.ookla_accept_gdpr}
        onCheckedChange={(checked) => setEngines({ ...engines, ookla_accept_gdpr: checked })}
      />
      <FormField id="engines-ttl" label="Server list TTL (seconds)">
        <Input id="engines-ttl" type="number" min={1}
          value={engines.server_list_ttl_seconds}
          onChange={(e) => setEngines({ ...engines, server_list_ttl_seconds: Number(e.target.value) })} />
      </FormField>

      <div className="grid gap-2 border-t border-line pt-4">
        <h3 className="text-sm font-semibold text-fg">iperf3 server list</h3>
        <p className="text-sm text-faint">
          A cached list of public iperf3 servers, refreshed daily, used by the target form's
          &quot;Pick from public list&quot; picker. Clear the URL to disable the list entirely.
        </p>
        <FormField id="engines-iperf3-list-url" label="iperf3 server list URL">
          <Input id="engines-iperf3-list-url" value={engines.iperf3_list_url}
            onChange={(e) => setEngines({ ...engines, iperf3_list_url: e.target.value })} />
        </FormField>
        <Iperf3ServerListSection />
      </div>
    </Section>
  );
}
