import type { ReactNode } from 'react';
import { HintTip } from '@/components/HintTip';
import { Label } from '@/components/ui/label';

interface Props {
  id: string;
  label: string;
  hint?: string;
  error?: string;
  className?: string;
  /** action renders trailing content (e.g. a link/button) in the label row. */
  action?: ReactNode;
  children: ReactNode;
}

/** FormField is the standard label + control + hint/error layout, replacing
 * the ad-hoc label/field class sets each form used to define separately. */
export function FormField({ id, label, hint, error, className, action, children }: Props) {
  return (
    <div className={className ? `grid gap-1 ${className}` : 'grid gap-1'}>
      <div className="flex h-5 items-center gap-1.5">
        <Label htmlFor={id}>{label}</Label>
        {hint && <HintTip hint={hint} />}
        {action && <span className="ml-auto">{action}</span>}
      </div>
      {children}
      {error && <p className="text-xs text-bad">{error}</p>}
    </div>
  );
}
