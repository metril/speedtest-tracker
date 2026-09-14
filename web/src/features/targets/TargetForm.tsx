import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { FormField } from '../../components/FormField';
import { SwitchField } from '../../components/SwitchField';
import { ENGINES, type Target, type TargetInput, type ThresholdSet } from '../../lib/api';
import { EngineOptionFields, validateEngineOptions, type Options } from './EngineOptionFields';
import { ThresholdFields, validateThresholds } from './ThresholdFields';

const LANES = ['wan', 'lan'] as const;

interface Props {
  initial?: Target;
  onSubmit: (input: TargetInput) => void;
  onCancel: () => void;
  submitting: boolean;
  error?: string;
}

const field = 'w-full h-9 rounded-md border border-line-strong bg-surface px-2 py-1 text-sm text-fg focus:border-accent focus:outline-none';

/** TargetForm creates or edits one target. */
export function TargetForm({ initial, onSubmit, onCancel, submitting, error }: Props) {
  const [name, setName] = useState(initial?.name ?? '');
  const [engine, setEngine] = useState(initial?.engine ?? 'ookla');
  const [lane, setLane] = useState(initial?.lane ?? 'wan');
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [options, setOptions] = useState<Options>(initial?.options ?? {});
  const [thresholds, setThresholds] = useState<ThresholdSet>((initial?.thresholds as ThresholdSet) ?? {});
  const [touched, setTouched] = useState(false);
  // Bumped on a failed submit blocked by optionsError, to force iperf3's
  // Custom section on (see EngineOptionFields' forceOpenAdvancedSignal) so
  // a hidden field can't hide the reason Save silently did nothing.
  const [forceOpenAdvancedSignal, setForceOpenAdvancedSignal] = useState(0);

  const nameInvalid = name.trim() === '';
  const optionsError = validateEngineOptions(engine, options);
  const thresholdsError = validateThresholds(thresholds);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{initial ? 'Edit target' : 'New target'}</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            setTouched(true);
            if (nameInvalid || optionsError || thresholdsError) {
              if (optionsError) setForceOpenAdvancedSignal((n) => n + 1);
              return;
            }
            onSubmit({
              name: name.trim(), engine, enabled, lane, options, thresholds: thresholds as Record<string, unknown>,
            });
          }}
        >
          <div className="grid gap-3 sm:grid-cols-3">
            <FormField id="target-name" label="Name" error={touched && nameInvalid ? 'Name is required' : undefined}>
              <input id="target-name" className={field} value={name}
                onChange={(e) => setName(e.target.value)} />
            </FormField>
            <FormField id="target-engine" label="Engine">
              <select id="target-engine" className={field} value={engine}
                onChange={(e) => { setEngine(e.target.value); setOptions({}); }}>
                {ENGINES.map((e) => <option key={e} value={e}>{e}</option>)}
              </select>
            </FormField>
            <FormField id="target-lane" label="Lane">
              <select id="target-lane" className={field} value={lane}
                onChange={(e) => setLane(e.target.value)}>
                {LANES.map((l) => <option key={l} value={l}>{l}</option>)}
              </select>
            </FormField>
          </div>

          <div className="w-fit">
            <SwitchField id="target-enabled" label="Enabled" checked={enabled} onCheckedChange={setEnabled} />
          </div>

          <div className="border-t border-line pt-3">
            <EngineOptionFields
              engine={engine} options={options} onChange={setOptions}
              forceOpenAdvancedSignal={forceOpenAdvancedSignal}
            />
          </div>

          <div className="border-t border-line pt-3">
            <ThresholdFields value={thresholds} onChange={setThresholds} />
            {touched && thresholdsError && (
              <p className="mt-1 text-xs text-bad">{thresholdsError}</p>
            )}
          </div>

          {touched && optionsError && <p className="text-sm text-bad">{optionsError}</p>}
          {error && <p className="text-sm text-bad">{error}</p>}

          <div className="flex gap-2">
            <Button type="submit" disabled={submitting}>
              Save target
            </Button>
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
