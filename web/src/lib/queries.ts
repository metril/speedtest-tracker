import {
  useInfiniteQuery, useMutation, useQuery, useQueryClient,
} from '@tanstack/react-query';
import * as api from './api';
import { ApiError } from './api';
import type {
  QueueInput, Range, ResultFilters, ScheduleInput, TargetInput,
} from './api';

export const queryKeys = {
  targets: ['targets'] as const,
  queues: ['queues'] as const,
  results: (filters: ResultFilters) => ['results', filters] as const,
  ooklaServers: (q: string, country?: string) => ['ookla-servers', q, country ?? ''] as const,
  iperf3Servers: (q: string) => ['iperf3-servers', q] as const,
  tags: ['tags'] as const,
  runs: ['runs'] as const,
  schedules: ['schedules'] as const,
  cronPreview: (cron: string, timezone: string) => ['cron-preview', cron, timezone] as const,
  history: (id: number, range: Range, offset?: 0 | 1) => ['history', id, range, offset ?? 0] as const,
  targetRevisions: (id: number) => ['target-revisions', id] as const,
  deletedTargets: ['deleted-targets'] as const,
  summary: (range: Range, offset?: 0 | 1) => ['summary', range, offset ?? 0] as const,
  outages: (range: Range) => ['outages', range] as const,
  settings: ['settings'] as const,
  me: ['me'] as const,
  tokens: ['tokens'] as const,
  authMode: ['auth-mode'] as const,
};

export function useTargets() {
  return useQuery({ queryKey: queryKeys.targets, queryFn: api.listTargets });
}

export function useQueues(enabled = true) {
  return useQuery({ queryKey: queryKeys.queues, queryFn: api.listQueues, enabled });
}

export function useCreateQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (q: QueueInput) => api.createQueue(q),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.queues }),
  });
}

export function useRenameQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, queue }: { id: number; queue: QueueInput }) => api.renameQueue(id, queue),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.queues });
      qc.invalidateQueries({ queryKey: queryKeys.targets });
    },
  });
}

export function useDeleteQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteQueue(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.queues }),
  });
}

export function useCreateTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (t: TargetInput) => api.createTarget(t),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.targets }),
  });
}

export function useUpdateTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, target }: { id: number; target: TargetInput }) =>
      api.updateTarget(id, target),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.targets }),
  });
}

export function useDeleteTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteTarget(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.targets });
      qc.invalidateQueries({ queryKey: queryKeys.deletedTargets });
    },
  });
}

export function useRunTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.runTarget(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.runs }),
  });
}

export function useTargetRevisions(id: number, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.targetRevisions(id),
    queryFn: () => api.listTargetRevisions(id),
    enabled,
  });
}

export function useRevertTargetRevision() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, version }: { id: number; version: number }) =>
      api.revertTargetRevision(id, version),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: queryKeys.targets });
      qc.invalidateQueries({ queryKey: queryKeys.targetRevisions(id) });
    },
  });
}

export function useDeletedTargets() {
  return useQuery({ queryKey: queryKeys.deletedTargets, queryFn: api.listDeletedTargets });
}

export function useRestoreTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.restoreTarget(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.targets });
      qc.invalidateQueries({ queryKey: queryKeys.deletedTargets });
    },
  });
}

export function useOoklaServers(q: string, country: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.ooklaServers(q, country),
    queryFn: () => api.listOoklaServers(q, country),
    enabled,
    staleTime: 60 * 60 * 1000,
  });
}

export function useIperf3Servers(q: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.iperf3Servers(q),
    queryFn: () => api.listIperf3Servers(q),
    enabled,
    staleTime: 60 * 60 * 1000,
  });
}

export function useRefreshIperf3Servers() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.refreshIperf3Servers(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['iperf3-servers'] }),
  });
}

export function useResults(filters: ResultFilters) {
  return useInfiniteQuery({
    queryKey: queryKeys.results(filters),
    queryFn: ({ pageParam }) => api.listResults(filters, pageParam as string | undefined),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.next_cursor ? last.next_cursor : undefined),
  });
}

export function useDeleteResult() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteResult(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['results'] }),
  });
}

export function useReexecute() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.reexecuteResult(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.runs }),
  });
}

export function useSetTags() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, tags }: { id: number; tags: string[] }) => api.setResultTags(id, tags),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['results'] });
      qc.invalidateQueries({ queryKey: queryKeys.tags });
    },
  });
}

export function useTags() {
  return useQuery({ queryKey: queryKeys.tags, queryFn: api.listTags });
}

export function useCancelRun() {
  return useMutation({ mutationFn: (id: number) => api.cancelRun(id) });
}

