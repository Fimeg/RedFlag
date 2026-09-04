import axios, { AxiosResponse } from 'axios';
import { useAuthStore, useServerStatusStore } from './store';
import {
  Agent,
  UpdatePackage,
  AgentUpdatePackage,
  DashboardStats,
  AgentListResponse,
  UpdateListResponse,
  PackageListResponse,
  PackageFleetResponse,
  PackageVersionsResponse,
  UpdateApprovalRequest,
  ScanRequest,
  ListQueryParams,
  ApiError,
  DockerContainerListResponse,
  DockerStats,
  RegistrationToken,
  CreateRegistrationTokenRequest,
  RegistrationTokenStats,
  SigningKey,
  RateLimitSettings,
  RateLimitStatsResponse,
  TrackedSoftware,
  CreateTrackedSoftwareRequest,
  UpdateTrackedSoftwareSettingsRequest,
  UpstreamDriftEvent,
  AgentTrackedSoftware,
  AgentTrackedSoftwareView,
  AgentInstallation,
  CreateAgentBindingRequest,
  AgentSubsystem,
  SubsystemConfig,
  SubsystemStats,
  MaintenanceWindow,
  CreateMaintenanceWindowRequest,
  CapabilityTokenStatusResponse,
  PackageSummary,
  PackageAgentsResponse,
  PackageVulnerabilitiesResponse,
  RuntimeDockerContainer,
  RuntimeDockerStack,
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

// Response interceptor to handle errors.
// BUG-017: do NOT hard-redirect via window.location.href — that triggers a
// full page reload, which re-runs WelcomeChecker/SetupCompletionChecker
// against a now-unauthenticated session and causes the flicker/loop. Clear
// auth state and let the React Router <ProtectedRoute> render <Navigate />
// to /login on the next render. /auth/* and /setup/* requests are excluded
// because a 401 there is expected (login form, setup wizard).
api.interceptors.response.use(
  (response: AxiosResponse) => response,
  async (error) => {
    if (error.response?.status === 401) {
      const url: string = error.config?.url || '';
      const isAuthEndpoint = url.includes('/auth/');
      if (!isAuthEndpoint) {
        useAuthStore.getState().logout();
      }
    }
    return Promise.reject(error);
  }
);

// Error logging interceptor
import { clientErrorLogger } from './client-error-logger';
api.interceptors.response.use(
  (response) => response,
  async (error) => {
    // Don't log errors from the error logger itself
    if (error.config?.headers?.['X-Error-Logger-Request']) {
      return Promise.reject(error);
    }

    // Extract subsystem from URL
    const subsystem = extractSubsystem(error.config?.url);

    // Log API errors
    clientErrorLogger.logError({
      subsystem,
      error_type: 'api_error',
      message: error.message,
      metadata: {
        status_code: error.response?.status,
        endpoint: error.config?.url,
        method: error.config?.method,
        response_data: error.response?.data,
      },
    }).catch(() => {
      // Don't let logging errors hide the original error
    });

    return Promise.reject(error);
  }
);

// Server status interceptor — piggybacks on every API call to track
// connection state and server version. No separate health poll needed.
api.interceptors.response.use(
  (response: AxiosResponse) => {
    const version = response.headers['x-redflag-version'];
    useServerStatusStore.getState().markConnected(version || null);
    return response;
  },
  async (error) => {
    // Network error (no response) = server unreachable. HTTP errors (4xx/5xx)
    // mean the server IS reachable — don't mark disconnected for those.
    if (!error.response) {
      useServerStatusStore.getState().markDisconnected();
    }
    return Promise.reject(error);
  }
);

function extractSubsystem(url: string = ''): string {
  const matches = url.match(/\/(storage|system|docker|updates|agent)/);
  return matches ? matches[1] : 'unknown';
}

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

  // Trigger screenshot capture on an agent
  captureScreenshot: async (agentId: string): Promise<{ message: string; command_id: string }> => {
    const response = await api.post(`/agents/${agentId}/screenshot`);
    return response.data;
  },

  // Set or clear the operator device-type override (SERVER-002).
  // deviceType null clears the override, reverting to auto-detected.
  reclassifyDeviceType: async (
    agentId: string,
    deviceType: string | null
  ): Promise<{ id: string; device_type: string; device_type_manual: string | null; effective_device_type: string }> => {
    const response = await api.put(`/admin/agents/${agentId}/device-type`, { device_type: deviceType });
    return response.data;
  },

  // Get a single command by ID (used to poll screenshot result)
  getCommand: async (commandId: string): Promise<any> => {
    const response = await api.get(`/commands/${commandId}`);
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

  // Subsystem Management
  getSubsystems: async (agentId: string): Promise<AgentSubsystem[]> => {
    const response = await api.get(`/agents/${agentId}/subsystems`);
    return response.data;
  },

  getSubsystem: async (agentId: string, subsystem: string): Promise<AgentSubsystem> => {
    const response = await api.get(`/agents/${agentId}/subsystems/${subsystem}`);
    return response.data;
  },

  updateSubsystem: async (agentId: string, subsystem: string, config: SubsystemConfig): Promise<{ message: string }> => {
    const response = await api.patch(`/agents/${agentId}/subsystems/${subsystem}`, config);
    return response.data;
  },

  enableSubsystem: async (agentId: string, subsystem: string): Promise<{ message: string }> => {
    const response = await api.post(`/agents/${agentId}/subsystems/${subsystem}/enable`);
    return response.data;
  },

  disableSubsystem: async (agentId: string, subsystem: string): Promise<{ message: string }> => {
    const response = await api.post(`/agents/${agentId}/subsystems/${subsystem}/disable`);
    return response.data;
  },

  triggerSubsystem: async (agentId: string, subsystem: string): Promise<{ message: string; command_id: string }> => {
    const response = await api.post(`/agents/${agentId}/subsystems/${subsystem}/trigger`);
    return response.data;
  },

  getSubsystemStats: async (agentId: string, subsystem: string): Promise<SubsystemStats> => {
    const response = await api.get(`/agents/${agentId}/subsystems/${subsystem}/stats`);
    return response.data;
  },

  setSubsystemAutoRun: async (agentId: string, subsystem: string, autoRun: boolean): Promise<{ message: string }> => {
    const response = await api.post(`/agents/${agentId}/subsystems/${subsystem}/auto-run`, { auto_run: autoRun });
    return response.data;
  },

  setSubsystemInterval: async (agentId: string, subsystem: string, intervalMinutes: number): Promise<{ message: string }> => {
    const response = await api.post(`/agents/${agentId}/subsystems/${subsystem}/interval`, { interval_minutes: intervalMinutes });
    return response.data;
  },

  // Update single agent
  updateAgent: async (agentId: string, updateData: {
    version: string;
    platform: string;
    scheduled?: string;
    nonce?: string;
  }): Promise<{ message: string; update_id: string; download_url: string; signature: string; checksum: string; file_size: number; estimated_time: number }> => {
    const response = await api.post(`/agents/${agentId}/update`, updateData);
    return response.data;
  },

  // Check if update available for agent
  checkForUpdateAvailable: async (agentId: string): Promise<{ hasUpdate: boolean; currentVersion: string; latestVersion?: string; reason?: string; platform?: string }> => {
    const response = await api.get(`/agents/${agentId}/updates/available`);
    return response.data;
  },

  // Get update status for agent
  getUpdateStatus: async (agentId: string): Promise<{ status: string; progress?: number; error?: string }> => {
    const response = await api.get(`/agents/${agentId}/updates/status`);
    return response.data;
  },

  // Generate update nonce for agent (new security feature)
  generateUpdateNonce: async (agentId: string, targetVersion: string): Promise<{ agent_id: string; target_version: string; update_nonce: string; expires_at: number; expires_in_seconds: number }> => {
    const response = await api.post(`/agents/${agentId}/update-nonce?target_version=${targetVersion}`);
    return response.data;
  },

  // Get agent metrics
  getAgentMetrics: async (agentId: string): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/metrics`);
    return response.data;
  },

  // Get agent storage metrics
  getAgentStorageMetrics: async (agentId: string): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/metrics/storage`);
    return response.data;
  },

  // Get dedicated storage metrics (new storage_metrics table)
  getStorageMetrics: async (agentId: string): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/storage-metrics`);
    return response.data;
  },

  // Process Explorer — on-demand process scanning
  getLatestProcessSnapshot: async (agentId: string, params?: Record<string, string>): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/processes`, { params });
    return response.data;
  },
  getProcessDetail: async (agentId: string, processId: string): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/processes/${processId}`);
    return response.data;
  },
  triggerProcessScan: async (agentId: string): Promise<any> => {
    const response = await api.post(`/agents/${agentId}/processes/scan`);
    return response.data;
  },

  // Get agent system metrics
  getAgentSystemMetrics: async (agentId: string): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/metrics/system`);
    return response.data;
  },

  // Get agent Docker images
  getAgentDockerImages: async (agentId: string, params?: any): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/docker-images`, { params });
    return response.data;
  },

  // Get agent Docker info
  getAgentDockerInfo: async (agentId: string): Promise<any> => {
    const response = await api.get(`/agents/${agentId}/docker-info`);
    return response.data;
  },

  // Update multiple agents (bulk)
  updateMultipleAgents: async (updateData: {
    agent_ids: string[];
    version: string;
    platform: string;
    scheduled?: string;
    nonces?: string[];
  }): Promise<{ message: string; updated: Array<{ agent_id: string; hostname: string; update_id: string; status: string }>; failed: string[]; total_agents: number; package_info: any }> => {
    const response = await api.post('/agents/bulk-update', updateData);
    return response.data;
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

  // Approve multiple updates
  approveMultiple: async (updateIds: string[]): Promise<void> => {
    await api.post('/updates/approve', { update_ids: updateIds });
  },

  // Reject/cancel update
  rejectUpdate: async (id: string): Promise<void> => {
    await api.post(`/updates/${id}/reject`);
  },

  // Install update immediately
  installUpdate: async (id: string): Promise<void> => {
    await api.post(`/updates/${id}/install`);
  },

  // Confirm dependencies and proceed with install
  confirmDependencies: async (id: string): Promise<void> => {
    await api.post(`/updates/${id}/confirm-dependencies`);
  },

  // Get the fleet rolled up by package (package-centric Updates view)
  getPackageList: async (params?: ListQueryParams): Promise<PackageListResponse> => {
    const response = await api.get('/packages', { params });
    return response.data;
  },

  // Get the fleet view for a package (every agent affected by the same package)
  getPackageFleet: async (id: string): Promise<PackageFleetResponse> => {
    const response = await api.get(`/updates/${id}/fleet`);
    return response.data;
  },

  // Get the version timeline catalog for a package
  getPackageVersions: async (id: string): Promise<PackageVersionsResponse> => {
    const response = await api.get(`/updates/${id}/versions`);
    return response.data;
  },

  // Package-centric endpoints (by type + name, not by update row ID)
  getPackageSummaryByCoords: async (pkgType: string, pkgName: string): Promise<PackageSummary> => {
    const response = await api.get(`/updates/package/${pkgType}/${pkgName}`);
    return response.data;
  },

  getPackageAgentsByCoords: async (pkgType: string, pkgName: string): Promise<PackageAgentsResponse> => {
    const response = await api.get(`/updates/package/${pkgType}/${pkgName}/agents`);
    return response.data;
  },

  getPackageVersionsByCoords: async (pkgType: string, pkgName: string): Promise<PackageVersionsResponse> => {
    const response = await api.get(`/updates/package/${pkgType}/${pkgName}/versions`);
    return response.data;
  },

  getPackageVulnerabilitiesByCoords: async (pkgType: string, pkgName: string): Promise<PackageVulnerabilitiesResponse> => {
    const response = await api.get(`/updates/package/${pkgType}/${pkgName}/vulnerabilities`);
    return response.data;
  },

  // Get update logs
  getUpdateLogs: async (id: string, limit?: number): Promise<{ logs: any[]; count: number }> => {
    const response = await api.get(`/updates/${id}/logs`, {
      params: limit ? { limit } : undefined
    });
    return response.data;
  },

  // Get lifecycle history (state transitions) for an update
  getUpdateLifecycle: async (id: string): Promise<{ history: any[]; package_type: string; package_name: string; count: number }> => {
    const response = await api.get(`/updates/${id}/lifecycle`);
    return response.data;
  },

  // Retry a failed, timed_out, or cancelled command
  retryCommand: async (commandId: string): Promise<{ message: string; command_id: string; new_id: string }> => {
    const response = await api.post(`/commands/${commandId}/retry`);
    return response.data;
  },

  // Re-open a failed update for another lifecycle attempt (failed → pending)
  reopenUpdate: async (updateId: string): Promise<{ message: string }> => {
    const response = await api.post(`/updates/${updateId}/reopen`);
    return response.data;
  },

  // Resolve a failed update as installed (update no longer applies)
  resolveUpdate: async (updateId: string): Promise<{ message: string }> => {
    const response = await api.post(`/updates/${updateId}/resolve`);
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

  // Get available update packages
  getPackages: async (params?: {
    version?: string;
    platform?: string;
    limit?: number;
    offset?: number;
  }): Promise<{ packages: AgentUpdatePackage[]; total: number; limit: number; offset: number }> => {
    const response = await api.get('/updates/packages', { params });
    return response.data;
  },

  // Sign new update package
  signPackage: async (packageData: {
    version: string;
    platform: string;
    architecture: string;
    binary_path: string;
  }): Promise<{ message: string; package: UpdatePackage }> => {
    const response = await api.post('/updates/packages/sign', packageData);
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

// Advisory-feed health. The OSV/repology breakers fail open (an unreachable feed
// never blocks a patch) while the auto-confirm gate is fail-closed — so a degraded
// feed silently suspends auto-approval. This surfaces that state for the banner.
export interface BreakerStat {
  name: string;
  state: 'closed' | 'open' | 'half-open' | string;
  recent_failures: number;
  consecutive_success: number;
  next_attempt?: string;
}

export interface AdvisoryHealth {
  osv: BreakerStat;
  repology: BreakerStat;
  deferred_packages: number; // -1 when the count couldn't be read
  degraded: boolean;
}

export const healthApi = {
  getAdvisory: async (): Promise<AdvisoryHealth> => {
    const response = await api.get('/health/advisory');
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

// Boundary logging for the setup flow (this instance carried no interceptors,
// so checkHealth/configure failures bypassed client_errors entirely).
setupApiInstance.interceptors.response.use(
  (response) => response,
  async (error) => {
    clientErrorLogger.logError({
      subsystem: 'setup',
      error_type: 'api_error',
      message: error.message,
      metadata: {
        status_code: error.response?.status,
        endpoint: error.config?.url,
        method: error.config?.method,
        response_data: error.response?.data,
      },
    }).catch(() => {});
    return Promise.reject(error);
  }
);

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
    publicURL: string;
    maxSeats: string;
    signingPrivateKey?: string;
    signingPublicKey?: string;
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

  // Runtime container state (DOCKER-ENRICHED-SCAN)
  getAgentRuntimeContainers: async (agentId: string): Promise<{ containers: RuntimeDockerContainer[] }> => {
    const response = await api.get(`/agents/${agentId}/docker-containers`);
    return response.data;
  },

  getAgentRuntimeStacks: async (agentId: string): Promise<{ stacks: RuntimeDockerStack[] }> => {
    const response = await api.get(`/agents/${agentId}/docker-stacks`);
    return response.data;
  },

  getFleetRuntimeContainers: async (): Promise<{ containers: RuntimeDockerContainer[] }> => {
    const response = await api.get('/docker/fleet-containers');
    return response.data;
  },

  getFleetRuntimeStacks: async (): Promise<{ stacks: RuntimeDockerStack[] }> => {
    const response = await api.get('/docker/fleet-stacks');
    return response.data;
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

    // Get agents bound to a registration token (by token UUID)
    getBoundAgents: async (id: string): Promise<{ agents: import('@/types').BoundAgent[]; count: number }> => {
      const response = await api.get(`/admin/registration-tokens/${id}/agents`);
      return response.data;
    },
  },

  // Admin agent operations (revoke, etc.)
  agents: {
    // Revoke an agent — invalidates refresh tokens so the agent can no longer
    // check in. Enrolled agents are NOT affected by token revocation; this is
    // the explicit per-agent path.
    revoke: async (agentId: string, reason?: string): Promise<{ status: string; agent_id: string }> => {
      const response = await api.post(`/admin/agents/${agentId}/revoke`, { reason });
      return response.data;
    },
  },

  // Signing key management — Ed25519 key rotation roster.
  signingKeys: {
    list: async (): Promise<{ keys: SigningKey[]; count: number }> => {
      const response = await api.get('/admin/signing-keys');
      return response.data;
    },
    deprecate: async (keyId: string): Promise<{ message: string; key_id: string }> => {
      const response = await api.post(`/admin/signing-keys/${keyId}/deprecate`);
      return response.data;
    },
  },

  // Upstream version sync (Repology + endoflife.date)
  upstream: {
    list: async (): Promise<{ software: TrackedSoftware[]; sources: string[] }> => {
      const response = await api.get('/admin/upstream');
      return response.data;
    },

    listDrifted: async (): Promise<TrackedSoftware[]> => {
      const response = await api.get('/admin/upstream/drift');
      return response.data.software ?? [];
    },

    recentDriftEvents: async (): Promise<UpstreamDriftEvent[]> => {
      const response = await api.get('/admin/upstream/drift/events');
      return response.data.events ?? [];
    },

    create: async (input: CreateTrackedSoftwareRequest): Promise<TrackedSoftware> => {
      const response = await api.post('/admin/upstream', input);
      return response.data;
    },

    updateSettings: async (id: string, input: UpdateTrackedSoftwareSettingsRequest): Promise<TrackedSoftware> => {
      const response = await api.patch(`/admin/upstream/${id}`, input);
      return response.data;
    },

    remove: async (id: string): Promise<void> => {
      await api.delete(`/admin/upstream/${id}`);
    },

    syncNow: async (id: string): Promise<TrackedSoftware> => {
      const response = await api.post(`/admin/upstream/${id}/sync`);
      return response.data;
    },

    // Symmetric view: which agents have this tracked_software bound?
    installations: async (softwareId: string): Promise<AgentInstallation[]> => {
      const response = await api.get(`/admin/upstream/${softwareId}/installations`);
      return response.data.installations ?? [];
    },
  },

  // Agent <-> tracked_software bindings. Migration 039 / agent_tracked_software.
  // Bindings live under the agent so the authorization boundary is the agent id.
  bindings: {
    listByAgent: async (agentId: string): Promise<AgentTrackedSoftwareView[]> => {
      const response = await api.get(`/admin/agents/${agentId}/tracked-software`);
      return response.data.bindings ?? [];
    },

    upsert: async (
      agentId: string,
      input: CreateAgentBindingRequest,
    ): Promise<AgentTrackedSoftware> => {
      const response = await api.post(`/admin/agents/${agentId}/tracked-software`, input);
      return response.data;
    },

    remove: async (agentId: string, bindingId: string): Promise<void> => {
      await api.delete(`/admin/agents/${agentId}/tracked-software/${bindingId}`);
    },
  },

  // Rate Limiting Management
  rateLimits: {
    getSettings: async (): Promise<RateLimitSettings> => {
      const response = await api.get('/admin/rate-limits');
      return response.data.settings;
    },

    updateSettings: async (settings: RateLimitSettings): Promise<RateLimitSettings> => {
      const response = await api.put('/admin/rate-limits', settings);
      return response.data.settings;
    },

    resetSettings: async (): Promise<RateLimitSettings> => {
      const response = await api.post('/admin/rate-limits/reset');
      return response.data.settings;
    },

    getStats: async (): Promise<RateLimitStatsResponse> => {
      const response = await api.get('/admin/rate-limits/stats');
      return response.data;
    },

    cleanup: async (): Promise<void> => {
      await api.post('/admin/rate-limits/cleanup');
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

  // Maintenance window management
  maintenanceWindows: {
    list: async (): Promise<{ windows: MaintenanceWindow[] }> => {
      const response = await api.get('/admin/maintenance-windows');
      return response.data;
    },
    get: async (id: string): Promise<MaintenanceWindow> => {
      const response = await api.get(`/admin/maintenance-windows/${id}`);
      return response.data;
    },
    create: async (request: CreateMaintenanceWindowRequest): Promise<MaintenanceWindow> => {
      const response = await api.post('/admin/maintenance-windows', request);
      return response.data;
    },
    update: async (id: string, data: Partial<CreateMaintenanceWindowRequest>): Promise<void> => {
      await api.put(`/admin/maintenance-windows/${id}`, data);
    },
    delete: async (id: string): Promise<void> => {
      await api.delete(`/admin/maintenance-windows/${id}`);
    },
    check: async (): Promise<{ inside_window: boolean }> => {
      const response = await api.get('/admin/maintenance-windows/check');
      return response.data;
    },
  },
};

// Capability token visibility API (LIFECYCLE-005)
export const capabilityTokenApi = {
  getTokenStatus: async (agentId: string, updateId?: string): Promise<CapabilityTokenStatusResponse> => {
    const params: Record<string, string> = { agent_id: agentId };
    if (updateId) params.update_id = updateId;
    const response = await api.get('/capability-tokens/status', { params });
    return response.data;
  },
};

// Security API endpoints
export const securityApi = {
  // Get comprehensive security overview
  getOverview: async (): Promise<{
    timestamp: string;
    overall_status: 'healthy' | 'degraded' | 'unhealthy';
    subsystems: {
      ed25519_signing: {
        status: string;
        enabled: boolean;
        metrics?: { total_pending_commands?: number; commands_last_hour?: number };
        checks?: { public_key_fingerprint?: string; algorithm?: string; recent_violations?: number; validation_failures?: number; max_age_minutes?: number; bound_agents?: string };
      };
      nonce_validation: {
        status: string;
        enabled: boolean;
        metrics?: { total_pending_commands?: number; commands_last_hour?: number };
        checks?: { public_key_fingerprint?: string; algorithm?: string; recent_violations?: number; validation_failures?: number; max_age_minutes?: number; bound_agents?: string };
      };
      machine_binding: {
        status: string;
        enabled: boolean;
        metrics?: { total_pending_commands?: number; commands_last_hour?: number };
        checks?: { public_key_fingerprint?: string; algorithm?: string; recent_violations?: number; validation_failures?: number; max_age_minutes?: number; bound_agents?: string };
      };
      command_validation: {
        status: string;
        enabled: boolean;
        metrics?: { total_pending_commands?: number; commands_last_hour?: number };
        checks?: { public_key_fingerprint?: string; algorithm?: string; recent_violations?: number; validation_failures?: number; max_age_minutes?: number; bound_agents?: string };
      };
    };
    alerts: string[];
    recommendations: string[];
  }> => {
    const response = await api.get('/security/overview');
    return response.data;
  },

  // Get Ed25519 signing service status
  getSigningStatus: async (): Promise<{
    status: string;
    timestamp: string;
    checks: {
      service_initialized: boolean;
      public_key_available: boolean;
      signing_operational: boolean;
    };
    public_key_fingerprint?: string;
    algorithm?: string;
  }> => {
    const response = await api.get('/security/signing');
    return response.data;
  },

  // Get nonce validation status
  getNonceStatus: async (): Promise<{
    status: string;
    timestamp: string;
    checks: {
      validation_enabled: boolean;
      max_age_minutes: number;
      recent_validations: number;
      validation_failures: number;
    };
    details: {
      nonce_format: string;
      signature_algorithm: string;
      replay_protection: string;
    };
  }> => {
    const response = await api.get('/security/nonce');
    return response.data;
  },

  // Get machine binding status
  getMachineBindingStatus: async (): Promise<{
    status: string;
    timestamp: string;
    checks: {
      binding_enforced: boolean;
      min_agent_version: string;
      fingerprint_required: boolean;
      recent_violations: number;
    };
    details: {
      enforcement_method: string;
      binding_scope: string;
      violation_action: string;
    };
  }> => {
    const response = await api.get('/security/machine-binding');
    return response.data;
  },

  // Get command validation status
  getCommandValidationStatus: async (): Promise<{
    status: string;
    timestamp: string;
    metrics: {
      total_pending_commands: number;
      agents_with_pending: number;
      commands_last_hour: number;
      commands_last_24h: number;
    };
    checks: {
      command_processing: string;
      backpressure_active: boolean;
      agent_responsive: string;
    };
  }> => {
    const response = await api.get('/security/commands');
    return response.data;
  },

  // Get detailed security metrics
  getMetrics: async (): Promise<{
    timestamp: string;
    signing: {
      public_key_fingerprint: string;
      algorithm: string;
      key_size: number;
      configured: boolean;
    };
    nonce: {
      max_age_seconds: number;
      format: string;
    };
    machine_binding: {
      min_version: string;
      enforcement: string;
    };
    command_processing: {
      backpressure_threshold: number;
      rate_limit_per_second: number;
    };
  }> => {
    const response = await api.get('/security/metrics');
    return response.data;
  },
};

// Storage Metrics API
export const storageMetricsApi = {
  // Report storage metrics (agent only)
  async reportStorageMetrics(agentID: string, data: any): Promise<void> {
    await api.post(`/agents/${agentID}/storage-metrics`, data);
  },

  // Get storage metrics for an agent
  async getStorageMetrics(agentID: string): Promise<any> {
    const response = await api.get(`/agents/${agentID}/storage-metrics`);
    return response.data;
  },
};

// Named export for api instance
export { api };

export default api;
