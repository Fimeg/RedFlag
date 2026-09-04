import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { updateApi, capabilityTokenApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import type { UpdatePackage, ListQueryParams, UpdateApprovalRequest, UpdateListResponse, PackageListResponse, PackageFleetResponse, PackageVersionsResponse, CapabilityTokenStatusResponse, PackageSummary, PackageAgentsResponse, PackageVulnerabilitiesResponse } from '@/types';
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

export const usePackages = (params?: ListQueryParams): UseQueryResult<PackageListResponse, Error> => {
  return useQuery({
    queryKey: ['packages', params],
    queryFn: () => updateApi.getPackageList(params),
  });
};

export const usePackageFleet = (id: string, enabled: boolean = true): UseQueryResult<PackageFleetResponse, Error> => {
  return useQuery({
    queryKey: ['package-fleet', id],
    queryFn: () => updateApi.getPackageFleet(id),
    enabled: enabled && !!id,
  });
};

export const usePackageVersions = (id: string, enabled: boolean = true): UseQueryResult<PackageVersionsResponse, Error> => {
  return useQuery({
    queryKey: ['package-versions', id],
    queryFn: () => updateApi.getPackageVersions(id),
    enabled: enabled && !!id,
  });
};

export const usePackageSummary = (pkgType: string, pkgName: string, enabled: boolean = true): UseQueryResult<PackageSummary, Error> => {
  return useQuery({
    queryKey: ['package-summary', pkgType, pkgName],
    queryFn: () => updateApi.getPackageSummaryByCoords(pkgType, pkgName),
    enabled: enabled && !!pkgType && !!pkgName,
  });
};

export const usePackageAgents = (pkgType: string, pkgName: string, enabled: boolean = true): UseQueryResult<PackageAgentsResponse, Error> => {
  return useQuery({
    queryKey: ['package-agents', pkgType, pkgName],
    queryFn: () => updateApi.getPackageAgentsByCoords(pkgType, pkgName),
    enabled: enabled && !!pkgType && !!pkgName,
    refetchInterval: POLL.DASHBOARD,
  });
};

export const usePackageVersionsByCoords = (pkgType: string, pkgName: string, enabled: boolean = true): UseQueryResult<PackageVersionsResponse, Error> => {
  return useQuery({
    queryKey: ['package-versions-coords', pkgType, pkgName],
    queryFn: () => updateApi.getPackageVersionsByCoords(pkgType, pkgName),
    enabled: enabled && !!pkgType && !!pkgName,
  });
};

export const usePackageVulnerabilities = (pkgType: string, pkgName: string, enabled: boolean = true): UseQueryResult<PackageVulnerabilitiesResponse, Error> => {
  return useQuery({
    queryKey: ['package-vulns', pkgType, pkgName],
    queryFn: () => updateApi.getPackageVulnerabilitiesByCoords(pkgType, pkgName),
    enabled: enabled && !!pkgType && !!pkgName,
  });
};

export const useUpdateLifecycle = (id: string, enabled: boolean = true) => {
  return useQuery({
    queryKey: ['update-lifecycle', id],
    queryFn: () => updateApi.getUpdateLifecycle(id),
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
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
    },
  });
};

export const useApproveMultipleUpdates = (): UseMutationResult<void, Error, UpdateApprovalRequest, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (request: UpdateApprovalRequest) => updateApi.approveUpdates(request),
    onSuccess: () => {
      // Match useApproveUpdate's set — bulk approve must refresh dashboard
      // counts and LiveOperations too, not just the updates list.
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
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
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
    },
  });
};

// Command retry/cancel live in useCommands (canonical, shared invalidation set).
// Re-exported here so existing Updates-page imports keep resolving.
export { useRetryCommand, useCancelCommand } from './useCommands';

export const useReopenUpdate =(): UseMutationResult<{ message: string }, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.reopenUpdate,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
    },
  });
};

export const useResolveUpdate = (): UseMutationResult<{ message: string }, Error, string, unknown> => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: updateApi.resolveUpdate,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
    },
  });
};

// Capability token status (LIFECYCLE-005)
export const useCapabilityTokenStatus = (
  agentId: string,
  updateId?: string,
  enabled: boolean = true,
): UseQueryResult<CapabilityTokenStatusResponse, Error> => {
  return useQuery({
    queryKey: ['capability-tokens', agentId, updateId || ''],
    queryFn: () => capabilityTokenApi.getTokenStatus(agentId, updateId),
    enabled: enabled && !!agentId,
  });
};