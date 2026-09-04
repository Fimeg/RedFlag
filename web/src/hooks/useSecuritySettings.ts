import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, securityApi } from '@/lib/api';
import { POLL } from '@/lib/polling';
import {
  SecuritySettings,
  SecuritySettingsResponse,
  SecurityEventsResponse,
  SecurityEvent,
  SecurityAuditResponse,
  EventFilters,
  KeyRotationRequest,
  KeyRotationResponse,
  MachineFingerprint
} from '@/types/security';
import { clientLogger } from '@/lib/client-logger';

// Default security settings
const defaultSecuritySettings: SecuritySettings = {
  command_signing: {
    stale_key_max_age_hours: 168,
  },
  update_security: {
    enabled: true,
    enforcement_mode: 'strict',
    nonce_timeout_seconds: 300,
    require_signature_verification: true,
    allowed_algorithms: ['ed25519', 'rsa-2048', 'ecdsa-p256'],
  },
  machine_binding: {
    enabled: true,
    enforcement_mode: 'strict',
    binding_components: {
      hardware_id: true,
      bios_uuid: true,
      mac_addresses: true,
      cpu_id: false,
      disk_serial: false,
    },
    violation_action: 'block',
    binding_grace_period_minutes: 5,
  },
  logging: {
    log_level: 'info',
    retention_days: 30,
    log_failures: true,
    log_successes: false,
    log_to_file: true,
    log_to_console: true,
    export_format: 'json',
  },
  key_management: {
    current_key: {
      key_id: '',
      algorithm: 'ed25519',
      created_at: '',
      fingerprint: '',
    },
    auto_rotation: false,
    rotation_interval_days: 90,
    grace_period_days: 7,
    key_history: [],
  },
};

// API calls
const fetchSecuritySettings = async (): Promise<SecuritySettings> => {
  try {
    const response = await api.get('/security/settings');
    return response.data.settings || defaultSecuritySettings;
  } catch (error) {
    // Return defaults if API fails
    console.warn('Failed to fetch security settings, using defaults:', error);
    return defaultSecuritySettings;
  }
};

const updateSecuritySetting = async (category: string, key: string, value: any): Promise<void> => {
  await api.put(`/security/settings/${category}/${key}`, { value });
};

const updateSecuritySettings = async (settings: Partial<SecuritySettings>): Promise<SecuritySettingsResponse> => {
  const response = await api.put('/security/settings', { settings });
  return response.data;
};

const fetchSecurityAudit = async (page: number = 1, pageSize: number = 20): Promise<SecurityAuditResponse> => {
  const response = await api.get('/security/settings/audit', {
    params: { page, page_size: pageSize }
  });
  return response.data;
};

const fetchSecurityEvents = async (
  page: number = 1,
  pageSize: number = 20,
  filters?: EventFilters
): Promise<SecurityEventsResponse> => {
  const params: any = { page, page_size: pageSize };

  if (filters) {
    if (filters.severity?.length) params.severity = filters.severity.join(',');
    if (filters.category?.length) params.category = filters.category.join(',');
    if (filters.date_range) {
      params.start_date = filters.date_range.start;
      params.end_date = filters.date_range.end;
    }
    if (filters.agent_id) params.agent_id = filters.agent_id;
    if (filters.user_id) params.user_id = filters.user_id;
    if (filters.search) params.search = filters.search;
  }

  const response = await api.get('/security/events', { params });
  return response.data;
};

const rotateKey = async (request: KeyRotationRequest): Promise<KeyRotationResponse> => {
  const response = await api.post('/security/keys/rotate', request);
  return response.data;
};

const getMachineFingerprint = async (agentId: string): Promise<MachineFingerprint> => {
  const response = await api.get(`/security/machine-binding/fingerprint/${agentId}`);
  return response.data;
};

const exportSecuritySettings = async (): Promise<Blob> => {
  const response = await api.get('/security/settings/export', {
    responseType: 'blob',
  });
  return response.data;
};

const importSecuritySettings = async (file: File): Promise<SecuritySettingsResponse> => {
  const formData = new FormData();
  formData.append('file', file);

  const response = await api.post('/security/settings/import', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  });
  return response.data;
};

