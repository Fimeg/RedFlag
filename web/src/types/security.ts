// Security Settings Types for RedFlag

export interface SecuritySettings {
  command_signing: CommandSigningSettings;
  update_security: UpdateSecuritySettings;
  machine_binding: MachineBindingSettings;
  logging: LoggingSettings;
  key_management: KeyManagementSettings;
}

export interface CommandSigningSettings {
  // How long an offline agent keeps trusting its cached server key before it
  // fails closed (SEC-028). Bounded 1-720h; the agent enforces the ceiling.
  stale_key_max_age_hours: number;
}

export interface UpdateSecuritySettings {
  enabled: boolean;
  enforcement_mode: 'strict' | 'warning' | 'disabled';
  nonce_timeout_seconds: number;
  require_signature_verification: boolean;
  allowed_algorithms: string[];
}

export interface MachineBindingSettings {
  enabled: boolean;
  enforcement_mode: 'strict' | 'warning' | 'disabled';
  binding_components: {
    hardware_id: boolean;
    bios_uuid: boolean;
    mac_addresses: boolean;
    cpu_id: boolean;
    disk_serial: boolean;
  };
  violation_action: 'block' | 'warn' | 'log_only';
  binding_grace_period_minutes: number;
}

export interface LoggingSettings {
  log_level: 'debug' | 'info' | 'warn' | 'error';
  retention_days: number;
  log_failures: boolean;
  log_successes: boolean;
  log_to_file: boolean;
  log_to_console: boolean;
  export_format: 'json' | 'csv' | 'syslog';
}

export interface KeyManagementSettings {
  current_key: {
    key_id: string;
    algorithm: string;
    created_at: string;
    expires_at?: string;
    fingerprint: string;
  };
  auto_rotation: boolean;
  rotation_interval_days: number;
  grace_period_days: number;
  key_history: KeyHistoryEntry[];
}

export interface KeyHistoryEntry {
  key_id: string;
  algorithm: string;
  created_at: string;
  retired_at: string;
  reason: 'rotation' | 'compromise' | 'manual';
}

// UI Component Types
export type SecuritySettingType =
  | 'toggle'
  | 'select'
  | 'number'
  | 'text'
  | 'json'
  | 'slider'
  | 'checkbox-group';

export interface SecuritySetting {
  key: string;
  label: string;
  type: SecuritySettingType;
  value: any;
  default?: any;
  description?: string;
  options?: string[] | Array<{ label: string; value: string }>;
  min?: number;
  max?: number;
  step?: number;
  validation?: (value: any) => string | null;
  disabled?: boolean;
  sensitive?: boolean;
  required?: boolean;
}

export interface SecurityCategorySectionProps {
  title: string;
  description: string;
  settings: SecuritySetting[];
  onSettingChange: (key: string, value: any) => void;
  disabled?: boolean;
  loading?: boolean;
  error?: string | null;
}

export interface SecuritySettingProps {
  setting: SecuritySetting;
  onChange: (value: any) => void;
  disabled?: boolean;
  error?: string | null;
}

export interface SecurityStatus {
  overall: 'healthy' | 'warning' | 'critical';
  features: SecurityFeatureStatus[];
  recent_events: number;
  last_updated: string;
}

export interface SecurityFeatureStatus {
  name: string;
  enabled: boolean;
  status: 'healthy' | 'warning' | 'error';
  last_check: string;
  details?: string;
}

export interface SecurityStatusCardProps {
  status: SecurityStatus;
  onRefresh?: () => void;
  onViewLogs?: () => void;
  onMonitorEvents?: () => void;
  loading?: boolean;
}

// Security Events and Audit Trail
export interface SecurityEvent {
  id: string;
  timestamp: string;
  severity: 'info' | 'warn' | 'error' | 'critical';
  category: 'command_signing' | 'update_security' | 'machine_binding' | 'key_management' | 'authentication';
  event_type: string;
  agent_id?: string;
  user_id?: string;
  message: string;
  details: Record<string, any>;
  trace_id?: string;
  correlation_id?: string;
}

export interface AuditEntry {
  id: string;
  timestamp: string;
  user_id: string;
  user_name: string;
  action: string;
  category: string;
  setting_key: string;
  old_value: any;
  new_value: any;
  ip_address: string;
  user_agent: string;
  reason?: string;
}

export interface SecurityEventsState {
  events: SecurityEvent[];
  loading: boolean;
  error: string | null;
  filters: EventFilters;
  pagination: {
    page: number;
    pageSize: number;
    total: number;
    hasMore: boolean;
  };
  liveUpdates: boolean;
}

export interface EventFilters {
  severity?: string[];
  category?: string[];
  date_range?: {
    start: string;
    end?: string;
  };
  agent_id?: string;
  user_id?: string;
  search?: string;
}

export interface SecurityEventsProps {
  events: SecurityEvent[];
  loading?: boolean;
  error?: string | null;
  filters: EventFilters;
  onFiltersChange: (filters: EventFilters) => void;
  onEventSelect?: (event: SecurityEvent) => void;
  onExport?: (format: 'json' | 'csv') => void;
  pagination?: any;
  liveUpdates?: boolean;
  onToggleLiveUpdates?: () => void;
}

// Confirmation Dialog Types
export interface ConfirmationDialogState {
  isOpen: boolean;
  title: string;
  message: string;
  details?: string;
  severity: 'warning' | 'danger';
  requiresConfirmation: boolean;
  confirmationText?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

// Security Settings State Management
export interface SecuritySettingsState {
  settings: SecuritySettings | null;
  loading: boolean;
  saving: boolean;
  error: string | null;
  errors: Record<string, string>;
  hasChanges: boolean;
  validationStatus: 'valid' | 'invalid' | 'pending';
  lastSaved: string | null;
}

// API Response Types
export interface SecuritySettingsResponse {
  settings: SecuritySettings;
  updated_at: string;
  version: string;
}

export interface SecurityAuditResponse {
  audit_entries: AuditEntry[];
  total: number;
  page: number;
  page_size: number;
}

export interface SecurityEventsResponse {
  events: SecurityEvent[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}

// Validation Rules
export interface ValidationRule {
  pattern?: RegExp;
  min?: number;
  max?: number;
  required?: boolean;
  custom?: (value: any) => string | null;
}

export interface SecurityValidationRules {
  [key: string]: ValidationRule;
}

// WebSocket Types for Real-time Updates
export interface SecurityWebSocketMessage {
  type: 'security_event' | 'setting_changed' | 'status_updated';
  data: any;
  timestamp: string;
}

// Export and Import Types
export interface SecurityExport {
  timestamp: string;
  version: string;
  settings: SecuritySettings;
  audit_trail: AuditEntry[];
  metadata: {
    exported_by: string;
    export_reason: string;
  };
}

// Machine Binding Detail Types
export interface MachineFingerprint {
  hardware_id: string;
  bios_uuid: string;
  mac_addresses: string[];
  cpu_id: string;
  disk_serials: string[];
  hostname: string;
  os_info: {
    platform: string;
    version: string;
    architecture: string;
  };
  generated_at: string;
  fingerprint_hash: string;
}

// Key Rotation Types
export interface KeyRotationRequest {
  reason: 'scheduled' | 'compromise' | 'manual';
  grace_period_days?: number;
  immediate?: boolean;
}

export interface KeyRotationResponse {
  new_key_id: string;
  old_key_id: string;
  grace_period_ends: string;
  rotation_complete: boolean;
  affected_agents: number;
}
