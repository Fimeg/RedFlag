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
  ApiResponse,
  ApiError
} from '@/types';

// Create axios instance
const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '/api/v1',
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
};

export const statsApi = {
  // Get dashboard statistics
  getDashboardStats: async (): Promise<DashboardStats> => {
    const response = await api.get('/stats/summary');
    return response.data;
  },
};

export const authApi = {
  // Simple login (using API key or token)
  login: async (credentials: { token: string }): Promise<{ token: string }> => {
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

    if (status >= 500) {
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

export default api;