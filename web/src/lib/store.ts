import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import { Agent, UpdatePackage, FilterState } from '@/types';

// Auth store
interface AuthState {
  token: string | null;
  isAuthenticated: boolean;
  setToken: (token: string) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      isAuthenticated: false,
      setToken: (token) => set({ token, isAuthenticated: true }),
      logout: () => {
        // Clear all auth-related localStorage keys. The zustand persist
        // middleware writes to 'auth-storage'; remove it explicitly so stale
        // tokens from a server reinstall (JWT_SECRET rotation) don't survive
        // a logout + re-login cycle.
        localStorage.removeItem('auth_token');
        localStorage.removeItem('auth-storage');
        localStorage.removeItem('user');
        set({ token: null, isAuthenticated: false });
      },
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ token: state.token, isAuthenticated: state.isAuthenticated }),
      storage: createJSONStorage(() => localStorage),
    }
  )
);

// UI store for global state
interface UIState {
  sidebarOpen: boolean;
  theme: 'light' | 'dark';
  activeTab: string;
  setSidebarOpen: (open: boolean) => void;
  setTheme: (theme: 'light' | 'dark') => void;
  setActiveTab: (tab: string) => void;
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      theme: 'light',
      activeTab: 'dashboard',
      setSidebarOpen: (open) => set({ sidebarOpen: open }),
      setTheme: (theme) => set({ theme }),
      setActiveTab: (tab) => set({ activeTab: tab }),
    }),
    {
      name: 'ui-storage',
      storage: createJSONStorage(() => localStorage),
    }
  )
);

// Agent store
interface AgentState {
  agents: Agent[];
  selectedAgent: Agent | null;
  loading: boolean;
  error: string | null;
  setAgents: (agents: Agent[]) => void;
  setSelectedAgent: (agent: Agent | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  updateAgentStatus: (agentId: string, status: Agent['status'], lastCheckin: string) => void;
  addAgent: (agent: Agent) => void;
  removeAgent: (agentId: string) => void;
}

export const useAgentStore = create<AgentState>((set, get) => ({
  agents: [],
  selectedAgent: null,
  loading: false,
  error: null,

  setAgents: (agents) => set({ agents }),
  setSelectedAgent: (agent) => set({ selectedAgent: agent }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),

  updateAgentStatus: (agentId, status, lastCheckin) => {
    const { agents } = get();
    const updatedAgents = agents.map(agent =>
      agent.id === agentId
        ? { ...agent, status, last_checkin: lastCheckin }
        : agent
    );
    set({ agents: updatedAgents });
  },

  addAgent: (agent) => {
    const { agents } = get();
    set({ agents: [...agents, agent] });
  },

  removeAgent: (agentId) => {
    const { agents } = get();
    set({ agents: agents.filter(agent => agent.id !== agentId) });
  },
}));

// Updates store
interface UpdateState {
  updates: UpdatePackage[];
  selectedUpdate: UpdatePackage | null;
  filters: FilterState;
  loading: boolean;
  error: string | null;
  setUpdates: (updates: UpdatePackage[]) => void;
  setSelectedUpdate: (update: UpdatePackage | null) => void;
  setFilters: (filters: Partial<FilterState>) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  updateUpdateStatus: (updateId: string, status: UpdatePackage['status']) => void;
  bulkUpdateStatus: (updateIds: string[], status: UpdatePackage['status']) => void;
}

export const useUpdateStore = create<UpdateState>((set, get) => ({
  updates: [],
  selectedUpdate: null,
  filters: {
    status: [],
    severity: [],
    type: [],
    search: '',
  },
  loading: false,
  error: null,

  setUpdates: (updates) => set({ updates }),
  setSelectedUpdate: (update) => set({ selectedUpdate: update }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),

  setFilters: (newFilters) => {
    const { filters } = get();
    set({ filters: { ...filters, ...newFilters } });
  },

  updateUpdateStatus: (updateId, status) => {
    const { updates } = get();
    const updatedUpdates = updates.map(update =>
      update.id === updateId
        ? { ...update, status, updated_at: new Date().toISOString() }
        : update
    );
    set({ updates: updatedUpdates });
  },

  bulkUpdateStatus: (updateIds, status) => {
    const { updates } = get();
    const updatedUpdates = updates.map(update =>
      updateIds.includes(update.id)
        ? { ...update, status, updated_at: new Date().toISOString() }
        : update
    );
    set({ updates: updatedUpdates });
  },
}));

// Real-time updates store
interface RealtimeState {
  isConnected: boolean;
  lastUpdate: string | null;
  notifications: Array<{
    id: string;
    type: 'info' | 'success' | 'warning' | 'error';
    title: string;
    message: string;
    timestamp: string;
    read: boolean;
    // Optional deep-link target; clicking the notification navigates here.
    href?: string;
  }>;
  setConnected: (connected: boolean) => void;
  setLastUpdate: (timestamp: string) => void;
  addNotification: (notification: Omit<RealtimeState['notifications'][0], 'id' | 'timestamp' | 'read'>) => void;
  markNotificationRead: (id: string) => void;
  clearNotifications: () => void;
}

export const useRealtimeStore = create<RealtimeState>((set, get) => ({
  isConnected: false,
  lastUpdate: null,
  notifications: [],

  setConnected: (isConnected) => set({ isConnected }),
  setLastUpdate: (lastUpdate) => set({ lastUpdate }),

  addNotification: (notification) => {
    const { notifications } = get();
    const newNotification = {
      ...notification,
      id: Math.random().toString(36).substring(7),
      timestamp: new Date().toISOString(),
      read: false,
    };
    set({ notifications: [newNotification, ...notifications] });
  },

  markNotificationRead: (id) => {
    const { notifications } = get();
    const updatedNotifications = notifications.map(notification =>
      notification.id === id ? { ...notification, read: true } : notification
    );
    set({ notifications: updatedNotifications });
  },

  clearNotifications: () => set({ notifications: [] }),
}));

// Settings store
interface SettingsState {
  autoRefresh: boolean;
  refreshInterval: number;
  compactView: boolean;
  setAutoRefresh: (enabled: boolean) => void;
  setRefreshInterval: (interval: number) => void;
  setCompactView: (enabled: boolean) => void;
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      autoRefresh: true,
      refreshInterval: 30000, // 30 seconds
      compactView: false,

