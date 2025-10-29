import { useQuery, useQueryClient } from '@tanstack/react-query';
import { agentApi } from '@/lib/api';
import type { UseQueryResult } from '@tanstack/react-query';

export interface HeartbeatStatus {
  enabled: boolean;
  until: string | null;
  active: boolean;
  duration_minutes: number;
}

export const useHeartbeatStatus = (agentId: string, enabled: boolean = true): UseQueryResult<HeartbeatStatus, Error> => {
  const queryClient = useQueryClient();

  return useQuery({
    queryKey: ['heartbeat', agentId],
    queryFn: () => agentApi.getHeartbeatStatus(agentId),
    enabled: enabled && !!agentId,
    staleTime: 5000, // Consider data stale after 5 seconds
    refetchInterval: (query) => {
      // Smart polling: only poll when heartbeat is active
      const data = query.state.data as HeartbeatStatus | undefined;

      // If heartbeat is enabled and still active, poll every 5 seconds
      if (data?.enabled && data?.active) {
        return 5000; // 5 seconds
      }

      // If heartbeat is not active, don't poll
      return false;
    },
    refetchOnWindowFocus: false, // Don't refresh when window gains focus
    refetchOnMount: true, // Always refetch when component mounts
  });
};

// Hook to manually invalidate heartbeat cache (used after commands)
export const useInvalidateHeartbeat = () => {
  const queryClient = useQueryClient();

  return (agentId: string) => {
    // Invalidate heartbeat cache
    queryClient.invalidateQueries({ queryKey: ['heartbeat', agentId] });

    // Also invalidate agent cache to synchronize data
    queryClient.invalidateQueries({ queryKey: ['agent', agentId] });
    queryClient.invalidateQueries({ queryKey: ['agents'] });
  };
};

// Hook to synchronize agent data when heartbeat status changes
export const useHeartbeatAgentSync = (agentId: string, heartbeatStatus?: HeartbeatStatus) => {
  const queryClient = useQueryClient();

  // Sync agent data when heartbeat status changes
  return () => {
    if (agentId && heartbeatStatus) {
      // Invalidate agent cache to get updated last_seen and status
      queryClient.invalidateQueries({ queryKey: ['agent', agentId] });
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    }
  };
};