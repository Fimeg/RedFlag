import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { agentApi } from '@/lib/api';
import type { Agent, ListQueryParams, AgentListResponse, ScanRequest } from '@/types';
import type { UseQueryResult, UseMutationResult } from '@tanstack/react-query';

export const useAgents = (params?: ListQueryParams): UseQueryResult<AgentListResponse, Error> => {
  return useQuery({
    queryKey: ['agents', params],
    queryFn: () => agentApi.getAgents(params),
    staleTime: 30 * 1000, // Consider data fresh for 30 seconds
    refetchInterval: 60 * 1000, // Poll every 60 seconds
    refetchIntervalInBackground: false, // Don't poll when tab is inactive
    refetchOnWindowFocus: true, // Refresh when window gains focus
  });
};

export const useAgent = (id: string, enabled: boolean = true): UseQueryResult<Agent, Error> => {
  return useQuery({
    queryKey: ['agent', id],
    queryFn: () => agentApi.getAgent(id),
    enabled: enabled && !!id,
    staleTime: 30 * 1000, // Consider data fresh for 30 seconds
    refetchInterval: 30 * 1000, // Poll every 30 seconds for selected agent
    refetchIntervalInBackground: false, // Don't poll when tab is inactive
    refetchOnWindowFocus: true, // Refresh when window gains focus
  });
};

export const useScanAgent = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: agentApi.scanAgent,
    onSuccess: () => {
      // Invalidate all agents queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      // Also invalidate specific agent queries
      queryClient.invalidateQueries({ queryKey: ['agent'] });
    },
  });
};

export const useScanMultipleAgents = (): UseMutationResult<void, Error, ScanRequest, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: agentApi.triggerScan,
    onSuccess: () => {
      // Invalidate all agents queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      // Also invalidate specific agent queries
      queryClient.invalidateQueries({ queryKey: ['agent'] });
    },
  });
};

export const useUnregisterAgent = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: agentApi.unregisterAgent,
    onSuccess: () => {
      // Invalidate all agents queries to trigger immediate refetch
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      // Also invalidate specific agent queries
      queryClient.invalidateQueries({ queryKey: ['agent'] });
    },
  });
};