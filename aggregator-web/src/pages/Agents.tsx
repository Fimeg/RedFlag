import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Computer,
  RefreshCw,
  Search,
  Filter,
  ChevronRight as ChevronRightIcon,
  Activity,
  Calendar,
  Package,
  Cpu,
  HardDrive,
  MemoryStick,
  GitBranch,
  Clock,
  Trash2,
  History as HistoryIcon,
  Download,
  CheckCircle,
  AlertCircle,
  XCircle,
} from 'lucide-react';
import { useAgents, useAgent, useScanAgent, useScanMultipleAgents, useUnregisterAgent } from '@/hooks/useAgents';
import { useActiveCommands, useCancelCommand } from '@/hooks/useCommands';
import { getStatusColor, formatRelativeTime, isOnline, formatBytes } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';
import { AgentSystemUpdates } from '@/components/AgentUpdates';
import HistoryTimeline from '@/components/HistoryTimeline';

const Agents: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [osFilter, setOsFilter] = useState<string>('all');
  const [showFilters, setShowFilters] = useState(false);
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState<'overview' | 'history'>('overview');

  // Debounce search query to avoid API calls on every keystroke
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery);
    }, 300); // 300ms delay

    return () => {
      clearTimeout(timer);
    };
  }, [searchQuery]);

  // Helper function to get system metadata from agent
  const getSystemMetadata = (agent: any) => {
    const metadata = agent.metadata || {};

    return {
      cpuModel: metadata.cpu_model || 'Unknown',
      cpuCores: metadata.cpu_cores || 'Unknown',
      memoryTotal: metadata.memory_total ? parseInt(metadata.memory_total) : 0,
      diskMount: metadata.disk_mount || 'Unknown',
      diskTotal: metadata.disk_total ? parseInt(metadata.disk_total) : 0,
      diskUsed: metadata.disk_used ? parseInt(metadata.disk_used) : 0,
      processes: metadata.processes || 'Unknown',
      uptime: metadata.uptime || 'Unknown',
      installationTime: metadata.installation_time || 'Unknown',
    };
  };

  // Helper function to parse OS information
  const parseOSInfo = (agent: any) => {
    const osType = agent.os_type || '';
    const osVersion = agent.os_version || '';

    // Extract platform and distribution
    let platform = osType;
    let distribution = '';
    let version = osVersion;

    // Handle Linux distributions
    if (osType.toLowerCase().includes('linux')) {
      platform = 'Linux';
      // Try to extract distribution from version string
      if (osVersion.toLowerCase().includes('ubuntu')) {
        distribution = 'Ubuntu';
        version = osVersion.replace(/ubuntu/i, '').trim();
      } else if (osVersion.toLowerCase().includes('fedora')) {
        distribution = 'Fedora';
        version = osVersion.replace(/fedora/i, '').trim();
      } else if (osVersion.toLowerCase().includes('debian')) {
        distribution = 'Debian';
        version = osVersion.replace(/debian/i, '').trim();
      } else if (osVersion.toLowerCase().includes('centos')) {
        distribution = 'CentOS';
        version = osVersion.replace(/centos/i, '').trim();
      } else if (osVersion.toLowerCase().includes('proxmox')) {
        distribution = 'Proxmox';
        version = osVersion.replace(/proxmox/i, '').trim();
      } else if (osVersion.toLowerCase().includes('arch')) {
        distribution = 'Arch Linux';
        version = osVersion.replace(/arch/i, '').trim();
      } else {
        // Try to get first word as distribution
        const words = osVersion.split(' ');
        distribution = words[0] || 'Unknown Distribution';
        version = words.slice(1).join(' ');
      }
    } else if (osType.toLowerCase().includes('windows')) {
      platform = 'Windows';
      distribution = osVersion; // Windows version info is all in one field
      version = '';
    } else if (osType.toLowerCase().includes('darwin') || osType.toLowerCase().includes('macos')) {
      platform = 'macOS';
      distribution = 'macOS';
      version = osVersion;
    }

    // Truncate long version strings
    if (version.length > 30) {
      version = version.substring(0, 30) + '...';
    }

    return { platform, distribution, version: version.trim() };
  };

  // Fetch agents list
  const { data: agentsData, isPending, error } = useAgents({
    search: debouncedSearchQuery || undefined,
    status: statusFilter !== 'all' ? statusFilter : undefined,
  });

  // Fetch single agent if ID is provided
  const { data: selectedAgentData } = useAgent(id || '', !!id);

  const scanAgentMutation = useScanAgent();
  const scanMultipleMutation = useScanMultipleAgents();
  const unregisterAgentMutation = useUnregisterAgent();

  // Active commands for live status
  const { data: activeCommandsData, refetch: refetchActiveCommands } = useActiveCommands();
  const cancelCommandMutation = useCancelCommand();

  const agents = agentsData?.agents || [];
  const selectedAgent = selectedAgentData || agents.find(a => a.id === id);

  // Filter agents based on OS
  const filteredAgents = agents.filter(agent => {
    if (osFilter === 'all') return true;
    return agent.os_type.toLowerCase().includes(osFilter.toLowerCase());
  });

  // Handle agent selection
  const handleSelectAgent = (agentId: string, checked: boolean) => {
    if (checked) {
      setSelectedAgents([...selectedAgents, agentId]);
    } else {
      setSelectedAgents(selectedAgents.filter(id => id !== agentId));
    }
  };

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedAgents(filteredAgents.map(agent => agent.id));
    } else {
      setSelectedAgents([]);
    }
  };

  // Handle scan operations
  const handleScanAgent = async (agentId: string) => {
    try {
      await scanAgentMutation.mutateAsync(agentId);
      toast.success('Scan triggered successfully');
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleScanSelected = async () => {
    if (selectedAgents.length === 0) {
      toast.error('Please select at least one agent');
      return;
    }

    try {
      await scanMultipleMutation.mutateAsync({ agent_ids: selectedAgents });
      setSelectedAgents([]);
      toast.success(`Scan triggered for ${selectedAgents.length} agents`);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  // Handle agent removal
  const handleRemoveAgent = async (agentId: string, hostname: string) => {
    if (!window.confirm(
      `Are you sure you want to remove agent "${hostname}"? This action cannot be undone and will remove the agent from the system.`
    )) {
      return;
    }

    try {
      await unregisterAgentMutation.mutateAsync(agentId);
      toast.success(`Agent "${hostname}" removed successfully`);

      // Navigate back to agents list if we're on the agent detail page
      if (id && id === agentId) {
        navigate('/agents');
      }
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  // Handle command cancellation
  const handleCancelCommand = async (commandId: string) => {
    try {
      await cancelCommandMutation.mutateAsync(commandId);
      toast.success('Command cancelled successfully');
      refetchActiveCommands();
    } catch (error: any) {
      toast.error(`Failed to cancel command: ${error.message || 'Unknown error'}`);
    }
  };

  // Get agent-specific active commands
  const getAgentActiveCommands = () => {
    if (!selectedAgent || !activeCommandsData?.commands) return [];
    return activeCommandsData.commands.filter(cmd => cmd.agent_id === selectedAgent.id);
  };

  // Helper function to get command display info
  const getCommandDisplayInfo = (command: any) => {
    const actionMap: { [key: string]: { icon: React.ReactNode; label: string } } = {
      'scan': { icon: <RefreshCw className="h-4 w-4" />, label: 'System scan' },
      'install_updates': { icon: <Package className="h-4 w-4" />, label: `Installing ${command.package_name || 'packages'}` },
      'dry_run_update': { icon: <Search className="h-4 w-4" />, label: `Checking dependencies for ${command.package_name || 'packages'}` },
      'confirm_dependencies': { icon: <CheckCircle className="h-4 w-4" />, label: `Installing confirmed dependencies` },
    };

    return actionMap[command.command_type] || {
      icon: <Activity className="h-4 w-4" />,
      label: command.command_type.replace('_', ' ')
    };
  };

  // Get command status
  const getCommandStatus = (command: any) => {
    switch (command.status) {
      case 'pending':
        return { text: 'Pending', color: 'text-amber-600 bg-amber-50 border-amber-200' };
      case 'sent':
        return { text: 'Sent to agent', color: 'text-blue-600 bg-blue-50 border-blue-200' };
      case 'running':
        return { text: 'Running', color: 'text-green-600 bg-green-50 border-green-200' };
      case 'completed':
        return { text: 'Completed', color: 'text-gray-600 bg-gray-50 border-gray-200' };
      case 'failed':
        return { text: 'Failed', color: 'text-red-600 bg-red-50 border-red-200' };
      case 'timed_out':
        return { text: 'Timed out', color: 'text-red-600 bg-red-50 border-red-200' };
      default:
        return { text: command.status, color: 'text-gray-600 bg-gray-50 border-gray-200' };
    }
  };

  // Get unique OS types for filter
  const osTypes = [...new Set(agents.map(agent => agent.os_type))];

  // Agent detail view
  if (id && selectedAgent) {
    return (
      <div className="px-4 sm:px-6 lg:px-8">
        <div className="mb-6">
          <button
            onClick={() => navigate('/agents')}
            className="text-sm text-gray-500 hover:text-gray-700 mb-4"
          >
            ← Back to Agents
          </button>

          {/* New Compact Header Design */}
          <div className="flex flex-col sm:flex-row sm:items-start justify-between mb-4">
            <div className="flex-1 mb-4 sm:mb-0">
              {/* Main hostname with integrated agent info */}
              <div className="flex flex-col sm:flex-row sm:items-center space-y-3 sm:space-y-0 sm:space-x-3 mb-2">
                <h1 className="text-2xl sm:text-3xl font-bold text-gray-900">
                  {selectedAgent.hostname}
                </h1>
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <span className="text-gray-500">[Agent ID:</span>
                  <span className="font-mono text-xs text-gray-700 bg-gray-100 px-2 py-1 rounded break-all">
                    {selectedAgent.id}
                  </span>
                  <span className="text-gray-500">|</span>
                  <span className="text-gray-500">Version:</span>
                  <div className="flex items-center space-x-1">
                    <span className="font-medium text-gray-900">
                      {selectedAgent.current_version || 'Unknown'}
                    </span>
                    {selectedAgent.update_available === true && (
                      <span className="flex items-center text-xs text-amber-600 bg-amber-50 px-2 py-0.5 rounded-full">
                        <AlertCircle className="h-3 w-3 mr-1" />
                        Update Available
                      </span>
                    )}
                    {selectedAgent.update_available === false && selectedAgent.current_version && (
                      <span className="flex items-center text-xs text-green-600 bg-green-50 px-2 py-0.5 rounded-full">
                        <CheckCircle className="h-3 w-3 mr-1" />
                        Up to Date
                      </span>
                    )}
                  </div>
                  <span className="text-gray-500">]</span>
                </div>
              </div>

              {/* Sub-line with registration info only */}
              <div className="text-sm text-gray-600">
                <span>Registered {formatRelativeTime(selectedAgent.created_at)}</span>
              </div>
            </div>

            <button
              onClick={() => handleScanAgent(selectedAgent.id)}
              disabled={scanAgentMutation.isPending}
              className="btn btn-primary sm:ml-4 w-full sm:w-auto"
            >
              {scanAgentMutation.isPending ? (
                <RefreshCw className="animate-spin h-4 w-4 mr-2" />
              ) : (
                <RefreshCw className="h-4 w-4 mr-2" />
              )}
              Scan Now
            </button>
          </div>
        </div>

        {/* Tabs */}
        <div className="mb-6">
          <div className="border-b border-gray-200">
            <nav className="-mb-px flex space-x-8">
              <button
                onClick={() => setActiveTab('overview')}
                className={cn(
                  'py-2 px-1 border-b-2 font-medium text-sm transition-colors',
                  activeTab === 'overview'
                    ? 'border-primary-500 text-primary-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                )}
              >
                Overview
              </button>
              <button
                onClick={() => setActiveTab('history')}
                className={cn(
                  'py-2 px-1 border-b-2 font-medium text-sm transition-colors flex items-center space-x-2',
                  activeTab === 'history'
                    ? 'border-primary-500 text-primary-600'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                )}
              >
                <HistoryIcon className="h-4 w-4" />
                <span>History</span>
              </button>
            </nav>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Main content area */}
          <div className="lg:col-span-2">
            {activeTab === 'overview' ? (
              <div className="space-y-6">
            {/* Agent Status Card - Compact Timeline Style */}
            <div className="card">
              <div className="flex items-center justify-between mb-3">
                <h2 className="text-lg font-medium text-gray-900">Agent Status</h2>
                <div className="flex items-center space-x-2">
                  <div className={cn(
                    'w-3 h-3 rounded-full',
                    isOnline(selectedAgent.last_seen) ? 'bg-green-500' : 'bg-gray-400'
                  )}></div>
                  <span className={cn('badge', getStatusColor(isOnline(selectedAgent.last_seen) ? 'online' : 'offline'))}>
                    {isOnline(selectedAgent.last_seen) ? 'Online' : 'Offline'}
                  </span>
                </div>
              </div>

              {/* Compact Timeline Display */}
              <div className="space-y-2 mb-3">
                {(() => {
                  const agentCommands = getAgentActiveCommands();
                  const activeCommands = agentCommands.filter(cmd =>
                    cmd.status === 'running' || cmd.status === 'sent' || cmd.status === 'pending'
                  );
                  const completedCommands = agentCommands.filter(cmd =>
                    cmd.status === 'completed' || cmd.status === 'failed' || cmd.status === 'timed_out'
                  ).slice(0, 1); // Only show last completed

                  const displayCommands = [
                    ...activeCommands.slice(0, 2), // Max 2 active
                    ...completedCommands.slice(0, 1) // Max 1 completed
                  ].slice(0, 3); // Total max 3 entries

                  if (displayCommands.length === 0) {
                    return (
                      <div className="text-center py-3 text-sm text-gray-500">
                        No active operations
                      </div>
                    );
                  }

                  return displayCommands.map((command, index) => {
                    const displayInfo = getCommandDisplayInfo(command);
                    const statusInfo = getCommandStatus(command);
                    const isActive = command.status === 'running' || command.status === 'sent' || command.status === 'pending';

                    return (
                      <div key={command.id} className="flex items-start space-x-2 p-2 bg-gray-50 rounded border border-gray-200">
                        <div className="flex-shrink-0 mt-0.5">
                          {displayInfo.icon}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center justify-between">
                            <span className="text-sm font-medium text-gray-900 truncate">
                              {isActive ? (
                                <span className="flex items-center space-x-1">
                                  <span className={cn(
                                    'inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border',
                                    statusInfo.color
                                  )}>
                                    {command.status === 'running' && <RefreshCw className="h-3 w-3 animate-spin mr-1" />}
                                    {command.status === 'pending' && <Clock className="h-3 w-3 mr-1" />}
                                    {isActive ? command.status.replace('_', ' ') : statusInfo.text}
                                  </span>
                                  <span className="ml-1">{displayInfo.label}</span>
                                </span>
                              ) : (
                                <span className="flex items-center space-x-1">
                                  <span className={cn(
                                    'inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border',
                                    statusInfo.color
                                  )}>
                                    {command.status === 'completed' && <CheckCircle className="h-3 w-3 mr-1" />}
                                    {command.status === 'failed' && <XCircle className="h-3 w-3 mr-1" />}
                                    {statusInfo.text}
                                  </span>
                                  <span className="ml-1">{displayInfo.label}</span>
                                </span>
                              )}
                            </span>
                          </div>
                          <div className="flex items-center justify-between mt-1">
                            <span className="text-xs text-gray-500">
                              {formatRelativeTime(command.created_at)}
                            </span>
                            {isActive && (command.status === 'pending' || command.status === 'sent') && (
                              <button
                                onClick={() => handleCancelCommand(command.id)}
                                disabled={cancelCommandMutation.isPending}
                                className="text-xs text-red-600 hover:text-red-800 disabled:opacity-50"
                              >
                                Cancel
                              </button>
                            )}
                          </div>
                        </div>
                      </div>
                    );
                  });
                })()}
              </div>

              {/* Basic Status Info */}
              <div className="flex items-center justify-between text-xs text-gray-500 pt-2 border-t border-gray-200">
                <span>Last seen: {formatRelativeTime(selectedAgent.last_seen)}</span>
                <span>Last scan: {selectedAgent.last_scan ? formatRelativeTime(selectedAgent.last_scan) : 'Never'}</span>
              </div>

              {/* Action Button */}
              <div className="flex justify-center mt-3 pt-3 border-t border-gray-200">
                <button
                  onClick={() => handleScanAgent(selectedAgent.id)}
                  disabled={scanAgentMutation.isPending}
                  className="btn btn-primary w-full sm:w-auto text-sm"
                >
                  {scanAgentMutation.isPending ? (
                    <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                  ) : (
                    <RefreshCw className="h-4 w-4 mr-2" />
                  )}
                  Scan Now
                </button>
              </div>
            </div>

            {/* System info */}
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">System Information</h2>

              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                {/* Basic System Info */}
                <div className="space-y-4">
                  <div>
                    <p className="text-sm text-gray-600">Platform</p>
                    <p className="text-sm font-medium text-gray-900">
                      {(() => {
                        const osInfo = parseOSInfo(selectedAgent);
                        return osInfo.platform;
                      })()}
                    </p>
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">Distribution</p>
                    <p className="text-sm font-medium text-gray-900">
                      {(() => {
                        const osInfo = parseOSInfo(selectedAgent);
                        return osInfo.distribution;
                      })()}
                    </p>
                    {(() => {
                      const osInfo = parseOSInfo(selectedAgent);
                      if (osInfo.version) {
                        return (
                          <p className="text-xs text-gray-500 mt-1">
                            Version: {osInfo.version}
                          </p>
                        );
                      }
                      return null;
                    })()}
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">Architecture</p>
                    <p className="text-sm font-medium text-gray-900">
                      {selectedAgent.os_architecture || selectedAgent.architecture}
                    </p>
                  </div>
                </div>

                {/* Hardware Specs */}
                <div className="space-y-4">
                  {(() => {
                    const meta = getSystemMetadata(selectedAgent);
                    return (
                      <>
                        <div>
                          <p className="text-sm text-gray-600 flex items-center">
                            <Cpu className="h-4 w-4 mr-1" />
                            CPU
                          </p>
                          <p className="text-sm font-medium text-gray-900">
                            {meta.cpuModel}
                          </p>
                          <p className="text-xs text-gray-500">
                            {meta.cpuCores} cores
                          </p>
                        </div>

                        {meta.memoryTotal > 0 && (
                          <div>
                            <p className="text-sm text-gray-600 flex items-center">
                              <MemoryStick className="h-4 w-4 mr-1" />
                              Memory
                            </p>
                            <p className="text-sm font-medium text-gray-900">
                              {formatBytes(meta.memoryTotal)}
                            </p>
                          </div>
                        )}

                        {meta.diskTotal > 0 && (
                          <div>
                            <p className="text-sm text-gray-600 flex items-center">
                              <HardDrive className="h-4 w-4 mr-1" />
                              Disk ({meta.diskMount})
                            </p>
                            <p className="text-sm font-medium text-gray-900">
                              {formatBytes(meta.diskUsed)} / {formatBytes(meta.diskTotal)}
                            </p>
                            <div className="w-full bg-gray-200 rounded-full h-2 mt-1">
                              <div
                                className="bg-blue-600 h-2 rounded-full"
                                style={{ width: `${Math.round((meta.diskUsed / meta.diskTotal) * 100)}%` }}
                              ></div>
                            </div>
                            <p className="text-xs text-gray-500">
                              {Math.round((meta.diskUsed / meta.diskTotal) * 100)}% used
                            </p>
                          </div>
                        )}

                        {meta.processes !== 'Unknown' && (
                          <div>
                            <p className="text-sm text-gray-600 flex items-center">
                              <GitBranch className="h-4 w-4 mr-1" />
                              Running Processes
                            </p>
                            <p className="text-sm font-medium text-gray-900">
                              {meta.processes}
                            </p>
                          </div>
                        )}

                        {meta.uptime !== 'Unknown' && (
                          <div>
                            <p className="text-sm text-gray-600 flex items-center">
                              <Clock className="h-4 w-4 mr-1" />
                              Uptime
                            </p>
                            <p className="text-sm font-medium text-gray-900">
                              {meta.uptime}
                            </p>
                          </div>
                        )}
                      </>
                    );
                  })()}
                </div>
              </div>

              </div>

              {/* System Updates */}
              <AgentSystemUpdates agentId={selectedAgent.id} />

              </div>
            ) : (
              <div>
                <HistoryTimeline agentId={selectedAgent.id} />
              </div>
            )}
          </div>

          {/* Quick actions */}
          <div className="space-y-6">
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">Quick Actions</h2>

              <div className="space-y-3">
                <button
                  onClick={() => navigate(`/updates?agent=${selectedAgent.id}`)}
                  className="w-full btn btn-secondary"
                >
                  <Package className="h-4 w-4 mr-2" />
                  View All Updates
                </button>

                <button
                  onClick={() => handleRemoveAgent(selectedAgent.id, selectedAgent.hostname)}
                  disabled={unregisterAgentMutation.isPending}
                  className="w-full btn btn-danger"
                >
                  {unregisterAgentMutation.isPending ? (
                    <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                  ) : (
                    <Trash2 className="h-4 w-4 mr-2" />
                  )}
                  Remove Agent
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Agents list view
  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Agents</h1>
        <p className="mt-1 text-sm text-gray-600">
          Monitor and manage your connected agents
        </p>
      </div>

      {/* Search and filters */}
      <div className="mb-6 space-y-4">
        <div className="flex flex-col sm:flex-row gap-4">
          {/* Search */}
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search agents by hostname..."
                className="pl-10 pr-4 py-2 w-full border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              />
            </div>
          </div>

          {/* Filter toggle */}
          <button
            onClick={() => setShowFilters(!showFilters)}
            className="flex items-center space-x-2 px-4 py-2 border border-gray-300 rounded-lg text-sm hover:bg-gray-50"
          >
            <Filter className="h-4 w-4" />
            <span>Filters</span>
            {(statusFilter !== 'all' || osFilter !== 'all') && (
              <span className="bg-primary-100 text-primary-800 px-2 py-0.5 rounded-full text-xs">
                {[
                  statusFilter !== 'all' ? statusFilter : null,
                  osFilter !== 'all' ? osFilter : null,
                ].filter(Boolean).length}
              </span>
            )}
          </button>

          {/* Bulk actions */}
          {selectedAgents.length > 0 && (
            <button
              onClick={handleScanSelected}
              disabled={scanMultipleMutation.isPending}
              className="btn btn-primary"
            >
              {scanMultipleMutation.isPending ? (
                <RefreshCw className="animate-spin h-4 w-4 mr-2" />
              ) : (
                <RefreshCw className="h-4 w-4 mr-2" />
              )}
              Scan Selected ({selectedAgents.length})
            </button>
          )}
        </div>

        {/* Filters */}
        {showFilters && (
          <div className="bg-white p-4 rounded-lg border border-gray-200">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Status
                </label>
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="all">All Status</option>
                  <option value="online">Online</option>
                  <option value="offline">Offline</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Operating System
                </label>
                <select
                  value={osFilter}
                  onChange={(e) => setOsFilter(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="all">All OS</option>
                  {osTypes.map(os => (
                    <option key={os} value={os}>{os}</option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Agents table */}
      {isPending ? (
        <div className="animate-pulse">
          <div className="bg-white rounded-lg border border-gray-200">
            {[...Array(5)].map((_, i) => (
              <div key={i} className="p-4 border-b border-gray-200">
                <div className="h-4 bg-gray-200 rounded w-1/4 mb-2"></div>
                <div className="h-3 bg-gray-200 rounded w-1/2"></div>
              </div>
            ))}
          </div>
        </div>
      ) : error ? (
        <div className="text-center py-12">
          <div className="text-red-500 mb-2">Failed to load agents</div>
          <p className="text-sm text-gray-600">Please check your connection and try again.</p>
        </div>
      ) : filteredAgents.length === 0 ? (
        <div className="text-center py-12">
          <Computer className="mx-auto h-12 w-12 text-gray-400" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">No agents found</h3>
          <p className="mt-1 text-sm text-gray-500">
            {debouncedSearchQuery || statusFilter !== 'all' || osFilter !== 'all'
              ? 'Try adjusting your search or filters.'
              : 'No agents have registered with the server yet.'}
          </p>
        </div>
      ) : (
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="table-header">
                    <input
                      type="checkbox"
                      checked={selectedAgents.length === filteredAgents.length}
                      onChange={(e) => handleSelectAll(e.target.checked)}
                      className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    />
                  </th>
                  <th className="table-header">Agent</th>
                  <th className="table-header">Status</th>
                  <th className="table-header">Version</th>
                  <th className="table-header">OS</th>
                  <th className="table-header">Last Check-in</th>
                  <th className="table-header">Last Scan</th>
                  <th className="table-header">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {filteredAgents.map((agent) => (
                  <tr key={agent.id} className="hover:bg-gray-50 group">
                    <td className="table-cell">
                      <input
                        type="checkbox"
                        checked={selectedAgents.includes(agent.id)}
                        onChange={(e) => handleSelectAgent(agent.id, e.target.checked)}
                        className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      />
                    </td>
                    <td className="table-cell">
                      <div className="flex items-center space-x-3">
                        <div className="w-8 h-8 bg-gray-100 rounded-full flex items-center justify-center">
                          <Computer className="h-4 w-4 text-gray-600" />
                        </div>
                        <div>
                          <div className="text-sm font-medium text-gray-900">
                            <button
                              onClick={() => navigate(`/agents/${agent.id}`)}
                              className="hover:text-primary-600"
                            >
                              {agent.hostname}
                            </button>
                          </div>
                          <div className="text-xs text-gray-500">
                            {agent.metadata && (() => {
                              const meta = getSystemMetadata(agent);
                              const parts = [];
                              if (meta.cpuCores !== 'Unknown') parts.push(`${meta.cpuCores} cores`);
                              if (meta.memoryTotal > 0) parts.push(formatBytes(meta.memoryTotal));
                              if (parts.length > 0) return parts.join(' • ');
                              return 'System info available';
                            })()}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="table-cell">
                      <span className={cn('badge', getStatusColor(isOnline(agent.last_seen) ? 'online' : 'offline'))}>
                        {isOnline(agent.last_seen) ? 'Online' : 'Offline'}
                      </span>
                    </td>
                    <td className="table-cell">
                      <div className="flex items-center space-x-2">
                        <span className="text-sm text-gray-900">
                          {agent.current_version || 'Unknown'}
                        </span>
                        {agent.update_available === true && (
                          <span className="flex items-center text-xs text-amber-600 bg-amber-50 px-1.5 py-0.5 rounded-full">
                            <Download className="h-3 w-3 mr-1" />
                            Update
                          </span>
                        )}
                        {agent.update_available === false && agent.current_version && (
                          <span className="flex items-center text-xs text-green-600 bg-green-50 px-1.5 py-0.5 rounded-full">
                            <CheckCircle className="h-3 w-3 mr-1" />
                            Current
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm text-gray-900">
                        {(() => {
                          const osInfo = parseOSInfo(agent);
                          return osInfo.distribution || agent.os_type;
                        })()}
                      </div>
                      <div className="text-xs text-gray-500">
                        {(() => {
                          const osInfo = parseOSInfo(agent);
                          if (osInfo.version) {
                            return `${osInfo.version} • ${agent.os_architecture || agent.architecture}`;
                          }
                          return `${agent.os_architecture || agent.architecture}`;
                        })()}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm text-gray-900">
                        {formatRelativeTime(agent.last_seen)}
                      </div>
                      <div className="text-xs text-gray-500">
                        {isOnline(agent.last_seen) ? 'Online' : 'Offline'}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm text-gray-900">
                        {agent.last_scan
                          ? formatRelativeTime(agent.last_scan)
                          : 'Never'}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="flex items-center space-x-2">
                        <button
                          onClick={() => handleScanAgent(agent.id)}
                          disabled={scanAgentMutation.isPending}
                          className="text-gray-400 hover:text-primary-600"
                          title="Trigger scan"
                        >
                          <RefreshCw className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => handleRemoveAgent(agent.id, agent.hostname)}
                          disabled={unregisterAgentMutation.isPending}
                          className="text-gray-400 hover:text-red-600"
                          title="Remove agent"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => navigate(`/agents/${agent.id}`)}
                          className="text-gray-400 hover:text-primary-600"
                          title="View details"
                        >
                          <ChevronRightIcon className="h-4 w-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
};

export default Agents;