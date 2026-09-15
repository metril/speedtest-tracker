import { Button } from '@/components/ui/button';
import { ApiError } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import { useIperf3Servers, useRefreshIperf3Servers } from '../../lib/queries';

function message(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Request failed.';
}

/** Iperf3ServerListSection is the SettingsCard footer for the cached
 * public iperf3 server list: last-refreshed time and count, with a
 * button to refresh it now. */
export function Iperf3ServerListSection() {
  const list = useIperf3Servers('', true);
  const refresh = useRefreshIperf3Servers();

  return (
    <>
      <p className="text-sm text-muted">
        Last refreshed{' '}
        {list.data?.fetched_at ? formatDateTime(list.data.fetched_at) : 'never'}
        {' · '}
        {list.data?.total ?? 0} servers
      </p>
      <Button type="button" variant="outline" disabled={refresh.isPending}
        onClick={() => refresh.mutate()}>
        {refresh.isPending ? 'Refreshing…' : 'Refresh'}
      </Button>
      {refresh.isError && <p role="alert" className="text-sm text-bad">{message(refresh.error)}</p>}
      {refresh.isSuccess && (
        <p className="text-sm text-ok">
          Refreshed {formatDateTime(refresh.data.fetched_at)} · {refresh.data.count} servers
        </p>
      )}
    </>
  );
}
