import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { updateApi, agentApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import type { UseQueryResult, UseMutationResult } from '@tanstack/react-query';

interface ActiveCommand {
  id: string;
  agent_id: string;
  agent_hostname: string;
  command_type: string;
  status: string;
  created_at: string;
  sent_at?: string;
  completed_at?: string;
  package_name: string;
  package_type: string;
}

export const useActiveCommands = (autoRefresh: boolean = true): UseQueryResult<{ commands: ActiveCommand[]; count: number }, Error> => {
  return useQuery({
    queryKey: ['activeCommands'],
    queryFn: () => updateApi.getActiveCommands(),
    refetchInterval: autoRefresh ? POLL.LIVE : false,
    staleTime: 0,
  });
};

export const useRecentCommands = (limit?: number): UseQueryResult<{ commands: ActiveCommand[]; count: number; limit: number }, Error> => {
  return useQuery({
    queryKey: ['recentCommands', limit],
    queryFn: () => updateApi.getRecentCommands(limit),
  });
};

// Canonical owner of the command retry/cancel mutations. useUpdates re-exports
// these so the Updates page and LiveOperations share one invalidation set —
// a retry/cancel from either surface must refresh both views plus dashboard
// counts. (Previously a divergent copy in useUpdates invalidated a dead
// ['active'] key and skipped activeCommands, so LiveOperations went stale.)
const invalidateCommandViews = (queryClient: ReturnType<typeof useQueryClient>) => {
  queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
  queryClient.invalidateQueries({ queryKey: ['recentCommands'] });
  queryClient.invalidateQueries({ queryKey: ['updates'] });
  queryClient.invalidateQueries({ queryKey: ['update'] });
  queryClient.invalidateQueries({ queryKey: ['logs'] });
  queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
};

export const useRetryCommand = (): UseMutationResult<{ message: string; command_id: string; new_id: string }, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.retryCommand,
    onSuccess: () => invalidateCommandViews(queryClient),
  });
};

export const useCancelCommand = (): UseMutationResult<{ message: string }, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.cancelCommand,
    onSuccess: () => invalidateCommandViews(queryClient),
  });
};

export const useClearFailedCommands = (): UseMutationResult<{ message: string; count: number; cheeky_warning?: string }, Error, {
  olderThanDays?: number;
  onlyRetried?: boolean;
  allFailed?: boolean;
} | undefined, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.clearFailedCommands,
    onSuccess: () => {
      // Invalidate active and recent commands queries to refresh the UI
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
      queryClient.invalidateQueries({ queryKey: ['recentCommands'] });
    },
  });
};

// Trigger a screenshot capture on an agent. Returns the command ID.
export const useCaptureScreenshot = (): UseMutationResult<{ message: string; command_id: string }, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (agentId: string) => agentApi.captureScreenshot(agentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
    },
  });
};

// Poll a specific command until it completes. Used to retrieve the
// screenshot image from the command result's stdout field.
export const useCommand = (commandId: string | null, enabled: boolean = true): UseQueryResult<any, Error> => {
  return useQuery({
    queryKey: ['command', commandId],
    queryFn: () => agentApi.getCommand(commandId!),
    enabled: !!commandId && enabled,
    refetchInterval: (query: any) => {
      // Stop polling once the command reaches a terminal state
      const status = query.state.data?.status;
      if (status === 'completed' || status === 'failed' || status === 'timed_out' || status === 'cancelled') {
        return false;
      }
      return 2000; // Poll every 2 seconds while in flight
    },
    staleTime: 0,
  });
};