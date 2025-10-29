import { useQuery, useMutation } from '@tanstack/react-query';
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
  return useMutation({
    mutationFn: agentApi.scanAgent,
  });
};

export const useScanMultipleAgents = (): UseMutationResult<void, Error, ScanRequest, unknown> => {
  return useMutation({
    mutationFn: agentApi.triggerScan,
  });
};

export const useUnregisterAgent = (): UseMutationResult<void, Error, string, unknown> => {
  return useMutation({
    mutationFn: agentApi.unregisterAgent,
  });
};