import React, { useState } from 'react';
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
} from 'lucide-react';
import { useAgents, useAgent, useScanAgent, useScanMultipleAgents, useUnregisterAgent } from '@/hooks/useAgents';
import { getStatusColor, formatRelativeTime, isOnline, formatBytes } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';
import { AgentSystemUpdates } from '@/components/AgentUpdates';

const Agents: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [osFilter, setOsFilter] = useState<string>('all');
  const [showFilters, setShowFilters] = useState(false);
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);

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
    search: searchQuery || undefined,
    status: statusFilter !== 'all' ? statusFilter : undefined,
  });

  // Fetch single agent if ID is provided
  const { data: selectedAgentData } = useAgent(id || '', !!id);

  const scanAgentMutation = useScanAgent();
  const scanMultipleMutation = useScanMultipleAgents();
  const unregisterAgentMutation = useUnregisterAgent();

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
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-gray-900">
                {selectedAgent.hostname}
              </h1>
              <p className="mt-2 text-sm text-gray-600">
                System details and update management for this agent
              </p>
            </div>
            <button
              onClick={() => handleScanAgent(selectedAgent.id)}
              disabled={scanAgentMutation.isPending}
              className="btn btn-primary"
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

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Agent info */}
          <div className="lg:col-span-2 space-y-6">
            {/* Agent Status Card */}
            <div className="card">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-medium text-gray-900">Agent Status</h2>
                <span className={cn('badge', getStatusColor(isOnline(selectedAgent.last_seen) ? 'online' : 'offline'))}>
                  {isOnline(selectedAgent.last_seen) ? 'Online' : 'Offline'}
                </span>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                {/* Agent Information */}
                <div className="space-y-4">
                  <div className="p-3 bg-gray-50 rounded-lg border border-gray-200">
                    <h3 className="text-sm font-medium text-gray-900 mb-3">Agent Information</h3>
                    <div className="space-y-3">
                      <div>
                        <p className="text-xs text-gray-500">Agent ID</p>
                        <p className="text-xs font-mono text-gray-700 break-all">
                          {selectedAgent.id}
                        </p>
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div>
                          <p className="text-xs text-gray-500">Version</p>
                          <p className="text-xs font-medium text-gray-900">
                            {selectedAgent.agent_version || selectedAgent.version || 'Unknown'}
                          </p>
                        </div>
                        <div>
                          <p className="text-xs text-gray-500">Registered</p>
                          <p className="text-xs font-medium text-gray-900">
                            {formatRelativeTime(selectedAgent.created_at)}
                          </p>
                        </div>
                      </div>
                      {(() => {
                        const meta = getSystemMetadata(selectedAgent);
                        if (meta.installationTime !== 'Unknown') {
                          return (
                            <div>
                              <p className="text-xs text-gray-500">Installation Time</p>
                              <p className="text-xs font-medium text-gray-900">
                                {formatRelativeTime(meta.installationTime)}
                              </p>
                            </div>
                          );
                        }
                        return null;
                      })()}
                    </div>
                  </div>
                </div>

                {/* Connection Status */}
                <div className="space-y-4">
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <div className="flex items-center space-x-2 text-sm text-gray-600">
                        <Activity className="h-4 w-4" />
                        <span>Last Check-in</span>
                      </div>
                      <p className="text-sm font-medium text-gray-900">
                        {formatRelativeTime(selectedAgent.last_seen)}
                      </p>
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-center space-x-2 text-sm text-gray-600">
                        <Calendar className="h-4 w-4" />
                        <span>Last Scan</span>
                      </div>
                      <p className="text-sm font-medium text-gray-900">
                        {selectedAgent.last_scan
                          ? formatRelativeTime(selectedAgent.last_scan)
                          : 'Never'}
                      </p>
                    </div>
                  </div>
                </div>
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
            {searchQuery || statusFilter !== 'all' || osFilter !== 'all'
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