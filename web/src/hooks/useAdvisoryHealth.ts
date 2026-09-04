import { useQuery } from '@tanstack/react-query';
import { healthApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import type { AdvisoryHealth } from '@/lib/api';
import type { UseQueryResult } from '@tanstack/react-query';

// Polls advisory-feed health for the degradation banner. The breaker heals on its
// own and packages re-vet on the next cycle, so DETAIL cadence is plenty — this is
// "is the feed up" awareness, not a real-time dashboard.
export const useAdvisoryHealth = (): UseQueryResult<AdvisoryHealth, Error> => {
  return useQuery({
    queryKey: ['advisory-health'],
    queryFn: healthApi.getAdvisory,
    refetchInterval: POLL.DETAIL,
    staleTime: POLL.DETAIL * 0.5,
    retry: false,
  });
};
