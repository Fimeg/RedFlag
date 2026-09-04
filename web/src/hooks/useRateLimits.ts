import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import { RateLimitSettings } from '@/types';

export const rateLimitKeys = {
  all: ['rate-limits'] as const,
  settings: () => [...rateLimitKeys.all, 'settings'] as const,
  stats: () => [...rateLimitKeys.all, 'stats'] as const,
};

export const useRateLimitSettings = () => {
  return useQuery({
    queryKey: rateLimitKeys.settings(),
    queryFn: () => adminApi.rateLimits.getSettings(),
    staleTime: POLL.STATIC * 0.2,
  });
};

export const useRateLimitStats = () => {
  return useQuery({
    queryKey: rateLimitKeys.stats(),
    queryFn: () => adminApi.rateLimits.getStats(),
    staleTime: POLL.DETAIL * 0.5,
    refetchInterval: POLL.DETAIL,
  });
};

export const useUpdateRateLimitSettings = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (settings: RateLimitSettings) =>
      adminApi.rateLimits.updateSettings(settings),
    onSuccess: () => {
      toast.success('Rate limit settings saved');
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.settings() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to update rate limit settings:', error);
      toast.error(error.response?.data?.error || 'Failed to update rate limit settings');
    },
  });
};

export const useResetRateLimitSettings = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => adminApi.rateLimits.resetSettings(),
    onSuccess: () => {
      toast.success('Rate limit settings reset to defaults');
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.settings() });
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to reset rate limit settings:', error);
      toast.error(error.response?.data?.error || 'Failed to reset rate limit settings');
    },
  });
};

export const useCleanupRateLimits = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => adminApi.rateLimits.cleanup(),
    onSuccess: () => {
      toast.success('Rate limit entries cleanup completed');
      queryClient.invalidateQueries({ queryKey: rateLimitKeys.stats() });
    },
    onError: (error: any) => {
      console.error('Failed to cleanup rate limits:', error);
      toast.error(error.response?.data?.error || 'Failed to cleanup rate limits');
    },
  });
};
