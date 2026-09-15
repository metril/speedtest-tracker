import type { ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

/** Section is the Card chrome shared by every settings tab's content:
 * title, a Save button (with a pending state and a flashed "Saved"
 * message), and an inline error. */
export function Section({
  id, title, onSave, saving, error, saved, readOnly, children,
}: {
  id: string;
  title: string;
  onSave: () => void;
  saving: boolean;
  error?: string;
  saved: boolean;
  /** readOnly disables Save (a non-admin viewer under oidc/forward_auth)
   * and shows a hint instead of letting the click reach the server. */
  readOnly?: boolean;
  children: ReactNode;
}) {
  return (
    <Card aria-labelledby={id} role="region" className="space-y-4">
      <CardHeader className="pb-0">
        <CardTitle id={id} className="text-lg">{title}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">
        {children}
        <div className="flex items-center gap-3">
          <Button type="button" disabled={saving || readOnly} onClick={onSave}>
            Save {title}
          </Button>
          {saved && <p className="text-sm text-ok">Saved</p>}
          {readOnly && <p className="text-sm text-faint">Read-only: admin group required</p>}
        </div>
        {error && <p role="alert" className="text-sm text-bad">{error}</p>}
      </CardContent>
    </Card>
  );
}
