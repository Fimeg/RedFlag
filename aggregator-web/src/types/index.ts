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
  os_architecture: string;
  architecture: string; // For backward compatibility
  agent_version: string;
  version: string; // For backward compatibility
  last_seen: string;
  last_checkin: string; // For backward compatibility
  last_scan: string | null;
  status: 'online' | 'offline';
  created_at: string;
  updated_at: string;
  metadata?: Record<string, any>;
  // Note: ip_address not available from API yet
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
  status: 'pending' | 'approved' | 'scheduled' | 'installing' | 'installed' | 'failed' | 'checking_dependencies' | 'pending_dependencies';
  // Timestamp fields - matching backend API response
  last_discovered_at: string;  // When package was first discovered
  last_updated_at: string;     // When package status was last updated
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

// Docker-specific types for dedicated Docker module
export interface DockerContainer {
  id: string;
  agent_id: string;
  name: string;
  image_id: string;
  image_name: string;
  image_tag: string;
  status: 'running' | 'stopped' | 'paused' | 'restarting' | 'removing' | 'exited' | 'dead';
  created_at: string;
  started_at: string | null;
  ports: DockerPort[];
  volumes: DockerVolume[];
  labels: Record<string, string>;
  metadata: Record<string, any>;
}

export interface DockerImage {
  id: string;
  agent_id: string;
  repository: string;
  tag: string;
  digest: string;
  size_bytes: number;
  created_at: string;
  last_pulled: string | null;
  update_available: boolean;
  current_version: string;
  available_version: string | null;
  severity: 'low' | 'medium' | 'high' | 'critical';
  status: 'up-to-date' | 'update-available' | 'update-approved' | 'update-scheduled' | 'update-installing' | 'update-failed';
  update_approved_at: string | null;
  update_scheduled_at: string | null;
  update_installed_at: string | null;
  metadata: Record<string, any>;
}

export interface DockerPort {
  container_port: number;
  host_port: number | null;
  protocol: 'tcp' | 'udp';
  host_ip: string;
}

export interface DockerVolume {
  name: string;
  source: string;
  destination: string;
  mode: 'ro' | 'rw';
  driver: string;
}

// Docker API response types
export interface DockerContainerListResponse {
  containers: DockerContainer[];
  images: DockerImage[];
  total_containers: number;
  total_images: number;
  page: number;
  page_size: number;
  total_pages: number;
}

export interface DockerStats {
  total_containers: number;
  running_containers: number;
  stopped_containers: number;
  total_images: number;
  images_with_updates: number;
  critical_updates: number;
  high_updates: number;
  medium_updates: number;
  low_updates: number;
  agents_with_docker: number;
  total_storage_used: number;
}

// Docker action types
export interface DockerUpdateRequest {
  image_id: string;
  scheduled_at?: string;
}

export interface BulkDockerUpdateRequest {
  updates: Array<{
    container_id: string;
    image_id: string;
  }>;
  scheduled_at?: string;
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
  page: number;
  page_size: number;
  stats?: UpdateStats;
}

export interface UpdateStats {
  total_updates: number;
  pending_updates: number;
  approved_updates: number;
  updated_updates: number;
  failed_updates: number;
  critical_updates: number;
  high_updates: number;
  important_updates: number;
  moderate_updates: number;
  low_updates: number;
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
  page_size?: number;
  limit?: number;
  status?: string;
  severity?: string;
  type?: string;
  search?: string;
  agent?: string;
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

// Registration Token types
export interface RegistrationToken {
  id: string;
  token: string;
  label: string | null;
  expires_at: string;
  created_at: string;
  used_at: string | null;
  used_by_agent_id: string | null;
  revoked: boolean;
  revoked_at: string | null;
  revoked_reason: string | null;
  status: 'active' | 'used' | 'expired' | 'revoked';
  created_by: string;
  metadata: Record<string, any>;
  max_seats: number;
  seats_used: number;
}

export interface CreateRegistrationTokenRequest {
  label: string;
  expires_in?: string;
  max_seats?: number;
  metadata?: Record<string, any>;
}

export interface RegistrationTokenStats {
  total_tokens: number;
  active_tokens: number;
  used_tokens: number;
  expired_tokens: number;
  revoked_tokens: number;
  total_seats_used: number;
  total_seats_available: number;
}

// Rate Limiting types
export interface RateLimitConfig {
  endpoint: string;
  requests_per_minute: number;
  window_minutes: number;
  max_requests: number;
  burst_allowance: number;
  metadata: Record<string, any>;
}

export interface RateLimitStats {
  endpoint: string;
  current_requests: number;
  limit: number;
  window_start: string;
  window_end: string;
  blocked_requests: number;
  top_clients: Array<{
    identifier: string;
    request_count: number;
  }>;
}

export interface RateLimitUsage {
  endpoint: string;
  limit: number;
  current: number;
  remaining: number;
  reset_time: string;
  window_minutes: number;
}

export interface RateLimitSummary {
  total_endpoints: number;
  active_endpoints: number;
  total_requests_per_minute: number;
  most_active_endpoint: string;
  average_utilization: number;
}