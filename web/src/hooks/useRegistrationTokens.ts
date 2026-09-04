import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import {
  CreateRegistrationTokenRequest,
} from '@/types';

// Query keys
export const registrationTokenKeys = {
  all: ['registration-tokens'] as const,
  lists: () => [...registrationTokenKeys.all, 'list'] as const,
  list: (params: any) => [...registrationTokenKeys.lists(), params] as const,
  details: () => [...registrationTokenKeys.all, 'detail'] as const,
  detail: (id: string) => [...registrationTokenKeys.details(), id] as const,
  stats: () => [...registrationTokenKeys.all, 'stats'] as const,
  boundAgents: (tokenId: string) => [...registrationTokenKeys.all, 'bound-agents', tokenId] as const,
};


// Hooks
export const useRegistrationTokens = (params?: {
  page?: number;
  page_size?: number;
  is_active?: boolean;
  label?: string;
}) => {
  return useQuery({
    queryKey: registrationTokenKeys.list(params),
    queryFn: () => adminApi.tokens.getTokens(params),
    staleTime: 1000 * 60, // 1 minute
  });
};

export const useRegistrationToken = (id: string) => {
  return useQuery({
    queryKey: registrationTokenKeys.detail(id),
    queryFn: () => adminApi.tokens.getToken(id),
    enabled: !!id,
    staleTime: 1000 * 60, // 1 minute
  });
};

export const useRegistrationTokenStats = () => {
  return useQuery({
    queryKey: registrationTokenKeys.stats(),
    queryFn: () => adminApi.tokens.getStats(),
    staleTime: POLL.STATIC * 0.2,
    refetchInterval: POLL.STATIC,
  });
};

export const useCreateRegistrationToken = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateRegistrationTokenRequest) =>
      adminApi.tokens.createToken(data),
    onSuccess: (newToken) => {
      toast.success(`Registration token "${newToken.label}" created successfully`);
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.lists() });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to create registration token:', error);
      toast.error(error.response?.data?.message || 'Failed to create registration token');
    },
  });
};

export const useRevokeRegistrationToken = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => adminApi.tokens.revokeToken(id),
    onSuccess: (_, tokenId) => {
      toast.success('Registration token revoked successfully');
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.lists() });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.detail(tokenId) });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to revoke registration token:', error);
      toast.error(error.response?.data?.message || 'Failed to revoke registration token');
    },
  });
};

export const useDeleteRegistrationToken = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => adminApi.tokens.deleteToken(id),
    onSuccess: (_, tokenId) => {
      toast.success('Registration token deleted successfully');
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.lists() });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.detail(tokenId) });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to delete registration token:', error);
      toast.error(error.response?.data?.message || 'Failed to delete registration token');
    },
  });
};

export const useCleanupRegistrationTokens = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => adminApi.tokens.cleanup(),
    onSuccess: (result) => {
      toast.success(`Cleaned up ${result.cleaned} expired tokens`);
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.lists() });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to cleanup registration tokens:', error);
      toast.error(error.response?.data?.message || 'Failed to cleanup registration tokens');
    },
  });
};

// useBoundAgents fetches agents that enrolled with a specific registration token.
// Enabled only when tokenId is non-empty so it doesn't fire on empty selection.
export const useBoundAgents = (tokenId: string) => {
  return useQuery({
    queryKey: registrationTokenKeys.boundAgents(tokenId),
    queryFn: () => adminApi.tokens.getBoundAgents(tokenId),
    enabled: !!tokenId,
    staleTime: 1000 * 30,
  });
};

// useRevokeAgent invalidates an agent's refresh tokens (explicit per-agent action,
// not a side effect of token revocation). Invalidates: bound-agents for the
// selected token, token list, and stats so seat counts refresh.
export const useRevokeAgent = (selectedTokenId?: string) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ agentId, reason }: { agentId: string; reason?: string }) =>
      adminApi.agents.revoke(agentId, reason),
    onSuccess: () => {
      toast.success('Agent revoked');
      if (selectedTokenId) {
        queryClient.invalidateQueries({ queryKey: registrationTokenKeys.boundAgents(selectedTokenId) });
      }
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.lists() });
      queryClient.invalidateQueries({ queryKey: registrationTokenKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to revoke agent:', error);
      toast.error(error.response?.data?.error || 'Failed to revoke agent');
    },
  });
};