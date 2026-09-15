import { useState } from 'react';
import { Navigate, useLocation } from 'react-router';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ErrorDialog } from '../components/ErrorDialog';
import { useAuthMode } from '../lib/queries';

/** ERROR_MESSAGES maps a /login?error=<code> code (set by the server after
 * a failed OIDC start/callback) to a human explanation shown in the
 * ErrorDialog. The keys must match the codes internal/api/oidc.go's
 * redirectLoginError calls actually send. */
const ERROR_MESSAGES: Record<string, string> = {
  exchange_failed: 'The identity provider rejected the sign-in exchange. Please try again.',
  authorize_failed: 'Your identity could not be authorized for this instance. Please try again.',
  session_failed: 'Could not start a session. Please try again.',
  provider_error: 'The identity provider reported an error during sign-in.',
  forbidden: 'Your account is not in an allowed group or email for this instance.',
  state: 'The sign-in request expired or was tampered with. Please try again.',
  oidc_not_configured: 'Single sign-on is not configured on this instance.',
};

/** Login is the unauthenticated landing page. What it renders depends on
 * the server's configured auth mode (fetched via useAuthMode, since this
 * page by definition has no session yet to ask /api/v1/me instead):
 * oidc gets the "Sign in with SSO" action below; token and forward_auth
 * have no login action at all, so they get an explanatory message
 * instead of a dead-end button; open mode never needed a login page, so
 * it redirects to /. */
export function Login() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const errorCode = params.get('error');
  const [dialogOpen, setDialogOpen] = useState(false);
  const authMode = useAuthMode();

  const from = (location.state as { from?: string } | null)?.from ?? '/';
  const startUrl = `/auth/oidc/start?return_to=${encodeURIComponent(from)}`;

  if (authMode.isLoading) return null;
  if (authMode.data === 'open') return <Navigate to="/" replace />;

  return (
    <div className="flex min-h-screen items-center justify-center bg-app px-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle className="text-center text-lg">speedtest-tracker</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4">
          {authMode.data === 'oidc' && (
            <Button asChild>
              <a href={startUrl}>Sign in with SSO</a>
            </Button>
          )}
          {authMode.data === 'token' && (
            <p className="text-center text-sm text-muted">
              This instance requires an API token. Configure one in your client, or ask an admin.
            </p>
          )}
          {authMode.data === 'forward_auth' && (
            <p className="text-center text-sm text-muted">
              Your reverse proxy did not send an identity header. Check the proxy configuration.
            </p>
          )}
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
