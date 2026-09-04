import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  HardDrive,
  RefreshCw,
} from 'lucide-react';
import { formatBytes, formatRelativeTime } from '@/lib/utils';
import { agentApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';
import { clientLogger } from '@/lib/client-logger';

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
  severity: string;
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
  const [lastRefreshed, setLastRefreshed] = useState<Date | null>(null);

  // Fetch agent details - no auto-refresh (heartbeat invalidation handles updates)
  const { data: agentData } = useQuery({
    queryKey: ['agent', agentId],
    queryFn: async () => {
      return await agentApi.getAgent(agentId);
    },
    staleTime: 60 * 1000, // Consider fresh for 1 minute
  });

  // Fetch storage metrics - NO auto-refresh, data persists until manual refresh
  // Cache forever (24 hours) to prevent data loss on navigation
  const { data: storageData, refetch: refetchStorage, error: storageError } = useQuery({
    queryKey: ['storage-metrics', agentId],
    queryFn: async () => {
      clientLogger.debug('Fetching storage metrics for agent:', { agentId });
      try {
        const result = await agentApi.getStorageMetrics(agentId);
        clientLogger.debug('Storage metrics result received', { agentId, hasMetrics: 'metrics' in result, count: result.metrics?.length || 0 });
        setLastRefreshed(new Date());
        return result;
      } catch (err) {
        clientLogger.debug('Error fetching storage metrics', { agentId, error: err instanceof Error ? err.message : String(err) });
        throw err;
      }
    },
    // Never auto-refresh
    refetchInterval: false,
    // Keep data fresh for 24 hours (essentially forever until manual refresh)
    staleTime: 24 * 60 * 60 * 1000,
    // Keep previous data on error (don't wipe on failure)
    placeholderData: (previousData) => previousData,
  });

  const handleFullStorageScan = async () => {
    setIsScanning(true);
    try {
      // Trigger storage scan only (not full system scan)
      await agentApi.triggerSubsystem(agentId, 'storage');
      toast.success('Storage scan initiated');

      // Refresh data after a short delay
      setTimeout(() => {
        refetchStorage();
        setIsScanning(false);
      }, 3000);
    } catch (error) {
      toast.error('Failed to initiate storage scan');
      setIsScanning(false);
    }
  };

  // Process storage metrics data
  const storageMetrics: StorageMetrics | null = useMemo(() => {
    if (!storageData?.metrics || storageData.metrics.length === 0) {
      return null;
    }

    // Find root disk for summary metrics
    const rootDisk = storageData.metrics.find((m: any) => m.is_root) || storageData.metrics[0];
    const largestDisk = storageData.metrics.find((m: any) => m.is_largest) || rootDisk;

    return {
      cpu_percent: 0, // CPU not included in storage metrics, comes from system metrics
      memory_percent: 0, // Memory not included in storage metrics, comes from system metrics
      memory_used_gb: 0,
      memory_total_gb: 0,
      disk_used_gb: largestDisk ? largestDisk.used_bytes / (1024 * 1024 * 1024) : 0,
      disk_total_gb: largestDisk ? largestDisk.total_bytes / (1024 * 1024 * 1024) : 0,
      disk_percent: largestDisk ? largestDisk.used_percent : 0,
      largest_disk_used_gb: largestDisk ? largestDisk.used_bytes / (1024 * 1024 * 1024) : 0,
      largest_disk_total_gb: largestDisk ? largestDisk.total_bytes / (1024 * 1024 * 1024) : 0,
      largest_disk_percent: largestDisk ? largestDisk.used_percent : 0,
      largest_disk_mount: largestDisk ? largestDisk.mountpoint : '',
      uptime: '', // Uptime not included in storage metrics
    };
  }, [storageData]);

  // Parse disk info from storage metrics
  const parseDiskInfo = (): DiskInfo[] => {
    if (!storageData?.metrics) return [];

    return storageData.metrics.map((disk: any) => ({
      mountpoint: disk.mountpoint,
      device: disk.device,
      disk_type: disk.disk_type,
      total: disk.total,
      available: disk.available,
      used: disk.used,
      used_percent: disk.used_percent,
      filesystem: disk.filesystem,
      is_root: disk.is_root || false,
      is_largest: disk.is_largest || false,
      severity: disk.severity || 'low',
    }));
  };


  // Show API error if request failed
  if (storageError) {
    return (
      <div className="space-y-4">
        <div className="alert alert-danger rounded-md">
          <div className="flex">
            <div className="flex-shrink-0">
              <svg className="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
              </svg>
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-red-800">Failed to load storage data</h3>
              <div className="mt-2 text-sm text-red-700">
                <p>{storageError instanceof Error ? storageError.message : 'Unknown error'}</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

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

  // Show error if no data
  if (!storageData || !storageData.metrics || storageData.metrics.length === 0) {
    return (
      <div className="space-y-4">
        <div className="alert alert-warning rounded-md">
          <div className="flex">
            <div className="flex-shrink-0">
              <HardDrive className="h-5 w-5 text-yellow-400" />
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-yellow-800">No storage data available</h3>
              <div className="mt-2 text-sm text-yellow-700">
                <p>Storage metrics have not been collected yet. Click 'Refresh' to trigger a scan.</p>
              </div>
            </div>
          </div>
        </div>
        <button
          onClick={handleFullStorageScan}
          disabled={isScanning}
          className="text-sm text-gray-500 hover:text-gray-900 flex items-center space-x-1.5"
        >
          <RefreshCw className={cn('h-4 w-4', isScanning && 'animate-spin')} />
          <span>{isScanning ? 'Scanning...' : 'Refresh'}</span>
        </button>
      </div>
    );
  }

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

      {/* Disk usage. Memory/CPU are not carried by the storage-metrics endpoint
          (they live in the system-metrics feed) — the dead memory block that
          read hardcoded zeros was removed. Wiring real memory here is a separate
          task against /agents/:id/metrics/system. */}
      <div className="space-y-4">
        {/* Quick Overview - Simple disk bars for at-a-glance view */}
        {disks.length > 0 && (
          <div className="space-y-3">
            <h3 className="text-sm font-medium text-gray-900">Disk Usage (Overview)</h3>
            {disks.map((disk, index) => (
              <div key={`overview-${index}`}>
                <div className="flex items-center justify-between">
                  <p className="text-sm text-gray-600 flex items-center">
                    <HardDrive className="h-4 w-4 mr-1" />
                    {disk.mountpoint} ({disk.filesystem})
                  </p>
                  <p className="text-sm font-medium text-gray-900">
                    {formatBytes(disk.used)} / {formatBytes(disk.total)} ({disk.used_percent.toFixed(0)}%)
                  </p>
                </div>
                <div className="w-full bg-gray-200 rounded-full h-2 mt-1">
                  <div
                    className="bg-blue-600 h-2 rounded-full transition-all"
                    style={{ width: `${Math.min(disk.used_percent, 100)}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Enhanced Disk Table - Shows all partitions with full details */}
        {disks.length > 0 && (
          <div className="overflow-hidden">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-medium text-gray-900">Disk Partitions (Detailed)</h3>
              <span className="text-xs text-gray-500">{disks.length} {disks.length === 1 ? 'partition' : 'partitions'} detected</span>
            </div>

            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <th className="text-left py-2 px-3 font-medium text-gray-700">Mount</th>
                    <th className="text-left py-2 px-3 font-medium text-gray-700">Device</th>
                    <th className="text-left py-2 px-3 font-medium text-gray-700">Type</th>
                    <th className="text-left py-2 px-3 font-medium text-gray-700">FS</th>
                    <th className="text-right py-2 px-3 font-medium text-gray-700">Size</th>
                    <th className="text-right py-2 px-3 font-medium text-gray-700">Used</th>
                    <th className="text-center py-2 px-3 font-medium text-gray-700">%</th>
                    <th className="text-center py-2 px-3 font-medium text-gray-700">Flags</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {disks.map((disk, index) => (
                    <tr key={index} className="hover:bg-gray-50">
                      <td className="py-2 px-3 text-gray-900">
                        <div className="flex items-center">
                          <HardDrive className="h-3 w-3 mr-1 text-gray-400" />
                          <span className="font-medium text-xs">{disk.mountpoint}</span>
                          {disk.is_root && <span className="ml-1 text-[10px] code code-info">ROOT</span>}
                          {disk.is_largest && <span className="ml-1 text-[10px] code code-purple">LARGEST</span>}
                        </div>
                      </td>
                      <td className="py-2 px-3 text-gray-700 text-xs font-mono">{disk.device}</td>
                      <td className="py-2 px-3 text-gray-700 text-xs capitalize">{disk.disk_type.toLowerCase()}</td>
                      <td className="py-2 px-3 text-gray-700 text-xs font-mono">{disk.filesystem}</td>
                      <td className="py-2 px-3 text-gray-900 text-xs text-right">{formatBytes(disk.total)}</td>
                      <td className="py-2 px-3 text-gray-900 text-xs text-right">{formatBytes(disk.used)}</td>
                      <td className="py-2 px-3 text-center">
                        <div className="inline-flex items-center">
                          <div className="w-12 bg-gray-200 rounded-full h-1.5 mr-2">
                            <div
                              className="bg-blue-600 h-1.5 rounded-full"
                              style={{ width: `${Math.min(disk.used_percent, 100)}%` }}
                            />
                          </div>
                          <span className="text-xs font-medium">{disk.used_percent.toFixed(0)}%</span>
                        </div>
                      </td>
                      <td className="py-2 px-3 text-center text-xs text-gray-500">
                        {disk.severity !== 'low' && (
                          <span className={cn(
                            disk.severity === 'critical' ? 'text-red-600' :
                            disk.severity === 'important' ? 'text-amber-600' :
                            'text-yellow-600'
                          )}>
                            {disk.severity.toUpperCase()}
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="mt-3 text-xs text-gray-500">
              Showing {disks.length} disk partitions • Manual refresh only
            </div>
          </div>
        )}

        {/* Fallback if no disk array but we have metadata */}
        {disks.length === 0 && storageMetrics && storageMetrics.disk_total_gb > 0 && (
          <div>
            <div className="flex items-center justify-between">
              <p className="text-sm text-gray-600 flex items-center">
                <HardDrive className="h-4 w-4 mr-1" />
                Disk (/)
              </p>
              <p className="text-sm font-medium text-gray-900">
                {storageMetrics.disk_used_gb.toFixed(1)} GB / {storageMetrics.disk_total_gb.toFixed(1)} GB
              </p>
            </div>
            <div className="w-full bg-gray-200 rounded-full h-2 mt-1">
              <div
                className="bg-blue-600 h-2 rounded-full transition-all"
                style={{ width: `${Math.min(storageMetrics.disk_percent, 100)}%` }}
              />
            </div>
            <p className="text-xs text-gray-500 mt-1">
              {storageMetrics.disk_percent.toFixed(0)}% used
            </p>
          </div>
        )}
      </div>

      {/* Refresh info */}
      <div className="text-xs text-gray-400 border-t border-gray-200 pt-4">
        Manual refresh only • {lastRefreshed ? `Last refreshed ${formatRelativeTime(lastRefreshed.toISOString())}` : 'Never refreshed'}
      </div>
    </div>
  );
}
