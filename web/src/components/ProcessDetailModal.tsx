import React, { useState } from 'react';
import { Activity, Network, FileText, Key, Layers, Box } from 'lucide-react';
import { useProcessDetail } from '@/hooks/useProcesses';
import { cn, formatUnixTime } from '@/lib/utils';
import Modal from '@/components/primitives/Modal';

// Safe JSON parse — returns null on malformed data instead of crashing.
const safeParse = (data: any): any => {
  if (typeof data === 'string') {
    try { return JSON.parse(data); } catch { return null; }
  }
  return data;
};

interface ProcessDetailModalProps {
  agentId: string;
  processId: string;
  onClose: () => void;
}

type DetailTab = 'overview' | 'network' | 'files' | 'environment' | 'memory' | 'namespaces';

export const ProcessDetailModal: React.FC<ProcessDetailModalProps> = ({
  agentId,
  processId,
  onClose,
}) => {
  const [activeTab, setActiveTab] = useState<DetailTab>('overview');
  const { data, isLoading } = useProcessDetail(agentId, processId, true);

  const proc = data?.process;

  const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
  };

  const tabs: { key: DetailTab; label: string; icon: React.ReactNode; count?: number }[] = data ? [
    { key: 'overview', label: 'Overview', icon: <Activity className="h-4 w-4" /> },
    { key: 'network', label: 'Network', icon: <Network className="h-4 w-4" />, count: (data.open_sockets?.length ?? 0) + (data.listening_ports?.length ?? 0) },
    { key: 'files', label: 'Files', icon: <FileText className="h-4 w-4" />, count: data.open_files?.length },
    { key: 'environment', label: 'Env', icon: <Key className="h-4 w-4" />, count: data.environment?.length },
    { key: 'memory', label: 'Memory', icon: <Layers className="h-4 w-4" />, count: data.memory_map?.length },
    { key: 'namespaces', label: 'Namespaces', icon: <Box className="h-4 w-4" />, count: data.namespaces?.length },
  ] : [];

  return (
    <Modal
      open
      onClose={onClose}
      title={
        proc ? (
          <div>
            <div className="text-lg font-medium text-gray-900">{proc.name}</div>
            <div className="text-xs text-gray-500 font-mono font-normal">PID {proc.pid} · {proc.user} · {proc.path || 'no path'}</div>
          </div>
        ) : 'Process Detail'
      }
      maxWidth="4xl"
      maxHeight="85vh"
    >
      {isLoading && (
        <Modal.Body>
          <div className="flex items-center gap-3 text-gray-500 py-4">
            <Activity className="h-5 w-5 animate-spin" />
            Loading process detail...
          </div>
        </Modal.Body>
      )}

      {!isLoading && data && proc && (
        <>
          {/* Tabs */}
          <div className="flex border-b px-6 flex-shrink-0">
            {tabs.map((tab) => (
              <button
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                className={cn(
                  'flex items-center gap-1.5 px-3 py-2.5 text-sm border-b-2 -mb-px transition-colors',
                  activeTab === tab.key
                    ? 'border-red-500 text-red-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700'
                )}
              >
                {tab.icon}
                {tab.label}
                {tab.count !== undefined && tab.count > 0 && (
                  <span className="ml-1 text-xs bg-gray-100 text-gray-600 rounded-full px-1.5">{tab.count}</span>
                )}
              </button>
            ))}
          </div>

          {/* Content */}
          <Modal.Body scrollable className="p-6">
          {activeTab === 'overview' && (
            <div className="grid grid-cols-2 gap-4">
              <Field label="Command" value={proc.cmdline || '—'} mono fullWidth />
              <Field label="Path" value={proc.path || '—'} mono fullWidth />
              <Field label="Working Directory" value={proc.cwd || '—'} mono />
              <Field label="State" value={stateLabel(proc.state)} />
              <Field label="UID / GID" value={`${proc.uid} / ${proc.gid}`} />
              <Field label="EUID / EGID" value={`${proc.euid} / ${proc.egid}`} />
              <Field label="Elevation" value={proc.elevation_status || 'none'} />
              <Field label="CPU%" value={`${proc.cpu_percent.toFixed(2)}%`} />
              <Field label="Mem%" value={`${proc.mem_percent.toFixed(2)}%`} />
              <Field label="RSS" value={formatBytes(proc.rss_bytes)} />
              <Field label="VMS" value={formatBytes(proc.vms_bytes)} />
              <Field label="Threads" value={String(proc.threads)} />
              <Field label="Nice" value={String(proc.nice)} />
              <Field label="Parent PID" value={String(proc.parent_pid)} />
              <Field label="Process Group" value={String(proc.process_group_id)} />
              <Field label="TTY" value={proc.tty_name || String(proc.tty)} />
              <Field label="Started" value={formatUnixTime(proc.start_time_seconds)} />
              <Field label="Disk Read" value={formatBytes(proc.disk_bytes_read)} />
              <Field label="Disk Written" value={formatBytes(proc.disk_bytes_written)} />
              <Field label="On Disk" value={proc.on_disk === 1 ? 'yes' : proc.on_disk === 0 ? 'no (deleted)' : 'unknown'} />
            </div>
          )}

          {activeTab === 'network' && (
            <div className="space-y-6">
              {data.listening_ports && data.listening_ports.length > 0 && (
                <div>
                  <h4 className="text-sm font-medium text-gray-700 mb-2">Listening Ports</h4>
                  <table className="table w-full text-xs">
                    <thead>
                      <tr className="table-header">
                        <th className="text-left py-1">Protocol</th>
                        <th className="text-left py-1">Address</th>
                        <th className="text-right py-1">Port</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.listening_ports.map((lp, i) => {
                        const d = safeParse(lp.data);
                        return (
                          <tr key={i} className="table-row">
                            <td className="table-cell">{d.protocol}</td>
                            <td className="table-cell font-mono">{d.local_addr || '0.0.0.0'}</td>
                            <td className="table-cell text-right font-mono">{d.local_port}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
              {data.open_sockets && data.open_sockets.length > 0 && (
                <div>
                  <h4 className="text-sm font-medium text-gray-700 mb-2">Open Sockets</h4>
                  <table className="table w-full text-xs">
                    <thead>
                      <tr className="table-header">
                        <th className="text-left py-1">Family</th>
                        <th className="text-left py-1">Protocol</th>
                        <th className="text-left py-1">Local</th>
                        <th className="text-left py-1">Remote</th>
                        <th className="text-left py-1">State</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.open_sockets.map((s, i) => {
                        const d = safeParse(s.data);
                        return (
                          <tr key={i} className="table-row">
                            <td className="table-cell">{d.family}</td>
                            <td className="table-cell">{d.protocol}</td>
                            <td className="table-cell font-mono">{d.local_addr}:{d.local_port}</td>
                            <td className="table-cell font-mono">{d.remote_addr}:{d.remote_port}</td>
                            <td className="table-cell">{d.state || '—'}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
              {(!data.listening_ports || data.listening_ports.length === 0) &&
                (!data.open_sockets || data.open_sockets.length === 0) && (
                  <p className="text-sm text-gray-500 italic">No network activity for this process.</p>
                )}
            </div>
          )}

          {activeTab === 'files' && (
            <div>
              {data.open_files && data.open_files.length > 0 ? (
                <table className="table w-full text-xs">
                  <thead>
                    <tr className="table-header">
                      <th className="text-right py-1">FD</th>
                      <th className="text-left py-1">Type</th>
                      <th className="text-left py-1">Path</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.open_files.map((f, i) => {
                      const d = safeParse(f.data);
                      return (
                        <tr key={i} className="table-row">
                          <td className="table-cell text-right font-mono">{d.fd}</td>
                          <td className="table-cell">{d.type}</td>
                          <td className="table-cell font-mono truncate max-w-[400px]" title={d.path}>{d.path}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              ) : (
                <p className="text-sm text-gray-500 italic">No open files recorded.</p>
              )}
            </div>
          )}

          {activeTab === 'environment' && (
            <div>
              {data.environment && data.environment.length > 0 ? (
                <div className="space-y-1">
                  <p className="text-xs text-gray-500 mb-3">Environment variable names only (values omitted for security).</p>
                  {data.environment.map((e, i) => {
                    const d = safeParse(e.data);
                    // d is an array of key names
                    if (Array.isArray(d)) {
                      return (
                        <div key={i} className="flex flex-wrap gap-1">
                          {d.map((key: string, j: number) => (
                            <span key={j} className="chip font-mono text-xs">{key}</span>
                          ))}
                        </div>
                      );
                    }
                    return null;
                  })}
                </div>
              ) : (
                <p className="text-sm text-gray-500 italic">No environment data (may require elevated permissions).</p>
              )}
            </div>
          )}

          {activeTab === 'memory' && (
            <div>
              {data.memory_map && data.memory_map.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="table w-full text-xs">
                    <thead>
                      <tr className="table-header">
                        <th className="text-left py-1">Address Range</th>
                        <th className="text-left py-1">Perms</th>
                        <th className="text-left py-1">Offset</th>
                        <th className="text-left py-1">Path</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.memory_map.slice(0, 200).map((m, i) => {
                        const d = safeParse(m.data);
                        return (
                          <tr key={i} className="table-row">
                            <td className="table-cell font-mono">
                              0x{d.start?.toString(16)}-0x{d.end?.toString(16)}
                            </td>
                            <td className="table-cell font-mono">{d.permissions}</td>
                            <td className="table-cell font-mono">0x{d.offset?.toString(16)}</td>
                            <td className="table-cell font-mono truncate max-w-[300px]" title={d.path}>{d.path || '—'}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                  {data.memory_map.length > 200 && (
                    <p className="text-xs text-gray-400 mt-2">Showing 200 of {data.memory_map.length} regions.</p>
                  )}
                </div>
              ) : (
                <p className="text-sm text-gray-500 italic">No memory map data.</p>
              )}
            </div>
          )}

          {activeTab === 'namespaces' && (
            <div>
              {data.namespaces && data.namespaces.length > 0 ? (
                <table className="table w-full text-xs">
                  <thead>
                    <tr className="table-header">
                      <th className="text-left py-1">Namespace</th>
                      <th className="text-left py-1">Inode</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.namespaces.map((ns, i) => {
                      const d = safeParse(ns.data);
                      return (
                        <tr key={i} className="table-row">
                          <td className="table-cell">{d.type}</td>
                          <td className="table-cell font-mono">{d.inode}</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              ) : (
                <p className="text-sm text-gray-500 italic">No namespace data.</p>
              )}
            </div>
          )}
          </Modal.Body>
        </>
      )}
    </Modal>
  );
};

// Helper components
const Field: React.FC<{ label: string; value: string; mono?: boolean; fullWidth?: boolean }> = ({
  label,
  value,
  mono,
  fullWidth,
}) => (
  <div className={fullWidth ? 'col-span-2' : ''}>
    <dt className="text-xs text-gray-500">{label}</dt>
    <dd className={cn('text-sm text-gray-900 mt-0.5', mono && 'font-mono break-all')}>{value}</dd>
  </div>
);

const stateLabel = (state: string) => {
  const map: Record<string, string> = {
    R: 'Running',
    S: 'Sleeping',
    D: 'Disk Sleep',
    Z: 'Zombie',
    T: 'Stopped',
    t: 'Tracing Stop',
    X: 'Dead',
    x: 'Dead',
    K: 'Wakekill',
    W: 'Waking',
    P: 'Parked',
  };
  return map[state] ?? state;
};
