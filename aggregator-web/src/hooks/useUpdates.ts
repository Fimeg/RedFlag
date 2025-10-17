import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { updateApi } from '@/lib/api';
import type { UpdatePackage, ListQueryParams, UpdateApprovalRequest, UpdateListResponse } from '@/types';
import type { UseQueryResult, UseMutationResult } from '@tanstack/react-query';

export const useUpdates = (params?: ListQueryParams): UseQueryResult<UpdateListResponse, Error> => {
  return useQuery({
    queryKey: ['updates', params],
    queryFn: () => updateApi.getUpdates(params),
  });
};

export const useUpdate = (id: string, enabled: boolean = true): UseQueryResult<UpdatePackage, Error> => {
  return useQuery({
    queryKey: ['update', id],
    queryFn: () => updateApi.getUpdate(id),
    enabled: enabled && !!id,
  });
};

export const useApproveUpdate = (): UseMutationResult<void, Error, { id: string; scheduledAt?: string; }, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, scheduledAt }: { id: string; scheduledAt?: string }) =>
      updateApi.approveUpdate(id, scheduledAt),
    onSuccess: () => {
      // Invalidate all updates queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      // Also invalidate specific update queries
      queryClient.invalidateQueries({ queryKey: ['update'] });
    },
  });
};

export const useApproveMultipleUpdates = (): UseMutationResult<void, Error, UpdateApprovalRequest, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (request: UpdateApprovalRequest) => updateApi.approveUpdates(request),
    onSuccess: () => {
      // Invalidate all updates queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      // Also invalidate specific update queries
      queryClient.invalidateQueries({ queryKey: ['update'] });
    },
  });
};

export const useRejectUpdate = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.rejectUpdate,
    onSuccess: () => {
      // Invalidate all updates queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      // Also invalidate specific update queries
      queryClient.invalidateQueries({ queryKey: ['update'] });
    },
  });
};

export const useInstallUpdate = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.installUpdate,
    onSuccess: () => {
      // Invalidate all updates queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      // Also invalidate specific update queries
      queryClient.invalidateQueries({ queryKey: ['update'] });
    },
  });
};

export const useRetryCommand = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.retryCommand,
    onSuccess: () => {
      // Invalidate all updates queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      // Also invalidate logs and active operations queries
      queryClient.invalidateQueries({ queryKey: ['logs'] });
      queryClient.invalidateQueries({ queryKey: ['active'] });
    },
  });
};

export const useCancelCommand = (): UseMutationResult<void, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.cancelCommand,
    onSuccess: () => {
      // Invalidate all updates queries to trigger refetch
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      // Also invalidate logs and active operations queries
      queryClient.invalidateQueries({ queryKey: ['logs'] });
      queryClient.invalidateQueries({ queryKey: ['active'] });
    },
  });
};