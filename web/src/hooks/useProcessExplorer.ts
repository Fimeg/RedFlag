import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '../lib/api'

// Process explorer data collection caps.
// Stored server-side in the security_settings store under 'operational' category.
// Delivered to agents via the config endpoint so changes propagate fleet-wide.
export interface ProcessExplorerSettings {
  max_open_files: number
  max_sockets: number
  max_pipes: number
  max_memory_map: number
  max_namespaces: number
  max_env_keys: number
  max_listening_ports: number
}

export const PROCESS_EXPLORER_DEFAULTS: ProcessExplorerSettings = {
  max_open_files: 2000,
  max_sockets: 500,
  max_pipes: 500,
  max_memory_map: 2000,
  max_namespaces: 50,
  max_env_keys: 200,
  max_listening_ports: 100,
}

const toNumber = (v: unknown, fallback: number): number => {
  const n = typeof v === 'string' ? parseInt(v, 10) : (v as number)
  return Number.isFinite(n) && n >= 0 ? (n as number) : fallback
}

export function useProcessExplorerSettings() {
  return useQuery({
    queryKey: ['process-explorer-settings'],
    queryFn: async (): Promise<ProcessExplorerSettings> => {
      const { data } = await api.get('/security/settings')
      const op = data?.settings?.operational ?? {}
      return {
        max_open_files: toNumber(op.process_explorer_max_open_files, PROCESS_EXPLORER_DEFAULTS.max_open_files),
        max_sockets: toNumber(op.process_explorer_max_sockets, PROCESS_EXPLORER_DEFAULTS.max_sockets),
        max_pipes: toNumber(op.process_explorer_max_pipes, PROCESS_EXPLORER_DEFAULTS.max_pipes),
        max_memory_map: toNumber(op.process_explorer_max_memory_map, PROCESS_EXPLORER_DEFAULTS.max_memory_map),
        max_namespaces: toNumber(op.process_explorer_max_namespaces, PROCESS_EXPLORER_DEFAULTS.max_namespaces),
        max_env_keys: toNumber(op.process_explorer_max_env_keys, PROCESS_EXPLORER_DEFAULTS.max_env_keys),
        max_listening_ports: toNumber(op.process_explorer_max_listening_ports, PROCESS_EXPLORER_DEFAULTS.max_listening_ports),
      }
    },
  })
}

export interface ProcessExplorerUpdate {
  key: keyof ProcessExplorerSettings
  value: number
  reason?: string
}

export function useUpdateProcessExplorerSetting() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ key, value, reason }: ProcessExplorerUpdate): Promise<void> => {
      // Keys are prefixed with "process_explorer_" in the security_settings store.
      await api.put(`/security/settings/operational/process_explorer_${key}`, {
        value,
        reason: reason || 'Updated via Process Explorer settings',
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['process-explorer-settings'] })
    },
  })
}
