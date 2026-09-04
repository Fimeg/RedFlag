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
  current_version?: string;
  reboot_required?: boolean;
  last_reboot_at?: string | null;
  reboot_reason?: string;
  metadata?: Record<string, any>;
  system_info?: Record<string, any>;
  is_updating?: boolean;
  updating_to_version?: string;
  update_available?: boolean;
  // Device classification (DEVICE-001/SERVER-001)
  device_type?: string;             // agent-detected: server | desktop | laptop | phone | tablet | vm | container
  device_type_manual?: string;      // operator override, absent = use auto
  device_model?: string;            // "Google Pixel 3", "Dell PowerEdge R740"
  os_distro?: string;               // /etc/os-release ID: "arch", "fedora"
  effective_device_type?: string;   // server-computed COALESCE(manual, auto)
  // Note: ip_address not available from API yet
}

export type DeviceType = 'server' | 'desktop' | 'laptop' | 'phone' | 'tablet' | 'vm' | 'container';

export const DEVICE_TYPES: DeviceType[] = ['server', 'desktop', 'laptop', 'phone', 'tablet', 'vm', 'container'];

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
  status: 'pending' | 'approved' | 'checking_dependencies' | 'pending_dependencies' | 'installing' | 'installed' | 'failed' | 'ignored';
  // Timestamp fields - matching backend API response
  last_discovered_at: string;  // When package was first discovered
  last_updated_at: string;     // When package status was last updated
  approved_at: string | null;
  scheduled_at: string | null;
  installed_at: string | null;
  created_at: string;
  recent_command_id?: string;
  dependencies?: string[]; // List of dependency packages found during dry run
  expected_sha256?: string | null; // Layer 1 hash registry: pinned artifact hash
  metadata: Record<string, any>;
}

