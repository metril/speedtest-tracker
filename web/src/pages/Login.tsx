import { useState } from 'react';
import { useLocation } from 'react-router';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ErrorDialog } from '../components/ErrorDialog';

/** ERROR_MESSAGES maps a /login?error=<code> code (set by the server after
 * a failed OIDC callback) to a human explanation shown in the ErrorDialog. */
const ERROR_MESSAGES: Record<string, string> = {
  forbidden: 'Your account is not in an allowed group or email for this instance.',
  state: 'The sign-in request expired or was tampered with. Please try again.',
  exchange: 'The identity provider rejected the sign-in exchange. Please try again.',
  oidc_not_configured: 'Single sign-on is not configured on this instance.',
};

/** Login is the unauthenticated landing page for oidc/forward/token modes:
 * a single "Sign in with SSO" action that redirects to the server's OIDC
 * start endpoint, carrying the path the caller was trying to reach so the
 * callback can send them back there. */
export function Login() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const errorCode = params.get('error');
  const [dialogOpen, setDialogOpen] = useState(false);

  const from = (location.state as { from?: string } | null)?.from ?? '/';
  const startUrl = `/auth/oidc/start?return_to=${encodeURIComponent(from)}`;

  return (
    <div className="flex min-h-screen items-center justify-center bg-app px-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-center text-lg">speedtest-tracker</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4">
          <Button asChild>
            <a href={startUrl}>Sign in with SSO</a>
          </Button>
          {errorCode && (
            <div className="grid gap-2 text-center">
              <p className="text-sm text-muted">Sign-in failed</p>
              <Button type="button" variant="outline" size="sm" onClick={() => setDialogOpen(true)}>
                View error
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
      {errorCode && (
        <ErrorDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          title="Sign-in failed"
          error={ERROR_MESSAGES[errorCode] ?? `Sign-in failed (${errorCode}).`}
        />
      )}
    </div>
  );
}
