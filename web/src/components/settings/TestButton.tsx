import { Button } from '@/components/ui/button';

interface Props {
  label: string;
  onTest: () => void;
  pending: boolean;
  result?: { ok: boolean; message: string } | null;
  disabled?: boolean;
  disabledReason?: string;
}

/** TestButton fires a live check (webhook/apprise/SMTP test send) and
 * shows the outcome inline. When disabled with a reason (e.g. "save
 * first"), the reason is shown instead of letting the click reach the
 * server. */
export function TestButton({ label, onTest, pending, result, disabled, disabledReason }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Button type="button" variant="outline" disabled={disabled || pending} onClick={onTest}>
        {label}
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