export function useSchedules() {
  return useQuery({ queryKey: queryKeys.schedules, queryFn: api.listSchedules });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (s: ScheduleInput) => api.createSchedule(s),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.schedules }),
  });
}

export function useUpdateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, schedule }: { id: number; schedule: ScheduleInput }) =>
      api.updateSchedule(id, schedule),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.schedules }),
  });
}

export function useDeleteSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteSchedule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.schedules }),
  });
}

export function useRunSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.runSchedule(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.runs }),
  });
}

/** useCronPreview previews an expression's next fire times. It is disabled
 * until an expression exists and never retries: a parse error is an answer,
 * not a transient failure. */
export function useCronPreview(cron: string, timezone: string) {
  return useQuery({
    queryKey: queryKeys.cronPreview(cron, timezone),
    queryFn: () => api.validateCron(cron, timezone),
    enabled: cron.trim().length > 0,
    retry: false,
    staleTime: 30_000,
  });
}

export function useSummary(range: Range, opts?: { offset?: 0 | 1 }) {
  const offset = opts?.offset;
  return useQuery({
    queryKey: queryKeys.summary(range, offset),
    queryFn: () => api.statsSummary(range, offset),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

/** usePreviousSummary fetches the window immediately before `range`, for
 * computing a previous-period delta on the KPI tiles. Its own hook keeps
 * offset:1 an explicit intent at call sites rather than a magic option. */
export function usePreviousSummary(range: Range) {
  return useSummary(range, { offset: 1 });
}

export function useTargetHistory(id: number, range: Range, opts?: { enabled?: boolean; offset?: 0 | 1 }) {
  const enabled = opts?.enabled ?? true;
  const offset = opts?.offset;
  return useQuery({
    queryKey: queryKeys.history(id, range, offset),
    queryFn: () => api.targetHistory(id, range, offset),
    enabled,
    staleTime: 60_000,
  });
}

export function useOutages(range: Range) {
  return useQuery({
    queryKey: queryKeys.outages(range),
    queryFn: () => api.listOutages(range),
    staleTime: 60_000,
  });
}

export function useRenameTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) => api.renameTag(id, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tags });
      qc.invalidateQueries({ queryKey: ['results'] });
    },
  });
}

export function useDeleteTag() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteTag(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tags });
      qc.invalidateQueries({ queryKey: ['results'] });
    },
  });
}

export function useSettings() {
  return useQuery({
    queryKey: queryKeys.settings,
    queryFn: api.getSettings,
    refetchOnWindowFocus: false,
  });
}

export function useUpdateSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: api.SettingsPatch) => api.updateSettings(patch),
    onSuccess: (result) => {
      // Seed the cache with the mutation's own response instead of just
      // invalidating: a refetch may be slow or never resolve (offline,
      // request coalescing, etc.), and until it lands `settings.data`
      // would stay stale, making dirty-tracking compare the freshly
      // saved draft against the old server value -- a transient
      // false-dirty right after a successful save.
      qc.setQueryData(queryKeys.settings, result);
      qc.invalidateQueries({ queryKey: queryKeys.me });
    },
  });
}

/** useMe is the caller's identity under the active auth mode. It never
 * changes under a live session, so it is fetched once and rarely retried --
 * a 401 is an answer (not authenticated), not a transient error, so it never
 * retries; any other failure gets a couple of retries. */
export function useMe() {
  return useQuery({
    queryKey: queryKeys.me,
    queryFn: api.getMe,
    staleTime: Infinity,
    retry: (n, err) => !(err instanceof ApiError && err.status === 401) && n < 2,
  });
}

export function useLogout() {
  return useMutation({ mutationFn: api.logout });
}

/** useAuthMode is the server's configured auth mode, fetched by the
 * unauthenticated Login page to decide what to render (an SSO button,
 * a token/forward_auth hint, or a redirect to / for open mode). Unlike
 * useMe() it never 401s -- /auth/mode is public -- so no retry tuning is
 * needed. */
export function useAuthMode() {
  return useQuery({ queryKey: queryKeys.authMode, queryFn: api.authMode, staleTime: Infinity });
}

export function useTokens() {
  return useQuery({ queryKey: queryKeys.tokens, queryFn: api.listTokens });
}

export function useCreateToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.createToken(name),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.tokens }),
  });
}

export function useDeleteToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.deleteToken(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.tokens }),
  });
}

export function useTestIntegration() {
  return useMutation({
    mutationFn: ({ target, body }: { target: 'vm' | 'vl'; body: { url?: string; auth?: api.ExportAuth } }) =>
      api.testIntegration(target, body),
  });
}

export function useTestNotifyChannel() {
  return useMutation({ mutationFn: (channelId: string) => api.testNotifyChannel(channelId) });
}
