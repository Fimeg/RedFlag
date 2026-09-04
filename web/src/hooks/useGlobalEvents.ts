import { useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import api from '@/lib/api';
import { useRealtimeStore } from '@/lib/store';
import { POLL } from '@/lib/polling';

// SystemEvent interface matching the backend model
interface SystemEvent {
  id: string;
  agent_id?: string;
  event_type: string;
  event_subtype: string;
  severity: 'info' | 'warning' | 'error' | 'critical';
  component: string;
  message: string;
  metadata?: Record<string, any>;
  created_at: string;
  narrative?: string;
}

export interface UseGlobalEventsOptions {
  severity?: string; // comma-separated: error,critical,warning,info
  limit?: number;    // default 50, max 200
  pollingInterval?: number; // milliseconds, default 30000
}

/**
 * Polls the global event feed and feeds new events into the notification bell.
 * Mount once in Layout — the bell is global, not per-page.
 * Tracks seen event IDs to avoid re-notifying on every poll cycle.
 */
export const useGlobalEvents = (options: UseGlobalEventsOptions = {}) => {
  const { addNotification } = useRealtimeStore();
  const seenRef = useRef<Set<string>>(new Set());
  const {
    severity = 'error,critical,warning',
    limit = 50,
    pollingInterval = POLL.DETAIL,
  } = options;

  const { data } = useQuery({
    queryKey: ['global-events', severity, limit],
    queryFn: async () => {
      const params = new URLSearchParams();
      if (severity) params.append('severity', severity);
      if (limit) params.append('limit', limit.toString());

      const response = await api.get(`/events?${params.toString()}`);
      return response.data as { events: SystemEvent[]; total: number };
    },
    refetchInterval: pollingInterval,
    staleTime: pollingInterval / 2,
  });

  useEffect(() => {
    if (data?.events && data.events.length > 0) {
      data.events.forEach((event) => {
        // Only push events we haven't seen this session.
        if (seenRef.current.has(event.id)) return;
        seenRef.current.add(event.id);

        addNotification({
          type:
            event.severity === 'critical'
              ? 'error'
              : event.severity === 'error'
              ? 'error'
              : event.severity === 'warning'
              ? 'warning'
              : 'info',
          title: `${event.component}: ${event.event_type}`,
          message: event.narrative || event.message,
        });
      });
    }
  }, [data?.events, addNotification]);
};
