import axios, { AxiosResponse } from 'axios';
import {
  Agent,
  UpdatePackage,
  DashboardStats,
  AgentListResponse,
  UpdateListResponse,
  UpdateApprovalRequest,
  ScanRequest,
  ListQueryParams,
  ApiError,
  DockerContainer,
  DockerImage,
  DockerContainerListResponse,
  DockerStats,
  DockerUpdateRequest,
  BulkDockerUpdateRequest,
  RegistrationToken,
  CreateRegistrationTokenRequest,
  RegistrationTokenStats,
  RateLimitConfig,
  RateLimitStats,
  RateLimitUsage,
  RateLimitSummary
} from '@/types';

// Base URL for API - use nginx proxy
export const API_BASE_URL = '/api/v1';

// Create axios instance
const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor to add auth token
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('auth_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Response interceptor to handle errors
api.interceptors.response.use(
  (response: AxiosResponse) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Clear token and redirect to login
      localStorage.removeItem('auth_token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

// API endpoints
export const agentApi = {
  // Get all agents
  getAgents: async (params?: ListQueryParams): Promise<AgentListResponse> => {
    const response = await api.get('/agents', { params });
    return response.data;
  },

  // Get single agent
  getAgent: async (id: string): Promise<Agent> => {
    const response = await api.get(`/agents/${id}`);
    return response.data;
  },

  // Trigger scan on agents
  triggerScan: async (request: ScanRequest): Promise<void> => {
    await api.post('/agents/scan', request);
  },

  // Trigger scan on single agent
  scanAgent: async (id: string): Promise<void> => {
    await api.post(`/agents/${id}/scan`);
  },

  // Trigger heartbeat toggle on single agent
  toggleHeartbeat: async (id: string, enabled: boolean, durationMinutes: number = 10): Promise<{ message: string; command_id: string; enabled: boolean }> => {
    const response = await api.post(`/agents/${id}/heartbeat`, {
      enabled: enabled,
      duration_minutes: durationMinutes,
    });
    return response.data;
  },

  // Get heartbeat status for single agent
  getHeartbeatStatus: async (id: string): Promise<{ enabled: boolean; until: string | null; active: boolean; duration_minutes: number }> => {
    const response = await api.get(`/agents/${id}/heartbeat`);
    return response.data;
  },

  // Trigger agent reboot
  rebootAgent: async (id: string, delayMinutes: number = 1, message?: string): Promise<void> => {
    await api.post(`/agents/${id}/reboot`, {
      delay_minutes: delayMinutes,
      message: message || 'System reboot requested by RedFlag'
    });
  },

  // Unregister/remove agent
  unregisterAgent: async (id: string): Promise<void> => {
    await api.delete(`/agents/${id}`);
  },
};

export const updateApi = {
  // Get all updates
  getUpdates: async (params?: ListQueryParams): Promise<UpdateListResponse> => {
    const response = await api.get('/updates', { params });
    return response.data;
  },

  // Get single update
  getUpdate: async (id: string): Promise<UpdatePackage> => {
    const response = await api.get(`/updates/${id}`);
    return response.data;
  },

  // Approve updates
  approveUpdates: async (request: UpdateApprovalRequest): Promise<void> => {
    await api.post('/updates/approve', request);
  },

  // Approve single update
  approveUpdate: async (id: string, scheduledAt?: string): Promise<void> => {
    await api.post(`/updates/${id}/approve`, { scheduled_at: scheduledAt });
  },

  // Reject/cancel update
  rejectUpdate: async (id: string): Promise<void> => {
    await api.post(`/updates/${id}/reject`);
  },

  // Install update immediately
  installUpdate: async (id: string): Promise<void> => {
    await api.post(`/updates/${id}/install`);
  },

  // Get update logs
  getUpdateLogs: async (id: string, limit?: number): Promise<{ logs: any[]; count: number }> => {
    const response = await api.get(`/updates/${id}/logs`, {
      params: limit ? { limit } : undefined
    });
    return response.data;
  },

  // Retry a failed, timed_out, or cancelled command
  retryCommand: async (commandId: string): Promise<{ message: string; command_id: string; new_id: string }> => {
    const response = await api.post(`/commands/${commandId}/retry`);
    return response.data;
  },

  // Cancel a pending or sent command
  cancelCommand: async (commandId: string): Promise<{ message: string }> => {
    const response = await api.post(`/commands/${commandId}/cancel`);
    return response.data;
  },

  // Get active commands for live command control
  getActiveCommands: async (): Promise<{ commands: any[]; count: number }> => {
    const response = await api.get('/commands/active');
    return response.data;
  },

  // Get recent commands for retry functionality
  getRecentCommands: async (limit?: number): Promise<{ commands: any[]; count: number; limit: number }> => {
    const response = await api.get('/commands/recent', {
      params: limit ? { limit } : undefined
    });
    return response.data;
  },

  // Clear failed commands with filtering options
  clearFailedCommands: async (options?: {
    olderThanDays?: number;
    onlyRetried?: boolean;
    allFailed?: boolean;
  }): Promise<{ message: string; count: number; cheeky_warning?: string }> => {
    const params = new URLSearchParams();

    if (options?.olderThanDays !== undefined) {
      params.append('older_than_days', options.olderThanDays.toString());
    }
    if (options?.onlyRetried) {
      params.append('only_retried', 'true');
    }
    if (options?.allFailed) {
      params.append('all_failed', 'true');
    }

    const response = await api.delete(`/commands/failed${params.toString() ? '?' + params.toString() : ''}`);
    return response.data;
  },
};

export const statsApi = {
  // Get dashboard statistics
  getDashboardStats: async (): Promise<DashboardStats> => {
    const response = await api.get('/stats/summary');
    return response.data;
  },
};

export const logApi = {
  // Get all logs with filtering for universal log view
  getAllLogs: async (params?: {
    page?: number;
    page_size?: number;
    agent_id?: string;
    action?: string;
    result?: string;
    since?: string;
  }): Promise<{ logs: any[]; total: number; page: number; page_size: number }> => {
    const response = await api.get('/logs', { params });
    return response.data;
  },

  // Get active operations for live status view
  getActiveOperations: async (): Promise<{ operations: any[]; count: number }> => {
    const response = await api.get('/logs/active');
    return response.data;
  },

  // Get active commands for live command control
  getActiveCommands: async (): Promise<{ commands: any[]; count: number }> => {
    const response = await api.get('/commands/active');
    return response.data;
  },

  // Get recent commands for retry functionality
  getRecentCommands: async (limit?: number): Promise<{ commands: any[]; count: number; limit: number }> => {
    const response = await api.get('/commands/recent', {
      params: limit ? { limit } : undefined
    });
    return response.data;
  },
};

export const authApi = {
  // Login with username and password
  login: async (credentials: { username: string; password: string }): Promise<{ token: string; user: any }> => {
    const response = await api.post('/auth/login', credentials);
    return response.data;
  },

  // Verify token
  verifyToken: async (): Promise<{ valid: boolean }> => {
    const response = await api.get('/auth/verify');
    return response.data;
  },

  // Logout
  logout: async (): Promise<void> => {
    await api.post('/auth/logout');
  },
};

// Setup API for server configuration (uses nginx proxy)
const setupApiInstance = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

export const setupApi = {
  // Check server health and status
  checkHealth: async (): Promise<{ status: string }> => {
    const response = await setupApiInstance.get('/health');
    return response.data;
  },

  // Submit server configuration
  configure: async (config: {
    adminUser: string;
    adminPassword: string;
    dbHost: string;
    dbPort: string;
    dbName: string;
    dbUser: string;
    dbPassword: string;
    serverHost: string;
    serverPort: string;
    maxSeats: string;
  }): Promise<{ message: string; jwtSecret?: string; envContent?: string; manualRestartRequired?: boolean; manualRestartCommand?: string; configFilePath?: string }> => {
    const response = await setupApiInstance.post('/setup/configure', config);
    return response.data;
  },
};

// Utility functions
export const createQueryString = (params: Record<string, any>): string => {
  const searchParams = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      if (Array.isArray(value)) {
        value.forEach(v => searchParams.append(key, v));
      } else {
        searchParams.append(key, value.toString());
      }
    }
  });
  return searchParams.toString();
};

