import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '../lib/api'

// Polling resilience tuning lives in the security_settings store under the
// 'operational' category (same tier as update_stuck_minutes). The server
// delivers these to agents over GET /api/v1/agents/:id/config, which the agent
// merges into its local PollingConfig — so a change here propagates fleet-wide.
export interface PollingSettings {
  jitter_max_seconds: number
  backoff_base_seconds: number
  backoff_max_seconds: number
}

export const POLLING_DEFAULTS: PollingSettings = {
  jitter_max_seconds: 30,
  backoff_base_seconds: 10,
  backoff_max_seconds: 300,
}

const toNumber = (v: unknown, fallback: number): number => {
  const n = typeof v === 'string' ? parseInt(v, 10) : (v as number)
  return Number.isFinite(n) ? (n as number) : fallback
}

export function usePollingSettings() {
  return useQuery({
    queryKey: ['polling-settings'],
    queryFn: async (): Promise<PollingSettings> => {
      const { data } = await api.get('/security/settings')
      const op = data?.settings?.operational ?? {}
      return {
        jitter_max_seconds: toNumber(op.jitter_max_seconds, POLLING_DEFAULTS.jitter_max_seconds),
        backoff_base_seconds: toNumber(op.backoff_base_seconds, POLLING_DEFAULTS.backoff_base_seconds),
        backoff_max_seconds: toNumber(op.backoff_max_seconds, POLLING_DEFAULTS.backoff_max_seconds),
      }
    },
  })
}

export interface PollingUpdate {
  key: keyof PollingSettings
  value: number
  reason?: string
}

export function useUpdatePollingSetting() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ key, value, reason }: PollingUpdate): Promise<void> => {
      await api.put(`/security/settings/operational/${key}`, {
        value,
        reason: reason || 'Updated via Agent Polling settings',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['polling-settings'] })
    },
  })
}
