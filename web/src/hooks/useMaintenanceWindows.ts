import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import { CreateMaintenanceWindowRequest } from '@/types';

export const maintenanceWindowKeys = {
  all: ['maintenance-windows'] as const,
  lists: () => [...maintenanceWindowKeys.all, 'list'] as const,
  list: (params?: any) => [...maintenanceWindowKeys.lists(), params] as const,
  details: () => [...maintenanceWindowKeys.all, 'detail'] as const,
  detail: (id: string) => [...maintenanceWindowKeys.details(), id] as const,
  check: () => [...maintenanceWindowKeys.all, 'check'] as const,
};

export const useMaintenanceWindows = () => {
  return useQuery({
    queryKey: maintenanceWindowKeys.list(),
    queryFn: () => adminApi.maintenanceWindows.list(),
    staleTime: POLL.OVERVIEW * 0.5,
  });
};

export const useMaintenanceWindow = (id: string) => {
  return useQuery({
    queryKey: maintenanceWindowKeys.detail(id),
    queryFn: () => adminApi.maintenanceWindows.get(id),
    enabled: !!id,
    staleTime: POLL.OVERVIEW * 0.5,
  });
};

export const useMaintenanceWindowCheck = () => {
  return useQuery({
    queryKey: maintenanceWindowKeys.check(),
    queryFn: () => adminApi.maintenanceWindows.check(),
    staleTime: POLL.DETAIL * 0.5,
    refetchInterval: POLL.OVERVIEW,
  });
};

export const useCreateMaintenanceWindow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateMaintenanceWindowRequest) =>
      adminApi.maintenanceWindows.create(data),
    onSuccess: () => {
      toast.success('Maintenance window created');
      queryClient.invalidateQueries({ queryKey: maintenanceWindowKeys.lists() });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to create maintenance window');
    },
  });
};

export const useUpdateMaintenanceWindow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<CreateMaintenanceWindowRequest> }) =>
      adminApi.maintenanceWindows.update(id, data),
    onSuccess: () => {
      toast.success('Maintenance window updated');
      queryClient.invalidateQueries({ queryKey: maintenanceWindowKeys.lists() });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to update maintenance window');
    },
  });
};

export const useDeleteMaintenanceWindow = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.maintenanceWindows.delete(id),
    onSuccess: () => {
      toast.success('Maintenance window deleted');
      queryClient.invalidateQueries({ queryKey: maintenanceWindowKeys.lists() });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Failed to delete maintenance window');
    },
  });
};
