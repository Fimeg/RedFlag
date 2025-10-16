import { useQuery } from '@tanstack/react-query';
import { statsApi } from '@/lib/api';
import type { DashboardStats } from '@/types';
import type { UseQueryResult } from '@tanstack/react-query';

export const useDashboardStats = (): UseQueryResult<DashboardStats, Error> => {
  return useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: statsApi.getDashboardStats,
    refetchInterval: 30000, // Refresh every 30 seconds
    staleTime: 15000, // Consider data stale after 15 seconds
  });
};