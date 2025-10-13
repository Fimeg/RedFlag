import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { updateApi } from '@/lib/api';
import { UpdatePackage, ListQueryParams, UpdateApprovalRequest } from '@/types';
import { useUpdateStore, useRealtimeStore } from '@/lib/store';
import { handleApiError } from '@/lib/api';

export const useUpdates = (params?: ListQueryParams) => {
  const { setUpdates, setLoading, setError } = useUpdateStore();

  return useQuery({
    queryKey: ['updates', params],
    queryFn: () => updateApi.getUpdates(params),
    onSuccess: (data) => {
      setUpdates(data.updates);
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

export const useUpdate = (id: string, enabled: boolean = true) => {
  const { setSelectedUpdate, setLoading, setError } = useUpdateStore();

  return useQuery({
    queryKey: ['update', id],
    queryFn: () => updateApi.getUpdate(id),
    enabled: enabled && !!id,
    onSuccess: (data) => {
      setSelectedUpdate(data);
      setLoading(false);
      setError(null);
    },
    onError: (error) => {
      setError(handleApiError(error).message);
      setLoading(false);
    },
  });
};

export const useApproveUpdate = () => {
  const queryClient = useQueryClient();
  const { updateUpdateStatus } = useUpdateStore();
  const { addNotification } = useRealtimeStore();

  return useMutation({
    mutationFn: ({ id, scheduledAt }: { id: string; scheduledAt?: string }) =>
      updateApi.approveUpdate(id, scheduledAt),
    onSuccess: (_, { id }) => {
      // Update local state
      updateUpdateStatus(id, 'approved');

      // Invalidate queries to refresh data
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update', id] });

      // Show success notification
      addNotification({
        type: 'success',
        title: 'Update Approved',
        message: 'The update has been approved successfully.',
      });
    },
    onError: (error) => {
      addNotification({
        type: 'error',
        title: 'Approval Failed',
        message: handleApiError(error).message,
      });
    },
  });
};

export const useApproveMultipleUpdates = () => {
  const queryClient = useQueryClient();
  const { bulkUpdateStatus } = useUpdateStore();
  const { addNotification } = useRealtimeStore();

  return useMutation({
    mutationFn: (request: UpdateApprovalRequest) => updateApi.approveUpdates(request),
    onSuccess: (_, request) => {
      // Update local state
      bulkUpdateStatus(request.update_ids, 'approved');

      // Invalidate queries to refresh data
      queryClient.invalidateQueries({ queryKey: ['updates'] });

      // Show success notification
      addNotification({
        type: 'success',
        title: 'Updates Approved',
        message: `${request.update_ids.length} update(s) have been approved successfully.`,
      });
    },
    onError: (error) => {
      addNotification({
        type: 'error',
        title: 'Bulk Approval Failed',
        message: handleApiError(error).message,
      });
    },
  });
};

export const useRejectUpdate = () => {
  const queryClient = useQueryClient();
  const { updateUpdateStatus } = useUpdateStore();
  const { addNotification } = useRealtimeStore();

  return useMutation({
    mutationFn: updateApi.rejectUpdate,
    onSuccess: (_, id) => {
      // Update local state
      updateUpdateStatus(id, 'pending');

      // Invalidate queries to refresh data
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update', id] });

      // Show success notification
      addNotification({
        type: 'success',
        title: 'Update Rejected',
        message: 'The update has been rejected and moved back to pending status.',
      });
    },
    onError: (error) => {
      addNotification({
        type: 'error',
        title: 'Rejection Failed',
        message: handleApiError(error).message,
      });
    },
  });
};

export const useInstallUpdate = () => {
  const queryClient = useQueryClient();
  const { updateUpdateStatus } = useUpdateStore();
  const { addNotification } = useRealtimeStore();

  return useMutation({
    mutationFn: updateApi.installUpdate,
    onSuccess: (_, id) => {
      // Update local state
      updateUpdateStatus(id, 'installing');

      // Invalidate queries to refresh data
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update', id] });

      // Show success notification
      addNotification({
        type: 'info',
        title: 'Installation Started',
        message: 'The update installation has been started. This may take a few minutes.',
      });
    },
    onError: (error) => {
      addNotification({
        type: 'error',
        title: 'Installation Failed',
        message: handleApiError(error).message,
      });
    },
  });
};