import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { inputClass } from '@/features/settings/styles';
import { FormField } from '../../components/FormField';
import { SwitchField } from '../../components/SwitchField';
import { ENGINES, type Target, type TargetInput, type ThresholdSet } from '../../lib/api';
import { useQueues } from '../../lib/queries';
import { EngineOptionFields, validateEngineOptions, type Options } from './EngineOptionFields';
import { ThresholdFields, validateThresholds } from './ThresholdFields';

interface Props {
  initial?: Target;
  onSubmit: (input: TargetInput) => void;
  onCancel: () => void;
  submitting: boolean;
  error?: string;
}


/** TargetForm creates or edits one target. */
export function TargetForm({ initial, onSubmit, onCancel, submitting, error }: Props) {
  const queues = useQueues();
  const [name, setName] = useState(initial?.name ?? '');
  const [engine, setEngine] = useState(initial?.engine ?? 'ookla');
  const [queueId, setQueueId] = useState(initial?.queue_id ?? 0);
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
  // thresholds is {} whenever Custom notification is off, and
  // validateThresholds({}) is always undefined, so thresholdsError can only
  // surface while ThresholdFields' Custom notification toggle is on.
  const thresholdsError = validateThresholds(thresholds);
  // Falls back to the first queue once the list loads, so a fresh form
  // (queueId still 0) always submits a real queue id.
  const selectedQueueId = queueId || queues.data?.[0]?.id || 0;

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
              name: name.trim(), engine, enabled, queue_id: selectedQueueId, options,
              thresholds: thresholds as Record<string, unknown>,
            });
          }}
        >
          <div className="grid gap-3 sm:grid-cols-3">
            <FormField id="target-name" label="Name" error={touched && nameInvalid ? 'Name is required' : undefined}>
              <input id="target-name" className={inputClass} value={name}
                onChange={(e) => setName(e.target.value)} />
            </FormField>
            <FormField id="target-engine" label="Engine">
              <select id="target-engine" className={inputClass} value={engine}
                onChange={(e) => { setEngine(e.target.value); setOptions({}); }}>
                {ENGINES.map((e) => <option key={e} value={e}>{e}</option>)}
              </select>
            </FormField>
            <FormField
              id="target-queue" label="Queue"
              hint="Targets in the same queue run one at a time; different queues run in parallel."
            >
              <select id="target-queue" className={inputClass} value={selectedQueueId}
                onChange={(e) => setQueueId(Number(e.target.value))}>
                {(queues.data ?? []).map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
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