// Main hook for security settings
export const useSecuritySettings = () => {
  const queryClient = useQueryClient();

  // Fetch security settings
  const {
    data: settings = defaultSecuritySettings,
    isLoading: loadingSettings,
    error: settingsError,
    refetch: refetchSettings,
  } = useQuery({
    queryKey: ['security', 'settings'],
    queryFn: fetchSecuritySettings,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });

  // Fetch security overview/status
  const {
    data: securityOverview,
    isLoading: loadingOverview,
  } = useQuery({
    queryKey: ['security', 'overview'],
    queryFn: () => securityApi.getOverview(),
    staleTime: 60 * 1000, // 1 minute
    refetchInterval: POLL.OVERVIEW,
  });

  // Update single setting mutation
  const updateSettingMutation = useMutation({
    mutationFn: ({ category, key, value }: { category: string; key: string; value: any }) =>
      updateSecuritySetting(category, key, value),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['security', 'settings'] });
      queryClient.invalidateQueries({ queryKey: ['security', 'overview'] });
    },
  });

  // Update all settings mutation
  const updateSettingsMutation = useMutation({
    mutationFn: updateSecuritySettings,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['security', 'settings'] });
      queryClient.invalidateQueries({ queryKey: ['security', 'overview'] });
      queryClient.invalidateQueries({ queryKey: ['security', 'audit'] });
    },
  });

  // Key rotation mutation
  const rotateKeyMutation = useMutation({
    mutationFn: rotateKey,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['security', 'settings'] });
      queryClient.invalidateQueries({ queryKey: ['security', 'overview'] });
    },
  });

  // Export settings mutation
  const exportSettingsMutation = useMutation({
    mutationFn: exportSecuritySettings,
  });

  // Import settings mutation
  const importSettingsMutation = useMutation({
    mutationFn: importSecuritySettings,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['security', 'settings'] });
    },
  });

  // Update a single setting
  const updateSetting = async (category: string, key: string, value: any) => {
    try {
      await updateSettingMutation.mutateAsync({ category, key, value });
    } catch (error) {
      console.error(`Failed to update ${category}.${key}:`, error);
      throw error;
    }
  };

  // Update multiple settings at once
  const updateSettings = async (newSettings: Partial<SecuritySettings>) => {
    try {
      await updateSettingsMutation.mutateAsync(newSettings);
    } catch (error) {
      console.error('Failed to update security settings:', error);
      throw error;
    }
  };

  // Rotate security key
  const rotateSecurityKey = async (request: KeyRotationRequest) => {
    try {
      return await rotateKeyMutation.mutateAsync(request);
    } catch (error) {
      console.error('Failed to rotate security key:', error);
      throw error;
    }
  };

  // Export settings to file
  const exportSettings = async () => {
    try {
      const blob = await exportSettingsMutation.mutateAsync();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.style.display = 'none';
      a.href = url;
      a.download = `redflag-security-settings-${new Date().toISOString().split('T')[0]}.json`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (error) {
      console.error('Failed to export security settings:', error);
      throw error;
    }
  };

  // Import settings from file
  const importSettings = async (file: File) => {
    try {
      return await importSettingsMutation.mutateAsync(file);
    } catch (error) {
      console.error('Failed to import security settings:', error);
      throw error;
    }
  };

  // Reset settings to defaults
  const resetToDefaults = async () => {
    try {
      await updateSettings(defaultSecuritySettings);
    } catch (error) {
      console.error('Failed to reset security settings to defaults:', error);
      throw error;
    }
  };

  return {
    // Data
    settings,
    securityOverview,

    // Loading states
    loading: loadingSettings || loadingOverview,
    saving: updateSettingMutation.isPending || updateSettingsMutation.isPending,

    // Errors
    error: settingsError || updateSettingMutation.error || updateSettingsMutation.error,

    // Actions
    updateSetting,
    updateSettings,
    rotateSecurityKey,
    exportSettings,
    importSettings,
    resetToDefaults,
    refetch: refetchSettings,
  };
};

// Hook for security audit trail
export const useSecurityAudit = (page: number = 1, pageSize: number = 20) => {
  return useQuery({
    queryKey: ['security', 'audit', page, pageSize],
    queryFn: () => fetchSecurityAudit(page, pageSize),
    staleTime: 2 * 60 * 1000, // 2 minutes
  });
};

// Hook for security events
export const useSecurityEvents = (
  page: number = 1,
  pageSize: number = 20,
  filters?: EventFilters
) => {
  return useQuery({
    queryKey: ['security', 'events', page, pageSize, filters],
    queryFn: () => fetchSecurityEvents(page, pageSize, filters),
    staleTime: 30 * 1000, // 30 seconds
  });
};

