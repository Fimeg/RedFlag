import React, { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import {
  MonitorPlay,
  RefreshCw,
  Settings,
  Activity,
  Clock,
  CheckCircle,
  XCircle,
  Play,
  Square,
  Database,
  Shield,
  Search,
} from 'lucide-react';
import { formatRelativeTime } from '@/lib/utils';
import { agentApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';

interface AgentScannersProps {
  agentId: string;
}

interface ScannerConfig {
  id: string;
  name: string;
  description: string;
  icon: React.ReactNode;
  enabled: boolean;
  frequency: number; // minutes
  last_run?: string;
  next_run?: string;
  status: 'idle' | 'running' | 'completed' | 'failed';
  category: 'storage' | 'security' | 'system' | 'network';
}

interface ScannerResponse {
  scanner_id: string;
  status: string;
  message: string;
  next_run?: string;
}

export function AgentScanners({ agentId }: AgentScannersProps) {
  // Mock agent health monitoring configs - in real implementation, these would come from the backend
  const [scanners, setScanners] = useState<ScannerConfig[]>([
    {
      id: 'disk-reporter',
      name: 'Disk Usage Reporter',
      description: 'Agent reports disk usage metrics to server',
      icon: <Database className="h-4 w-4" />,
      enabled: true,
      frequency: 15, // 15 minutes
      last_run: new Date(Date.now() - 10 * 60 * 1000).toISOString(), // 10 minutes ago
      status: 'completed',
      category: 'storage',
    },
    {
      id: 'docker-check',
      name: 'Docker Check-in',
      description: 'Agent checks for Docker container status',
      icon: <Search className="h-4 w-4" />,
      enabled: true,
      frequency: 60, // 1 hour
      last_run: new Date(Date.now() - 45 * 60 * 1000).toISOString(), // 45 minutes ago
      status: 'completed',
      category: 'system',
    },
    {
      id: 'security-check',
      name: 'Security Check-in (Coming Soon)',
      description: 'CVE scanning & security advisory checks - not yet implemented',
      icon: <Shield className="h-4 w-4" />,
      enabled: false,
      frequency: 240, // 4 hours
      status: 'idle',
      category: 'security',
    },
    {
      id: 'agent-heartbeat',
      name: 'Agent Heartbeat',
      description: 'Agent check-in interval and health reporting',
      icon: <Activity className="h-4 w-4" />,
      enabled: true,
      frequency: 30, // 30 minutes
      last_run: new Date(Date.now() - 5 * 60 * 1000).toISOString(), // 5 minutes ago
      status: 'running',
      category: 'system',
    },
  ]);

  // Toggle scanner mutation
  const toggleScannerMutation = useMutation({
    mutationFn: async ({ scannerId, enabled, frequency }: { scannerId: string; enabled: boolean; frequency: number }) => {
      const response = await agentApi.toggleScanner(agentId, scannerId, enabled, frequency);
      return response;
    },
    onSuccess: (data: ScannerResponse, variables) => {
      toast.success(`Scanner ${variables.enabled ? 'enabled' : 'disabled'} successfully`);
      // Update local state
      setScanners(prev => prev.map(scanner =>
        scanner.id === variables.scannerId
          ? {
              ...scanner,
              enabled: variables.enabled,
              frequency: variables.frequency,
              status: variables.enabled ? 'idle' : 'disabled' as any,
              next_run: data.next_run
            }
          : scanner
      ));
    },
    onError: (error: any, variables) => {
      toast.error(`Failed to ${variables.enabled ? 'enable' : 'disable'} scanner: ${error.message || 'Unknown error'}`);
    },
  });

  // Run scanner mutation
  const runScannerMutation = useMutation({
    mutationFn: async (scannerId: string) => {
      const response = await agentApi.runScanner(agentId, scannerId);
      return response;
    },
    onSuccess: (data: ScannerResponse, scannerId) => {
      toast.success('Scanner execution initiated');
      // Update local state
      setScanners(prev => prev.map(scanner =>
        scanner.id === scannerId
          ? { ...scanner, status: 'running', last_run: new Date().toISOString() }
          : scanner
      ));
    },
    onError: (error: any) => {
      toast.error(`Failed to run scanner: ${error.message || 'Unknown error'}`);
    },
  });

  const handleToggleScanner = (scannerId: string, enabled: boolean, frequency: number) => {
    toggleScannerMutation.mutate({ scannerId, enabled, frequency });
  };

  const handleRunScanner = (scannerId: string) => {
    runScannerMutation.mutate(scannerId);
  };

  const handleFrequencyChange = (scannerId: string, frequency: number) => {
    const scanner = scanners.find(s => s.id === scannerId);
    if (scanner) {
      handleToggleScanner(scannerId, scanner.enabled, frequency);
    }
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'running':
        return <RefreshCw className="h-3 w-3 animate-spin text-blue-500" />;
      case 'completed':
        return <CheckCircle className="h-3 w-3 text-green-500" />;
      case 'failed':
        return <XCircle className="h-3 w-3 text-red-500" />;
      default:
        return <Clock className="h-3 w-3 text-gray-400" />;
    }
  };

  const getFrequencyLabel = (frequency: number) => {
    if (frequency < 60) return `${frequency}m`;
    if (frequency < 1440) return `${frequency / 60}h`;
    return `${frequency / 1440}d`;
  };

  const frequencyOptions = [
    { value: 5, label: '5 min' },
    { value: 15, label: '15 min' },
    { value: 30, label: '30 min' },
    { value: 60, label: '1 hour' },
    { value: 240, label: '4 hours' },
    { value: 720, label: '12 hours' },
    { value: 1440, label: '24 hours' },
  ];

  const enabledCount = scanners.filter(s => s.enabled).length;
  const runningCount = scanners.filter(s => s.status === 'running').length;
  const failedCount = scanners.filter(s => s.status === 'failed').length;

  return (
    <div className="space-y-6">
      {/* Compact Summary */}
      <div className="card">
        <div className="flex items-center justify-between text-sm">
          <div className="flex items-center space-x-6">
            <div>
              <span className="text-gray-600">Active:</span>
              <span className="ml-2 font-medium text-green-600">{enabledCount}/{scanners.length}</span>
            </div>
            <div>
              <span className="text-gray-600">Running:</span>
              <span className="ml-2 font-medium text-blue-600">{runningCount}</span>
            </div>
            {failedCount > 0 && (
              <div>
                <span className="text-gray-600">Failed:</span>
                <span className="ml-2 font-medium text-red-600">{failedCount}</span>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Agent Health Monitoring Table */}
      <div className="card">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-medium text-gray-900">Agent Check-in Configuration</h3>
        </div>

        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="border-b border-gray-200">
                <th className="text-left py-2 pr-4 font-medium text-gray-700">Check Type</th>
                <th className="text-left py-2 pr-4 font-medium text-gray-700">Category</th>
                <th className="text-center py-2 pr-4 font-medium text-gray-700">Status</th>
                <th className="text-center py-2 pr-4 font-medium text-gray-700">Enabled</th>
                <th className="text-right py-2 pr-4 font-medium text-gray-700">Check Interval</th>
                <th className="text-right py-2 pr-4 font-medium text-gray-700">Last Check</th>
                <th className="text-center py-2 font-medium text-gray-700">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {scanners.map((scanner) => (
                <tr key={scanner.id} className="hover:bg-gray-50">
                  {/* Scanner Name */}
                  <td className="py-2 pr-4 text-gray-900">
                    <div className="flex items-center space-x-2">
                      <span className="text-gray-600">{scanner.icon}</span>
                      <div>
                        <div className="font-medium">{scanner.name}</div>
                        <div className="text-xs text-gray-500">{scanner.description}</div>
                      </div>
                    </div>
                  </td>

                  {/* Category */}
                  <td className="py-2 pr-4 text-gray-600 capitalize text-xs">{scanner.category}</td>

                  {/* Status */}
                  <td className="py-2 pr-4 text-center">
                    <div className="flex items-center justify-center space-x-1">
                      {getStatusIcon(scanner.status)}
                      <span className={cn(
                        'text-xs',
                        scanner.status === 'running' ? 'text-blue-600' :
                        scanner.status === 'completed' ? 'text-green-600' :
                        scanner.status === 'failed' ? 'text-red-600' : 'text-gray-500'
                      )}>
                        {scanner.status}
                      </span>
                    </div>
                  </td>

                  {/* Enabled Toggle */}
                  <td className="py-2 pr-4 text-center">
                    <span className={cn(
                      'text-xs px-2 py-1 rounded',
                      scanner.enabled
                        ? 'text-green-700 bg-green-50'
                        : 'text-gray-600 bg-gray-50'
                    )}>
                      {scanner.enabled ? 'ON' : 'OFF'}
                    </span>
                  </td>

                  {/* Frequency */}
                  <td className="py-2 pr-4 text-right">
                    {scanner.enabled ? (
                      <span className="text-xs text-gray-600">{getFrequencyLabel(scanner.frequency)}</span>
                    ) : (
                      <span className="text-xs text-gray-400">-</span>
                    )}
                  </td>

                  {/* Last Run */}
                  <td className="py-2 pr-4 text-right text-xs text-gray-600">
                    {scanner.last_run ? formatRelativeTime(scanner.last_run) : '-'}
                  </td>

                  {/* Actions */}
                  <td className="py-2 text-center">
                    <span className="text-xs text-gray-400">Auto</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Compact note */}
      <div className="text-xs text-gray-500">
        Agent check-ins report system state to the server on scheduled intervals. The agent initiates all communication - the server never "scans" your machine.
      </div>
    </div>
  );
}
