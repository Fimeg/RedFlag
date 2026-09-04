import React, { useState, useEffect, useRef, useMemo } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import {
  RefreshCw,
  ChevronRight as ChevronRightIcon,
  ChevronDown,
  Activity,
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
  Ban,
  Power,
  MonitorPlay,
  Upload,
  UserPlus,
} from 'lucide-react';
import { useFilterUrl, buildFilterPills } from '@/hooks/useFilterUrl';
import {
  FilterBar,
  PageState,
  ScreenshotCard,
  MetricItem,
  ProcessTable,
  CommandCard,
  SortableTable,
  StatusBadge,
  useConfirm,
} from '@/components/primitives';
import type { Column } from '@/components/primitives';
import { useDebounce } from '@/hooks/useDebounce';
import { useColumnSort } from '@/hooks/useColumnSort';
import { useAgents, useAgent, useScanMultipleAgents, useUnregisterAgent } from '@/hooks/useAgents';
import { useRevokeAgent } from '@/hooks/useRegistrationTokens';
import { useActiveCommands, useCancelCommand, useCaptureScreenshot, useCommand } from '@/hooks/useCommands';
import { useHeartbeatStatus } from '@/hooks/useHeartbeat';
import { agentApi } from '@/lib/api';
import { useQueryClient } from '@tanstack/react-query';
import { formatRelativeTime, isOnline, formatBytes } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';
import { AgentStorage } from '@/components/AgentStorage';
import { DeviceTypeIcon, DeviceTypeBadge, deviceTypeLabel } from '@/components/DeviceTypeIcon';
import { DEVICE_TYPES } from '@/types';
import { AgentUpdatesEnhanced } from '@/components/AgentUpdatesEnhanced';
import { AgentHealth } from '@/components/AgentHealth';
import { AgentUpdatesModal } from '@/components/AgentUpdatesModal';
import { BulkAgentUpdate } from '@/components/RelayList';
import ChatTimeline from '@/components/ChatTimeline';
import AgentSoftwareBindings from '@/components/AgentSoftwareBindings';
import { ProcessesTab } from '@/components/ProcessesTab';
import { readIntegrations, resolveState } from '@/types/integrations';

type AgentDetailTab = 'overview' | 'processes' | 'storage' | 'updates' | 'software' | 'scanners' | 'history';

const AGENT_DETAIL_TABS: AgentDetailTab[] = ['overview', 'processes', 'storage', 'updates', 'software', 'scanners', 'history'];

const parseAgentDetailTab = (tab: string | null): AgentDetailTab => {
  return AGENT_DETAIL_TABS.includes(tab as AgentDetailTab) ? tab as AgentDetailTab : 'overview';
};

