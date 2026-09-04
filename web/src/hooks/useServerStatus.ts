import { useEffect, useRef, useCallback } from 'react';
import { useServerStatusStore, useAuthStore } from '@/lib/store';
import { POLL } from '@/lib/polling';

// useServerStatus is a thin wrapper around the useServerStatusStore.
//
// Connection state and version tracking are handled by the axios interceptor
// in api.ts — every authenticated API call updates the store. This hook adds
// one thing: a fallback health poll for when the user is NOT authenticated
// (login page) and no authenticated API calls are running.
//
// The fallback polls the unauthenticated /api/health endpoint at POLL.HEALTH
// cadence, but ONLY when connected === false (to avoid redundant fetches when
// the authenticated polling is already keeping the store warm).

const HEALTH_TIMEOUT = 5_000;

export interface ServerStatus {
  connected: boolean;
  version: string | null;
  versionChanged: boolean;
  checkNow: () => void;
}

export function useServerStatus(): ServerStatus {
  const { connected, version, versionChanged } = useServerStatusStore();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const healthCheck = useCallback(async () => {
    try {
      const res = await fetch('/api/health', {
        method: 'GET',
        cache: 'no-store',
        signal: AbortSignal.timeout(HEALTH_TIMEOUT),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      useServerStatusStore.getState().markConnected(data?.version || null);
    } catch {
      useServerStatusStore.getState().markDisconnected();
    }
  }, []);

  // Manual retry — fires an immediate health check.
  const checkNow = useCallback(() => {
    healthCheck();
  }, [healthCheck]);

  // Fallback health poll — only when disconnected AND not authenticated.
  // Authenticated users get connection signals from the existing API polling.
  useEffect(() => {
    if (connected || isAuthenticated) {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
      return;
    }

    healthCheck();
    timerRef.current = setInterval(healthCheck, POLL.HEALTH);

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [connected, isAuthenticated, healthCheck]);

  return { connected, version, versionChanged, checkNow };
}