// Hook for machine fingerprint
export const useMachineFingerprint = (agentId: string) => {
  return useQuery({
    queryKey: ['security', 'machine-fingerprint', agentId],
    queryFn: () => getMachineFingerprint(agentId),
    enabled: !!agentId,
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
};

// Hook for real-time security events (WebSocket)
export const useSecurityWebSocket = () => {
  const [events, setEvents] = React.useState<SecurityEvent[]>([]);
  const [connected, setConnected] = React.useState(false);
  const ws = React.useRef<WebSocket | null>(null);

  React.useEffect(() => {
    const wsUrl = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/v1/security/ws`;

    let reconnectAttempts = 0;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    // Set on unmount so a pending close handler doesn't resurrect the socket.
    let stopped = false;

    const connect = () => {
      if (stopped) return;
      const socket = new WebSocket(wsUrl, []);
      ws.current = socket;

      socket.onopen = () => {
        reconnectAttempts = 0;
        setConnected(true);
        clientLogger.debug('Security WebSocket connected');
      };

      socket.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          if (message.type === 'security_event') {
            setEvents(prev => [message.data, ...prev.slice(0, 999)]); // Keep last 1000 events
          }
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };

      socket.onerror = (error) => {
        console.error('Security WebSocket error:', error);
        setConnected(false);
      };

      socket.onclose = () => {
        setConnected(false);
        clientLogger.debug('Security WebSocket disconnected');
        if (stopped) return;
        // Exponential backoff capped at 30s: 1s, 2s, 4s, 8s, 16s, 30s…
        const delay = Math.min(30000, 1000 * 2 ** reconnectAttempts);
        reconnectAttempts += 1;
        reconnectTimer = setTimeout(connect, delay);
      };
    };

    connect();

    return () => {
      stopped = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (ws.current) ws.current.close();
    };
  }, []);

  return {
    events,
    connected,
    clearEvents: () => setEvents([]),
  };
};

// Helper hook for form validation
export const useSecurityValidation = () => {
  const validateSetting = (key: string, value: any): string | null => {
    switch (key) {
      case 'nonce_timeout_seconds':
        if (value < 60 || value > 3600) {
          return 'Nonce timeout must be between 60 and 3600 seconds';
        }
        break;

      case 'retention_days':
        if (value < 1 || value > 365) {
          return 'Retention period must be between 1 and 365 days';
        }
        break;

      case 'rotation_interval_days':
        if (value < 7 || value > 365) {
          return 'Rotation interval must be between 7 and 365 days';
        }
        break;

      case 'binding_grace_period_minutes':
        if (value < 1 || value > 60) {
          return 'Grace period must be between 1 and 60 minutes';
        }
        break;

      default:
        return null;
    }

    return null;
  };

  const validateAll = (settings: SecuritySettings): Record<string, string> => {
    const errors: Record<string, string> = {};

    // Validate update security
    const updateSec = settings.update_security;
    if (updateSec.enabled) {
      const nonceError = validateSetting('nonce_timeout_seconds', updateSec.nonce_timeout_seconds);
      if (nonceError) errors['update_security.nonce_timeout_seconds'] = nonceError;
    }

    // Validate machine binding
    const machineBinding = settings.machine_binding;
    if (machineBinding.enabled) {
      const hasAnyComponent = Object.values(machineBinding.binding_components).some(v => v);
      if (!hasAnyComponent) {
        errors['machine_binding.binding_components'] = 'At least one binding component must be selected';
      }

      const graceError = validateSetting('binding_grace_period_minutes', machineBinding.binding_grace_period_minutes);
      if (graceError) errors['machine_binding.binding_grace_period_minutes'] = graceError;
    }

    // Validate logging
    const logging = settings.logging;
    const retentionError = validateSetting('retention_days', logging.retention_days);
    if (retentionError) errors['logging.retention_days'] = retentionError;

    // Validate key management
    const keyMgmt = settings.key_management;
    if (keyMgmt.auto_rotation) {
      const rotationError = validateSetting('rotation_interval_days', keyMgmt.rotation_interval_days);
      if (rotationError) errors['key_management.rotation_interval_days'] = rotationError;

      const graceError = validateSetting('grace_period_days', keyMgmt.grace_period_days);
      if (graceError) errors['key_management.grace_period_days'] = graceError;
    }

    return errors;
  };

  return { validateSetting, validateAll };
};

// Import React for WebSocket hook
import React from 'react';
