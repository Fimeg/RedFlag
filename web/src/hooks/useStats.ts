import { useQuery } from '@tanstack/react-query';
import { statsApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import type { DashboardStats } from '@/types';
import type { UseQueryResult } from '@tanstack/react-query';

export const useDashboardStats = (): UseQueryResult<DashboardStats, Error> => {
  return useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: statsApi.getDashboardStats,
    refetchInterval: POLL.DASHBOARD,
    staleTime: POLL.DASHBOARD * 0.67,
  });
};