import { HintTip } from '@/components/HintTip';
import { Switch } from '@/components/ui/switch';

interface Props {
  id: string;
  label: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  hint?: string;
}

/** SwitchField is the standard enable/disable row: label left, switch
 * right, clicking the label toggles the switch (same target as clicking
 * the switch itself). */
export function SwitchField({ id, label, checked, onCheckedChange, disabled, hint }: Props) {
  return (
    <div className="flex items-center justify-between gap-3">
      <div className="flex items-center gap-1.5">
        <label htmlFor={id} className="text-sm text-muted">{label}</label>
        {hint && <HintTip hint={hint} />}
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  );
}