const Agents: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const confirm = useConfirm();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const [searchQuery, setSearchQuery] = useState(searchParams.get('search') || '');
  const debouncedSearchQuery = useDebounce(searchQuery, 300);
  const filterConfig = {
    status: { urlParam: 'status', label: 'Status' },
    os: { urlParam: 'os', label: 'OS' },
    device: { urlParam: 'device', label: 'Device' },
  };
  const filter = useFilterUrl(filterConfig);
  const { sortBy, sortOrder, handleSort, applySort } = useColumnSort({
    defaultSortBy: 'last_seen',
    defaultOrder: 'asc',
  });
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);
  const activeTab = parseAgentDetailTab(searchParams.get('tab'));
  const [heartbeatDuration, setHeartbeatDuration] = useState<number>(10); // Default 10 minutes
  const [showDurationDropdown, setShowDurationDropdown] = useState(false);
  const [heartbeatLoading, setHeartbeatLoading] = useState(false); // Loading state for heartbeat toggle
  const [showUpdateModal, setShowUpdateModal] = useState(false); // Update modal state
  const [singleAgentUpdate, setSingleAgentUpdate] = useState<string | null>(null); // Single agent update modal
  const [screenshotCommandId, setScreenshotCommandId] = useState<string | null>(null);
  const [showRestartDropdown, setShowRestartDropdown] = useState(false);
  const [showReclassifyDropdown, setShowReclassifyDropdown] = useState(false);
  const [reclassifyPending, setReclassifyPending] = useState(false);
  const reclassifyDropdownRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const restartDropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdowns when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setShowDurationDropdown(false);
      }
      if (restartDropdownRef.current && !restartDropdownRef.current.contains(event.target as Node)) {
        setShowRestartDropdown(false);
      }
      if (reclassifyDropdownRef.current && !reclassifyDropdownRef.current.contains(event.target as Node)) {
        setShowReclassifyDropdown(false);
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

  const selectActiveTab = (tab: AgentDetailTab) => {
    const params = new URLSearchParams(searchParams);
    if (tab === 'overview') {
      params.delete('tab');
    } else {
      params.set('tab', tab);
    }
    setSearchParams(params, { replace: true });
  };

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
  const { data: agentsData, isPending, error, refetch: _refetch } = useAgents({
    search: debouncedSearchQuery || undefined,
    status: filter.values.status || undefined,
  });

  // Fetch single agent if ID is provided
  const { data: selectedAgentData } = useAgent(id || '', !!id);

  const scanMultipleMutation = useScanMultipleAgents();
  const unregisterAgentMutation = useUnregisterAgent();
  const revokeAgentMutation = useRevokeAgent();

  // Active commands for live status
  const { data: activeCommandsData, refetch: refetchActiveCommands } = useActiveCommands();
  const cancelCommandMutation = useCancelCommand();

  // Screenshot capture
  const captureScreenshotMutation = useCaptureScreenshot();
  const { data: screenshotCommand } = useCommand(screenshotCommandId, !!screenshotCommandId);

  const agents = agentsData?.agents || [];
  const selectedAgent = selectedAgentData || agents.find(a => a.id === id);

  // Heartbeat status derived from agent metadata — no separate endpoint poll.
  // Server writes rapid_polling_enabled/rapid_polling_until/heartbeat_source
  // to agent metadata on every poll response and toggle.
  const heartbeatStatus = useHeartbeatStatus(selectedAgent?.metadata);

  // Filter agents based on OS and device type
  const filteredAgents = agents.filter(agent => {
    if (filter.values.os && !agent.os_type.toLowerCase().includes(filter.values.os.toLowerCase())) {
      return false;
    }
    if (filter.values.device && (agent.effective_device_type || 'server') !== filter.values.device) {
      return false;
    }
    return true;
  });

  // Sort agents client-side (fleet size doesn't warrant server-side pagination yet)
  const sortedAgents = applySort(filteredAgents, (agent) => {
    switch (sortBy) {
      case 'hostname': return agent.hostname;
      case 'status': return isOnline(agent.last_seen) ? 0 : 1; // online first when asc
      case 'version': return agent.current_version || agent.agent_version || '';
      case 'os': return agent.os_type;
      case 'last_seen': return agent.last_seen ? new Date(agent.last_seen) : null;
      case 'last_scan': return agent.last_scan ? new Date(agent.last_scan) : null;
      default: return null;
    }
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
      setSelectedAgents(sortedAgents.map(agent => agent.id));
    } else {
      setSelectedAgents([]);
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
  // Operator device-type override (WEB-001/SERVER-002). null clears back to auto.
  const handleReclassify = async (agentId: string, deviceType: string | null) => {
    setReclassifyPending(true);
    try {
      const result = await agentApi.reclassifyDeviceType(agentId, deviceType);
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agent', agentId] });
      toast.success(
        deviceType
          ? `Reclassified as ${deviceTypeLabel(result.effective_device_type)}`
          : `Override cleared — auto-detected as ${deviceTypeLabel(result.effective_device_type)}`
      );
    } catch (error: any) {
      toast.error(error.message || 'Failed to reclassify device');
    } finally {
      setReclassifyPending(false);
      setShowReclassifyDropdown(false);
    }
  };

  const handleRebootAgent = async (agentId: string, hostname: string) => {
    if (!(await confirm({
      title: 'Schedule System Restart',
      body: `Schedule a system restart for agent "${hostname}"? The system will restart in 1 minute. Any unsaved work may be lost.`,
      confirmLabel: 'Restart',
      danger: true,
    }))) return;

    try {
      await agentApi.rebootAgent(agentId);
      toast.success(`Restart command sent to "${hostname}". System will restart in 1 minute.`);
    } catch (error: any) {
      toast.error(error.message || `Failed to send restart command to "${hostname}"`);
    }
  };

  // Handle agent removal
  const handleRemoveAgent = async (agentId: string, hostname: string) => {
    if (!(await confirm({
      title: 'Remove Agent',
      body: `Remove agent "${hostname}"? This action cannot be undone and will remove the agent from the system.`,
      confirmLabel: 'Remove',
      danger: true,
    }))) return;

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

  // Revoke an agent — invalidates its refresh tokens so it can no longer check
  // in. Distinct from Remove (which deletes the record). Enrolled agents are not
  // affected by key revocation; this is the explicit per-agent path.
  const handleRevokeAgent = async (agentId: string, hostname: string) => {
    if (!(await confirm({
      title: 'Revoke agent',
      body: `Revoke agent "${hostname}"? Its refresh tokens are invalidated so it can no longer check in. The agent record and its history are kept; re-enrolling needs a new registration key.`,
      confirmLabel: 'Revoke agent',
      danger: true,
    }))) return;
    revokeAgentMutation.mutate(
      { agentId },
      { onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }) },
    );
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

  // Handle screenshot capture
  const handleCaptureScreenshot = async (agentId: string) => {
    try {
      const result = await captureScreenshotMutation.mutateAsync(agentId);
      setScreenshotCommandId(result.command_id);
      toast.success('Screenshot capture requested');
    } catch (error: any) {
      toast.error(`Failed to capture screenshot: ${error.message || 'Unknown error'}`);
    }
  };

  // Extract base64 screenshot from command result
  const screenshotImage = screenshotCommand?.status === 'completed'
    ? screenshotCommand?.result?.stdout || null
    : null;

  // Handle rapid polling toggle
  const handleRapidPollingToggle = async (agentId: string, enabled: boolean, durationMinutes?: number) => {
    // Prevent multiple clicks
    if (heartbeatLoading) return;

    setHeartbeatLoading(true);
    try {
      const duration = durationMinutes || heartbeatDuration;
      await agentApi.toggleHeartbeat(agentId, enabled, duration);

      // Invalidate agent query so the next refetch picks up the new metadata.
      // The server writes rapid_polling_enabled/rapid_polling_until to agent
      // metadata immediately on toggle, so the next agent data fetch (30s max)
      // will reflect the new state. Clear loading after a short delay.
      queryClient.invalidateQueries({ queryKey: ['agent', agentId] });
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      setTimeout(() => setHeartbeatLoading(false), 2000);

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
    }
  };

  // Get agent-specific active commands
  const getAgentActiveCommands = () => {
    if (!selectedAgent || !activeCommandsData?.commands) return [];
    return activeCommandsData.commands.filter(cmd => cmd.agent_id === selectedAgent.id);
  };

  // Get unique OS types for filter
  const osTypes = [...new Set(agents.map(agent => agent.os_type))];

  // Column definitions for the fleet table
  const agentColumns: Column<typeof sortedAgents[number]>[] = useMemo(() => [
    {
      key: 'hostname', label: 'Agent', sortKey: 'hostname',
      render: (agent) => (
        <div className="flex items-center space-x-3">
          <div
            className="w-8 h-8 bg-gray-100 rounded-full flex items-center justify-center"
            title={deviceTypeLabel(agent.effective_device_type)}
          >
            <DeviceTypeIcon type={agent.effective_device_type} className="h-4 w-4 text-gray-600" />
          </div>
          <div>
            <div className="text-sm font-medium text-gray-900">
              <button onClick={() => navigate(`/agents/${agent.id}`)} className="hover:text-primary-600">
                {agent.hostname}
              </button>
            </div>
            <div className="text-xs text-gray-500">
              {agent.device_model || (agent.metadata && (() => {
                const meta = getSystemMetadata(agent);
                const parts = [];
                if (meta.cpuCores !== 'Unknown') parts.push(`${meta.cpuCores} cores`);
                if (meta.memoryTotal > 0) parts.push(formatBytes(meta.memoryTotal));
                if (parts.length > 0) return parts.join(' • ');
                return 'System info available';
              })())}
            </div>
          </div>
        </div>
      ),
    },
    {
      key: 'status', label: 'Status', sortKey: 'status',
      render: (agent) => (
        <div className="flex flex-col space-y-1 items-start">
          <StatusBadge
            status={isOnline(agent.last_seen) ? 'online' : 'offline'}
            label={isOnline(agent.last_seen) ? 'Online' : 'Offline'}
          />
          {agent.reboot_required && (
            <span className="inline-flex items-center text-xs text-amber-700 bg-amber-50 px-1.5 py-0.5 rounded-full w-fit">
              <Power className="h-3 w-3 mr-1" />
              Restart
            </span>
          )}
        </div>
      ),
    },
    {
      key: 'version', label: 'Version', sortKey: 'version',
      render: (agent) => (
        <div className="flex items-center space-x-2">
          <span className="text-sm text-gray-900">{agent.current_version || 'Initial Registration'}</span>
          {agent.update_available === true && (
            <button
              onClick={(e) => { e.stopPropagation(); setSingleAgentUpdate(agent.id); setShowUpdateModal(true); }}
              className="inline-flex items-center text-xs text-amber-600 bg-amber-50 hover:bg-amber-100 px-1.5 py-0.5 rounded-full cursor-pointer transition-colors"
              title="Click to update agent"
            >
              <Download className="h-3 w-3 mr-1" />Update
            </button>
          )}
          {agent.update_available === false && agent.current_version && (
            <span className="badge text-green-600 bg-green-50"><CheckCircle className="h-3 w-3 mr-1" />Current</span>
          )}
        </div>
      ),
    },
    {
      key: 'os', label: 'OS', sortKey: 'os',
      render: (agent) => {
        const osInfo = parseOSInfo(agent);
        return (
          <div>
            <div className="text-sm text-gray-900">{osInfo.distribution || agent.os_type}</div>
            <div className="text-xs text-gray-500">
              {[agent.os_distro, osInfo.version, agent.os_architecture || agent.architecture]
                .filter(Boolean).join(' • ')}
            </div>
          </div>
        );
      },
    },
    {
      key: 'last_seen', label: 'Last Check-in', sortKey: 'last_seen',
      render: (agent) => (
        <div>
          <div className="text-sm text-gray-900">{formatRelativeTime(agent.last_seen)}</div>
          <div className="text-xs text-gray-500">{isOnline(agent.last_seen) ? 'Online' : 'Offline'}</div>
        </div>
      ),
    },
    {
      key: 'last_scan', label: 'Last Scan', sortKey: 'last_scan',
      render: (agent) => (
        <div className="text-sm text-gray-900">
          {agent.last_scan ? formatRelativeTime(agent.last_scan) : 'Never'}
        </div>
      ),
    },
    {
      key: 'actions', label: 'Actions', sortable: false,
      render: (agent) => (
        <div className="flex items-center space-x-2">
          <button
            onClick={() => { setSelectedAgents([agent.id]); setShowUpdateModal(true); }}
            disabled={agent.is_updating}
            className={cn("text-gray-400 hover:text-primary-600", agent.is_updating && "text-amber-600 animate-pulse")}
            title={agent.is_updating ? "Agent is updating..." : "Update agent"}
          >
            <Upload className="h-4 w-4" />
          </button>
          <button
            onClick={() => handleRevokeAgent(agent.id, agent.hostname)}
            disabled={revokeAgentMutation.isPending}
            className="text-gray-400 hover:text-orange-600"
            title="Revoke agent (invalidate refresh tokens)"
          >
            <Ban className="h-4 w-4" />
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
      ),
    },
  ] as Column<typeof sortedAgents[number]>[], [navigate, handleRemoveAgent, handleRevokeAgent, unregisterAgentMutation.isPending, revokeAgentMutation.isPending]);

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
                      <span className="badge text-amber-600 bg-amber-50">
                        <AlertCircle className="h-3 w-3 mr-1" />
                        Update Available
                      </span>
                    )}
                    {selectedAgent.update_available === false && selectedAgent.current_version && (
                      <span className="badge text-green-600 bg-green-50">
                        <CheckCircle className="h-3 w-3 mr-1" />
                        Up to Date
                      </span>
                    )}
                  </div>
                  <span className="text-gray-500">]</span>
                </div>
              </div>

              {/* Sub-line: device classification + registration info */}
              <div className="flex flex-wrap items-center gap-2 text-sm text-gray-600">
                <div className="relative" ref={reclassifyDropdownRef}>
                  <button
                    onClick={() => setShowReclassifyDropdown(!showReclassifyDropdown)}
                    disabled={reclassifyPending}
                    className="hover:opacity-75 transition-opacity"
                    title="Reclassify device type"
                  >
                    <DeviceTypeBadge
                      type={selectedAgent.effective_device_type || 'server'}
                      overridden={!!selectedAgent.device_type_manual}
                    />
                  </button>
                  {showReclassifyDropdown && (
                    <div className="absolute left-0 mt-1 w-48 bg-white border border-gray-200 rounded-lg shadow-lg z-20">
                      {DEVICE_TYPES.map(t => (
                        <button
                          key={t}
                          onClick={() => handleReclassify(selectedAgent.id, t)}
                          disabled={reclassifyPending}
                          className={cn(
                            'w-full text-left px-3 py-2 text-sm hover:bg-gray-50 flex items-center space-x-2',
                            selectedAgent.effective_device_type === t && 'bg-gray-50 font-medium'
                          )}
                        >
                          <DeviceTypeIcon type={t} className="h-4 w-4 text-gray-500" />
                          <span>{deviceTypeLabel(t)}</span>
                        </button>
                      ))}
                      {selectedAgent.device_type_manual && (
                        <button
                          onClick={() => handleReclassify(selectedAgent.id, null)}
                          disabled={reclassifyPending}
                          className="w-full text-left px-3 py-2 text-sm text-gray-500 hover:bg-gray-50 border-t border-gray-100"
                        >
                          Clear override (use auto-detect)
                        </button>
                      )}
                    </div>
                  )}
                </div>
                {selectedAgent.device_model && (
                  <>
                    <span className="text-gray-400">•</span>
                    <span>{selectedAgent.device_model}</span>
                  </>
                )}
                {selectedAgent.os_distro && (
                  <>
                    <span className="text-gray-400">•</span>
                    <span>{selectedAgent.os_distro}</span>
                  </>
                )}
                <span className="text-gray-400">•</span>
                <span>Registered {formatRelativeTime(selectedAgent.created_at)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Restart Required Alert */}
        {selectedAgent.reboot_required && (
          <div className="mb-6 alert alert-warning">
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
                onClick={() => selectActiveTab('overview')}
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
                onClick={() => selectActiveTab('processes')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center space-x-2',
                  activeTab === 'processes'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <Activity className="h-4 w-4" />
                <span>Processes</span>
              </button>
              <button
                onClick={() => selectActiveTab('storage')}
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
                onClick={() => selectActiveTab('updates')}
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
                onClick={() => selectActiveTab('software')}
                className={cn(
                  'py-3 px-4 border-b-2 font-medium text-sm transition-colors whitespace-nowrap flex items-center space-x-2',
                  activeTab === 'software'
                    ? 'border-primary-500 text-primary-600 bg-primary-50 rounded-t-lg'
                    : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <GitBranch className="h-4 w-4" />
                <span>Tracked Software</span>
              </button>
              <button
                onClick={() => selectActiveTab('scanners')}
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
                onClick={() => selectActiveTab('history')}
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
                      <StatusBadge
                        status={isOnline(selectedAgent.last_seen) ? 'online' : 'offline'}
                        label={isOnline(selectedAgent.last_seen) ? 'Online' : 'Offline'}
                      />
                    </div>

                    {/* Heartbeat Status Indicator */}
                    <div className="flex items-center space-x-2">
                      {(() => {
                        // Use dedicated heartbeat status instead of general agent metadata
                        const isRapidPolling = heartbeatStatus?.enabled && heartbeatStatus?.active;

                        // Get source from heartbeat status (stored in agent metadata)
                        const heartbeatSource = heartbeatStatus?.source;

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
                                ? 'bg-info-100 text-info-800 border border-info-200 hover:bg-info-200 cursor-pointer'
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
                                isRapidPolling && isSystemHeartbeat ? 'text-info-600 animate-pulse' :
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

                      return displayCommands.map((command) => (
                        <CommandCard
                          key={command.id}
                          command={command}
                          onCancel={handleCancelCommand}
                          onDismiss={handleCancelCommand}
                          disabled={cancelCommandMutation.isPending}
                        />
                      ));
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
                      const heartbeatSource = heartbeatStatus?.source;
                      const isSystemHeartbeat = heartbeatSource === 'system';
                      const isManualHeartbeat = heartbeatSource === 'manual';

                      return (
                        <div className={cn(
                          "text-xs px-2 py-1 rounded-md mt-2",
                          isSystemHeartbeat
                            ? "text-info-600 bg-info-50"
                            : isManualHeartbeat
                            ? "text-pink-600 bg-pink-50"
                            : "text-gray-600 bg-gray-50"
                        )}>
                          {isSystemHeartbeat ? 'System ' : isManualHeartbeat ? 'Manual ' : ''}heartbeat active for {formatHeartExpiration(heartbeatStatus.until || '')}
                        </div>
                      );
                    })()
                  )}
                </div>

                {/* System info — screenshot pane + metrics grid */}
                <div className="card">
                  <h2 className="text-lg font-medium text-gray-900 mb-4">System Information</h2>

                  {(() => {
                    const integrations = readIntegrations(selectedAgent.metadata);
                    const sunshine = integrations.sunshine;
                    const osInfo = parseOSInfo(selectedAgent);
                    const meta = getSystemMetadata(selectedAgent);
                    const topProcesses = selectedAgent.metadata?.top_processes;

                    return (
                      <div className="flex flex-col gap-6 md:flex-row">
                        {/* Left: screenshot grows to keep the panes level, platform identity below */}
                        <div className="flex flex-col gap-3 md:w-[45%] md:shrink-0">
                          <ScreenshotCard
                            className="grow"
                            image={screenshotImage}
                            isCapturing={captureScreenshotMutation.isPending}
                            isPolling={screenshotCommand && !['completed', 'failed', 'timed_out', 'cancelled'].includes(screenshotCommand.status)}
                            sunshine={sunshine ? { ...sunshine, state: resolveState(sunshine) } : undefined}
                            screenshotStatus={screenshotCommand}
                            onCapture={() => handleCaptureScreenshot(selectedAgent.id)}
                            formatRelativeTime={formatRelativeTime}
                          />
                          <MetricItem label="Platform" value={osInfo.platform} />
                          <MetricItem label="Distribution" value={osInfo.distribution} sub={osInfo.version ? `(${osInfo.version})` : undefined} />
                          <MetricItem label="Architecture" value={selectedAgent.os_architecture || selectedAgent.architecture} />
                        </div>
                        {/* Right: hardware metrics, ProcessTable spanning full width below */}
                        <div className="grid flex-1 grid-cols-1 content-start gap-3 min-w-0 sm:grid-cols-2">
                          <MetricItem label="CPU" value={meta.cpuModel} icon={Cpu} sub={`${meta.cpuCores} cores`} />
                          {meta.memoryTotal > 0 && (
                            <MetricItem label="Memory" value={formatBytes(meta.memoryTotal)} icon={MemoryStick} />
                          )}
                          {meta.diskTotal > 0 && (
                            <MetricItem
                              label={`Disk (${meta.diskMount})`}
                              value={`${formatBytes(meta.diskUsed)} / ${formatBytes(meta.diskTotal)}`}
                              icon={HardDrive}
                              progress={{ used: meta.diskUsed, total: meta.diskTotal }}
                            />
                          )}
                          {meta.processes !== 'Unknown' && (
                            <MetricItem label="Running Processes" value={meta.processes} icon={GitBranch} />
                          )}
                          {meta.uptime !== 'Unknown' && (
                            <MetricItem label="Uptime" value={meta.uptime} icon={Clock} />
                          )}
                          <ProcessTable
                            className="col-span-full"
                            processes={topProcesses}
                            processCount={meta.processes}
                            onSeeMore={() => selectActiveTab('processes')}
                          />
                        </div>
                      </div>
                    );
                  })()}
                </div>
              </div>
            )}

            {activeTab === 'processes' && (
              <ProcessesTab agentId={selectedAgent.id} />
            )}

            {activeTab === 'storage' && (
              <AgentStorage agentId={selectedAgent.id} />
            )}

            {activeTab === 'updates' && (
              <AgentUpdatesEnhanced
                agentId={selectedAgent.id}
                onNavigateToHistory={() => selectActiveTab('history')}
              />
            )}

            {activeTab === 'software' && (
              <AgentSoftwareBindings agentId={selectedAgent.id} />
            )}

            {activeTab === 'scanners' && (
              <AgentHealth agentId={selectedAgent.id} />
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
                  <div className="relative shrink-0" ref={dropdownRef}>
                    <button
                      onClick={() => setShowDurationDropdown(!showDurationDropdown)}
                      className="btn btn-secondary px-3 whitespace-nowrap"
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

                {/* Split button: Restart Host (main) / Remove Agent (dropdown) */}
                <div className="flex" ref={restartDropdownRef}>
                  <button
                    onClick={() => handleRebootAgent(selectedAgent.id, selectedAgent.hostname)}
                    className="flex-1 btn btn-warning rounded-r-none border-r border-warning-700"
                  >
                    <Power className="h-4 w-4 mr-2" />
                    Restart Host
                  </button>
                  <div className="relative flex">
                    <button
                      onClick={() => setShowRestartDropdown(!showRestartDropdown)}
                      className="btn btn-warning rounded-l-none px-2 self-stretch"
                      title="More actions"
                    >
                      <ChevronDown className="h-4 w-4" />
                    </button>
                    {showRestartDropdown && (
                      <div className="absolute right-0 mt-1 w-48 bg-white rounded-lg shadow-lg border border-gray-200 z-10">
                        <button
                          onClick={() => {
                            setShowRestartDropdown(false);
                            handleRemoveAgent(selectedAgent.id, selectedAgent.hostname);
                          }}
                          disabled={unregisterAgentMutation.isPending}
                          className="w-full px-4 py-2 text-left text-sm text-danger-700 hover:bg-danger-50 flex items-center rounded-lg transition-colors disabled:opacity-50"
                        >
                          {unregisterAgentMutation.isPending ? (
                            <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                          ) : (
                            <Trash2 className="h-4 w-4 mr-2" />
                          )}
                          Remove Agent
                        </button>
                      </div>
                    )}
                  </div>
                </div>
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
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Agents</h1>
          <p className="mt-1 text-sm text-gray-600">
            Monitor and manage your connected agents
          </p>
        </div>
        <button
          onClick={() => navigate('/settings/agents?install=1')}
          className="btn btn-primary shrink-0"
          title="Enroll a new agent — pick a registration key and copy the install one-liner"
        >
          <UserPlus className="h-4 w-4 mr-2" />
          Enroll agent
        </button>
      </div>

      <FilterBar
        search={{ value: searchQuery, onChange: setSearchQuery, placeholder: 'Search agents by hostname...' }}
        filters={[
          { label: 'Status', value: filter.values.status, onChange: (v) => filter.setFilter('status', v), options: [
            { value: 'online', label: 'Online' },
            { value: 'offline', label: 'Offline' },
          ], placeholder: 'All Status' },
          { label: 'OS', value: filter.values.os, onChange: (v) => filter.setFilter('os', v), options: osTypes.map(os => ({ value: os, label: os })), placeholder: 'All OS' },
          { label: 'Device', value: filter.values.device, onChange: (v) => filter.setFilter('device', v), options: DEVICE_TYPES.map(t => ({ value: t, label: deviceTypeLabel(t) })), placeholder: 'All Devices' },
        ]}
        pills={buildFilterPills(filter, filterConfig)}
        onClearAll={() => filter.clearAll()}
        activeCount={filter.activeCount}
        actions={
          selectedAgents.length > 0 ? (
            <>
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
              <BulkAgentUpdate
                agents={agents.filter(agent => selectedAgents.includes(agent.id))}
                onBulkUpdateComplete={() => {
                  queryClient.invalidateQueries({ queryKey: ['agents'] });
                }}
              />
            </>
          ) : undefined
        }
        className="mb-6"
      />

      {/* Agents table — PageState primitive */}
      <PageState
        loading={isPending}
        error={error ? 'Failed to load agents' : null}
        empty={sortedAgents.length === 0}
        emptyTitle="No agents found"
        emptyMessage={
          debouncedSearchQuery || filter.values.status || filter.values.os
            ? 'Try adjusting your search or filters.'
            : 'No agents have registered with the server yet.'
        }
      >
        <SortableTable
          columns={agentColumns}
          data={sortedAgents}
          getKey={(a) => a.id}
          sortBy={sortBy}
          sortOrder={sortOrder}
          onSort={handleSort}
          selectable
          selected={selectedAgents}
          onSelectAll={(all) => handleSelectAll(all)}
          onSelectOne={(id, checked) => handleSelectAgent(id, checked)}
          emptyMessage="No agents match the current filters."
        />
      </PageState>

      {/* Agent Updates Modal */}
      <AgentUpdatesModal
        isOpen={showUpdateModal}
        onClose={() => {
          setShowUpdateModal(false);
          setSingleAgentUpdate(null);
          setSelectedAgents([]);
        }}
        selectedAgentIds={singleAgentUpdate ? [singleAgentUpdate] : selectedAgents}
        onAgentsUpdated={() => {
          // Refresh agents data after update
          queryClient.invalidateQueries({ queryKey: ['agents'] });
          setSelectedAgents([]);
          setSingleAgentUpdate(null);
        }}
      />
    </div>
  );
};

export default Agents;

