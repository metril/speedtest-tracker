import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import type { CreatedToken } from '../../lib/api';
import { ApiError } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import { useCreateToken, useDeleteToken, useTokens } from '../../lib/queries';

function message(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Request failed.';
}

/** TokenPanel creates and lists API tokens. The plaintext of a newly
 * created token exists only in this component's local state -- never in
 * the query cache or any browser storage -- and is cleared as soon as the
 * user dismisses it. */
export function TokenPanel() {
  const tokens = useTokens();
  const create = useCreateToken();
  const del = useDeleteToken();

  const [name, setName] = useState('');
  const [created, setCreated] = useState<CreatedToken | null>(null);
  const [copied, setCopied] = useState(false);
  const [confirmId, setConfirmId] = useState<number | null>(null);
  const [error, setError] = useState<string | undefined>();

  const copyTimer = useRef<ReturnType<typeof setTimeout>>();
  const tokenTextRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => () => {
    if (copyTimer.current) clearTimeout(copyTimer.current);
  }, []);

  const submit = () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setError(undefined);
    create.mutate(trimmed, {
      onSuccess: (token) => {
        setCreated(token);
        setName('');
      },
      onError: (err) => setError(message(err)),
    });
  };

  const copy = async () => {
    if (!created) return;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(created.token);
      } else if (tokenTextRef.current) {
        // No Clipboard API (e.g. a plain-HTTP origin): select the text so
        // the user can copy it themselves.
        const range = document.createRange();
        range.selectNodeContents(tokenTextRef.current);
        const selection = window.getSelection();
        selection?.removeAllRanges();
        selection?.addRange(range);
      }
      setCopied(true);
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard access can be denied by the browser; nothing to do here.
    }
  };

  const revoke = (id: number) => {
    setError(undefined);
    del.mutate(id, {
      onSuccess: () => setConfirmId((c) => (c === id ? null : c)),
      onError: (err) => setError(message(err)),
    });
  };

  return (
    <div className="grid gap-3">
      {error && <p role="alert" className="text-sm text-bad">{error}</p>}

      {created ? (
        <div className="grid gap-2 rounded-md border border-line bg-raised p-3">
          <p ref={tokenTextRef} className="break-all font-mono text-sm text-fg">{created.token}</p>
          <div className="flex items-center gap-3">
            <Button type="button" onClick={copy}>Copy</Button>
            {copied && <span className="text-sm text-ok">Copied</span>}
          </div>
          <p className="text-sm text-warn">
            This token is shown only once. Store it now; it cannot be retrieved later.
          </p>
          <div>
            <Button type="button" onClick={() => setCreated(null)}>Done</Button>
          </div>
        </div>
      ) : (
        <div className="flex items-end gap-3">
          <FormField id="token-name" label="Token name">
            <Input id="token-name" value={name}
              onChange={(e) => setName(e.target.value)} />
          </FormField>
          <Button type="button" disabled={create.isPending || !name.trim()}
            onClick={submit}>
            Create token
          </Button>
        </div>
      )}

      <div className="grid gap-2">
        {(tokens.data ?? []).map((t) => (
          <div key={t.id} className="flex items-center justify-between gap-3 rounded-md border border-line p-2 text-sm">
            <div className="grid gap-0.5">
              <span className="text-fg">{t.name}</span>
              <span className="text-faint">
                <span className="font-mono">{t.prefix}…</span>
                {' · created '}{formatDateTime(t.created_at)}
                {' · last used '}{t.last_used_at ? formatDateTime(t.last_used_at) : '—'}
              </span>
            </div>
            {confirmId === t.id ? (
              <div className="flex items-center gap-2">
                <Button type="button" variant="outline" size="sm" className="text-bad hover:bg-bad/10"
                  disabled={del.isPending} onClick={() => revoke(t.id)}>
                  Confirm revoke
                </Button>
                <Button type="button" variant="ghost" size="sm" onClick={() => setConfirmId(null)}>
                  Cancel
                </Button>
              </div>
            ) : (
              <Button type="button" variant="outline" size="sm" className="text-muted hover:text-bad"
                onClick={() => setConfirmId(t.id)}>
                Revoke {t.name}
              </Button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
