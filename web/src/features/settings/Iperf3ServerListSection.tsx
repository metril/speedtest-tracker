import { Button } from '@/components/ui/button';
import { ApiError } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import { useIperf3Servers, useRefreshIperf3Servers } from '../../lib/queries';

function message(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Request failed.';
}

/** Iperf3ServerListSection shows the cached public iperf3 server list's
 * last-refreshed time and count, with a button to refresh it now. */
export function Iperf3ServerListSection() {
  const list = useIperf3Servers('', true);
  const refresh = useRefreshIperf3Servers();

  return (
    <div className="grid gap-3">
      <p className="text-sm text-faint">
        Last refreshed{' '}
        {list.data?.fetched_at ? formatDateTime(list.data.fetched_at) : 'never'}
        {' · '}
        {list.data?.total ?? 0} servers
      </p>
      <div>
        <Button type="button" variant="outline" disabled={refresh.isPending}
          onClick={() => refresh.mutate()}>
          {refresh.isPending ? 'Refreshing…' : 'Refresh'}
        </Button>
      </div>
      {refresh.isError && <p role="alert" className="text-sm text-bad">{message(refresh.error)}</p>}
      {refresh.isSuccess && (
        <p className="text-sm text-ok">
          Refreshed {formatDateTime(refresh.data.fetched_at)} · {refresh.data.count} servers
        </p>
      )}
    </div>
  );
}
