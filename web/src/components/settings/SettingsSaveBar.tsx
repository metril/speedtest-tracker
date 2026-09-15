import { Button } from '@/components/ui/button';

interface Props {
  dirtyCount: number;
  saving: boolean;
  onSave: () => void;
  onDiscard: () => void;
  error?: string | null;
  readOnly?: boolean;
}

/** SettingsSaveBar is the sticky footer that appears once a settings
 * form has unsaved changes (or an error to show), letting the user save
 * or discard everything at once. Renders nothing when there's nothing
 * to report. */
export function SettingsSaveBar({ dirtyCount, saving, onSave, onDiscard, error, readOnly }: Props) {
  if (dirtyCount === 0 && !error) return null;

  return (
    <div
      role="region"
      aria-label="Unsaved changes"
      className="sticky bottom-0 z-10 mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-line bg-surface/95 px-4 py-3 backdrop-blur"
    >
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm text-muted">
          {dirtyCount} unsaved {dirtyCount === 1 ? 'change' : 'changes'}
        </p>
        {error && <p role="alert" className="text-sm text-bad">{error}</p>}
      </div>
      {readOnly ? (
        <p className="text-sm text-faint">Read-only: admin group required</p>
      ) : (
        <div className="flex items-center gap-3">
          <Button type="button" variant="outline" onClick={onDiscard}>
            Discard
          </Button>
          <Button type="button" disabled={saving || readOnly} onClick={onSave}>
            Save changes
          </Button>
        </div>
      )}
    </div>
  );
}
