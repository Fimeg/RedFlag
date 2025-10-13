import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import {
  Package,
  CheckCircle,
  XCircle,
  Clock,
  AlertTriangle,
  Search,
  Filter,
  ChevronDown as ChevronDownIcon,
  RefreshCw,
  Calendar,
  Computer,
  ExternalLink,
} from 'lucide-react';
import { useUpdates, useUpdate, useApproveUpdate, useRejectUpdate, useInstallUpdate, useApproveMultipleUpdates } from '@/hooks/useUpdates';
import { UpdatePackage } from '@/types';
import { getSeverityColor, getStatusColor, getPackageTypeIcon, formatBytes, formatRelativeTime } from '@/lib/utils';
import { useUpdateStore } from '@/lib/store';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';

const Updates: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  // Get filters from URL params
  const [searchQuery, setSearchQuery] = useState(searchParams.get('search') || '');
  const [statusFilter, setStatusFilter] = useState(searchParams.get('status') || '');
  const [severityFilter, setSeverityFilter] = useState(searchParams.get('severity') || '');
  const [typeFilter, setTypeFilter] = useState(searchParams.get('type') || '');
  const [agentFilter, setAgentFilter] = useState(searchParams.get('agent') || '');
  const [showFilters, setShowFilters] = useState(false);
  const [selectedUpdates, setSelectedUpdates] = useState<string[]>([]);

  // Store filters in URL
  useEffect(() => {
    const params = new URLSearchParams();
    if (searchQuery) params.set('search', searchQuery);
    if (statusFilter) params.set('status', statusFilter);
    if (severityFilter) params.set('severity', severityFilter);
    if (typeFilter) params.set('type', typeFilter);
    if (agentFilter) params.set('agent', agentFilter);

    const newUrl = `${window.location.pathname}${params.toString() ? '?' + params.toString() : ''}`;
    if (newUrl !== window.location.href) {
      window.history.replaceState({}, '', newUrl);
    }
  }, [searchQuery, statusFilter, severityFilter, typeFilter, agentFilter]);

  // Fetch updates list
  const { data: updatesData, isLoading, error } = useUpdates({
    search: searchQuery || undefined,
    status: statusFilter || undefined,
    severity: severityFilter || undefined,
    type: typeFilter || undefined,
    agent_id: agentFilter || undefined,
  });

  // Fetch single update if ID is provided
  const { data: selectedUpdateData } = useUpdate(id || '', !!id);

  const approveMutation = useApproveUpdate();
  const rejectMutation = useRejectUpdate();
  const installMutation = useInstallUpdate();
  const bulkApproveMutation = useApproveMultipleUpdates();

  const updates = updatesData?.updates || [];
  const selectedUpdate = selectedUpdateData || updates.find(u => u.id === id);

  // Handle update selection
  const handleSelectUpdate = (updateId: string, checked: boolean) => {
    if (checked) {
      setSelectedUpdates([...selectedUpdates, updateId]);
    } else {
      setSelectedUpdates(selectedUpdates.filter(id => id !== updateId));
    }
  };

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedUpdates(updates.map(update => update.id));
    } else {
      setSelectedUpdates([]);
    }
  };

  // Handle update actions
  const handleApproveUpdate = async (updateId: string) => {
    try {
      await approveMutation.mutateAsync({ id: updateId });
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleRejectUpdate = async (updateId: string) => {
    try {
      await rejectMutation.mutateAsync(updateId);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleInstallUpdate = async (updateId: string) => {
    try {
      await installMutation.mutateAsync(updateId);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleBulkApprove = async () => {
    if (selectedUpdates.length === 0) {
      toast.error('Please select at least one update');
      return;
    }

    try {
      await bulkApproveMutation.mutateAsync({ update_ids: selectedUpdates });
      setSelectedUpdates([]);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  // Get unique values for filters
  const statuses = [...new Set(updates.map(u => u.status))];
  const severities = [...new Set(updates.map(u => u.severity))];
  const types = [...new Set(updates.map(u => u.package_type))];
  const agents = [...new Set(updates.map(u => u.agent_id))];

  // Update detail view
  if (id && selectedUpdate) {
    return (
      <div className="px-4 sm:px-6 lg:px-8">
        <div className="mb-6">
          <button
            onClick={() => navigate('/updates')}
            className="text-sm text-gray-500 hover:text-gray-700 mb-4"
          >
            ← Back to Updates
          </button>
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center space-x-3 mb-2">
                <span className="text-2xl">{getPackageTypeIcon(selectedUpdate.package_type)}</span>
                <h1 className="text-2xl font-bold text-gray-900">
                  {selectedUpdate.package_name}
                </h1>
                <span className={cn('badge', getSeverityColor(selectedUpdate.severity))}>
                  {selectedUpdate.severity}
                </span>
                <span className={cn('badge', getStatusColor(selectedUpdate.status))}>
                  {selectedUpdate.status}
                </span>
              </div>
              <p className="text-sm text-gray-600">
                Update details and available actions
              </p>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Update info */}
          <div className="lg:col-span-2 space-y-6">
            {/* Version info */}
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">Version Information</h2>

              <div className="grid grid-cols-2 gap-6">
                <div>
                  <p className="text-sm text-gray-600">Current Version</p>
                  <p className="text-sm font-medium text-gray-900">
                    {selectedUpdate.current_version}
                  </p>
                </div>
                <div>
                  <p className="text-sm text-gray-600">Available Version</p>
                  <p className="text-sm font-medium text-gray-900">
                    {selectedUpdate.available_version}
                  </p>
                </div>
              </div>
            </div>

            {/* Metadata */}
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">Additional Information</h2>

              <div className="grid grid-cols-2 gap-6">
                <div className="space-y-4">
                  <div>
                    <p className="text-sm text-gray-600">Package Type</p>
                    <p className="text-sm font-medium text-gray-900">
                      {selectedUpdate.package_type.toUpperCase()}
                    </p>
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">Severity</p>
                    <span className={cn('badge', getSeverityColor(selectedUpdate.severity))}>
                      {selectedUpdate.severity}
                    </span>
                  </div>
                </div>

                <div className="space-y-4">
                  <div>
                    <p className="text-sm text-gray-600">Discovered</p>
                    <p className="text-sm font-medium text-gray-900">
                      {formatRelativeTime(selectedUpdate.created_at)}
                    </p>
                  </div>

                  <div>
                    <p className="text-sm text-gray-600">Last Updated</p>
                    <p className="text-sm font-medium text-gray-900">
                      {formatRelativeTime(selectedUpdate.updated_at)}
                    </p>
                  </div>
                </div>
              </div>

              {selectedUpdate.metadata && Object.keys(selectedUpdate.metadata).length > 0 && (
                <div className="mt-6">
                  <p className="text-sm text-gray-600 mb-2">Metadata</p>
                  <pre className="bg-gray-50 p-3 rounded-md text-xs overflow-x-auto">
                    {JSON.stringify(selectedUpdate.metadata, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          </div>

          {/* Actions */}
          <div className="space-y-6">
            <div className="card">
              <h2 className="text-lg font-medium text-gray-900 mb-4">Actions</h2>

              <div className="space-y-3">
                {selectedUpdate.status === 'pending' && (
                  <>
                    <button
                      onClick={() => handleApproveUpdate(selectedUpdate.id)}
                      disabled={approveMutation.isLoading}
                      className="w-full btn btn-success"
                    >
                      <CheckCircle className="h-4 w-4 mr-2" />
                      Approve Update
                    </button>

                    <button
                      onClick={() => handleRejectUpdate(selectedUpdate.id)}
                      disabled={rejectMutation.isLoading}
                      className="w-full btn btn-secondary"
                    >
                      <XCircle className="h-4 w-4 mr-2" />
                      Reject Update
                    </button>
                  </>
                )}

                {selectedUpdate.status === 'approved' && (
                  <button
                    onClick={() => handleInstallUpdate(selectedUpdate.id)}
                    disabled={installMutation.isLoading}
                    className="w-full btn btn-primary"
                  >
                    <Package className="h-4 w-4 mr-2" />
                    Install Now
                  </button>
                )}

                <button
                  onClick={() => navigate(`/agents/${selectedUpdate.agent_id}`)}
                  className="w-full btn btn-ghost"
                >
                  <Computer className="h-4 w-4 mr-2" />
                  View Agent
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  // Updates list view
  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Updates</h1>
        <p className="mt-1 text-sm text-gray-600">
          Review and approve available updates for your agents
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
                placeholder="Search updates by package name..."
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
            {[statusFilter, severityFilter, typeFilter, agentFilter].filter(Boolean).length > 0 && (
              <span className="bg-primary-100 text-primary-800 px-2 py-0.5 rounded-full text-xs">
                {[statusFilter, severityFilter, typeFilter, agentFilter].filter(Boolean).length}
              </span>
            )}
          </button>

          {/* Bulk actions */}
          {selectedUpdates.length > 0 && (
            <button
              onClick={handleBulkApprove}
              disabled={bulkApproveMutation.isLoading}
              className="btn btn-success"
            >
              <CheckCircle className="h-4 w-4 mr-2" />
              Approve Selected ({selectedUpdates.length})
            </button>
          )}
        </div>

        {/* Filters */}
        {showFilters && (
          <div className="bg-white p-4 rounded-lg border border-gray-200">
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Status
                </label>
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="">All Status</option>
                  {statuses.map(status => (
                    <option key={status} value={status}>{status}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Severity
                </label>
                <select
                  value={severityFilter}
                  onChange={(e) => setSeverityFilter(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="">All Severities</option>
                  {severities.map(severity => (
                    <option key={severity} value={severity}>{severity}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Package Type
                </label>
                <select
                  value={typeFilter}
                  onChange={(e) => setTypeFilter(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="">All Types</option>
                  {types.map(type => (
                    <option key={type} value={type}>{type.toUpperCase()}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Agent
                </label>
                <select
                  value={agentFilter}
                  onChange={(e) => setAgentFilter(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
                >
                  <option value="">All Agents</option>
                  {agents.map(agentId => (
                    <option key={agentId} value={agentId}>{agentId}</option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Updates table */}
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
          <div className="text-red-500 mb-2">Failed to load updates</div>
          <p className="text-sm text-gray-600">Please check your connection and try again.</p>
        </div>
      ) : updates.length === 0 ? (
        <div className="text-center py-12">
          <Package className="mx-auto h-12 w-12 text-gray-400" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">No updates found</h3>
          <p className="mt-1 text-sm text-gray-500">
            {searchQuery || statusFilter || severityFilter || typeFilter || agentFilter
              ? 'Try adjusting your search or filters.'
              : 'All agents are up to date!'}
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
                      checked={selectedUpdates.length === updates.length}
                      onChange={(e) => handleSelectAll(e.target.checked)}
                      className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    />
                  </th>
                  <th className="table-header">Package</th>
                  <th className="table-header">Type</th>
                  <th className="table-header">Versions</th>
                  <th className="table-header">Severity</th>
                  <th className="table-header">Status</th>
                  <th className="table-header">Agent</th>
                  <th className="table-header">Discovered</th>
                  <th className="table-header">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {updates.map((update) => (
                  <tr key={update.id} className="hover:bg-gray-50">
                    <td className="table-cell">
                      <input
                        type="checkbox"
                        checked={selectedUpdates.includes(update.id)}
                        onChange={(e) => handleSelectUpdate(update.id, e.target.checked)}
                        className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      />
                    </td>
                    <td className="table-cell">
                      <div className="flex items-center space-x-3">
                        <span className="text-xl">{getPackageTypeIcon(update.package_type)}</span>
                        <div>
                          <div className="text-sm font-medium text-gray-900">
                            <button
                              onClick={() => navigate(`/updates/${update.id}`)}
                              className="hover:text-primary-600"
                            >
                              {update.package_name}
                            </button>
                          </div>
                          {update.metadata?.size_bytes && (
                            <div className="text-xs text-gray-500">
                              {formatBytes(update.metadata.size_bytes)}
                            </div>
                          )}
                        </div>
                      </div>
                    </td>
                    <td className="table-cell">
                      <span className="text-xs font-medium text-gray-900 bg-gray-100 px-2 py-1 rounded">
                        {update.package_type.toUpperCase()}
                      </span>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm">
                        <div className="text-gray-900">{update.current_version}</div>
                        <div className="text-success-600">→ {update.available_version}</div>
                      </div>
                    </td>
                    <td className="table-cell">
                      <span className={cn('badge', getSeverityColor(update.severity))}>
                        {update.severity}
                      </span>
                    </td>
                    <td className="table-cell">
                      <span className={cn('badge', getStatusColor(update.status))}>
                        {update.status}
                      </span>
                    </td>
                    <td className="table-cell">
                      <button
                        onClick={() => navigate(`/agents/${update.agent_id}`)}
                        className="text-sm text-gray-900 hover:text-primary-600"
                        title="View agent"
                      >
                        {update.agent_id.substring(0, 8)}...
                      </button>
                    </td>
                    <td className="table-cell">
                      <div className="text-sm text-gray-900">
                        {formatRelativeTime(update.created_at)}
                      </div>
                    </td>
                    <td className="table-cell">
                      <div className="flex items-center space-x-2">
                        {update.status === 'pending' && (
                          <>
                            <button
                              onClick={() => handleApproveUpdate(update.id)}
                              disabled={approveMutation.isLoading}
                              className="text-success-600 hover:text-success-800"
                              title="Approve"
                            >
                              <CheckCircle className="h-4 w-4" />
                            </button>
                            <button
                              onClick={() => handleRejectUpdate(update.id)}
                              disabled={rejectMutation.isLoading}
                              className="text-gray-600 hover:text-gray-800"
                              title="Reject"
                            >
                              <XCircle className="h-4 w-4" />
                            </button>
                          </>
                        )}

                        {update.status === 'approved' && (
                          <button
                            onClick={() => handleInstallUpdate(update.id)}
                            disabled={installMutation.isLoading}
                            className="text-primary-600 hover:text-primary-800"
                            title="Install"
                          >
                            <Package className="h-4 w-4" />
                          </button>
                        )}

                        <button
                          onClick={() => navigate(`/updates/${update.id}`)}
                          className="text-gray-400 hover:text-primary-600"
                          title="View details"
                        >
                          <ExternalLink className="h-4 w-4" />
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

export default Updates;