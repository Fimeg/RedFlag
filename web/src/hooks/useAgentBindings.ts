import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import { CreateAgentBindingRequest } from '@/types';
import { upstreamKeys } from './useUpstream';

// Per-agent binding cache. Cross-invalidates the global upstream keys
// because a binding upsert/delete recomputes tracked_software.current_version,
// which the StackDriftPanel and UpstreamTracking settings page read.
export const agentBindingKeys = {
  all: ['agent-bindings'] as const,
  byAgent: (agentId: string) => [...agentBindingKeys.all, agentId] as const,
};

export const useAgentBindings = (agentId: string, enabled: boolean = true) => {
  return useQuery({
    queryKey: agentBindingKeys.byAgent(agentId),
    queryFn: () => adminApi.bindings.listByAgent(agentId),
    enabled: enabled && !!agentId,
    staleTime: 1000 * 30,
  });
};

export const useUpsertAgentBinding = (agentId: string) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateAgentBindingRequest) => adminApi.bindings.upsert(agentId, input),
    onSuccess: () => {
      toast.success('Binding saved');
      qc.invalidateQueries({ queryKey: agentBindingKeys.byAgent(agentId) });
      qc.invalidateQueries({ queryKey: upstreamKeys.all });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || 'Failed to save binding');
    },
  });
};

export const useDeleteAgentBinding = (agentId: string) => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (bindingId: string) => adminApi.bindings.remove(agentId, bindingId),
    onSuccess: () => {
      toast.success('Binding removed');
      qc.invalidateQueries({ queryKey: agentBindingKeys.byAgent(agentId) });
      qc.invalidateQueries({ queryKey: upstreamKeys.all });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || 'Failed to remove binding');
    },
  });
};
