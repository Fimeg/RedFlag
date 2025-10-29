import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import {
  RateLimitConfig,
  RateLimitStats,
  RateLimitUsage,
  RateLimitSummary
} from '@/types';

// Query keys
export const rateLimitKeys = {
  all: ['rate-limits'] as const,
  configs: () => [...rateLimitKeys.all, 'configs'] as const,
  stats: () => [...rateLimitKeys.all, 'stats'] as const,
  usage: () => [...rateLimitKeys.all, 'usage'] as const,
  summary: () => [...rateLimitKeys.all, 'summary'] as const,
};

// Hooks
export const useRateLimitConfigs = () => {
  return useQuery({
    queryKey: rateLimitKeys.configs(),
    queryFn: () => adminApi.rateLimits.getConfigs(),
    staleTime: 1000 * 60 * 5, // 5 minutes
  });
};

export const useRateLimitStats = () => {
  return useQuery({
    queryKey: rateLimitKeys.stats(),
    queryFn: () => adminApi.rateLimits.getStats(),
    staleTime: 1000 * 30, // 30 seconds
    refetchInterval: 1000 * 30, // Refresh every 30 seconds for real-time monitoring
  });
};

export const useRateLimitUsage = () => {
  return useQuery({
    queryKey: rateLimitKeys.usage(),
    queryFn: () => adminApi.rateLimits.getUsage(),
    staleTime: 1000 * 15, // 15 seconds
    refetchInterval: 1000 * 15, // Refresh every 15 seconds for live usage
  });
};

export const useRateLimitSummary = () => {
  return useQuery({
    queryKey: rateLimitKeys.summary(),
    queryFn: () => adminApi.rateLimits.getSummary(),
    staleTime: 1000 * 60, // 1 minute
    refetchInterval: 1000 * 60, // Refresh every minute
  });
};

export const useUpdateRateLimitConfig = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ endpoint, config }: { endpoint: string; config: Partial<RateLimitConfig> }) =>
      adminApi.rateLimits.updateConfig(endpoint, config),
    onSuccess: (_, { endpoint }) => {
      toast.success(`Rate limit configuration for ${endpoint} updated successfully`);
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.configs() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.usage() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.summary() });
    },
    onError: (error: any, { endpoint }) => {
      console.error(`Failed to update rate limit config for ${endpoint}:`, error);
      toast.error(error.response?.data?.message || `Failed to update rate limit configuration for ${endpoint}`);
    },
  });
};

export const useUpdateAllRateLimitConfigs = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (configs: RateLimitConfig[]) =>
      adminApi.rateLimits.updateAllConfigs(configs),
    onSuccess: () => {
      toast.success('All rate limit configurations updated successfully');
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.configs() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.usage() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.summary() });
    },
    onError: (error: any) => {
      console.error('Failed to update rate limit configurations:', error);
      toast.error(error.response?.data?.message || 'Failed to update rate limit configurations');
    },
  });
};

export const useResetRateLimitConfigs = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => adminApi.rateLimits.resetConfigs(),
    onSuccess: () => {
      toast.success('Rate limit configurations reset to defaults successfully');
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.configs() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.usage() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.summary() });
    },
    onError: (error: any) => {
      console.error('Failed to reset rate limit configurations:', error);
      toast.error(error.response?.data?.message || 'Failed to reset rate limit configurations');
    },
  });
};

export const useCleanupRateLimits = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => adminApi.rateLimits.cleanup(),
    onSuccess: (result) => {
      toast.success(`Cleaned up ${result.cleaned} expired rate limit entries`);
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.usage() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.summary() });
    },
    onError: (error: any) => {
      console.error('Failed to cleanup rate limits:', error);
      toast.error(error.response?.data?.message || 'Failed to cleanup rate limits');
    },
  });
};