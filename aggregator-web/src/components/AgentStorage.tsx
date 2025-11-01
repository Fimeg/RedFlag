import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  HardDrive,
  RefreshCw,
  Database,
  Search,
  Activity,
  Monitor,
  AlertTriangle,
  CheckCircle,
  Info,
  TrendingUp,
  Server,
} from 'lucide-react';
import { formatBytes, formatRelativeTime } from '@/lib/utils';
import { agentApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';

interface AgentStorageProps {
  agentId: string;
}

interface DiskInfo {
  mountpoint: string;
  total: number;
  available: number;
  used: number;
  used_percent: number;
  filesystem: string;
  is_root: boolean;
  is_largest: boolean;
  disk_type: string;
  device: string;
}

interface StorageMetrics {
  cpu_percent: number;
  memory_percent: number;
  memory_used_gb: number;
  memory_total_gb: number;
  disk_used_gb: number;
  disk_total_gb: number;
  disk_percent: number;
  largest_disk_used_gb: number;
  largest_disk_total_gb: number;
  largest_disk_percent: number;
  largest_disk_mount: string;
  uptime: string;
}

export function AgentStorage({ agentId }: AgentStorageProps) {
  const [isScanning, setIsScanning] = useState(false);

  // Fetch agent's latest system info with enhanced disk data
  const { data: agentData, refetch: refetchAgent } = useQuery({
    queryKey: ['agent', agentId],
    queryFn: async () => {
      return await agentApi.getAgent(agentId);
    },
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  const handleFullStorageScan = async () => {
    setIsScanning(true);
    try {
      // Trigger a system scan to get full disk inventory
      await agentApi.scanAgent(agentId);
      toast.success('Full storage scan initiated');

      // Refresh data after a short delay
      setTimeout(() => {
        refetchAgent();
        setIsScanning(false);
      }, 3000);
    } catch (error) {
      toast.error('Failed to initiate storage scan');
      setIsScanning(false);
    }
  };

  // Extract storage metrics from agent metadata
  const storageMetrics: StorageMetrics | null = agentData ? {
    cpu_percent: 0,
    memory_percent: agentData.metadata?.memory_percent || 0,
    memory_used_gb: agentData.metadata?.memory_used_gb || 0,
    memory_total_gb: agentData.metadata?.memory_total_gb || 0,
    disk_used_gb: agentData.metadata?.disk_used_gb || 0,
    disk_total_gb: agentData.metadata?.disk_total_gb || 0,
    disk_percent: agentData.metadata?.disk_percent || 0,
    largest_disk_used_gb: agentData.metadata?.largest_disk_used_gb || 0,
    largest_disk_total_gb: agentData.metadata?.largest_disk_total_gb || 0,
    largest_disk_percent: agentData.metadata?.largest_disk_percent || 0,
    largest_disk_mount: agentData.metadata?.largest_disk_mount || '',
    uptime: agentData.metadata?.uptime || '',
  } : null;

  // Parse disk info from system information if available
  const parseDiskInfo = (): DiskInfo[] => {
    const systemInfo = agentData?.system_info;
    if (!systemInfo?.disk_info) return [];

    return systemInfo.disk_info.map((disk: any) => ({
      mountpoint: disk.mountpoint,
      total: disk.total,
      available: disk.available,
      used: disk.used,
      used_percent: disk.used_percent,
      filesystem: disk.filesystem,
      is_root: disk.is_root || false,
      is_largest: disk.is_largest || false,
      disk_type: disk.disk_type || 'Unknown',
      device: disk.device || disk.filesystem,
    }));
  };

  const getDiskTypeIcon = (diskType: string) => {
    switch (diskType.toLowerCase()) {
      case 'nvme': return <Database className="h-4 w-4 text-purple-500" />;
      case 'ssd': return <Server className="h-4 w-4 text-blue-500" />;
      case 'hdd': return <HardDrive className="h-4 w-4 text-gray-500" />;
      default: return <Monitor className="h-4 w-4 text-gray-400" />;
    }
  };

  if (!agentData) {
    return (
      <div className="space-y-6">
        <div className="animate-pulse">
          <div className="h-8 bg-gray-200 rounded w-1/4 mb-4"></div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="bg-white p-6 rounded-lg border border-gray-200">
                <div className="h-6 bg-gray-200 rounded w-1/3 mb-3"></div>
                <div className="h-4 bg-gray-200 rounded w-full mb-2"></div>
                <div className="h-4 bg-gray-200 rounded w-2/3"></div>
              </div>
            ))}
          </div>
        </div>
      </div>
    );
  }

  const disks = parseDiskInfo();

  return (
    <div className="space-y-8">
      {/* Clean minimal header */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-medium text-gray-900">System Resources</h2>
        <button
          onClick={handleFullStorageScan}
          disabled={isScanning}
          className="text-sm text-gray-500 hover:text-gray-900 flex items-center space-x-1.5"
        >
          <RefreshCw className={cn('h-4 w-4', isScanning && 'animate-spin')} />
          <span>{isScanning ? 'Scanning...' : 'Refresh'}</span>
        </button>
      </div>

      {/* Simple list - no boxes, just clean rows */}
      <div className="space-y-6">
        {/* Memory */}
        {storageMetrics && storageMetrics.memory_total_gb > 0 && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-gray-600">Memory</span>
              <span className="text-gray-900 font-mono">
                {storageMetrics.memory_used_gb.toFixed(1)} / {storageMetrics.memory_total_gb.toFixed(1)} GB
                <span className="text-gray-500 ml-2">({storageMetrics.memory_percent.toFixed(0)}%)</span>
              </span>
            </div>
            <div className="w-full h-1 bg-gray-100 rounded-full overflow-hidden">
              <div
                className="h-full bg-gray-900 transition-all"
                style={{ width: `${Math.min(storageMetrics.memory_percent, 100)}%` }}
              />
            </div>
          </div>
        )}

        {/* Root Disk */}
        {storageMetrics && storageMetrics.disk_total_gb > 0 && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-gray-600">Root filesystem</span>
              <span className="text-gray-900 font-mono">
                {storageMetrics.disk_used_gb.toFixed(1)} / {storageMetrics.disk_total_gb.toFixed(1)} GB
                <span className="text-gray-500 ml-2">({storageMetrics.disk_percent.toFixed(0)}%)</span>
              </span>
            </div>
            <div className="w-full h-1 bg-gray-100 rounded-full overflow-hidden">
              <div
                className="h-full bg-gray-900 transition-all"
                style={{ width: `${Math.min(storageMetrics.disk_percent, 100)}%` }}
              />
            </div>
          </div>
        )}

        {/* Largest disk if different */}
        {storageMetrics && storageMetrics.largest_disk_total_gb > 0 && storageMetrics.largest_disk_mount !== '/' && (
          <div className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-gray-600">{storageMetrics.largest_disk_mount}</span>
              <span className="text-gray-900 font-mono">
                {storageMetrics.largest_disk_used_gb.toFixed(1)} / {storageMetrics.largest_disk_total_gb.toFixed(1)} GB
                <span className="text-gray-500 ml-2">({storageMetrics.largest_disk_percent.toFixed(0)}%)</span>
              </span>
            </div>
            <div className="w-full h-1 bg-gray-100 rounded-full overflow-hidden">
              <div
                className="h-full bg-gray-900 transition-all"
                style={{ width: `${Math.min(storageMetrics.largest_disk_percent, 100)}%` }}
              />
            </div>
          </div>
        )}
      </div>

      {/* All partitions - minimal table */}
      {disks.length > 0 && (
        <div className="space-y-3">
          <h3 className="text-sm font-medium text-gray-600">All partitions</h3>
          <div className="border border-gray-200 rounded-lg overflow-hidden">
            <table className="min-w-full text-sm divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="text-left px-4 py-2 text-xs font-medium text-gray-500">Mount</th>
                <th className="text-left px-4 py-2 text-xs font-medium text-gray-500">Device</th>
                <th className="text-left px-4 py-2 text-xs font-medium text-gray-500">Type</th>
                <th className="text-right px-4 py-2 text-xs font-medium text-gray-500">Used</th>
                <th className="text-right px-4 py-2 text-xs font-medium text-gray-500">Total</th>
                <th className="text-right px-4 py-2 text-xs font-medium text-gray-500">Usage</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 bg-white">
              {disks.map((disk, index) => (
                <tr key={index} className="hover:bg-gray-50 transition-colors">
                  <td className="px-4 py-3 text-sm text-gray-900">
                    <div className="flex items-center space-x-2">
                      <span className="font-mono">{disk.mountpoint}</span>
                      {disk.is_root && <span className="text-xs text-gray-500">root</span>}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-xs text-gray-500 font-mono">{disk.device}</td>
                  <td className="px-4 py-3 text-xs text-gray-500">{disk.disk_type}</td>
                  <td className="px-4 py-3 text-sm text-right text-gray-900">{formatBytes(disk.used)}</td>
                  <td className="px-4 py-3 text-sm text-right text-gray-500">{formatBytes(disk.total)}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end space-x-2">
                      <span className="text-sm text-gray-900">{disk.used_percent.toFixed(0)}%</span>
                      <div className="w-16 h-1 bg-gray-100 rounded-full overflow-hidden">
                        <div
                          className="h-full bg-gray-900"
                          style={{ width: `${Math.min(disk.used_percent, 100)}%` }}
                        />
                      </div>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        </div>
      )}

      {/* Last updated - minimal */}
      {agentData && (
        <div className="text-xs text-gray-400">
          Last updated {agentData.last_seen ? formatRelativeTime(agentData.last_seen) : 'unknown'}
        </div>
      )}
    </div>
  );
}
