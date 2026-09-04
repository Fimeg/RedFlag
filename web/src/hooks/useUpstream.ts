import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import { CreateTrackedSoftwareRequest, UpdateTrackedSoftwareSettingsRequest } from '@/types';

export const upstreamKeys = {
  all: ['upstream'] as const,
  list: () => [...upstreamKeys.all, 'list'] as const,
  drift: () => [...upstreamKeys.all, 'drift'] as const,
  events: () => [...upstreamKeys.all, 'events'] as const,
  installations: (softwareId: string) =>
    [...upstreamKeys.all, 'installations', softwareId] as const,
};

export const useTrackedSoftware = () => {
  return useQuery({
    queryKey: upstreamKeys.list(),
    queryFn: () => adminApi.upstream.list(),
    staleTime: POLL.STATIC * 0.2,
  });
};

export const useDriftedSoftware = () => {
  return useQuery({
    queryKey: upstreamKeys.drift(),
    queryFn: () => adminApi.upstream.listDrifted(),
    staleTime: POLL.STATIC * 0.2,
    refetchInterval: POLL.STATIC,
  });
};

export const useRecentDriftEvents = () => {
  return useQuery({
    queryKey: upstreamKeys.events(),
    queryFn: () => adminApi.upstream.recentDriftEvents(),
    staleTime: 1000 * 60,
  });
};

export const useAddTrackedSoftware = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTrackedSoftwareRequest) => adminApi.upstream.create(input),
    onSuccess: (row) => {
      toast.success(`Tracking ${row.name} — initial sync queued`);
      qc.invalidateQueries({ queryKey: upstreamKeys.all });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || 'Failed to add tracked software');
    },
  });
};

export const useRemoveTrackedSoftware = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.upstream.remove(id),
    onSuccess: () => {
      toast.success('Removed from tracking');
      qc.invalidateQueries({ queryKey: upstreamKeys.all });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || 'Failed to remove tracked software');
    },
  });
};

export const useUpdateTrackedSoftwareSettings = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateTrackedSoftwareSettingsRequest }) =>
      adminApi.upstream.updateSettings(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: upstreamKeys.all });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || 'Failed to update tracked software');
    },
  });
};

export const useInstallations = (softwareId: string, enabled: boolean = true) => {
  return useQuery({
    queryKey: upstreamKeys.installations(softwareId),
    queryFn: () => adminApi.upstream.installations(softwareId),
    enabled: enabled && !!softwareId,
    staleTime: 1000 * 30,
  });
};

export const useSyncTrackedSoftware = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.upstream.syncNow(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: upstreamKeys.all });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || 'Sync failed');
    },
  });
};
