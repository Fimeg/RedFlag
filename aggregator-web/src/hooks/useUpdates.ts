import { useQuery, useMutation } from '@tanstack/react-query';
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
  return useMutation({
    mutationFn: ({ id, scheduledAt }: { id: string; scheduledAt?: string }) =>
      updateApi.approveUpdate(id, scheduledAt),
  });
};

export const useApproveMultipleUpdates = (): UseMutationResult<void, Error, UpdateApprovalRequest, unknown> => {
  return useMutation({
    mutationFn: (request: UpdateApprovalRequest) => updateApi.approveUpdates(request),
  });
};

export const useRejectUpdate = (): UseMutationResult<void, Error, string, unknown> => {
  return useMutation({
    mutationFn: updateApi.rejectUpdate,
  });
};

export const useInstallUpdate = (): UseMutationResult<void, Error, string, unknown> => {
  return useMutation({
    mutationFn: updateApi.installUpdate,
  });
};