// Error handling utility
export const handleApiError = (error: any): ApiError => {
  if (axios.isAxiosError(error)) {
    const status = error.response?.status;
    const data = error.response?.data;

    if (status === 401) {
      return {
        message: 'Authentication required. Please log in.',
        code: 'UNAUTHORIZED',
      };
    }

    if (status === 403) {
      return {
        message: 'Access denied. You do not have permission to perform this action.',
        code: 'FORBIDDEN',
      };
    }

    if (status === 404) {
      return {
        message: 'The requested resource was not found.',
        code: 'NOT_FOUND',
      };
    }

    if (status === 429) {
      return {
        message: 'Too many requests. Please try again later.',
        code: 'RATE_LIMIT_EXCEEDED',
      };
    }

    if (status && status >= 500) {
      return {
        message: 'Server error. Please try again later.',
        code: 'SERVER_ERROR',
      };
    }

    return {
      message: data?.message || error.message || 'An error occurred',
      code: data?.code || 'UNKNOWN_ERROR',
      details: data?.details,
    };
  }

  return {
    message: error.message || 'An unexpected error occurred',
    code: 'UNKNOWN_ERROR',
  };
};

// Docker-specific API endpoints
export const dockerApi = {
  // Get all Docker containers and images across all agents
  getContainers: async (params?: {
    page?: number;
    page_size?: number;
    agent?: string;
    status?: string;
    search?: string;
  }): Promise<DockerContainerListResponse> => {
    const response = await api.get('/docker/containers', { params });
    return response.data;
  },

  // Get Docker containers for a specific agent
  getAgentContainers: async (agentId: string, params?: {
    page?: number;
    page_size?: number;
    status?: string;
    search?: string;
  }): Promise<DockerContainerListResponse> => {
    const response = await api.get(`/agents/${agentId}/docker`, { params });
    return response.data;
  },

  // Get Docker statistics
  getStats: async (): Promise<DockerStats> => {
    const response = await api.get('/docker/stats');
    return response.data;
  },

  // Approve Docker image update
  approveUpdate: async (containerId: string, imageId: string, scheduledAt?: string): Promise<void> => {
    await api.post(`/docker/containers/${containerId}/images/${imageId}/approve`, {
      scheduled_at: scheduledAt,
    });
  },

  // Reject Docker image update
  rejectUpdate: async (containerId: string, imageId: string): Promise<void> => {
    await api.post(`/docker/containers/${containerId}/images/${imageId}/reject`);
  },

  // Install Docker image update
  installUpdate: async (containerId: string, imageId: string): Promise<void> => {
    await api.post(`/docker/containers/${containerId}/images/${imageId}/install`);
  },

  // Bulk approve Docker updates
  bulkApproveUpdates: async (updates: Array<{ containerId: string; imageId: string }>, scheduledAt?: string): Promise<{ approved: number }> => {
    const response = await api.post('/docker/updates/bulk-approve', {
      updates,
      scheduled_at: scheduledAt,
    });
    return response.data;
  },

  // Bulk reject Docker updates
  bulkRejectUpdates: async (updates: Array<{ containerId: string; imageId: string }>): Promise<{ rejected: number }> => {
    const response = await api.post('/docker/updates/bulk-reject', {
      updates,
    });
    return response.data;
  },

  // Trigger Docker scan on agents
  triggerScan: async (agentIds?: string[]): Promise<void> => {
    await api.post('/docker/scan', { agent_ids: agentIds });
  },
};