      setAutoRefresh: (autoRefresh) => set({ autoRefresh }),
      setRefreshInterval: (refreshInterval) => set({ refreshInterval }),
      setCompactView: (compactView) => set({ compactView }),
    }),
    {
      name: 'settings-storage',
      storage: createJSONStorage(() => localStorage),
    }
  )
);

// Server connection status. Updated by the axios response interceptor in
// api.ts on every authenticated API call — no separate polling timer needed.
// Not persisted; a fresh tab starts connected=true (optimistic) and the first
// failed poll corrects it.
const VERSION_STORAGE_KEY = 'redflag_server_version';

interface ServerStatusState {
  connected: boolean;
  version: string | null;
  versionChanged: boolean;
  markConnected: (version: string | null) => void;
  markDisconnected: () => void;
}

export const useServerStatusStore = create<ServerStatusState>((set) => ({
  connected: true,
  version: null,
  versionChanged: false,

  markConnected: (version) => {
    if (!version) {
      set({ connected: true });
      return;
    }
    const stored = sessionStorage.getItem(VERSION_STORAGE_KEY);
    if (stored === null) {
      // First successful response this tab — record baseline.
      sessionStorage.setItem(VERSION_STORAGE_KEY, version);
      set({ connected: true, version });
    } else if (stored !== version) {
      // Server restarted with a new version.
      sessionStorage.setItem(VERSION_STORAGE_KEY, version);
      set({ connected: true, version, versionChanged: true });
    } else {
      set({ connected: true, version });
    }
  },

  markDisconnected: () => {
    set({ connected: false });
  },
}));