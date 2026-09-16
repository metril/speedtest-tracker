import { X } from 'lucide-react';
import { useState, type KeyboardEvent } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface Props {
  id: string;
  value: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  validate?: (s: string) => string | undefined;
  disabled?: boolean;
  label: string;
  'aria-invalid'?: boolean;
  'aria-describedby'?: string;
}

/** ListInput edits a list of strings as chips inside one bordered field:
 * type a value and press Enter or comma (or blur the field) to commit
 * it, Backspace on an empty draft removes the last chip. Values that
 * fail `validate` stay in the list (so a mistake isn't silently
 * dropped) but are flagged with the message as both a title and
 * aria-invalid. */
export function ListInput({
  id, value, onChange, placeholder, validate, disabled, label,
  'aria-invalid': ariaInvalid, 'aria-describedby': ariaDescribedby,
}: Props) {
  const [draft, setDraft] = useState('');

  const commit = (raw: string) => {
    const trimmed = raw.trim();
    setDraft('');
    if (!trimmed) return;
    if (value.includes(trimmed)) return;
    onChange([...value, trimmed]);
  };

  const remove = (v: string) => onChange(value.filter((x) => x !== v));

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      commit(draft);
      return;
    }
    if (e.key === 'Backspace' && draft === '' && value.length > 0) {
      e.preventDefault();
      remove(value[value.length - 1]);
    }
  };

  return (
    <div
      className={cn(
        'flex h-9 flex-wrap items-center gap-1.5 rounded-md border border-input bg-transparent px-2 py-1 text-sm shadow-sm',
        'h-auto min-h-9',
        disabled && 'cursor-not-allowed opacity-50',
      )}
    >
      {value.map((v) => {
        const message = validate?.(v);
        const invalid = Boolean(message);
        return (
          <Badge
            key={v}
            variant={invalid ? 'outline' : 'secondary'}
            className={cn('gap-1', invalid && 'border-bad text-bad')}
            title={message}
            aria-invalid={invalid || undefined}
          >
            {v}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`Remove ${v}`}
              onClick={() => remove(v)}
              disabled={disabled}
              className="h-4 w-4 p-0 hover:bg-transparent"
            >
              <X className="size-3" />
            </Button>
          </Badge>
        );
      })}
      <input
        id={id}
        aria-label={label}
        aria-invalid={ariaInvalid}
        aria-describedby={ariaDescribedby}
        className="min-w-[6rem] flex-1 bg-transparent text-fg outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed"
        placeholder={value.length === 0 ? placeholder : undefined}
        value={draft}
        disabled={disabled}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKeyDown}
        onBlur={() => commit(draft)}
      />
    </div>
  );
}