// Admin API endpoints
export const adminApi = {
  // Registration Token Management
  tokens: {
    // Get all registration tokens
    getTokens: async (params?: {
      page?: number;
      page_size?: number;
      is_active?: boolean;
      label?: string;
    }): Promise<{ tokens: RegistrationToken[]; total: number; page: number; page_size: number }> => {
      const response = await api.get('/admin/registration-tokens', { params });
      return response.data;
    },

    // Get single registration token
    getToken: async (id: string): Promise<RegistrationToken> => {
      const response = await api.get(`/admin/registration-tokens/${id}`);
      return response.data;
    },

    // Create new registration token
    createToken: async (request: CreateRegistrationTokenRequest): Promise<RegistrationToken> => {
      const response = await api.post('/admin/registration-tokens', request);
      return response.data;
    },

    // Revoke registration token (soft delete)
    revokeToken: async (id: string): Promise<void> => {
      await api.delete(`/admin/registration-tokens/${id}`);
    },

    // Delete registration token (hard delete)
    deleteToken: async (id: string): Promise<void> => {
      await api.delete(`/admin/registration-tokens/delete/${id}`);
    },

    // Get registration token statistics
    getStats: async (): Promise<RegistrationTokenStats> => {
      const response = await api.get('/admin/registration-tokens/stats');
      return response.data;
    },

    // Cleanup expired tokens
    cleanup: async (): Promise<{ cleaned: number }> => {
      const response = await api.post('/admin/registration-tokens/cleanup');
      return response.data;
    },
  },

  // Rate Limiting Management
  rateLimits: {
    // Get all rate limit configurations
    getConfigs: async (): Promise<RateLimitConfig[]> => {
      const response = await api.get('/admin/rate-limits');

      // Backend returns { settings: {...}, updated_at: "..." }
      // Transform settings object to array format expected by frontend
      const settings = response.data.settings || {};
      const configs: RateLimitConfig[] = Object.entries(settings).map(([endpoint, config]: [string, any]) => ({
        ...config,
        endpoint,
        updated_at: response.data.updated_at, // Preserve update timestamp
      }));

      return configs;
    },

    // Update rate limit configuration
    updateConfig: async (endpoint: string, config: Partial<RateLimitConfig>): Promise<RateLimitConfig> => {
      const response = await api.put(`/admin/rate-limits/${endpoint}`, config);
      return response.data;
    },

    // Update all rate limit configurations
    updateAllConfigs: async (configs: RateLimitConfig[]): Promise<RateLimitConfig[]> => {
      const response = await api.put('/admin/rate-limits', { configs });
      return response.data;
    },

    // Reset rate limit configurations to defaults
    resetConfigs: async (): Promise<RateLimitConfig[]> => {
      const response = await api.post('/admin/rate-limits/reset');
      return response.data;
    },

    // Get rate limit statistics
    getStats: async (): Promise<RateLimitStats[]> => {
      const response = await api.get('/admin/rate-limits/stats');
      return response.data;
    },

    // Get rate limit usage
    getUsage: async (): Promise<RateLimitUsage[]> => {
      const response = await api.get('/admin/rate-limits/usage');
      return response.data;
    },

    // Get rate limit summary
    getSummary: async (): Promise<RateLimitSummary> => {
      const response = await api.get('/admin/rate-limits/summary');
      return response.data;
    },

    // Cleanup expired rate limit data
    cleanup: async (): Promise<{ cleaned: number }> => {
      const response = await api.post('/admin/rate-limits/cleanup');
      return response.data;
    },
  },

  // System Administration
  system: {
    // Get system health and status
    getHealth: async (): Promise<{
      status: 'healthy' | 'degraded' | 'unhealthy';
      uptime: number;
      version: string;
      database_status: 'connected' | 'disconnected';
      active_agents: number;
      active_tokens: number;
      rate_limits_enabled: boolean;
    }> => {
      const response = await api.get('/admin/system/health');
      return response.data;
    },

    // Get active agents
    getActiveAgents: async (): Promise<{
      agents: Array<{
        id: string;
        hostname: string;
        last_seen: string;
        status: string;
      }>;
      count: number;
    }> => {
      const response = await api.get('/admin/system/active-agents');
      return response.data;
    },

    // Get system configuration
    getConfig: async (): Promise<Record<string, any>> => {
      const response = await api.get('/admin/system/config');
      return response.data;
    },

    // Update system configuration
    updateConfig: async (config: Record<string, any>): Promise<Record<string, any>> => {
      const response = await api.put('/admin/system/config', config);
      return response.data;
    },
  },
};

export default api;