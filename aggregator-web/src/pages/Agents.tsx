import React, { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Computer,
  RefreshCw,
  Search,
  Filter,
  ChevronDown,
  ChevronRight as ChevronRightIcon,
  Activity,
  HardDrive,
  Cpu,
  Globe,
  MapPin,
  Calendar,
  Package,
} from 'lucide-react';
import { useAgents, useAgent, useScanAgent, useScanMultipleAgents } from '@/hooks/useAgents';
import { Agent } from '@/types';
import { getStatusColor, formatRelativeTime, isOnline, formatBytes } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';

const Agents: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [osFilter, setOsFilter] = useState<string>('all');
  const [showFilters, setShowFilters] = useState(false);
  const [selectedAgents, setSelectedAgents] = useState<string[]>([]);

  // Fetch agents list
  const { data: agentsData, isLoading, error } = useAgents({
    search: searchQuery || undefined,
    status: statusFilter !== 'all' ? statusFilter : undefined,
  });

  // Fetch single agent if ID is provided
  const { data: selectedAgentData } = useAgent(id || '', !!id);

  const scanAgentMutation = useScanAgent();
  const scanMultipleMutation = useScanMultipleAgents();

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
              <h1 className="text-2xl font-bold text-gray-900">
                {selectedAgent.hostname}
              </h1>
              <p className="mt-1 text-sm text-gray-600">
                Agent details and system information
              </p>
            </div>
            <button
              onClick={() => handleScanAgent(selectedAgent.id)}
              disabled={scanAgentMutation.isLoading}
              className="btn btn-primary"
            >
              {scanAgentMutation.isLoading ? (
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
            {/* Status card */}
            <div className="card">
              <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-medium text-gray-900">Status</h2>
                <span className={cn('badge', getStatusColor(selectedAgent.status))}>
                  {selectedAgent.status}
                </span>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <div className="flex items-center space-x-2 text-sm text-gray-600">
                    <Activity className="h-4 w-4" />
                    <span>Last Check-in:</span>
                  </div>
                  <p className="text-sm font-medium text-gray-900">
                    {formatRelativeTime(selectedAgent.last_checkin)}
                  </p>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center space-x-2 text-sm text-gray-600">
                    <Calendar className="h-4 w-4" />
                    <span>Last Scan:</span>
                  </div>
                  <p className="text-sm font-medium text-gray-900">
                    {selectedAgent.last_scan
                      ? formatRelativeTime(selectedAgent.last_scan)
                      : 'Never'}
                  </p>
                </div>
              </div>
            </div>

            {/* System info */}
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">System Information</h2>

              <div className="grid grid-cols-2 gap-6">
                <div className="space-y-4">
                  <div>
                    <p className="text-sm text-gray-600">Operating System</p>
                    <p className="text-sm font-medium text-gray-900">
                      {selectedAgent.os_type} {selectedAgent.os_version}
                    </p>
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">Architecture</p>
                    <p className="text-sm font-medium text-gray-900">
                      {selectedAgent.architecture}
                    </p>
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">IP Address</p>
                    <p className="text-sm font-medium text-gray-900">
                      {selectedAgent.ip_address}
                    </p>
                  </div>
                </div>

                <div className="space-y-4">
                  <div>
                    <p className="text-sm text-gray-600">Agent Version</p>
                    <p className="text-sm font-medium text-gray-900">
                      {selectedAgent.version}
                    </p>
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">Registered</p>
                    <p className="text-sm font-medium text-gray-900">
                      {formatRelativeTime(selectedAgent.created_at)}
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Quick actions */}
          <div className="space-y-6">
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">Quick Actions</h2>

              <div className="space-y-3">
                <button
                  onClick={() => handleScanAgent(selectedAgent.id)}
                  disabled={scanAgentMutation.isLoading}
                  className="w-full btn btn-primary"
                >
                  {scanAgentMutation.isLoading ? (
                    <RefreshCw className="animate-spin h-4 w-4 mr-2" />
                  ) : (
                    <RefreshCw className="h-4 w-4 mr-2" />
                  )}
                  Trigger Scan
                </button>

                <button
                  onClick={() => navigate(`/updates?agent=${selectedAgent.id}`)}
                  className="w-full btn btn-secondary"
                >
                  <Package className="h-4 w-4 mr-2" />
                  View Updates
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
              disabled={scanMultipleMutation.isLoading}
              className="btn btn-primary"
            >
              {scanMultipleMutation.isLoading ? (
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
      {isLoading ? (
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
                  <tr key={agent.id} className="hover:bg-gray-50">
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
                            {agent.ip_address}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="table-cell">
                      <span className={cn('badge', getStatusColor(agent.status))}>
                        {agent.status}
                      </span>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm text-gray-900">
                        {agent.os_type}
                      </div>
                      <div className="text-xs text-gray-500">
                        {agent.architecture}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm text-gray-900">
                        {formatRelativeTime(agent.last_checkin)}
                      </div>
                      <div className="text-xs text-gray-500">
                        {isOnline(agent.last_checkin) ? 'Online' : 'Offline'}
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
                          disabled={scanAgentMutation.isLoading}
                          className="text-gray-400 hover:text-primary-600"
                          title="Trigger scan"
                        >
                          <RefreshCw className="h-4 w-4" />
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