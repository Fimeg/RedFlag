// Process Explorer types — mirror the server models.

export interface ProcessSnapshot {
  id: string;
  agent_id: string;
  command_id: string;
  process_count: number;
  scanned_at: string;
  scan_duration_ms: number;
  created_at: string;
}

export interface Process {
  id: string;
  snapshot_id: string;
  agent_id: string;
  pid: number;
  name: string;
  path: string;
  cmdline: string;
  cwd: string;
  state: string;
  uid: number;
  gid: number;
  euid: number;
  egid: number;
  user: string;
  group: string;
  tty: number;
  tty_name: string;
  cpu_seconds_user: number;
  cpu_seconds_system: number;
  cpu_percent: number;
  rss_bytes: number;
  vms_bytes: number;
  mem_percent: number;
  threads: number;
  nice: number;
  start_time_seconds: number;
  parent_pid: number;
  process_group_id: number;
  elevation_status: string;
  on_disk: number;
  disk_bytes_read: number;
  disk_bytes_written: number;
  created_at: string;
}

export interface ProcessSnapshotResponse {
  snapshot: ProcessSnapshot | null;
  processes: Process[];
  total: number;
}

export interface ProcessDetailResponse {
  process: Process;
  open_files?: ProcessRelated[];
  open_sockets?: ProcessRelated[];
  open_pipes?: ProcessRelated[];
  environment?: ProcessRelated[];
  memory_map?: ProcessRelated[];
  namespaces?: ProcessRelated[];
  listening_ports?: ProcessRelated[];
}

export interface ProcessRelated {
  id: string;
  process_id: string;
  relation_type: string;
  data: Record<string, any>;
  created_at: string;
}

export interface ProcessFilter {
  name?: string;
  user?: string;
  state?: string;
  sort_by?: 'cpu' | 'mem' | 'pid' | 'name' | 'threads' | 'rss';
  sort_dir?: 'asc' | 'desc';
  limit?: number;
  offset?: number;
}
