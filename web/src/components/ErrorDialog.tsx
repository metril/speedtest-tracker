import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog';

/** ErrorDialog shows a full error message that would otherwise be
 * truncated or hidden inline -- a monospace, scrollable body with a Copy
 * action, built on the same Dialog primitive as ConfirmDialog. */
export function ErrorDialog({
  open, onOpenChange, title = 'Error', error,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: string;
  error: string;
}) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => () => clearTimeout(timer.current), []);

  const canCopy = typeof navigator !== 'undefined' && !!navigator.clipboard?.writeText;

  const copy = () => {
    if (!canCopy) return;
    navigator.clipboard.writeText(error)
      .then(() => {
        setCopied(true);
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setCopied(false), 1500);
      })
      .catch(() => {});
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <pre className="max-h-[60vh] overflow-auto whitespace-pre-wrap break-words text-xs text-bad">
          {error}
        </pre>
        <DialogFooter>
          {canCopy && (
            <Button type="button" variant="outline" onClick={copy}>{copied ? 'Copied' : 'Copy'}</Button>
          )}
          <Button type="button" onClick={() => onOpenChange(false)}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