// Agent Update Package (for agent binary updates)
export interface AgentUpdatePackage {
  id: string;
  version: string;
  platform: string;
  architecture: string;
  file_size: number;
  checksum: string;
  created_at: string;
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
  agent_name?: string;
  agent_hostname?: string;
  name: string;
  image_id: string;
  image: string;
  tag: string;
  status: 'running' | 'stopped' | 'paused' | 'restarting' | 'removing' | 'exited' | 'dead';
  severity?: 'low' | 'medium' | 'high' | 'critical';
  created_at: string;
  started_at: string | null;
  ports: DockerPort[];
  volumes: DockerVolume[];
  labels: Record<string, string>;
  metadata: Record<string, any>;
  update_available?: boolean;
  current_version?: string;
  available_version?: string | null;
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

// Runtime container state reported by agents (DOCKER-ENRICHED-SCAN)
export interface RuntimeDockerContainer {
  id: string;
  agent_id: string;
  container_id: string;
  name: string;
  image: string;
  image_id: string;
  state: string;
  health: string;
  stack_name: string;
  ports: string;
  created_at_epoch: number;
  labels: Record<string, string>;
  last_seen_at: string;
}

export interface RuntimeDockerStack {
  id: string;
  agent_id: string;
  name: string;
  container_count: number;
  running_count: number;
  last_seen_at: string;
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
  command_type: 'scan' | 'install' | 'update' | 'reboot' | 'enable_heartbeat' | 'disable_heartbeat';
  payload: Record<string, any>;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'sent' | 'timed_out' | 'cancelled';
  source: 'manual' | 'system'; // manual = user-initiated, system = auto-triggered
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

// One row of the dashboard's worst-open-threats summary
export interface TopThreat {
  id: string;
  severity?: string;
  cvss_score?: number;
  known_exploited?: boolean;
  packages: string;
}

// Dashboard stats
export interface DashboardStats {
  total_agents: number;
  online_agents: number;
  offline_agents: number;
  total_updates: number;
  pending_updates: number;
  approved_updates: number;
  installed_updates: number;
  failed_updates: number;
  available_fix_count: number;    // distinct advisories on available version (remediation)
  open_threat_count: number;      // distinct advisories on installed version (threat)
  top_threats?: TopThreat[];      // worst open threats, KEV then CVSS order
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

export interface PackageFleetAgent {
  agent_id: string;
  hostname: string;
  update_id: string;
  current_version: string;
  available_version: string;
  status: string;
  severity: string;
  expected_sha256: string | null;
  last_updated_at: string;
  selected_version: string | null;
  can_approve: boolean;
  can_install: boolean;
  can_retry: boolean;
}

export interface VulnerabilityEntry {
  id: string;
  summary?: string;
  severity?: string;
  description?: string;
  source?: string;
  fixed_version?: string;
  aliases?: string[];
  cvss_vector?: string;
  cvss_score?: number;
  published?: string;
  advisory_type?: string;
  affected_ranges?: string[];
  known_exploited?: boolean;
}

export interface PackageSummary {
  package_type: string;
  package_name: string;
  severity: string;
  package_description: string;
  homepage_url: string;
  size_bytes: number;
  cve_list: string[];
  vulnerabilities: VulnerabilityEntry[];
  total_agents: number;
  pending_count: number;
  approved_count: number;
  active_count: number;
  installed_count: number;
  failed_count: number;
  ignored_count: number;
  latest_available: string;
  latest_installed: string | null;
}

export interface PackageAgentsResponse {
  package_type: string;
  package_name: string;
  agents: PackageFleetAgent[];
}

export interface PackageVulnerabilitiesResponse {
  package_type: string;
  package_name: string;
  vulnerabilities: VulnerabilityEntry[];
}

export interface PackageFleetResponse {
  package_name: string;
  package_type: string;
  agents: PackageFleetAgent[];
}

export interface PackageVersion {
  id: string;
  package_type: string;
  package_name: string;
  version: string;
  published_at: string | null;
  first_scanned_at: string;
  last_seen_at: string;
  sha256: string | null;
  osv_status: string | null; // 'clean' | 'vulnerable' | 'unknown'
  osv_vulns: string | null;  // raw JSON text, parse client-side
  source: string | null;
}

export interface PackageVersionsResponse {
  package_name: string;
  package_type: string;
  versions: PackageVersion[];
}

export interface AggregatedPackage {
  package_type: string;
  package_name: string;
  agent_count: number;
  version_count: number;
  sample_current_version: string;
  sample_available_version: string;
  max_severity_rank: number;
  max_severity: string;
  has_vulns: boolean;
  vuln_count: number;
  hash_pinned_count: number;
  pending_count: number;
  approved_count: number;
  installing_count: number;
  installed_count: number;
  failed_count: number;
  pending_dependencies_count: number;
  last_discovered_at: string;
  representative_id: string;
}

export interface PackageListResponse {
  packages: AggregatedPackage[];
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

// BoundAgent — per-seat view returned by GET /admin/registration-tokens/:id/agents.
// Fields match the Go BoundAgent struct in queries/registration_tokens.go.
export interface BoundAgent {
  agent_id: string;
  hostname: string;
  os_type: string;
  status: string;
  last_seen: string;
  used_at: string;
}

// Registration Token types
export interface RegistrationToken {
  id: string;
  // Plaintext token, decrypted server-side for active, non-expired tokens
  // (SEC-001 B: reversible encryption at rest). Absent for expired/used/revoked
  // tokens, so the install one-liner can only be built while the token is live.
  token?: string;
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

// Ed25519 signing key — one entry per key the server has ever held.
// `is_primary` marks the current signer; `is_active` includes accepted-but-
// not-primary keys (verify but don't sign); deprecated keys carry a non-null
// `deprecated_at` and `is_active=false`.
export interface SigningKey {
  key_id: string;
  public_key: string;
  algorithm: string;
  is_active: boolean;
  is_primary: boolean;
  version: number;
  created_at: string;
  deprecated_at?: string | null;
}

// Rate Limiting types
//
// The server exposes six fixed categories (not per-endpoint). Each route in
// main.go declares which category it falls under at registration time:
//   - agent_registration: new agent enrollment (anti-flood)
//   - agent_checkin: agent polling for commands
//   - agent_reports: agent telemetry uploads (updates, system info, metrics)
//   - admin_token_generation: creating registration tokens
//   - admin_operations: general admin API calls
//   - public_access: login, public-key, info, downloads
//
// `window` is the Go server's time.Duration serialized as integer nanoseconds.
export type RateLimitCategory =
  | 'agent_registration'
  | 'agent_checkin'
  | 'agent_reports'
  | 'admin_token_generation'
  | 'admin_operations'
  | 'public_access';

export interface RateLimitConfig {
  requests: number;
  window: number; // nanoseconds (Go time.Duration on the wire)
  enabled: boolean;
}

export type RateLimitSettings = Record<RateLimitCategory, RateLimitConfig>;

export interface RateLimitStatsResponse {
  total_configured_limits: number;
  enabled_limits: number;
  total_requests_per_minute: number;
  settings: RateLimitSettings;
}

// Upstream version sync — tracks software the operator wants compared
// against canonical upstream releases (Repology, endoflife.date, ...).
// See server/internal/services/upstream/ and migration 035.
export type UpstreamSource =
  | 'repology'
  | 'endoflife'
  | 'anitya'
  | 'github'
  | 'forgejo'
  | 'gitea'
  | 'gitlab'
  | 'bitbucket'
  | 'git'
  | 'npm'
  | 'pypi';

export interface TrackedSoftware {
  id: string;
  name: string;
  ecosystem: string;
  source: UpstreamSource;
  source_ref: string;
  current_version?: string | null;
  latest_version?: string | null;
  latest_at?: string | null;
  eol_at?: string | null;
  last_checked_at?: string | null;
  last_synced_at?: string | null;
  last_error?: string | null;
  enabled: boolean;
  track_prereleases: boolean;
  repology_slug?: string | null;
  container_image_pattern?: string | null;
  binary_probe?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateTrackedSoftwareRequest {
  name: string;
  ecosystem: string;
  source: UpstreamSource;
  source_ref: string;
  current_version?: string;
  enabled?: boolean;
  track_prereleases?: boolean;
  repology_slug?: string;
  container_image_pattern?: string;
  binary_probe?: string;
}

export interface UpdateTrackedSoftwareSettingsRequest {
  enabled?: boolean;
  track_prereleases?: boolean;
}

export interface UpstreamDriftEvent {
  id: string;
  tracked_software_id: string;
  observed_at: string;
  drift_severity: 'minor' | 'major' | 'eol' | 'metadata';
  from_version?: string | null;
  to_version?: string | null;
  note?: string | null;
}

// Agent <-> tracked_software bindings (migration 039). One row per
// (agent, tracked_software) pair, recording "this host has this thing
// installed at this version." Drift / past-EOL flags are computed
// server-side by the join view.
export interface AgentTrackedSoftware {
  id: string;
  agent_id: string;
  tracked_software_id: string;
  installed_version: string;
  install_path?: string | null;
  notes?: string | null;
  last_observed_at: string;
  created_at: string;
  updated_at: string;
}

export interface AgentTrackedSoftwareView {
  binding_id: string;
  agent_id: string;
  tracked_software_id: string;
  name: string;
  ecosystem: string;
  source: UpstreamSource;
  source_ref: string;
  installed_version: string;
  latest_version?: string | null;
  latest_at?: string | null;
  eol_at?: string | null;
  install_path?: string | null;
  notes?: string | null;
  last_observed_at: string;
  last_synced_at?: string | null;
  last_error?: string | null;
  drifted: boolean;
  past_eol: boolean;
}

export interface AgentInstallation {
  binding_id: string;
  agent_id: string;
  hostname: string;
  installed_version: string;
  install_path?: string | null;
  last_observed_at: string;
}

export interface CreateAgentBindingRequest {
  tracked_software_id: string;
  installed_version: string;
  install_path?: string | null;
  notes?: string | null;
}

// Supply chain gate / capability token status (LIFECYCLE-005)
export interface CapabilityTokenHistory {
  token_id: string;
  agent_id: string;
  package_type: string;
  operation: string;
  key_id: string;
  closure_hash: string;
  created_at: string;
  delivered_at: string | null;
  consumed_at: string | null;
  expires_at: number;
  signature: string;
  status: 'pending' | 'delivered' | 'consumed';
}

export interface CapabilityTokenStatusResponse {
  tokens: CapabilityTokenHistory[];
  count: number;
  active_count: number;
}

//
// `subsystem` is a string because the server creates per-scanner rows (apt,
// dnf, winget, windows) in addition to the original aggregate subsystems
// (storage, system, docker). The 'updates' value is now only produced by the
// UI as a synthetic aggregate row over the per-scanner backers — no DB row.
export interface AgentSubsystem {
  id: string;
  agent_id: string;
  subsystem: string;
  enabled: boolean;
  interval_minutes: number;
  auto_run: boolean;
  last_run_at: string | null;
  next_run_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface SubsystemConfig {
  enabled?: boolean;
  interval_minutes?: number;
  auto_run?: boolean;
}

export interface SubsystemStats {
  subsystem: string;
  enabled: boolean;
  last_run_at: string | null;
  next_run_at: string | null;
  interval_minutes: number;
  auto_run: boolean;
  run_count: number;
  last_status: string;
  last_duration: number;
}

// Security subsystem types
export interface SecuritySubsystem {
  status: string;
  enabled: boolean;
  metrics?: {
    total_pending_commands?: number;
    commands_last_hour?: number;
  };
  checks?: {
    recent_violations?: number;
    validation_failures?: number;
    max_age_minutes?: number;
    bound_agents?: number;
    public_key_fingerprint?: string;
    algorithm?: string;
  };
}

// Maintenance window types
export interface MaintenanceWindow {
  id: string;
  day_of_week: number;
  start_time: string;
  end_time: string;
  description: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateMaintenanceWindowRequest {
  day_of_week: number;
  start_time: string;
  end_time: string;
  description?: string;
  enabled?: boolean;
}

export interface SecurityOverview {
  overall_status: 'healthy' | 'degraded' | 'unhealthy';
  timestamp: string;
  subsystems: {
    ed25519_signing: SecuritySubsystem;
    nonce_validation: SecuritySubsystem;
    machine_binding: SecuritySubsystem;
    command_validation: SecuritySubsystem;
  };
  alerts: string[];
  recommendations: string[];
}
