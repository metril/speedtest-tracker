import { Input } from '@/components/ui/input';
import { SwitchField } from '../../components/SwitchField';
import { Iperf3ServerListSection } from './Iperf3ServerListSection';
import { fieldClass, labelClass } from './styles';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

export function EnginesSection() {
  const { engines, setEngines, saving, error, saved, save } = useSettingsSection('engines');

  return (
    <Section
      id="engines-heading" title="Engines" saving={saving}
      error={error} saved={saved}
      onSave={() => save('engines', { engines })}
    >
      <div className={fieldClass}>
        <label htmlFor="engines-speedtest-bin" className={labelClass}>Speedtest binary path</label>
        <Input id="engines-speedtest-bin" value={engines.speedtest_bin}
          onChange={(e) => setEngines({ ...engines, speedtest_bin: e.target.value })} />
      </div>
      <div className={fieldClass}>
        <label htmlFor="engines-iperf3-bin" className={labelClass}>iperf3 binary path</label>
        <Input id="engines-iperf3-bin" value={engines.iperf3_bin}
          onChange={(e) => setEngines({ ...engines, iperf3_bin: e.target.value })} />
      </div>
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
      <div className={fieldClass}>
        <label htmlFor="engines-ttl" className={labelClass}>Server list TTL (seconds)</label>
        <Input id="engines-ttl" type="number" min={1}
          value={engines.server_list_ttl_seconds}
          onChange={(e) => setEngines({ ...engines, server_list_ttl_seconds: Number(e.target.value) })} />
      </div>

      <div className="grid gap-2 border-t border-line pt-4">
        <h3 className="text-sm font-semibold text-fg">iperf3 server list</h3>
        <p className="text-sm text-faint">
          A cached list of public iperf3 servers, refreshed daily, used by the target form's
          &quot;Pick from public list&quot; picker. Clear the URL to disable the list entirely.
        </p>
        <div className={fieldClass}>
          <label htmlFor="engines-iperf3-list-url" className={labelClass}>iperf3 server list URL</label>
          <Input id="engines-iperf3-list-url" value={engines.iperf3_list_url}
            onChange={(e) => setEngines({ ...engines, iperf3_list_url: e.target.value })} />
        </div>
        <Iperf3ServerListSection />
      </div>
    </Section>
  );
}
