import {
  useInfiniteQuery, useMutation, useQuery, useQueryClient,
} from '@tanstack/react-query';
import * as api from './api';
import type {
  Range, ResultFilters, ScheduleInput, TargetInput,
} from './api';

export const queryKeys = {
  targets: ['targets'] as const,
  results: (filters: ResultFilters) => ['results', filters] as const,
  ooklaServers: (q: string) => ['ookla-servers', q] as const,
  tags: ['tags'] as const,
  runs: ['runs'] as const,
  schedules: ['schedules'] as const,
  cronPreview: (cron: string, timezone: string) => ['cron-preview', cron, timezone] as const,
  targetLatest: (id: number) => ['target-latest', id] as const,
  history: (id: number, range: Range) => ['history', id, range] as const,
  summary: (range: Range) => ['summary', range] as const,
  outages: (range: Range) => ['outages', range] as const,
};

export function useTargets() {
  return useQuery({ queryKey: queryKeys.targets, queryFn: api.listTargets });
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
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.targets }),
  });
}

export function useRunTarget() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.runTarget(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.runs }),
  });
}

export function useOoklaServers(q: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.ooklaServers(q),
    queryFn: () => api.listOoklaServers(q),
    enabled,
    staleTime: 60 * 60 * 1000,
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

/** useTargetLatest is the per-target latest result. Live `result` SSE
 * events patch this cache entry directly (see LiveRunProvider) instead of
 * forcing a refetch. */
export function useTargetLatest(id: number) {
  return useQuery({
    queryKey: queryKeys.targetLatest(id),
    queryFn: () => api.targetLatest(id),
  });
}

export function useSummary(range: Range) {
  return useQuery({
    queryKey: queryKeys.summary(range),
    queryFn: () => api.statsSummary(range),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
}

export function useTargetHistory(id: number, range: Range, enabled = true) {
  return useQuery({
    queryKey: queryKeys.history(id, range),
    queryFn: () => api.targetHistory(id, range),
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
