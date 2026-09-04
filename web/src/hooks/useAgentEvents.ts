import { useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import api from '@/lib/api';
import { useRealtimeStore } from '@/lib/store';
import { POLL } from '@/lib/polling';

// SystemEvent interface matching the backend model
interface SystemEvent {
  id: string;
  agent_id: string;
  event_type: string;
  event_subtype: string;
  severity: 'info' | 'warning' | 'error' | 'critical';
  component: string;
  message: string;
  metadata?: Record<string, any>;
  created_at: string; // ISO timestamp string
  // Server-rendered operator-facing summary. Backend always populates this;
  // UI shows it verbatim instead of composing from event_type/event_subtype.
  narrative?: string;
}

export interface UseAgentEventsOptions {
  severity?: string; // comma-separated: error,critical,warning,info
  limit?: number; // default 50, max 1000
  pollingInterval?: number; // milliseconds, default 30000 (30s)
}

export const useAgentEvents = (
  agentId: string | null | undefined,
  options: UseAgentEventsOptions = {}
) => {
  const { addNotification } = useRealtimeStore();
  // Track seen event IDs so we notify once per event, not once per poll tick.
  const seenRef = useRef<Set<string>>(new Set());
  const {
    severity = 'error,critical,warning',
    limit = 50,
    pollingInterval = POLL.DETAIL,
  } = options;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['agent-events', agentId, severity, limit],
    queryFn: async () => {
      if (!agentId) {
        return { events: [] as SystemEvent[], total: 0 };
      }

      const params = new URLSearchParams();
      if (severity) params.append('severity', severity);
      if (limit) params.append('limit', limit.toString());

      const response = await api.get(
        `/agents/${agentId}/events?${params.toString()}`
      );
      return response.data as { events: SystemEvent[]; total: number };
    },
    enabled: !!agentId,
    refetchInterval: pollingInterval,
    staleTime: pollingInterval / 2, // Consider data stale after half the polling interval
  });

  useEffect(() => {
    if (data?.events && data.events.length > 0) {
      // Map system events to notification format and add to notification store
      data.events.forEach((event) => {
        // Only push events we haven't seen this session.
        if (seenRef.current.has(event.id)) return;
        seenRef.current.add(event.id);

        // Map severity to notification type
        const type =
          event.severity === 'critical'
            ? 'error'
            : event.severity === 'error'
            ? 'error'
            : event.severity === 'warning'
            ? 'warning'
            : 'info';

        addNotification({
          type,
          title: `${event.component}: ${event.event_type}`,
          message: event.narrative || event.message,
        });
      });
    }
  }, [data?.events, addNotification]);

  return {
    events: data?.events ?? [],
    total: data?.total ?? 0,
    isLoading,
    error,
    refetch,
  };
};