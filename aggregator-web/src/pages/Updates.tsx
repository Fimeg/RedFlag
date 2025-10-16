import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import {
  Package,
  CheckCircle,
  XCircle,
  Search,
  Filter,
  Computer,
  ExternalLink,
  ChevronLeft,
  ChevronRight,
  AlertTriangle,
  Clock,
  Calendar,
} from 'lucide-react';
import { useUpdates, useUpdate, useApproveUpdate, useRejectUpdate, useInstallUpdate, useApproveMultipleUpdates } from '@/hooks/useUpdates';
import type { UpdatePackage } from '@/types';
import { getSeverityColor, getStatusColor, getPackageTypeIcon, formatBytes, formatRelativeTime } from '@/lib/utils';
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
  const [currentPage, setCurrentPage] = useState(parseInt(searchParams.get('page') || '1'));
  const [pageSize, setPageSize] = useState(100);

  // Store filters in URL
  useEffect(() => {
    const params = new URLSearchParams();
    if (searchQuery) params.set('search', searchQuery);
    if (statusFilter) params.set('status', statusFilter);
    if (severityFilter) params.set('severity', severityFilter);
    if (typeFilter) params.set('type', typeFilter);
    if (agentFilter) params.set('agent', agentFilter);
    if (currentPage > 1) params.set('page', currentPage.toString());
    if (pageSize !== 100) params.set('page_size', pageSize.toString());

    const newUrl = `${window.location.pathname}${params.toString() ? '?' + params.toString() : ''}`;
    if (newUrl !== window.location.href) {
      window.history.replaceState({}, '', newUrl);
    }
  }, [searchQuery, statusFilter, severityFilter, typeFilter, agentFilter, currentPage, pageSize]);

  // Fetch updates list
  const { data: updatesData, isPending, error } = useUpdates({
    search: searchQuery || undefined,
    status: statusFilter || undefined,
    severity: severityFilter || undefined,
    type: typeFilter || undefined,
    agent: agentFilter || undefined,
    page: currentPage,
    page_size: pageSize,
  });

  // Fetch single update if ID is provided
  const { data: selectedUpdateData } = useUpdate(id || '', !!id);

  const approveMutation = useApproveUpdate();
  const rejectMutation = useRejectUpdate();
  const installMutation = useInstallUpdate();
  const bulkApproveMutation = useApproveMultipleUpdates();

  const updates = updatesData?.updates || [];
  const totalCount = updatesData?.total || 0;
  const selectedUpdate = selectedUpdateData || updates.find((u: UpdatePackage) => u.id === id);

  // Pagination calculations
  const totalPages = Math.ceil(totalCount / pageSize);
  const hasNextPage = currentPage < totalPages;
  const hasPrevPage = currentPage > 1;

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
      setSelectedUpdates(updates.map((update: UpdatePackage) => update.id));
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
  const statuses = [...new Set(updates.map((u: UpdatePackage) => u.status))];
  const severities = [...new Set(updates.map((u: UpdatePackage) => u.severity))];
  const types = [...new Set(updates.map((u: UpdatePackage) => u.package_type))];
  const agents = [...new Set(updates.map((u: UpdatePackage) => u.agent_id))];

  // Quick filter functions
  const handleQuickFilter = (filter: string) => {
    switch (filter) {
      case 'critical':
        setSeverityFilter('critical');
        setStatusFilter('pending');
        break;
      case 'pending':
        setStatusFilter('pending');
        setSeverityFilter('');
        break;
      case 'approved':
        setStatusFilter('approved');
        setSeverityFilter('');
        break;
      default:
        // Clear all filters
        setStatusFilter('');
        setSeverityFilter('');
        setTypeFilter('');
        setAgentFilter('');
        break;
    }
    setCurrentPage(1);
  };

  // Group updates
  const groupUpdates = (updates: UpdatePackage[], groupBy: string) => {
    const groups: Record<string, UpdatePackage[]> = {};

    updates.forEach(update => {
      let key: string;
      switch (groupBy) {
        case 'severity':
          key = update.severity;
          break;
        case 'type':
          key = update.package_type;
          break;
        case 'status':
          key = update.status;
          break;
        default:
          key = 'all';
      }

      if (!groups[key]) {
        groups[key] = [];
      }
      groups[key].push(update);
    });

    return groups;
  };

  // Get total statistics from API (not just current page)
  const totalStats = {
    total: totalCount,
    pending: updatesData?.stats?.pending_updates || 0,
    approved: updatesData?.stats?.approved_updates || 0,
    critical: updatesData?.stats?.critical_updates || 0,
    high: updatesData?.stats?.high_updates || 0,
  };

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
                      disabled={approveMutation.isPending}
                      className="w-full btn btn-success"
                    >
                      <CheckCircle className="h-4 w-4 mr-2" />
                      Approve Update
                    </button>

                    <button
                      onClick={() => handleRejectUpdate(selectedUpdate.id)}
                      disabled={rejectMutation.isPending}
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
                    disabled={installMutation.isPending}
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

  // Pagination handlers
  const handlePageChange = (page: number) => {
    setCurrentPage(page);
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    setCurrentPage(1); // Reset to first page when changing page size
  };

  // Updates list view
  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold text-gray-900">Updates</h1>
            <p className="mt-1 text-sm text-gray-600">
              Review and approve available updates for your agents
            </p>
          </div>
          <div className="text-right">
            <div className="text-sm text-gray-600">
              Showing {updates.length} of {totalCount} updates
            </div>
            {totalCount > 100 && (
              <select
                value={pageSize}
                onChange={(e) => handlePageSizeChange(Number(e.target.value))}
                className="mt-1 text-sm border border-gray-300 rounded px-3 py-1"
              >
                <option value={50}>50 per page</option>
                <option value={100}>100 per page</option>
                <option value={200}>200 per page</option>
                <option value={500}>500 per page</option>
              </select>
            )}
          </div>
        </div>

        {/* Statistics Cards - Show total counts across all updates */}
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4 mb-6">
          <div className="bg-white p-4 rounded-lg border border-gray-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Total Updates</p>
                <p className="text-2xl font-bold text-gray-900">{totalStats.total}</p>
              </div>
              <Package className="h-8 w-8 text-gray-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-orange-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Pending</p>
                <p className="text-2xl font-bold text-orange-600">{totalStats.pending}</p>
              </div>
              <Clock className="h-8 w-8 text-orange-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-green-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Approved</p>
                <p className="text-2xl font-bold text-green-600">{totalStats.approved}</p>
              </div>
              <CheckCircle className="h-8 w-8 text-green-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-red-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Critical</p>
                <p className="text-2xl font-bold text-red-600">{totalStats.critical}</p>
              </div>
              <AlertTriangle className="h-8 w-8 text-red-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-yellow-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">High Priority</p>
                <p className="text-2xl font-bold text-yellow-600">{totalStats.high}</p>
              </div>
              <AlertTriangle className="h-8 w-8 text-yellow-400" />
            </div>
          </div>
        </div>

        {/* Quick Filters */}
        <div className="flex flex-wrap gap-2 mb-4">
          <button
            onClick={() => handleQuickFilter('all')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              !statusFilter && !severityFilter && !typeFilter && !agentFilter
                ? "bg-primary-100 border-primary-300 text-primary-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            All Updates
          </button>
          <button
            onClick={() => handleQuickFilter('critical')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'pending' && severityFilter === 'critical'
                ? "bg-red-100 border-red-300 text-red-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <AlertTriangle className="h-4 w-4 mr-1 inline" />
            Critical
          </button>
          <button
            onClick={() => handleQuickFilter('pending')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'pending' && !severityFilter
                ? "bg-orange-100 border-orange-300 text-orange-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <Clock className="h-4 w-4 mr-1 inline" />
            Pending Approval
          </button>
          <button
            onClick={() => handleQuickFilter('approved')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'approved' && !severityFilter
                ? "bg-green-100 border-green-300 text-green-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <CheckCircle className="h-4 w-4 mr-1 inline" />
            Approved
          </button>
        </div>
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
              disabled={bulkApproveMutation.isPending}
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
                  {statuses.map((status: string) => (
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
                  {severities.map((severity: string) => (
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
                  {types.map((type: string) => (
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
                  {agents.map((agentId: string) => (
                    <option key={agentId} value={agentId}>{agentId}</option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Updates table */}
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
                {updates.map((update: UpdatePackage) => (
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
                              disabled={approveMutation.isPending}
                              className="text-success-600 hover:text-success-800"
                              title="Approve"
                            >
                              <CheckCircle className="h-4 w-4" />
                            </button>
                            <button
                              onClick={() => handleRejectUpdate(update.id)}
                              disabled={rejectMutation.isPending}
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
                            disabled={installMutation.isPending}
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

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="bg-white px-4 py-3 border-t border-gray-200 sm:px-6">
              <div className="flex items-center justify-between">
                <div className="flex-1 flex justify-between sm:hidden">
                  <button
                    onClick={() => handlePageChange(currentPage - 1)}
                    disabled={!hasPrevPage}
                    className="relative inline-flex items-center px-4 py-2 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Previous
                  </button>
                  <button
                    onClick={() => handlePageChange(currentPage + 1)}
                    disabled={!hasNextPage}
                    className="ml-3 relative inline-flex items-center px-4 py-2 border border-gray-300 text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Next
                  </button>
                </div>
                <div className="hidden sm:flex-1 sm:flex sm:items-center sm:justify-between">
                  <div>
                    <p className="text-sm text-gray-700">
                      Showing <span className="font-medium">{(currentPage - 1) * pageSize + 1}</span> to{' '}
                      <span className="font-medium">{Math.min(currentPage * pageSize, totalCount)}</span> of{' '}
                      <span className="font-medium">{totalCount}</span> results
                    </p>
                  </div>
                  <div>
                    <nav className="relative z-0 inline-flex rounded-md shadow-sm -space-x-px" aria-label="Pagination">
                      <button
                        onClick={() => handlePageChange(currentPage - 1)}
                        disabled={!hasPrevPage}
                        className="relative inline-flex items-center px-2 py-2 rounded-l-md border border-gray-300 bg-white text-sm font-medium text-gray-500 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <span className="sr-only">Previous</span>
                        <ChevronLeft className="h-5 w-5" />
                      </button>

                      {/* Page numbers */}
                      {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
                        let pageNum;
                        if (totalPages <= 5) {
                          pageNum = i + 1;
                        } else if (currentPage <= 3) {
                          pageNum = i + 1;
                        } else if (currentPage >= totalPages - 2) {
                          pageNum = totalPages - 4 + i;
                        } else {
                          pageNum = currentPage - 2 + i;
                        }

                        return (
                          <button
                            key={pageNum}
                            onClick={() => handlePageChange(pageNum)}
                            className={`relative inline-flex items-center px-4 py-2 border text-sm font-medium ${
                              currentPage === pageNum
                                ? 'z-10 bg-primary-50 border-primary-500 text-primary-600'
                                : 'bg-white border-gray-300 text-gray-500 hover:bg-gray-50'
                            }`}
                          >
                            {pageNum}
                          </button>
                        );
                      })}

                      <button
                        onClick={() => handlePageChange(currentPage + 1)}
                        disabled={!hasNextPage}
                        className="relative inline-flex items-center px-2 py-2 rounded-r-md border border-gray-300 bg-white text-sm font-medium text-gray-500 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        <span className="sr-only">Next</span>
                        <ChevronRight className="h-5 w-5" />
                      </button>
                    </nav>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default Updates;