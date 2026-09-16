import type { ReactNode } from 'react';
import { Button, type ButtonProps } from '@/components/ui/button';

interface Props {
  /** Accessible name for the button (rendered as aria-label), e.g.
   * "Test Discord" — always specific even when buttonText is generic. */
  label: string;
  /** Visible button content; defaults to `label` when omitted. */
  buttonText?: ReactNode;
  icon?: ReactNode;
  onTest: () => void;
  pending: boolean;
  result?: { ok: boolean; message: string } | null;
  disabled?: boolean;
  disabledReason?: string;
  variant?: ButtonProps['variant'];
  size?: ButtonProps['size'];
}

/** TestButton fires a live check (webhook/apprise/SMTP test send) and
 * shows the outcome inline. When disabled with a reason (e.g. "save
 * first"), the reason is shown instead of letting the click reach the
 * server. */
export function TestButton({
  label, buttonText, icon, onTest, pending, result, disabled, disabledReason, variant = 'outline', size,
}: Props) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Button
        type="button" variant={variant} size={size} aria-label={label}
        disabled={disabled || pending} onClick={onTest}
      >
        {icon}
        {buttonText ?? label}
      </Button>
      {disabled && disabledReason && <p className="text-sm text-faint">{disabledReason}</p>}
      {result && (
        <p aria-live="polite" className={result.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>
          {result.message}
        </p>
      )}
    </div>
  );
}
