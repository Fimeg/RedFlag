import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { agentApi } from '@/lib/api';
import { Agent, ListQueryParams } from '@/types';
import { useAgentStore, useRealtimeStore } from '@/lib/store';
import { handleApiError } from '@/lib/api';

export const useAgents = (params?: ListQueryParams) => {
  const { setAgents, setLoading, setError, updateAgentStatus } = useAgentStore();

  return useQuery({
    queryKey: ['agents', params],
    queryFn: () => agentApi.getAgents(params),
    onSuccess: (data) => {
      setAgents(data.agents);
      setLoading(false);
      setError(null);
    },
    onError: (error) => {
      setError(handleApiError(error).message);
      setLoading(false);
    },
    onSettled: () => {
      setLoading(false);
    },
  });
};

export const useAgent = (id: string, enabled: boolean = true) => {
  const { setSelectedAgent, setLoading, setError } = useAgentStore();

  return useQuery({
    queryKey: ['agent', id],
    queryFn: () => agentApi.getAgent(id),
    enabled: enabled && !!id,
    onSuccess: (data) => {
      setSelectedAgent(data);
      setLoading(false);
      setError(null);
    },
    onError: (error) => {
      setError(handleApiError(error).message);
      setLoading(false);
    },
  });
};

export const useScanAgent = () => {
  const queryClient = useQueryClient();
  const { addNotification } = useRealtimeStore();

  return useMutation({
    mutationFn: agentApi.scanAgent,
    onSuccess: () => {
      // Invalidate agents query to refresh data
      queryClient.invalidateQueries({ queryKey: ['agents'] });

      // Show success notification
      addNotification({
        type: 'success',
        title: 'Scan Triggered',
        message: 'Agent scan has been triggered successfully.',
      });
    },
    onError: (error) => {
      addNotification({
        type: 'error',
        title: 'Scan Failed',
        message: handleApiError(error).message,
      });
    },
  });
};

export const useScanMultipleAgents = () => {
  const queryClient = useQueryClient();
  const { addNotification } = useRealtimeStore();

  return useMutation({
    mutationFn: agentApi.triggerScan,
    onSuccess: () => {
      // Invalidate agents query to refresh data
      queryClient.invalidateQueries({ queryKey: ['agents'] });

      // Show success notification
      addNotification({
        type: 'success',
        title: 'Bulk Scan Triggered',
        message: 'Scan has been triggered for selected agents.',
      });
    },
    onError: (error) => {
      addNotification({
        type: 'error',
        title: 'Bulk Scan Failed',
        message: handleApiError(error).message,
      });
    },
  });
};