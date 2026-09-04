import { useMemo } from 'react';

export interface HeartbeatStatus {
  enabled: boolean;
  until: string | null;
  active: boolean;
  source?: string | null;
}

/**
 * Derives heartbeat status from agent metadata instead of a separate endpoint.
 * The server writes `rapid_polling_enabled`, `rapid_polling_until`, and
 * `heartbeat_source` to agent metadata on every poll response and toggle.
 * Reading from the same agent object eliminates the split-brain between
 * the agent cache and a separate heartbeat cache.
 */
export const useHeartbeatStatus = (metadata?: Record<string, any>): HeartbeatStatus => {
  return useMemo(() => {
    if (!metadata) {
      return { enabled: false, until: null, active: false, source: null };
    }

    const enabled = metadata.rapid_polling_enabled === true;
    const until = metadata.rapid_polling_until || null;
    const source = metadata.heartbeat_source || null;

    // Active = enabled AND the until timestamp hasn't expired
    let active = false;
    if (enabled && until) {
      active = new Date(until).getTime() > Date.now();
    }

    return { enabled, until, active, source };
  }, [metadata]);
};
