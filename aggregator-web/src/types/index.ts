// API Response types
export interface ApiResponse<T = any> {
  data?: T;
  error?: string;
  message?: string;
}

// Agent types
export interface Agent {
  id: string;
  hostname: string;
  os_type: string;
  os_version: string;
  architecture: string;
  status: 'online' | 'offline';
  last_checkin: string;
  last_scan: string | null;
  created_at: string;
  updated_at: string;
  version: string;
  ip_address: string;
}

export interface AgentSpec {
  id: string;
  agent_id: string;
  cpu_cores: number;
  memory_mb: number;
  disk_gb: number;
  docker_version: string | null;
  kernel_version: string;
  metadata: Record<string, any>;
  created_at: string;
}

// Update types
export interface UpdatePackage {
  id: string;
  agent_id: string;
  package_type: 'apt' | 'docker' | 'yum' | 'dnf' | 'windows' | 'winget';
  package_name: string;
  current_version: string;
  available_version: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  status: 'pending' | 'approved' | 'scheduled' | 'installing' | 'installed' | 'failed';
  created_at: string;
  updated_at: string;
  approved_at: string | null;
  scheduled_at: string | null;
  installed_at: string | null;
  metadata: Record<string, any>;
}

// Update specific types
export interface DockerUpdateInfo {
  local_digest: string;
  remote_digest: string;
  image_name: string;
  tag: string;
  registry: string;
  size_bytes: number;
}

export interface AptUpdateInfo {
  package_name: string;
  current_version: string;
  new_version: string;
  section: string;
  priority: string;
  repository: string;
  size_bytes: number;
  cves: string[];
}

// Command types
export interface Command {
  id: string;
  agent_id: string;
  command_type: 'scan' | 'install' | 'update' | 'reboot';
  payload: Record<string, any>;
  status: 'pending' | 'running' | 'completed' | 'failed';
  created_at: string;
  updated_at: string;
  executed_at: string | null;
  completed_at: string | null;
}

// Log types
export interface UpdateLog {
  id: string;
  agent_id: string;
  update_package_id: string | null;
  command_id: string | null;
  level: 'info' | 'warn' | 'error' | 'debug';
  message: string;
  metadata: Record<string, any>;
  created_at: string;
}

// Dashboard stats
export interface DashboardStats {
  total_agents: number;
  online_agents: number;
  offline_agents: number;
  pending_updates: number;
  approved_updates: number;
  installed_updates: number;
  failed_updates: number;
  critical_updates: number;
  high_updates: number;
  medium_updates: number;
  low_updates: number;
  updates_by_type: Record<string, number>;
}

// API request/response types
export interface AgentListResponse {
  agents: Agent[];
  total: number;
}

export interface UpdateListResponse {
  updates: UpdatePackage[];
  total: number;
}

export interface UpdateApprovalRequest {
  update_ids: string[];
  scheduled_at?: string;
}

export interface ScanRequest {
  agent_ids?: string[];
  force?: boolean;
}

// Query parameters
export interface ListQueryParams {
  page?: number;
  limit?: number;
  status?: string;
  severity?: string;
  type?: string;
  search?: string;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
}

// UI State types
export interface FilterState {
  status: string[];
  severity: string[];
  type: string[];
  search: string;
}

export interface PaginationState {
  page: number;
  limit: number;
  total: number;
}

// WebSocket message types (for future real-time updates)
export interface WebSocketMessage {
  type: 'agent_status' | 'update_discovered' | 'update_installed' | 'command_completed';
  data: any;
  timestamp: string;
}

// Error types
export interface ApiError {
  message: string;
  code?: string;
  details?: any;
}