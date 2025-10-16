import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import api from '../lib/api'

export interface TimezoneOption {
  value: string
  label: string
}

export interface TimezoneSettings {
  timezone: string
  label: string
}

export function useTimezones() {
  return useQuery({
    queryKey: ['timezones'],
    queryFn: async (): Promise<TimezoneOption[]> => {
      const { data } = await api.get('/settings/timezones')
      return data.timezones
    },
  })
}

export function useTimezone() {
  return useQuery({
    queryKey: ['timezone'],
    queryFn: async (): Promise<TimezoneSettings> => {
      const { data } = await api.get('/settings/timezone')
      return data
    },
  })
}

export function useUpdateTimezone() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (timezone: string): Promise<TimezoneSettings> => {
      const { data } = await api.put('/settings/timezone', { timezone })
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['timezone'] })
    },
  })
}