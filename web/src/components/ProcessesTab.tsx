import React, { useState, useMemo } from 'react';
import { Activity, Search, RefreshCw, ArrowUpDown } from 'lucide-react';
import { useProcessSnapshot, useTriggerProcessScan } from '@/hooks/useProcesses';
import { ProcessDetailModal } from '@/components/ProcessDetailModal';
import type { ProcessFilter } from '@/types/process';
import { cn } from '@/lib/utils';

interface ProcessesTabProps {
  agentId: string;
}

export const ProcessesTab: React.FC<ProcessesTabProps> = ({ agentId }) => {
  const [selectedProcessId, setSelectedProcessId] = useState<string | null>(null);
  const [filter, setFilter] = useState<ProcessFilter>({
    sort_by: 'cpu',
    sort_dir: 'desc',
    limit: 500,
  });
  const [searchText, setSearchText] = useState('');

  const { data, isLoading, isFetching } = useProcessSnapshot(agentId, filter);
  const triggerScan = useTriggerProcessScan();

  const processes = data?.processes ?? [];
  const snapshot = data?.snapshot;
  const total = data?.total ?? 0;

  // Client-side search filtering (name/cmdline)
  const filteredProcesses = useMemo(() => {
    if (!searchText) return processes;
    const lower = searchText.toLowerCase();
    return processes.filter(
      (p) =>
        p.name.toLowerCase().includes(lower) ||
        p.cmdline.toLowerCase().includes(lower)
    );
  }, [processes, searchText]);

  const handleSort = (col: ProcessFilter['sort_by']) => {
    setFilter((prev) => ({
      ...prev,
      sort_by: col,
      sort_dir: prev.sort_by === col && prev.sort_dir === 'desc' ? 'asc' : 'desc',
    }));
  };

  const handleScan = () => {
    triggerScan.mutate(agentId);
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
  };

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  const SortHeader: React.FC<{ label: string; col: ProcessFilter['sort_by']; className?: string }> = ({
    label,
    col,
    className,
  }) => (
    <th
      className={cn('text-left py-2 px-2 font-medium cursor-pointer hover:text-gray-900 select-none', className)}
      onClick={() => handleSort(col)}
    >
      <span className="inline-flex items-center gap-1">
        {label}
        {filter.sort_by === col && (
          <ArrowUpDown className="h-3 w-3 text-gray-400" />
        )}
      </span>
    </th>
  );

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Activity className="h-5 w-5 text-gray-500" />
          <div>
            <h2 className="text-lg font-medium text-gray-900">Processes</h2>
            {snapshot && (
              <p className="text-xs text-gray-500">
                {total} processes · scanned {formatDuration(snapshot.scan_duration_ms)} ·{' '}
                {new Date(snapshot.scanned_at).toLocaleString()}
              </p>
            )}
          </div>
        </div>
        <button
          onClick={handleScan}
          disabled={triggerScan.isPending}
          className={cn(
            'btn btn-secondary flex items-center gap-2',
            triggerScan.isPending && 'opacity-50 cursor-not-allowed'
          )}
        >
          <RefreshCw className={cn('h-4 w-4', triggerScan.isPending && 'animate-spin')} />
          {triggerScan.isPending ? 'Scanning...' : 'Scan Now'}
        </button>
      </div>

      {/* Search and filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-400" />
          <input
            type="text"
            placeholder="Filter by name or command..."
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className="pl-9 pr-4 py-2 w-full border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />
        </div>
        <select
          value={filter.state ?? ''}
          onChange={(e) => setFilter((prev) => ({ ...prev, state: e.target.value || undefined }))}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
        >
          <option value="">All states</option>
          <option value="R">Running</option>
          <option value="S">Sleeping</option>
          <option value="D">Disk Sleep</option>
          <option value="Z">Zombie</option>
          <option value="T">Stopped</option>
        </select>
        <select
          value={filter.user ?? ''}
          onChange={(e) => setFilter((prev) => ({ ...prev, user: e.target.value || undefined }))}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500"
        >
          <option value="">All users</option>
          {/* Populated from process data */}
          {[...new Set(processes.map((p) => p.user))].filter(Boolean).sort().map((u) => (
            <option key={u} value={u}>{u}</option>
          ))}
        </select>
      </div>

      {/* Process table */}
      {isLoading || (isFetching && !snapshot) ? (
        <div className="flex items-center justify-center py-12 text-gray-500">
          <RefreshCw className="h-5 w-5 animate-spin mr-2" />
          Loading process data...
        </div>
      ) : !snapshot ? (
        <div className="text-center py-12 text-gray-500">
          <Activity className="h-8 w-8 mx-auto mb-3 text-gray-300" />
          <p className="text-sm">No process data available.</p>
          <p className="text-xs text-gray-400 mt-1">Click Scan to collect process information.</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="table w-full">
            <thead>
              <tr className="table-header">
                <SortHeader label="Name" col="name" />
                <SortHeader label="PID" col="pid" className="text-right" />
                <th className="text-left py-2 px-2 font-medium">User</th>
                <th className="text-left py-2 px-2 font-medium">State</th>
                <SortHeader label="CPU%" col="cpu" className="text-right" />
                <SortHeader label="Mem%" col="mem" className="text-right" />
                <SortHeader label="RSS" col="rss" className="text-right" />
                <SortHeader label="Threads" col="threads" className="text-right" />
                <th className="text-right py-2 px-2 font-medium">Nice</th>
              </tr>
            </thead>
            <tbody>
              {filteredProcesses.map((proc) => (
                <tr
                  key={proc.id}
                  className="table-row cursor-pointer hover:bg-gray-50"
                  onClick={() => setSelectedProcessId(proc.id)}
                >
                  <td className="table-cell font-medium max-w-[200px] truncate" title={proc.cmdline}>
                    {proc.name}
                  </td>
                  <td className="table-cell text-right text-gray-600 font-mono text-xs">
                    {proc.pid}
                  </td>
                  <td className="table-cell text-gray-600">{proc.user || '—'}</td>
                  <td className="table-cell">
                    <span
                      className={cn(
                        'badge badge-sm',
                        proc.state === 'R' && 'badge-success',
                        proc.state === 'S' && 'badge-info',
                        proc.state === 'D' && 'badge-warning',
                        proc.state === 'Z' && 'badge-danger',
                        proc.state === 'T' && 'badge-warning'
                      )}
                    >
                      {proc.state}
                    </span>
                  </td>
                  <td className="table-cell text-right font-mono text-xs">
                    {proc.cpu_percent > 0 ? `${proc.cpu_percent.toFixed(1)}%` : '—'}
                  </td>
                  <td className="table-cell text-right font-mono text-xs">
                    {proc.mem_percent > 0 ? `${proc.mem_percent.toFixed(1)}%` : '—'}
                  </td>
                  <td className="table-cell text-right font-mono text-xs">
                    {formatBytes(proc.rss_bytes)}
                  </td>
                  <td className="table-cell text-right">{proc.threads}</td>
                  <td className="table-cell text-right">{proc.nice}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {filteredProcesses.length === 0 && processes.length > 0 && (
            <p className="text-center py-6 text-sm text-gray-500">
              No processes match your filter.
            </p>
          )}
        </div>
      )}

      {/* Process detail modal */}
      {selectedProcessId && (
        <ProcessDetailModal
          agentId={agentId}
          processId={selectedProcessId}
          onClose={() => setSelectedProcessId(null)}
        />
      )}
    </div>
  );
};
