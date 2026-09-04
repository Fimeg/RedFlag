import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { agentApi } from '@/lib/api';
import type { ProcessSnapshotResponse, ProcessDetailResponse, ProcessFilter } from '@/types/process';

// Fetch the latest process snapshot for an agent.
export function useProcessSnapshot(agentId: string, filter?: ProcessFilter) {
  return useQuery<ProcessSnapshotResponse>({
    queryKey: ['process-snapshot', agentId, filter],
    queryFn: () => agentApi.getLatestProcessSnapshot(agentId, filter as Record<string, string>),
    staleTime: 24 * 60 * 60 * 1000, // 24h — data only changes on explicit scan
    refetchInterval: false,
    enabled: !!agentId,
  });
}

// Trigger an on-demand process scan (mutation).
export function useTriggerProcessScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (agentId: string) => agentApi.triggerProcessScan(agentId),
    onSuccess: (_data, agentId) => {
      // Invalidate snapshot so it refetches after scan completes
      queryClient.invalidateQueries({ queryKey: ['process-snapshot', agentId] });
    },
  });
}

// Fetch process detail with related data (for drill-down modal).
export function useProcessDetail(agentId: string, processId: string, enabled: boolean) {
  return useQuery<ProcessDetailResponse>({
    queryKey: ['process-detail', agentId, processId],
    queryFn: () => agentApi.getProcessDetail(agentId, processId),
    enabled: enabled && !!agentId && !!processId,
    staleTime: 24 * 60 * 60 * 1000,
  });
}
