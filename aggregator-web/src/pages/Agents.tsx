import React, { useState, useEffect, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Computer,
  RefreshCw,
  Search,
  Filter,
  ChevronRight as ChevronRightIcon,
  ChevronDown,
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
  Power,
  Database,
  Settings,
  MonitorPlay,
} from 'lucide-react';
import { useAgents, useAgent, useScanAgent, useScanMultipleAgents, useUnregisterAgent } from '@/hooks/useAgents';
import { useActiveCommands, useCancelCommand } from '@/hooks/useCommands';
import { useHeartbeatStatus, useInvalidateHeartbeat, useHeartbeatAgentSync } from '@/hooks/useHeartbeat';
import { agentApi } from '@/lib/api';
import { useQueryClient } from '@tanstack/react-query';
import { getStatusColor, formatRelativeTime, isOnline, formatBytes } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';
import { AgentSystemUpdates } from '@/components/AgentUpdates';
import { AgentStorage } from '@/components/AgentStorage';
import { AgentUpdatesEnhanced } from '@/components/AgentUpdatesEnhanced';
import { AgentScanners } from '@/components/AgentScanners';
import ChatTimeline from '@/components/ChatTimeline';

const Agents: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [osFilter, setOsFilter] = useState<string>('all');
  const [showFilters, setShowFilters] = useState(false);
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState<'overview' | 'storage' | 'updates' | 'scanners' | 'history'>('overview');
  const [currentTime, setCurrentTime] = useState(new Date());
  const [heartbeatDuration, setHeartbeatDuration] = useState<number>(10); // Default 10 minutes
  const [showDurationDropdown, setShowDurationDropdown] = useState(false);
  const [heartbeatLoading, setHeartbeatLoading] = useState(false); // Loading state for heartbeat toggle
  const [heartbeatCommandId, setHeartbeatCommandId] = useState<string | null>(null); // Track specific heartbeat command
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setShowDurationDropdown(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  // Duration options for heartbeat
  const durationOptions = [
    { label: '10 minutes', value: 10 },
    { label: '30 minutes', value: 30 },
    { label: '1 hour', value: 60 },
    { label: 'Permanent', value: -1 },
  ];

  // Get duration label for display
  const getDurationLabel = (duration: number) => {
    const option = durationOptions.find(opt => opt.value === duration);
    return option?.label || '10 minutes';
  };

  // Debounce search query to avoid API calls on every keystroke
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery);
    }, 300); // 300ms delay

    return () => {
      clearTimeout(timer);
    };
  }, [searchQuery]);

  
  // Update current time every second for countdown timers
  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentTime(new Date());
    }, 1000);

    return () => {
      clearInterval(timer);
    };
  }, []);

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

  // Helper function to format heartbeat expiration time
  const formatHeartExpiration = (untilString: string) => {
    const until = new Date(untilString);
    const now = new Date();
    const diffMs = until.getTime() - now.getTime();

    if (diffMs <= 0) {
      return 'expired';
    }

    const diffMinutes = Math.floor(diffMs / (1000 * 60));

    if (diffMinutes < 60) {
      return `${diffMinutes} minute${diffMinutes !== 1 ? 's' : ''}`;
    }

    const diffHours = Math.floor(diffMinutes / 60);
    const remainingMinutes = diffMinutes % 60;

    if (diffHours < 24) {
      return remainingMinutes > 0
        ? `${diffHours} hour${diffHours !== 1 ? 's' : ''} ${remainingMinutes} min`
        : `${diffHours} hour${diffHours !== 1 ? 's' : ''}`;
    }

    const diffDays = Math.floor(diffHours / 24);
    const remainingHours = diffHours % 24;

    return remainingHours > 0
      ? `${diffDays} day${diffDays !== 1 ? 's' : ''} ${remainingHours} hour${remainingHours !== 1 ? 's' : ''}`
      : `${diffDays} day${diffDays !== 1 ? 's' : ''}`;
  };

  // Fetch agents list
  const { data: agentsData, isPending, error, refetch } = useAgents({
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

  // Get heartbeat status for selected agent (smart polling - only when active)
  const { data: heartbeatStatus } = useHeartbeatStatus(selectedAgent?.id || '', !!selectedAgent);
  const invalidateHeartbeat = useInvalidateHeartbeat();
  const syncAgentData = useHeartbeatAgentSync(selectedAgent?.id || '', heartbeatStatus);

  
  // Simple completion handling - clear loading state quickly
  useEffect(() => {
    if (!heartbeatCommandId) return;

    // Clear loading state quickly since smart polling will handle UI updates
    const timeout = setTimeout(() => {
      setHeartbeatCommandId(null);
      setHeartbeatLoading(false);
    }, 2000); // 2 seconds - enough time for command to process

    return () => {
      clearTimeout(timeout);
    };
  }, [heartbeatCommandId]);

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

  // Handle agent reboot
  const handleRebootAgent = async (agentId: string, hostname: string) => {
    if (!window.confirm(
      `Schedule a system restart for agent "${hostname}"?\n\nThe system will restart in 1 minute. Any unsaved work may be lost.`
    )) {
      return;
    }

    try {
      await agentApi.rebootAgent(agentId);
      toast.success(`Restart command sent to "${hostname}". System will restart in 1 minute.`);
    } catch (error: any) {
      toast.error(error.message || `Failed to send restart command to "${hostname}"`);
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

  // Handle rapid polling toggle
  const handleRapidPollingToggle = async (agentId: string, enabled: boolean, durationMinutes?: number) => {
    // Prevent multiple clicks
    if (heartbeatLoading) return;

    setHeartbeatLoading(true);
    try {
      const duration = durationMinutes || heartbeatDuration;
      const result = await agentApi.toggleHeartbeat(agentId, enabled, duration);

      // Immediately invalidate cache to force fresh data
      invalidateHeartbeat(agentId);

      // Store the command ID for minimal tracking
      if (result.command_id) {
        setHeartbeatCommandId(result.command_id);
      }

      if (enabled) {
        if (duration === -1) {
          toast.success('Heartbeat enabled permanently');
        } else {
          toast.success(`Heartbeat enabled for ${duration} minutes`);
        }
      } else {
        toast.success('Heartbeat disabled');
      }
    } catch (error: any) {
      toast.error(`Failed to send heartbeat command: ${error.message || 'Unknown error'}`);
      setHeartbeatLoading(false);
      setHeartbeatCommandId(null);
    }
  };

  // Get agent-specific active commands
  const getAgentActiveCommands = () => {
    if (!selectedAgent || !activeCommandsData?.commands) return [];
    return activeCommandsData.commands.filter(cmd => cmd.agent_id === selectedAgent.id);
  };

  // Helper function to get command display info
  const getCommandDisplayInfo = (command: any) => {
    // Helper to get package name from command params
    const getPackageName = (cmd: any) => {
      if (cmd.package_name) return cmd.package_name;
      if (cmd.params?.package_name) return cmd.params.package_name;
      if (cmd.params?.update_id && cmd.update_name) return cmd.update_name;
      return 'unknown package';
    };

    const actionMap: { [key: string]: { icon: React.ReactNode; label: string } } = {
      'scan': { icon: <RefreshCw className="h-4 w-4" />, label: 'System scan' },
      'install_updates': { icon: <Package className="h-4 w-4" />, label: `Installing ${getPackageName(command)}` },
      'dry_run_update': { icon: <Search className="h-4 w-4" />, label: `Checking dependencies for ${getPackageName(command)}` },
      'confirm_dependencies': { icon: <CheckCircle className="h-4 w-4" />, label: `Installing ${getPackageName(command)}` },
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
                      {selectedAgent.current_version || 'Initial Registration'}
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

        {/* Restart Required Alert */}
        {selectedAgent.reboot_required && (
          <div className="mb-6 bg-amber-50 border border-amber-200 rounded-lg p-4">
            <div className="flex items-start">
              <AlertCircle className="h-5 w-5 text-amber-600 mt-0.5 mr-3 flex-shrink-0" />
              <div className="flex-1">
                <h3 className="text-sm font-medium text-amber-900">System Restart Required</h3>
                <p className="text-sm text-amber-700 mt-1">
                  {selectedAgent.reboot_reason || 'This system requires a restart to complete updates.'}
                </p>
                {selectedAgent.last_reboot_at && (
                  <p className="text-xs text-amber-600 mt-1">
                    Last reboot: {formatRelativeTime(selectedAgent.last_reboot_at)}
                  </p>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Enhanced Tabs */}
        <div className="mb-6">
          <div className="border-b border-gray-200">
            <nav className="-mb-px flex space-x-1 overflow-x-auto">
              <button
                onClick={() => setActiveTab('overview')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap',
                  activeTab === 'overview'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <span>Overview</span>
              </button>
              <button
                onClick={() => setActiveTab('storage')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center space-x-2',
                  activeTab === 'storage'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <HardDrive className="h-4 w-4" />
                <span>Storage & Disks</span>
              </button>
              <button
                onClick={() => setActiveTab('updates')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center space-x-2',
                  activeTab === 'updates'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <Package className="h-4 w-4" />
                <span>Updates & Packages</span>
              </button>
              <button
                onClick={() => setActiveTab('scanners')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center space-x-2',
                  activeTab === 'scanners'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <MonitorPlay className="h-4 w-4" />
                <span>Agent Health</span>
              </button>
              <button
                onClick={() => setActiveTab('history')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center space-x-2',
                  activeTab === 'history'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
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
            {activeTab === 'overview' && (
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

                    {/* Heartbeat Status Indicator */}
                    <div className="flex items-center space-x-2">
                      {(() => {
                        // Use dedicated heartbeat status instead of general agent metadata
                        const isRapidPolling = heartbeatStatus?.enabled && heartbeatStatus?.active;

                        // Get source from heartbeat status (stored in agent metadata)
                        const heartbeatSource = heartbeatStatus?.source;

                        // Debug: Log the source field
                        console.log('[Heartbeat Debug]', {
                          isRapidPolling,
                          source: heartbeatSource,
                          sourceType: typeof heartbeatSource,
                          heartbeatStatus
                        });

                        // Check if heartbeat is system-initiated (blue) or manual (pink)
                        const isSystemHeartbeat = heartbeatSource === 'system';
                        const isManualHeartbeat = heartbeatSource === 'manual';

                        return (
                          <button
                            onClick={() => handleRapidPollingToggle(selectedAgent.id, !isRapidPolling)}
                            disabled={heartbeatLoading}
                            className={cn(
                              'flex items-center space-x-1 px-2 py-1 rounded-md text-xs font-medium transition-colors',
                              heartbeatLoading
                                ? 'bg-gray-100 text-gray-400 border border-gray-200 cursor-not-allowed'
                                : isRapidPolling && isSystemHeartbeat
                                ? 'bg-blue-100 text-blue-800 border border-blue-200 hover:bg-blue-200 cursor-pointer'
                                : isRapidPolling && isManualHeartbeat
                                ? 'bg-pink-100 text-pink-800 border border-pink-200 hover:bg-pink-200 cursor-pointer'
                                : isRapidPolling
                                ? 'bg-gray-100 text-gray-800 border border-gray-200 hover:bg-gray-200 cursor-pointer'
                                : 'bg-gray-100 text-gray-600 border border-gray-200 hover:bg-gray-200 cursor-pointer'
                            )}
                            title={heartbeatLoading ? 'Sending command...' : `Click to toggle ${isRapidPolling ? 'normal' : 'heartbeat'} mode`}
                          >
                            {heartbeatLoading ? (
                              <RefreshCw className="h-3 w-3 animate-spin" />
                            ) : (
                              <Activity className={cn(
                                'h-3 w-3',
                                isRapidPolling && isSystemHeartbeat ? 'text-blue-600 animate-pulse' :
                                isRapidPolling && isManualHeartbeat ? 'text-pink-600 animate-pulse' :
                                isRapidPolling ? 'text-gray-600 animate-pulse' : 'text-gray-400'
                              )} />
                            )}
                            <span>
                              {heartbeatLoading ? 'Sending...' : isRapidPolling ? 'Heartbeat (5s)' : 'Normal (5m)'}
                            </span>
                          </button>
                        );
                      })()}
                    </div>
                  </div>

                  {/* Compact Timeline Display */}
                  <div className="space-y-2 mb-3">
                    {(() => {
                      const agentCommands = getAgentActiveCommands();

                      // Separate heartbeat commands from other commands
                      const heartbeatCommands = agentCommands.filter(cmd =>
                        cmd.command_type === 'enable_heartbeat' || cmd.command_type === 'disable_heartbeat'
                      );
                      const otherCommands = agentCommands.filter(cmd =>
                        cmd.command_type !== 'enable_heartbeat' && cmd.command_type !== 'disable_heartbeat'
                      );

                      // For heartbeat commands: only show the MOST RECENT one, but exclude old completed ones
                      const recentHeartbeatCommands = heartbeatCommands.filter(cmd => {
                        const createdTime = new Date(cmd.created_at);
                        const now = new Date();
                        const hoursOld = (now.getTime() - createdTime.getTime()) / (1000 * 60 * 60);

                        // Exclude completed/failed heartbeat commands older than 30 minutes
                        if ((cmd.status === 'completed' || cmd.status === 'failed' || cmd.status === 'timed_out') && hoursOld > 0.5) {
                          return false;
                        }
                        return true;
                      });

                      const latestHeartbeatCommand = recentHeartbeatCommands.length > 0
                        ? [recentHeartbeatCommands.reduce((latest, cmd) =>
                            new Date(cmd.created_at) > new Date(latest.created_at) ? cmd : latest
                          )]
                        : [];

                      // For other commands: show active ones normally
                      const activeOtherCommands = otherCommands.filter(cmd =>
                        cmd.status === 'running' || cmd.status === 'sent' || cmd.status === 'pending'
                      );
                      const completedOtherCommands = otherCommands.filter(cmd =>
                        cmd.status === 'completed' || cmd.status === 'failed' || cmd.status === 'timed_out'
                      ).slice(0, 1); // Only show last completed

                      const displayCommands = [
                        ...latestHeartbeatCommand.slice(0, 1), // Max 1 heartbeat (latest only)
                        ...activeOtherCommands.slice(0, 2), // Max 2 active other commands
                        ...completedOtherCommands.slice(0, 1) // Max 1 completed other command
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
                                  {(() => {
                                    const createdTime = new Date(command.created_at);
                                    const now = new Date();
                                    const hoursOld = (now.getTime() - createdTime.getTime()) / (1000 * 60 * 60);

                                    // Show exact time for commands older than 1 hour, relative time for recent ones
                                    if (hoursOld > 1) {
                                      return createdTime.toLocaleString();
                                    } else {
                                      return formatRelativeTime(command.created_at);
                                    }
                                  })()}
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

                  {/* Heartbeat Status Info */}
                  {heartbeatStatus?.enabled && heartbeatStatus?.active && (
                    (() => {
                      // Get source from heartbeat status (stored in agent metadata)
                      const heartbeatSource = heartbeatStatus?.source;
                      const isSystemHeartbeat = heartbeatSource === 'system';

                      return (
                        <div className={cn(
                          "text-xs px-2 py-1 rounded-md mt-2",
                          isSystemHeartbeat
                            ? "text-blue-600 bg-blue-50"
                            : "text-pink-600 bg-pink-50"
                        )}>
                          {isSystemHeartbeat ? 'System ' : 'Manual '}heartbeat active for {formatHeartExpiration(heartbeatStatus.until)}
                        </div>
                      );
                    })()
                  )}

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
              </div>
            )}

            {activeTab === 'storage' && (
              <AgentStorage agentId={selectedAgent.id} />
            )}

            {activeTab === 'updates' && (
              <AgentUpdatesEnhanced agentId={selectedAgent.id} />
            )}

            {activeTab === 'scanners' && (
              <AgentScanners agentId={selectedAgent.id} />
            )}

            {activeTab === 'history' && (
              <ChatTimeline agentId={selectedAgent.id} isScopedView={true} />
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

                {/* Split button for heartbeat with duration */}
                <div className="flex space-x-2">
                  <button
                    onClick={() => {
                      // Use dedicated heartbeat status instead of general agent metadata
                      const isRapidPolling = heartbeatStatus?.enabled && heartbeatStatus?.active;
                      handleRapidPollingToggle(selectedAgent.id, !isRapidPolling);
                    }}
                    disabled={heartbeatLoading}
                    className={cn(
                      'flex-1 btn transition-colors',
                      heartbeatLoading
                        ? 'opacity-50 cursor-not-allowed'
                        : heartbeatStatus?.enabled && heartbeatStatus?.active
                        ? 'btn-primary' // Use primary style for active heartbeat
                        : 'btn-secondary' // Use secondary style for normal mode
                    )}
                  >
                    {heartbeatLoading ? (
                      <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
                    ) : (
                      <Activity className="h-4 w-4 mr-2" />
                    )}
                    {heartbeatLoading
                      ? 'Sending Command...'
                      : heartbeatStatus?.enabled && heartbeatStatus?.active
                      ? 'Disable Heartbeat'
                      : 'Enable Heartbeat (5s)'
                    }
                  </button>

                  {/* Duration dropdown */}
                  <div className="relative" ref={dropdownRef}>
                    <button
                      onClick={() => setShowDurationDropdown(!showDurationDropdown)}
                      className="btn btn-secondary px-3 min-w-[100px]"
                    >
                      {getDurationLabel(heartbeatDuration)}
                      <ChevronDown className="h-4 w-4 ml-1" />
                    </button>

                    {showDurationDropdown && (
                      <div className="absolute right-0 mt-1 w-48 bg-white rounded-lg shadow-lg border border-gray-200 z-10">
                        <div className="py-1">
                          {durationOptions.map((option) => (
                            <button
                              key={option.value}
                              onClick={() => {
                                setHeartbeatDuration(option.value);
                                setShowDurationDropdown(false);
                              }}
                              className={cn(
                                'w-full px-4 py-2 text-left text-sm hover:bg-gray-100 transition-colors',
                                heartbeatDuration === option.value ? 'bg-gray-100 font-medium' : 'text-gray-700'
                              )}
                            >
                              {option.label}
                            </button>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                </div>

                <button
                  onClick={() => handleRebootAgent(selectedAgent.id, selectedAgent.hostname)}
                  className="w-full btn btn-warning"
                >
                  <Power className="h-4 w-4 mr-2" />
                  Restart Host
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
                      <div className="flex flex-col space-y-1">
                        <span className={cn('badge', getStatusColor(isOnline(agent.last_seen) ? 'online' : 'offline'))}>
                          {isOnline(agent.last_seen) ? 'Online' : 'Offline'}
                        </span>
                        {agent.reboot_required && (
                          <span className="flex items-center text-xs text-amber-700 bg-amber-50 px-1.5 py-0.5 rounded-full w-fit">
                            <Power className="h-3 w-3 mr-1" />
                            Restart
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="flex items-center space-x-2">
                        <span className="text-sm text-gray-900">
                          {agent.current_version || 'Initial Registration'}
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