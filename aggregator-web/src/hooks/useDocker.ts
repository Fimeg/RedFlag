import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { dockerApi } from '@/lib/api';
import type { DockerContainer, DockerImage } from '@/types';
import toast from 'react-hot-toast';

// Hook for fetching all Docker containers/images across all agents
export const useDockerContainers = (params?: {
  page?: number;
  page_size?: number;
  agent?: string;
  status?: string;
  search?: string;
}) => {
  return useQuery({
    queryKey: ['docker-containers', params],
    queryFn: async () => {
      const response = await dockerApi.getContainers(params || {});
      return response;
    },
    staleTime: 30000, // 30 seconds
  });
};

// Hook for fetching Docker containers for a specific agent
export const useAgentDockerContainers = (agentId: string, params?: {
  page?: number;
  page_size?: number;
  status?: string;
  search?: string;
}) => {
  return useQuery({
    queryKey: ['agent-docker-containers', agentId, params],
    queryFn: async () => {
      const response = await dockerApi.getAgentContainers(agentId, params || {});
      return response;
    },
    staleTime: 30000,
    enabled: !!agentId,
  });
};

// Hook for Docker statistics
export const useDockerStats = () => {
  return useQuery({
    queryKey: ['docker-stats'],
    queryFn: async () => {
      const response = await dockerApi.getStats();
      return response;
    },
    staleTime: 60000, // 1 minute
  });
};

// Hook for approving Docker updates
export const useApproveDockerUpdate = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ containerId, imageId }: {
      containerId: string;
      imageId: string;
    }) => {
      const response = await dockerApi.approveUpdate(containerId, imageId);
      return response;
    },
    onSuccess: () => {
      toast.success('Docker update approved successfully');
      queryClient.invalidateQueries({ queryKey: ['docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['agent-docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['docker-stats'] });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.message || 'Failed to approve Docker update');
    },
  });
};

// Hook for rejecting Docker updates
export const useRejectDockerUpdate = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ containerId, imageId }: {
      containerId: string;
      imageId: string;
    }) => {
      const response = await dockerApi.rejectUpdate(containerId, imageId);
      return response;
    },
    onSuccess: () => {
      toast.success('Docker update rejected');
      queryClient.invalidateQueries({ queryKey: ['docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['agent-docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['docker-stats'] });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.message || 'Failed to reject Docker update');
    },
  });
};

// Hook for installing Docker updates
export const useInstallDockerUpdate = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ containerId, imageId }: {
      containerId: string;
      imageId: string;
    }) => {
      const response = await dockerApi.installUpdate(containerId, imageId);
      return response;
    },
    onSuccess: () => {
      toast.success('Docker update installation started');
      queryClient.invalidateQueries({ queryKey: ['docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['agent-docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['docker-stats'] });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.message || 'Failed to install Docker update');
    },
  });
};

// Hook for bulk Docker operations
export const useBulkDockerActions = () => {
  const queryClient = useQueryClient();

  const approveMultiple = useMutation({
    mutationFn: async ({ updates }: {
      updates: Array<{ containerId: string; imageId: string }>;
    }) => {
      const response = await dockerApi.bulkApproveUpdates(updates);
      return response;
    },
    onSuccess: (data) => {
      toast.success(`${data.approved} Docker updates approved`);
      queryClient.invalidateQueries({ queryKey: ['docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['agent-docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['docker-stats'] });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.message || 'Failed to approve Docker updates');
    },
  });

  const rejectMultiple = useMutation({
    mutationFn: async ({ updates }: {
      updates: Array<{ containerId: string; imageId: string }>;
    }) => {
      const response = await dockerApi.bulkRejectUpdates(updates);
      return response;
    },
    onSuccess: (data) => {
      toast.success(`${data.rejected} Docker updates rejected`);
      queryClient.invalidateQueries({ queryKey: ['docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['agent-docker-containers'] });
      queryClient.invalidateQueries({ queryKey: ['docker-stats'] });
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.message || 'Failed to reject Docker updates');
    },
  });

  return {
    approveMultiple,
    rejectMultiple,
  };
};