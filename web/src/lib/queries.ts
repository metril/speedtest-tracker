import {
  useInfiniteQuery, useMutation, useQuery, useQueryClient,
} from '@tanstack/react-query';
import * as api from './api';
import type { ResultFilters, TargetInput } from './api';

export const queryKeys = {
  targets: ['targets'] as const,
  results: (filters: ResultFilters) => ['results', filters] as const,
  ooklaServers: (q: string) => ['ookla-servers', q] as const,
  tags: ['tags'] as const,
  runs: ['runs'] as const,
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
