import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { updateApi, logApi } from '@/lib/api';
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

export const useActiveCommands = (): UseQueryResult<{ commands: ActiveCommand[]; count: number }, Error> => {
  return useQuery({
    queryKey: ['activeCommands'],
    queryFn: () => updateApi.getActiveCommands(),
    refetchInterval: 5000, // Auto-refresh every 5 seconds
  });
};

export const useRecentCommands = (limit?: number): UseQueryResult<{ commands: ActiveCommand[]; count: number; limit: number }, Error> => {
  return useQuery({
    queryKey: ['recentCommands', limit],
    queryFn: () => updateApi.getRecentCommands(limit),
  });
};

export const useRetryCommand = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.retryCommand,
    onSuccess: () => {
      // Invalidate active and recent commands queries
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
      queryClient.invalidateQueries({ queryKey: ['recentCommands'] });
    },
  });
};

export const useCancelCommand = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.cancelCommand,
    onSuccess: () => {
      // Invalidate active and recent commands queries
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
      queryClient.invalidateQueries({ queryKey: ['recentCommands'] });
    },
  });
};