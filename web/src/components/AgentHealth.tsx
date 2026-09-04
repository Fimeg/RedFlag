import React, { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  RefreshCw,
  Activity,
  Play,
  HardDrive,
  Cpu,
  Container,
  Package,
  Shield,
  Fingerprint,
  CheckCircle,
  AlertCircle,
  XCircle,
  Upload,
} from 'lucide-react';
import { formatRelativeTime } from '@/lib/utils';
import { agentApi, securityApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';
import { securitySubsystemStatusColor } from '@/components/primitives/statusColors';
import { AgentSubsystem } from '@/types';
import { AgentUpdatesModal } from './AgentUpdatesModal';

interface AgentHealthProps {
  agentId: string;
}

// Map subsystem types to icons and display names
const subsystemConfig: Record<string, { icon: React.ReactNode; name: string; description: string; category: string }> = {
  updates: {
    icon: <Package className="h-4 w-4" />,
    name: 'System Update Scanner',
    description: 'Scans for available system package updates',
    category: 'system',
  },
  // Platform-specific package scanners (shown if present in database)
  apt: {
    icon: <Package className="h-4 w-4" />,
    name: 'APT Package Scanner',
    description: 'Scans for available Debian/Ubuntu package updates',
    category: 'system',
  },
  dnf: {
    icon: <Package className="h-4 w-4" />,
    name: 'DNF Package Scanner',
    description: 'Scans for available Fedora/RHEL/CentOS package updates',
    category: 'system',
  },
  winget: {
    icon: <Package className="h-4 w-4" />,
    name: 'Winget Package Scanner',
    description: 'Scans for available Windows package updates',
    category: 'system',
  },
  windows: {
    icon: <Package className="h-4 w-4" />,
    name: 'Windows Update Scanner',
    description: 'Scans for available Windows OS updates',
    category: 'system',
  },
  storage: {
    icon: <HardDrive className="h-4 w-4" />,
    name: 'Disk Usage Reporter',
    description: 'Reports disk usage metrics and storage availability',
    category: 'storage',
  },
  system: {
    icon: <Cpu className="h-4 w-4" />,
    name: 'System Metrics Scanner',
    description: 'Reports CPU, memory, processes, and system uptime',
    category: 'system',
  },
  docker: {
    icon: <Container className="h-4 w-4" />,
    name: 'Docker Image Scanner',
    description: 'Scans Docker containers for available image updates',
    category: 'system',
  },
};

export function AgentHealth({ agentId }: AgentHealthProps) {
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const queryClient = useQueryClient();

  // Fetch subsystems from API
  const { data: subsystems = [], isLoading, refetch: _refetch } = useQuery({
    queryKey: ['subsystems', agentId],
    queryFn: async () => {
      const data = await agentApi.getSubsystems(agentId);
      return data;
    },
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  // Fetch agent data for Update Agent button
  const { data: agent } = useQuery({
    queryKey: ['agent', agentId],
    queryFn: () => agentApi.getAgent(agentId),
    refetchInterval: 30000,
  });

  // Fetch security health status
  const { data: securityOverview, isLoading: securityLoading } = useQuery({
    queryKey: ['security-overview'],
    queryFn: async () => {
      const data = await securityApi.getOverview();
      return data;
    },
    refetchInterval: 60000, // Refresh every minute
  });

  // ARC-003: derive the set of scanners the agent currently advertises.
  // Populated by the server's syncAvailableScanners (ARC-001) into
  // agent.metadata.available_scanners. Empty set means the agent hasn't
  // reported yet (legacy / pre-ARC-001) — in that case we don't render
  // the detected/not-detected badge so we don't make false claims.
  const reportedScanners: Set<string> = (() => {
    const list = agent?.metadata?.available_scanners;
    if (!Array.isArray(list)) return new Set();
    return new Set(list.filter((x: unknown): x is string => typeof x === 'string'));
  })();
  const hasReportedScanners = reportedScanners.size > 0;
  const scannerSubsystems = new Set(['apt', 'dnf', 'winget', 'windows', 'docker']);

  // Package-manager scanners are surfaced through one unified "System Update
  // Scanner" row. The agent runs each scanner on its own schedule (per-scanner
  // rows still exist in agent_subsystems), but the UI collapses them.
  const packageScannerNames = ['apt', 'dnf', 'winget', 'windows'] as const;
  const packageScannerSet: Set<string> = new Set(packageScannerNames);

  // Returns the per-scanner subsystems currently backing the unified row.
  const backingScanners = subsystems.filter(s => packageScannerSet.has(s.subsystem));

  // Build the unified row from backing per-scanner subsystems. Returns null
  // when no package scanners exist for this agent (no row to render).
  const synthesizeUpdatesRow = (): AgentSubsystem | null => {
    if (backingScanners.length === 0) return null;

    const intervals = backingScanners.map(s => s.interval_minutes).filter(n => n > 0);
    const lastRuns = backingScanners
      .map(s => s.last_run_at)
      .filter((t): t is string => !!t)
      .sort();
    const nextRuns = backingScanners
      .map(s => s.next_run_at)
      .filter((t): t is string => !!t)
      .sort();

    return {
      id: `synth-updates-${agentId}`,
      agent_id: agentId,
      subsystem: 'updates',
      enabled: backingScanners.some(s => s.enabled),
      auto_run: backingScanners.some(s => s.auto_run && s.enabled),
      interval_minutes: intervals.length > 0 ? Math.min(...intervals) : 60,
      last_run_at: lastRuns.length > 0 ? lastRuns[lastRuns.length - 1] : null,
      next_run_at: nextRuns.length > 0 ? nextRuns[0] : null,
      created_at: '',
      updated_at: '',
    };
  };

  // Final list rendered in the table: synthesized updates row first (when
  // backing scanners exist), then every non-package-scanner subsystem. The
  // per-scanner backing rows are not rendered — they're folded into the
  // unified row above.
  const updatesRow = synthesizeUpdatesRow();
  const displaySubsystems: AgentSubsystem[] = [
    ...(updatesRow ? [updatesRow] : []),
    ...subsystems.filter(s => !packageScannerSet.has(s.subsystem) && s.subsystem !== 'updates'),
  ];

  // Get security icon for subsystem type
  const getSecurityIcon = (type: string) => {
    switch (type) {
      case 'ed25519_signing':
        return <Shield className="h-4 w-4" />;
      case 'nonce_validation':
        return <RefreshCw className="h-4 w-4" />;
      case 'machine_binding':
        return <Fingerprint className="h-4 w-4" />;
      case 'command_validation':
        return <CheckCircle className="h-4 w-4" />;
      default:
        return <Shield className="h-4 w-4" />;
    }
  };

  // Get display name for security subsystem
  const getSecurityDisplayName = (type: string) => {
    switch (type) {
      case 'ed25519_signing':
        return 'Ed25519 Signing';
      case 'nonce_validation':
        return 'Nonce Protection';
      case 'machine_binding':
        return 'Machine Binding';
      case 'command_validation':
        return 'Command Validation';
      default:
        return type;
    }
  };

  // Toggle subsystem enabled/disabled
  const toggleSubsystemMutation = useMutation({
    mutationFn: async ({ subsystem, enabled }: { subsystem: string; enabled: boolean }) => {
      if (enabled) {
        return await agentApi.enableSubsystem(agentId, subsystem);
      } else {
        return await agentApi.disableSubsystem(agentId, subsystem);
      }
    },
    onSuccess: (_, variables) => {
      toast.success(`${subsystemConfig[variables.subsystem]?.name || variables.subsystem} ${variables.enabled ? 'enabled' : 'disabled'}`);
      queryClient.invalidateQueries({ queryKey: ['subsystems', agentId] });
    },
    onError: (error: any, variables) => {
      toast.error(`Failed to ${variables.enabled ? 'enable' : 'disable'} subsystem: ${error.response?.data?.error || error.message}`);
    },
  });

  // Update subsystem interval
  const updateIntervalMutation = useMutation({
    mutationFn: async ({ subsystem, intervalMinutes }: { subsystem: string; intervalMinutes: number }) => {
      return await agentApi.setSubsystemInterval(agentId, subsystem, intervalMinutes);
    },
    onSuccess: (_, variables) => {
      toast.success(`Interval updated to ${variables.intervalMinutes} minutes`);
      queryClient.invalidateQueries({ queryKey: ['subsystems', agentId] });
    },
    onError: (error: any) => {
      toast.error(`Failed to update interval: ${error.response?.data?.error || error.message}`);
    },
  });

  // Toggle auto-run
  const toggleAutoRunMutation = useMutation({
    mutationFn: async ({ subsystem, autoRun }: { subsystem: string; autoRun: boolean }) => {
      return await agentApi.setSubsystemAutoRun(agentId, subsystem, autoRun);
    },
    onSuccess: (_, variables) => {
      toast.success(`Auto-run ${variables.autoRun ? 'enabled' : 'disabled'}`);
      queryClient.invalidateQueries({ queryKey: ['subsystems', agentId] });
    },
    onError: (error: any) => {
      toast.error(`Failed to toggle auto-run: ${error.response?.data?.error || error.message}`);
    },
  });

  // Trigger manual scan
  const triggerScanMutation = useMutation({
    mutationFn: async (subsystem: string) => {
      return await agentApi.triggerSubsystem(agentId, subsystem);
    },
    onSuccess: (_, subsystem) => {
      toast.success(`${subsystemConfig[subsystem]?.name || subsystem} scan triggered`);
      queryClient.invalidateQueries({ queryKey: ['subsystems', agentId] });
    },
    onError: (error: any) => {
      toast.error(`Failed to trigger scan: ${error.response?.data?.error || error.message}`);
    },
  });

  // When the unified 'updates' row is acted on, cascade to every backing
  // per-scanner subsystem that actually exists for this agent.
  const cascadeTargets = (subsystem: string): string[] => {
    if (subsystem !== 'updates') return [subsystem];
    return backingScanners.map(s => s.subsystem);
  };

  const handleToggleEnabled = (subsystem: string, currentEnabled: boolean) => {
    const targets = cascadeTargets(subsystem);
    targets.forEach(t => toggleSubsystemMutation.mutate({ subsystem: t, enabled: !currentEnabled }));
  };

  const handleIntervalChange = (subsystem: string, intervalMinutes: number) => {
    const targets = cascadeTargets(subsystem);
    targets.forEach(t => updateIntervalMutation.mutate({ subsystem: t, intervalMinutes }));
  };

  const handleToggleAutoRun = (subsystem: string, currentAutoRun: boolean) => {
    const targets = cascadeTargets(subsystem);
    targets.forEach(t => toggleAutoRunMutation.mutate({ subsystem: t, autoRun: !currentAutoRun }));
  };

  const handleTriggerScan = (subsystem: string) => {
    const targets = cascadeTargets(subsystem);
    targets.forEach(t => triggerScanMutation.mutate(t));
  };

  const frequencyOptions = [
    { value: 5, label: '5 min' },
    { value: 15, label: '15 min' },
    { value: 30, label: '30 min' },
    { value: 60, label: '1 hour' },
    { value: 240, label: '4 hours' },
    { value: 720, label: '12 hours' },
    { value: 1440, label: '24 hours' },
    { value: 10080, label: '1 week' },
    { value: 20160, label: '2 weeks' },
  ];

  // Counts reflect what the user sees in the table (unified row + non-package
  // subsystems), not raw DB backer rows.
  const enabledCount = displaySubsystems.filter(s => s.enabled).length;
  const autoRunCount = displaySubsystems.filter(s => s.auto_run && s.enabled).length;

  // Chip palette for the unified row's per-scanner availability indicators.
  const getPackageManagerBadgeStyle = (pm: string) => {
    switch (pm) {
      case 'apt': return 'bg-purple-100 text-purple-700';
      case 'dnf': return 'bg-green-100 text-green-700';
      case 'winget': return 'bg-blue-100 text-blue-700';
      case 'windows': return 'bg-blue-100 text-blue-700';
      default: return 'bg-gray-100 text-gray-500';
    }
  };

  return (
    <div className="space-y-6">
      {/* Subsystems Section - Continuous Surface */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-base font-semibold text-gray-900">Subsystems</h3>
            <p className="text-xs text-gray-600 mt-0.5">
              {enabledCount} enabled • {autoRunCount} auto-running • {displaySubsystems.length} total
            </p>
          </div>
          <button
            onClick={() => setShowUpdateModal(true)}
            className="text-sm text-primary-600 hover:text-primary-800 flex items-center space-x-1 border border-primary-300 px-2 py-1 rounded"
          >
            <Upload className="h-4 w-4" />
            <span>Update Agent</span>
          </button>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-12">
            <RefreshCw className="h-6 w-6 animate-spin text-gray-400" />
            <span className="ml-2 text-gray-600">Loading subsystems...</span>
          </div>
        ) : subsystems.length === 0 ? (
          <div className="text-center py-12">
            <Activity className="mx-auto h-12 w-12 text-gray-400" />
            <h3 className="mt-2 text-sm font-medium text-gray-900">No subsystems found</h3>
            <p className="mt-1 text-sm text-gray-500">
              Subsystems will be created automatically when the agent checks in.
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200">
                  <th className="text-left py-2 pr-4 font-medium text-gray-700">Subsystem</th>
                  <th className="text-left py-2 pr-4 font-medium text-gray-700">Category</th>
                  <th className="text-center py-2 pr-4 font-medium text-gray-700">Enabled</th>
                  <th className="text-center py-2 pr-4 font-medium text-gray-700">Auto-Run</th>
                  <th className="text-center py-2 pr-4 font-medium text-gray-700">Interval</th>
                  <th className="text-right py-2 pr-4 font-medium text-gray-700">Last Run</th>
                  <th className="text-right py-2 pr-4 font-medium text-gray-700">Next Run</th>
                  <th className="text-center py-2 font-medium text-gray-700">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {displaySubsystems.map((subsystem: AgentSubsystem) => {
                  const config = subsystemConfig[subsystem.subsystem] || {
                    icon: <Activity className="h-4 w-4" />,
                    name: subsystem.subsystem,
                    description: 'Custom subsystem',
                    category: 'system',
                  };

                  return (
                    <tr key={subsystem.id} className="hover:bg-gray-50">
                      {/* Subsystem Name */}
                      <td className="py-3 pr-4 text-gray-900">
                        <div className="flex items-center space-x-2">
                          <span className="text-gray-600">{config.icon}</span>
                          <div>
                            <div className="font-medium flex items-center gap-2">
                              <span>{config.name}</span>
                              {/* ARC-003: detected vs configured indicator. Only shown
                                  for scanner subsystems and only when the agent has
                                  reported its scanners at least once. */}
                              {hasReportedScanners && scannerSubsystems.has(subsystem.subsystem) && (
                                reportedScanners.has(subsystem.subsystem) ? (
                                  <span
                                    title="Agent currently reports this scanner is available"
                                    className="badge badge-success badge-sm text-[10px]"
                                  >
                                    Detected
                                  </span>
                                ) : (
                                  <span
                                    title="Configured in DB but agent does not currently report this scanner — scheduler will skip"
                                    className="badge badge-warning badge-sm text-[10px]"
                                  >
                                    Not detected
                                  </span>
                                )
                              )}
                            </div>
                            <div className="text-xs text-gray-500">
                              {subsystem.subsystem === 'updates' ? (
                                <div className="text-xs text-gray-500">
                                  <span>Scans for available package updates (</span>
                                  {packageScannerNames.map((pm, index) => {
                                    // Chip lights up when the agent has reported
                                    // this scanner as available. When no scanners
                                    // have been reported yet, fall back to the
                                    // presence of a backing subsystem row.
                                    const isAvailable = hasReportedScanners
                                      ? reportedScanners.has(pm)
                                      : backingScanners.some(b => b.subsystem === pm);
                                    const isLast = index === packageScannerNames.length - 1;

                                    return (
                                      <span key={pm}>
                                        {index > 0 && ', '}
                                        <span className={cn(
                                          'text-[10px] px-1 py-0.5 rounded',
                                          isAvailable
                                            ? getPackageManagerBadgeStyle(pm)
                                            : 'bg-gray-100 text-gray-500'
                                        )}>
                                          {pm === 'windows' ? 'Windows Update' : pm.toUpperCase()}
                                        </span>
                                        {isLast && ')'}
                                      </span>
                                    );
                                  })}
                                </div>
                              ) : (
                                config.description
                              )}
                            </div>
                          </div>
                        </div>
                      </td>

                      {/* Category */}
                      <td className="py-3 pr-4 text-gray-600 capitalize text-xs">{config.category}</td>

                      {/* Enabled Toggle */}
                      <td className="py-3 pr-4 text-center">
                        <button
                          onClick={() => handleToggleEnabled(subsystem.subsystem, subsystem.enabled)}
                          disabled={toggleSubsystemMutation.isPending}
                          className={cn(
                            'px-3 py-1 rounded text-xs font-medium transition-colors',
                            subsystem.enabled
                              ? 'bg-green-100 text-green-700 hover:bg-green-200'
                              : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                          )}
                        >
                          {subsystem.enabled ? 'ON' : 'OFF'}
                        </button>
                      </td>

                      {/* Auto-Run Toggle */}
                      <td className="py-3 pr-4 text-center">
                        <button
                          onClick={() => handleToggleAutoRun(subsystem.subsystem, subsystem.auto_run)}
                          disabled={!subsystem.enabled || toggleAutoRunMutation.isPending}
                          className={cn(
                            'px-3 py-1 rounded text-xs font-medium transition-colors',
                            !subsystem.enabled ? 'bg-gray-50 text-gray-400 cursor-not-allowed' :
                            subsystem.auto_run
                              ? 'bg-blue-100 text-blue-700 hover:bg-blue-200'
                              : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
                          )}
                        >
                          {subsystem.auto_run ? 'AUTO' : 'MANUAL'}
                        </button>
                      </td>

                      {/* Interval Selector */}
                      <td className="py-3 pr-4 text-center">
                        {subsystem.enabled ? (
                          <select
                            value={subsystem.interval_minutes}
                            onChange={(e) => handleIntervalChange(subsystem.subsystem, parseInt(e.target.value))}
                            disabled={updateIntervalMutation.isPending}
                            className="px-2 py-1 text-xs border border-gray-300 rounded hover:border-gray-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
                          >
                            {frequencyOptions.map(option => (
                              <option key={option.value} value={option.value}>
                                {option.label}
                              </option>
                            ))}
                          </select>
                        ) : (
                          <span className="text-xs text-gray-400">-</span>
                        )}
                      </td>

                      {/* Last Run */}
                      <td className="py-3 pr-4 text-right text-xs text-gray-600">
                        {subsystem.last_run_at ? formatRelativeTime(subsystem.last_run_at) : '-'}
                      </td>

                      {/* Next Run */}
                      <td className="py-3 pr-4 text-right text-xs text-gray-600">
                        {subsystem.next_run_at && subsystem.auto_run ? (
                          new Date(subsystem.next_run_at) <= new Date()
                            ? <span className="text-orange-600 font-medium">Overdue</span>
                            : formatRelativeTime(subsystem.next_run_at)
                        ) : '-'}
                      </td>

                      {/* Actions */}
                      <td className="py-3 text-center">
                        <button
                          onClick={() => handleTriggerScan(subsystem.subsystem)}
                          disabled={!subsystem.enabled || triggerScanMutation.isPending}
                          className={cn(
                            'px-3 py-1 rounded text-xs font-medium transition-colors inline-flex items-center space-x-1',
                            !subsystem.enabled
                              ? 'bg-gray-50 text-gray-400 cursor-not-allowed'
                              : 'bg-blue-100 text-blue-700 hover:bg-blue-200'
                          )}
                          title="Trigger manual scan"
                        >
                          <Play className="h-3 w-3" />
                          <span>Scan</span>
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Security Health Section - Continuous Surface */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <Shield className="h-5 w-5 text-blue-600" />
            <div>
              <h3 className="text-base font-semibold text-gray-900">Security Health</h3>
              {/* This surface reads the fleet-wide security overview, not this
                  agent's status — label it so it isn't mistaken for per-agent. */}
              <p className="text-xs text-gray-500">Fleet-wide — not specific to this agent</p>
            </div>
          </div>
          <button
            onClick={() => queryClient.invalidateQueries({ queryKey: ['security-overview'] })}
            disabled={securityLoading}
            className="flex items-center space-x-1 px-3 py-1 text-xs text-gray-600 hover:text-gray-800 hover:bg-gray-50/50 rounded-md transition-colors"
          >
            <RefreshCw className={cn('h-3 w-3', securityLoading && 'animate-spin')} />
            <span>Refresh</span>
          </button>
        </div>

        {securityLoading ? (
          <div className="flex items-center justify-center py-6">
            <RefreshCw className="h-5 w-5 animate-spin text-gray-400" />
            <span className="ml-2 text-sm text-gray-600">Loading security status...</span>
          </div>
        ) : securityOverview ? (
          <div className="p-4 space-y-3">
            {/* Overall Status - Compact */}
            <div className="flex items-center justify-between p-3 bg-white/70 backdrop-blur-sm rounded-lg border border-gray-200/30">
              <div className="flex items-center space-x-3">
                <div className={cn(
                  'w-3 h-3 rounded-full',
                  securityOverview.overall_status === 'healthy' ? 'bg-green-500' :
                  securityOverview.overall_status === 'degraded' ? 'bg-amber-500' : 'bg-red-500'
                )}></div>
                <div>
                  <p className="text-xs font-medium text-gray-900">Overall Status</p>
                  <p className="text-xs text-gray-600">
                    {securityOverview.overall_status === 'healthy' ? 'All systems nominal' :
                     securityOverview.overall_status === 'degraded' ? `${securityOverview.alerts.length} issue(s)` :
                     'Critical issues'}
                  </p>
                </div>
              </div>
              <div className={cn(
                'inline-flex items-center gap-1 px-2 py-1 rounded-full text-xs font-medium border',
                securityOverview.overall_status === 'healthy' ? 'bg-green-100 text-green-700 border-green-200' :
                securityOverview.overall_status === 'degraded' ? 'bg-amber-100 text-amber-700 border-amber-200' :
                'bg-red-100 text-red-700 border-red-200'
              )}>
                {securityOverview.overall_status === 'healthy' && <CheckCircle className="w-3 h-3" />}
                {securityOverview.overall_status === 'degraded' && <AlertCircle className="w-3 h-3" />}
                {securityOverview.overall_status === 'unhealthy' && <XCircle className="w-3 h-3" />}
                {securityOverview.overall_status.toUpperCase()}
              </div>
            </div>

            {/* Security Grid - 2x2 Layout */}
            <div className="grid grid-cols-2 gap-3">
              {Object.entries(securityOverview.subsystems).map(([key, subsystem]) => {

                return (
                  <div key={key} className="group relative">
                    <div className={cn(
                      'p-3 bg-white/70 backdrop-blur-sm rounded-lg border border-gray-200/30',
                      'hover:bg-white/90 hover:shadow-sm transition-all duration-150 cursor-pointer'
                    )}>
                      <div className="flex items-center justify-between">
                        <div className="flex items-center space-x-2">
                          <div className="p-1.5 rounded-md bg-gray-50/80 group-hover:bg-gray-100 transition-colors">
                            {getSecurityIcon(key)}
                          </div>
                          <div className="min-w-0">
                            <p className="text-xs font-medium text-gray-900 truncate">
                              {getSecurityDisplayName(key)}
                            </p>
                            <p className="text-xs text-gray-600 truncate">
                              {key === 'command_validation' ?
                                `${subsystem.metrics?.total_pending_commands || 0} pending` :
                               key === 'ed25519_signing' ?
                                'Key valid' :
                               key === 'machine_binding' ?
                                `${subsystem.checks?.recent_violations || 0} violations` :
                               key === 'nonce_validation' ?
                                `${subsystem.checks?.validation_failures || 0} blocked` :
                                subsystem.status}
                            </p>
                          </div>
                        </div>
                        <div className={cn(
                          'px-1.5 py-0.5 rounded text-[10px] font-medium border',
                          securitySubsystemStatusColor(subsystem.status)
                        )}>
                          {subsystem.status === 'healthy' && <CheckCircle className="w-3 h-3 inline" />}
                          {subsystem.status === 'enforced' && <Shield className="w-3 h-3 inline" />}
                          {subsystem.status === 'degraded' && <AlertCircle className="w-3 h-3 inline" />}
                          {subsystem.status === 'unhealthy' && <XCircle className="w-3 h-3 inline" />}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Detailed Info Panel */}
            <div className="grid grid-cols-1 gap-2">
              {Object.entries(securityOverview.subsystems).map(([key, subsystem]) => {
                const checks = subsystem.checks || {};

                return (
                  <div key={`${key}-details`} className="opacity-70 hover:opacity-100 transition-opacity">
                    <div className="p-2 bg-gray-50/70 rounded border border-gray-200/50">
                      <p className="text-[10px] text-gray-700 font-mono truncate">
                        {key === 'nonce_validation' ?
                          `Nonces: ${subsystem.metrics?.total_pending_commands || 0} | Max: ${checks.max_age_minutes || 5}m | Failures: ${checks.validation_failures || 0}` :
                         key === 'machine_binding' ?
                          `Bound: ${checks.bound_agents || 'N/A'} | Violations: ${checks.recent_violations || 0} | Method: Hardware` :
                         key === 'ed25519_signing' ?
                          `Key: ${checks.public_key_fingerprint?.substring(0, 16) || 'N/A'}... | Algo: ${checks.algorithm || 'Ed25519'}` :
                         key === 'command_validation' ?
                          `Processed: ${subsystem.metrics?.commands_last_hour || 0}/hr | Pending: ${subsystem.metrics?.total_pending_commands || 0}` :
                          `Status: ${subsystem.status}`}
                      </p>
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Security Alerts & Recommendations */}
            {(securityOverview.alerts.length > 0 || securityOverview.recommendations.length > 0) && (
              <div className="flex gap-2">
                {securityOverview.alerts.length > 0 && (
                  <div className="alert alert-danger rounded p-2 flex-1">
                    <div className="flex items-center gap-1 mb-1">
                      <XCircle className="h-3 w-3 text-red-500" />
                      <p className="text-xs font-medium text-red-800">Alerts ({securityOverview.alerts.length})</p>
                    </div>
                    <ul className="text-[10px] text-red-700 space-y-0.5">
                      {securityOverview.alerts.slice(0, 1).map((alert, index) => (
                        <li key={index} className="truncate">• {alert}</li>
                      ))}
                      {securityOverview.alerts.length > 1 && (
                        <li className="text-red-600">+{securityOverview.alerts.length - 1} more</li>
                      )}
                    </ul>
                  </div>
                )}

                {securityOverview.recommendations.length > 0 && (
                  <div className="alert alert-warning rounded p-2 flex-1">
                    <div className="flex items-center gap-1 mb-1">
                      <AlertCircle className="h-3 w-3 text-amber-500" />
                      <p className="text-xs font-medium text-amber-800">Recs ({securityOverview.recommendations.length})</p>
                    </div>
                    <ul className="text-[10px] text-amber-700 space-y-0.5">
                      {securityOverview.recommendations.slice(0, 1).map((rec, index) => (
                        <li key={index} className="truncate">• {rec}</li>
                      ))}
                      {securityOverview.recommendations.length > 1 && (
                        <li className="text-amber-600">+{securityOverview.recommendations.length - 1} more</li>
                      )}
                    </ul>
                  </div>
                )}
              </div>
            )}

            {/* Stats Row */}
            <div className="flex justify-between pt-2 border-t border-gray-200/50">
              <div className="text-center">
                <p className="text-[11px] font-medium text-gray-900">{Object.keys(securityOverview.subsystems).length}</p>
                <p className="text-[10px] text-gray-600">Systems</p>
              </div>
              <div className="text-center">
                <p className="text-[11px] font-medium text-gray-900">
                  {Object.values(securityOverview.subsystems).filter(s => s.status === 'healthy' || s.status === 'enforced').length}
                </p>
                <p className="text-[10px] text-gray-600">Healthy</p>
              </div>
              <div className="text-center">
                <p className="text-[11px] font-medium text-gray-900">{securityOverview.alerts.length}</p>
                <p className="text-[10px] text-gray-600">Alerts</p>
              </div>
              <div className="text-center">
                <p className="text-[11px] font-medium text-gray-600">
                  {new Date(securityOverview.timestamp).toLocaleTimeString()}
                </p>
                <p className="text-[10px] text-gray-600">Updated</p>
              </div>
            </div>
          </div>
        ) : (
          <div className="text-center py-6">
            <Shield className="mx-auto h-6 w-6 text-gray-400" />
            <p className="mt-1 text-xs text-gray-600">Unable to load security status</p>
          </div>
        )}
      </div>

      {/* Agent Updates Modal */}
      <AgentUpdatesModal
        isOpen={showUpdateModal}
        onClose={() => {
          setShowUpdateModal(false);
        }}
        selectedAgentIds={[agentId]}  // Single agent for this scanner view
        onAgentsUpdated={() => {
          // Refresh agent and subsystems data after update
          queryClient.invalidateQueries({ queryKey: ['agent', agentId] });
          queryClient.invalidateQueries({ queryKey: ['subsystems', agentId] });
        }}
      />
    </div>
  );
}