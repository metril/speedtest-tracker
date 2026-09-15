import { useState } from 'react';
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

  const copy = () => {
    if (!navigator.clipboard?.writeText) return;
    navigator.clipboard.writeText(error);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
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
          <Button type="button" variant="outline" onClick={copy}>{copied ? 'Copied' : 'Copy'}</Button>
          <Button type="button" onClick={() => onOpenChange(false)}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
