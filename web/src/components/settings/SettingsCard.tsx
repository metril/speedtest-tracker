import { useId, type ReactNode } from 'react';
import { Card, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

interface Props {
  title: string;
  description?: ReactNode;
  headerAction?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}

/** SettingsCard is the Card chrome for the redesigned settings layout:
 * a header (title/description/action) and a body of SettingsRows
 * separated by dividers instead of the old title+content padding. Rows
 * carry their own px-4 py-3, so the body has no CardContent padding. */
export function SettingsCard({ title, description, headerAction, footer, children }: Props) {
  const id = useId();
  return (
    <section aria-labelledby={id}>
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-4">
          <div className="flex flex-col gap-1.5">
            <CardTitle id={id} className="text-base font-semibold">{title}</CardTitle>
            {description && <CardDescription className="text-sm text-faint">{description}</CardDescription>}
          </div>
          {headerAction}
        </CardHeader>
        <div className="divide-y divide-line">{children}</div>
        {footer && (
          <div className="flex flex-wrap items-center gap-3 rounded-b-xl border-t border-line bg-raised/40 px-4 py-3">
            {footer}
          </div>
        )}
      </Card>
    </section>
  );
}
