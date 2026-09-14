import { Link } from 'react-router';
import { useMe } from '../lib/queries';

/** OpenModeBanner renders a persistent, non-dismissible strip while the
 * instance has no authentication at all -- the default on a fresh
 * deployment, and the state most likely to be wrong in production. */
export function OpenModeBanner() {
  const me = useMe();
  if (me.data?.mode !== 'open') return null;

  return (
    <div role="status" className="border-b border-line bg-raised px-4 py-2 text-sm text-warn">
      This instance has no authentication — anyone who can reach it can change settings and run tests.{' '}
      <Link to="/settings" className="underline">Configure auth</Link>
    </div>
  );
}
