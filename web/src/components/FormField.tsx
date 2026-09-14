import type { ReactNode } from 'react';
import { Label } from '@/components/ui/label';

interface Props {
  id: string;
  label: string;
  hint?: string;
  error?: string;
  className?: string;
  children: ReactNode;
}

/** FormField is the standard label + control + hint/error layout, replacing
 * the ad-hoc label/field class sets each form used to define separately. */
export function FormField({ id, label, hint, error, className, children }: Props) {
  return (
    <div className={className ? `grid gap-1 ${className}` : 'grid gap-1'}>
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && !error && <p className="text-xs text-faint">{hint}</p>}
      {error && <p className="text-xs text-bad">{error}</p>}
    </div>
  );
}
