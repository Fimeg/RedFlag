import { useQuery } from '@tanstack/react-query';
import { statsApi } from '@/lib/api';
import { DashboardStats } from '@/types';
import { handleApiError } from '@/lib/api';

export const useDashboardStats = () => {
  return useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: statsApi.getDashboardStats,
    refetchInterval: 30000, // Refresh every 30 seconds
    staleTime: 15000, // Consider data stale after 15 seconds
    onError: (error) => {
      console.error('Failed to fetch dashboard stats:', handleApiError(error));
    },
  });
};