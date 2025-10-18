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
  X,
  Loader2,
  RotateCcw,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
} from 'lucide-react';
import { useUpdates, useUpdate, useApproveUpdate, useRejectUpdate, useInstallUpdate, useApproveMultipleUpdates, useRetryCommand, useCancelCommand } from '@/hooks/useUpdates';
import { useRecentCommands } from '@/hooks/useCommands';
import type { UpdatePackage } from '@/types';
import { getSeverityColor, getStatusColor, getPackageTypeIcon, formatBytes, formatRelativeTime } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';
import { updateApi } from '@/lib/api';


const Updates: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  // Get filters from URL params
  const [searchQuery, setSearchQuery] = useState(searchParams.get('search') || '');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState(searchParams.get('search') || '');
  const [statusFilter, setStatusFilter] = useState(searchParams.get('status') || '');
  const [severityFilter, setSeverityFilter] = useState(searchParams.get('severity') || '');
  const [typeFilter, setTypeFilter] = useState(searchParams.get('type') || '');
  const [agentFilter, setAgentFilter] = useState(searchParams.get('agent') || '');
  const [sortBy, setSortBy] = useState(searchParams.get('sort_by') || '');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(searchParams.get('sort_order') as 'asc' | 'desc' || 'desc');

  // Debounce search query to avoid API calls on every keystroke
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery);
    }, 300); // 300ms delay

    return () => {
      clearTimeout(timer);
    };
  }, [searchQuery]);
  const [showFilters, setShowFilters] = useState(false);
  const [selectedUpdates, setSelectedUpdates] = useState<string[]>([]);
  const [currentPage, setCurrentPage] = useState(parseInt(searchParams.get('page') || '1'));
  const [pageSize, setPageSize] = useState(100);
  const [showLogModal, setShowLogModal] = useState(false);
  const [logs, setLogs] = useState<any[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [showDependencyModal, setShowDependencyModal] = useState(false);
  const [pendingDependencies, setPendingDependencies] = useState<string[]>([]);
  const [dependencyUpdateId, setDependencyUpdateId] = useState<string | null>(null);
  const [dependencyLoading, setDependencyLoading] = useState(false);
  const [activeTab, setActiveTab] = useState<'updates' | 'commands'>('updates');

  // Store filters in URL
  useEffect(() => {
    const params = new URLSearchParams();
    if (debouncedSearchQuery) params.set('search', debouncedSearchQuery);
    if (statusFilter) params.set('status', statusFilter);
    if (severityFilter) params.set('severity', severityFilter);
    if (typeFilter) params.set('type', typeFilter);
    if (agentFilter) params.set('agent', agentFilter);
    if (sortBy) params.set('sort_by', sortBy);
    if (sortOrder) params.set('sort_order', sortOrder);
    if (currentPage > 1) params.set('page', currentPage.toString());
    if (pageSize !== 100) params.set('page_size', pageSize.toString());

    const newUrl = `${window.location.pathname}${params.toString() ? '?' + params.toString() : ''}`;
    if (newUrl !== window.location.href) {
      window.history.replaceState({}, '', newUrl);
    }
  }, [debouncedSearchQuery, statusFilter, severityFilter, typeFilter, agentFilter, sortBy, sortOrder, currentPage, pageSize]);

  // Fetch updates list
  const { data: updatesData, isPending, error } = useUpdates({
    search: debouncedSearchQuery || undefined,
    status: statusFilter || undefined,
    severity: severityFilter || undefined,
    type: typeFilter || undefined,
    agent: agentFilter || undefined,
    sort_by: sortBy || undefined,
    sort_order: sortOrder || undefined,
    page: currentPage,
    page_size: pageSize,
  });

  // Fetch single update if ID is provided
  const { data: selectedUpdateData } = useUpdate(id || '', !!id);

  // Fetch recent commands for retry functionality
  const { data: recentCommandsData } = useRecentCommands(50);

  const approveMutation = useApproveUpdate();
  const rejectMutation = useRejectUpdate();
  const installMutation = useInstallUpdate();
  const bulkApproveMutation = useApproveMultipleUpdates();
  const retryMutation = useRetryCommand();
  const cancelMutation = useCancelCommand();

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

  // Handle retry command
  const handleRetryCommand = async (commandId: string) => {
    try {
      await retryMutation.mutateAsync(commandId);
      toast.success('Command retry initiated successfully');
    } catch (error: any) {
      toast.error(`Failed to retry command: ${error.message || 'Unknown error'}`);
    }
  };

  // Handle cancel command
  const handleCancelCommand = async (commandId: string) => {
    try {
      await cancelMutation.mutateAsync(commandId);
      toast.success('Command cancelled successfully');
    } catch (error: any) {
      toast.error(`Failed to cancel command: ${error.message || 'Unknown error'}`);
    }
  };

  // Handle dependency confirmation
  const handleConfirmDependencies = async (updateId: string) => {
    setDependencyLoading(true);
    try {
      const response = await fetch(`/api/v1/updates/${updateId}/confirm-dependencies`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error('Failed to confirm dependencies');
      }

      toast.success('Dependency installation confirmed');
      setShowDependencyModal(false);
      setPendingDependencies([]);
      setDependencyUpdateId(null);

      // Refresh the update data
      window.location.reload();
    } catch (error) {
      toast.error('Failed to confirm dependencies');
      console.error('Failed to confirm dependencies:', error);
    } finally {
      setDependencyLoading(false);
    }
  };

  // Handle dependency cancellation
  const handleCancelDependencies = async () => {
    setShowDependencyModal(false);
    setPendingDependencies([]);
    setDependencyUpdateId(null);
    toast('Dependency installation cancelled');
  };

  // Handle viewing logs
  const handleViewLogs = async (updateId: string) => {
    setLogsLoading(true);
    try {
      const result = await updateApi.getUpdateLogs(updateId, 50);
      setLogs(result.logs || []);
      setShowLogModal(true);
    } catch (error) {
      toast.error('Failed to load installation logs');
      console.error('Failed to load logs:', error);
    } finally {
      setLogsLoading(false);
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
      case 'installing':
        setStatusFilter('installing');
        setSeverityFilter('');
        break;
      case 'installed':
        setStatusFilter('installed');
        setSeverityFilter('');
        break;
      case 'failed':
        setStatusFilter('failed');
        setSeverityFilter('');
        break;
      case 'dependencies':
        setStatusFilter('pending_dependencies');
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

  // Handle column sorting
  const handleSort = (column: string) => {
    if (sortBy === column) {
      // Toggle sort order if clicking the same column
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      // Set new column with default desc order
      setSortBy(column);
      setSortOrder('desc');
    }
    setCurrentPage(1);
  };

  // Render sort icon for column headers
  const renderSortIcon = (column: string) => {
    if (sortBy !== column) {
      return <ArrowUpDown className="h-4 w-4 ml-1 text-gray-400" />;
    }
    return sortOrder === 'asc' ? (
      <ArrowUp className="h-4 w-4 ml-1 text-primary-600" />
    ) : (
      <ArrowDown className="h-4 w-4 ml-1 text-primary-600" />
    );
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
                  {selectedUpdate.status === 'checking_dependencies' ? (
                    <div className="flex items-center space-x-1">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      <span>Checking dependencies...</span>
                    </div>
                  ) : (
                    selectedUpdate.status
                  )}
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

                {selectedUpdate.status === 'checking_dependencies' && (
                  <div className="w-full btn btn-secondary opacity-75 cursor-not-allowed">
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    Checking Dependencies...
                  </div>
                )}

                {selectedUpdate.status === 'pending_dependencies' && (
                  <button
                    onClick={() => {
                      // Extract dependencies from metadata
                      const deps = selectedUpdate.metadata?.dependencies || [];
                      setPendingDependencies(Array.isArray(deps) ? deps : []);
                      setDependencyUpdateId(selectedUpdate.id);
                      setShowDependencyModal(true);
                    }}
                    className="w-full btn btn-warning"
                  >
                    <AlertTriangle className="h-4 w-4 mr-2" />
                    Review Dependencies
                  </button>
                )}

                {['installing', 'completed', 'failed'].includes(selectedUpdate.status) && (
                  <button
                    onClick={() => handleViewLogs(selectedUpdate.id)}
                    disabled={logsLoading}
                    className="w-full btn btn-ghost"
                  >
                    <Package className="h-4 w-4 mr-2" />
                    {logsLoading ? 'Loading...' : 'View Log'}
                  </button>
                )}

                {selectedUpdate.status === 'failed' && (
                  <button
                    onClick={() => {
                      // This would need a way to find the associated command ID
                      // For now, we'll show a message indicating this needs to be implemented
                      toast.info('Retry functionality will be available in the command history view');
                    }}
                    className="w-full btn btn-warning"
                  >
                    <RotateCcw className="h-4 w-4 mr-2" />
                    Retry Update
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

        {/* Dependency Confirmation Modal */}
        {showDependencyModal && (
          <div className="fixed inset-0 z-50 overflow-y-auto">
            <div className="flex min-h-full items-end justify-center p-4 text-center sm:p-0">
              <div className="relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-2xl border border-gray-200">
                {/* Header */}
                <div className="bg-white border-b border-gray-200 px-6 py-4 flex items-center justify-between rounded-t-lg">
                  <h3 className="text-lg font-semibold text-gray-900">
                    Dependencies Required
                  </h3>
                  <button
                    type="button"
                    className="text-gray-400 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-primary-500 rounded-md p-1"
                    onClick={handleCancelDependencies}
                  >
                    <span className="sr-only">Close</span>
                    <X className="h-5 w-5" />
                  </button>
                </div>

                {/* Content */}
                <div className="bg-white px-6 py-4">
                  <div className="space-y-4">
                    <div className="flex items-start space-x-3">
                      <div className="flex-shrink-0">
                        <AlertTriangle className="h-6 w-6 text-amber-500" />
                      </div>
                      <div className="flex-1">
                        <h4 className="text-base font-medium text-gray-900">
                          Additional packages are required
                        </h4>
                        <p className="mt-1 text-sm text-gray-600">
                          To install <span className="font-medium text-gray-900">{selectedUpdate?.package_name}</span>, the following additional packages will also be installed:
                        </p>
                      </div>
                    </div>

                    {/* Dependencies List */}
                    {pendingDependencies.length > 0 && (
                      <div className="bg-gray-50 rounded-lg p-4">
                        <h5 className="text-sm font-medium text-gray-700 mb-3">Required Dependencies:</h5>
                        <ul className="space-y-2">
                          {pendingDependencies.map((dep, index) => (
                            <li key={index} className="flex items-center space-x-2 text-sm">
                              <Package className="h-4 w-4 text-gray-400" />
                              <span className="font-medium text-gray-700">{dep}</span>
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}

                    {/* Warning Message */}
                    <div className="bg-amber-50 border border-amber-200 rounded-md p-3">
                      <div className="flex">
                        <AlertTriangle className="h-4 w-4 text-amber-500 mr-2 flex-shrink-0" />
                        <div className="text-sm text-amber-800">
                          <p className="font-medium">Please review the dependencies before proceeding.</p>
                          <p className="mt-1">These additional packages will be installed alongside your requested package.</p>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                {/* Footer */}
                <div className="bg-gray-50 px-6 py-4 sm:flex sm:flex-row-reverse rounded-b-lg border-t border-gray-200">
                  <button
                    type="button"
                    className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-primary-600 text-base font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm"
                    onClick={() => handleConfirmDependencies(dependencyUpdateId!)}
                    disabled={dependencyLoading}
                  >
                    {dependencyLoading ? (
                      <>
                        <Loader2 className="h-4 w-4 animate-spin mr-2" />
                        Approving & Installing...
                      </>
                    ) : (
                      <>
                        <CheckCircle className="h-4 w-4 mr-2" />
                        Approve & Install All
                      </>
                    )}
                  </button>
                  <button
                    type="button"
                    className="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
                    onClick={handleCancelDependencies}
                    disabled={dependencyLoading}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Log Modal */}
        {showLogModal && (
          <div className="fixed inset-0 z-50 overflow-y-auto">
            <div className="flex min-h-full items-end justify-center p-4 text-center sm:p-0">
              <div className="relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-4xl border border-gray-200">
                {/* Modern Header */}
                <div className="bg-white border-b border-gray-200 px-6 py-4 flex items-center justify-between rounded-t-lg">
                  <h3 className="text-lg font-semibold text-gray-900">
                    Installation Logs - {selectedUpdate?.package_name}
                  </h3>
                  <button
                    type="button"
                    className="text-gray-400 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-primary-500 rounded-md p-1"
                    onClick={() => setShowLogModal(false)}
                  >
                    <span className="sr-only">Close</span>
                    <X className="h-5 w-5" />
                  </button>
                </div>

                {/* Terminal Content Area */}
                <div className="bg-gray-900 text-green-400 p-4 max-h-96 overflow-y-auto" style={{ fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace' }}>
                  {logsLoading ? (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="h-5 w-5 animate-spin text-green-400 mr-2" />
                      <span className="text-green-400">Loading logs...</span>
                    </div>
                  ) : logs.length === 0 ? (
                    <div className="text-gray-500 text-center py-8">
                      No installation logs available for this update.
                    </div>
                  ) : (
                    <div className="space-y-3">
                      {logs.map((log, index) => (
                        <div key={index} className="border-b border-gray-700 pb-3 last:border-b-0">
                          <div className="flex items-center space-x-3 mb-2 text-xs">
                            <span className="text-gray-500">
                              {new Date(log.executedAt).toLocaleString()}
                            </span>
                            <span className={cn(
                              "px-2 py-1 rounded font-medium",
                              log.action === 'install' ? "bg-blue-900/50 text-blue-300" :
                              log.action === 'configure' ? "bg-yellow-900/50 text-yellow-300" :
                              log.action === 'cleanup' ? "bg-gray-700 text-gray-300" :
                              "bg-gray-700 text-gray-300"
                            )}>
                              {log.action?.toUpperCase() || 'UNKNOWN'}
                            </span>
                            {log.exit_code !== undefined && (
                              <span className={cn(
                                "px-2 py-1 rounded font-medium",
                                log.exit_code === 0 ? "bg-green-900/50 text-green-300" : "bg-red-900/50 text-red-300"
                              )}>
                                Exit: {log.exit_code}
                              </span>
                            )}
                            {log.duration_seconds && (
                              <span className="text-gray-500">
                                {log.duration_seconds}s
                              </span>
                            )}
                          </div>

                          {log.stdout && (
                            <div className="text-sm text-gray-300 whitespace-pre-wrap mb-2 font-mono">
                              {log.stdout}
                            </div>
                          )}

                          {log.stderr && (
                            <div className="text-sm text-red-400 whitespace-pre-wrap font-mono">
                              {log.stderr}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Modern Footer */}
                <div className="bg-gray-50 px-6 py-4 sm:flex sm:flex-row-reverse rounded-b-lg border-t border-gray-200">
                  <button
                    type="button"
                    className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-primary-600 text-base font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm"
                    onClick={() => setShowLogModal(false)}
                  >
                    Close
                  </button>
                  <button
                    type="button"
                    className="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
                    onClick={() => {
                      // Copy logs to clipboard functionality could be added here
                      navigator.clipboard.writeText(logs.map(log =>
                        `${log.action?.toUpperCase() || 'UNKNOWN'} - ${new Date(log.executedAt).toLocaleString()}\n${log.stdout || ''}\n${log.stderr || ''}`
                      ).join('\n\n'));
                      // Could add toast notification here
                    }}
                  >
                    Copy Logs
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}
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

  // Commands view
  if (activeTab === 'commands') {
    const commands = recentCommandsData?.commands || [];

    return (
      <div className="px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="mb-6">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Command History</h1>
              <p className="mt-1 text-sm text-gray-600">
                Review and retry failed or cancelled commands
              </p>
            </div>
            <button
              onClick={() => setActiveTab('updates')}
              className="btn btn-ghost"
            >
              ← Back to Updates
            </button>
          </div>
        </div>

        {/* Commands list */}
        {commands.length === 0 ? (
          <div className="text-center py-12">
            <Package className="mx-auto h-12 w-12 text-gray-400" />
            <h3 className="mt-2 text-sm font-medium text-gray-900">No commands found</h3>
            <p className="mt-1 text-sm text-gray-500">
              No command history available yet.
            </p>
          </div>
        ) : (
          <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="table-header">Command</th>
                    <th className="table-header">Package</th>
                    <th className="table-header">Agent</th>
                    <th className="table-header">Status</th>
                    <th className="table-header">Created</th>
                    <th className="table-header">Actions</th>
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {commands.map((command: any) => (
                    <tr key={command.id} className="hover:bg-gray-50">
                      <td className="table-cell">
                        <div className="text-sm font-medium text-gray-900">
                          {command.command_type.replace('_', ' ')}
                        </div>
                      </td>
                      <td className="table-cell">
                        <div className="text-sm text-gray-900">
                          {command.package_name}
                        </div>
                      </td>
                      <td className="table-cell">
                        <div className="text-sm text-gray-900">
                          {command.agent_hostname}
                        </div>
                      </td>
                      <td className="table-cell">
                        <span className={cn(
                          'badge',
                          command.status === 'completed' ? 'bg-green-100 text-green-800' :
                          command.status === 'failed' ? 'bg-red-100 text-red-800' :
                          command.status === 'cancelled' ? 'bg-gray-100 text-gray-800' :
                          command.status === 'pending' || command.status === 'sent' ? 'bg-blue-100 text-blue-800' :
                          'bg-gray-100 text-gray-800'
                        )}>
                          {command.status}
                        </span>
                      </td>
                      <td className="table-cell">
                        <div className="text-sm text-gray-900">
                          {formatRelativeTime(command.created_at)}
                        </div>
                      </td>
                      <td className="table-cell">
                        <div className="flex items-center space-x-2">
                          {(command.status === 'failed' || command.status === 'cancelled' || command.status === 'timed_out') && (
                            <button
                              onClick={() => handleRetryCommand(command.id)}
                              disabled={retryMutation.isLoading}
                              className="text-amber-600 hover:text-amber-800"
                              title="Retry command"
                            >
                              <RotateCcw className="h-4 w-4" />
                            </button>
                          )}

                          {(command.status === 'pending' || command.status === 'sent') && (
                            <button
                              onClick={() => handleCancelCommand(command.id)}
                              disabled={cancelMutation.isLoading}
                              className="text-red-600 hover:text-red-800"
                              title="Cancel command"
                            >
                              <X className="h-4 w-4" />
                            </button>
                          )}
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
  }

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

        {/* Statistics Cards - Compact design with combined visual boxes */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          {/* Total Updates - Standalone */}
          <div className="bg-white p-4 rounded-lg border border-gray-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Total Updates</p>
                <p className="text-2xl font-bold text-gray-900">{totalStats.total}</p>
              </div>
              <Package className="h-8 w-8 text-gray-400" />
            </div>
          </div>

          {/* Approved / Pending - Combined with divider */}
          <div className="bg-white p-4 rounded-lg border border-gray-200 shadow-sm">
            <div className="flex items-center justify-between divide-x divide-gray-200">
              <div className="flex-1 pr-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-gray-600">Approved</p>
                    <p className="text-xl font-bold text-green-600">{totalStats.approved}</p>
                  </div>
                  <CheckCircle className="h-6 w-6 text-green-400" />
                </div>
              </div>
              <div className="flex-1 pl-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-gray-600">Pending</p>
                    <p className="text-xl font-bold text-orange-600">{totalStats.pending}</p>
                  </div>
                  <Clock className="h-6 w-6 text-orange-400" />
                </div>
              </div>
            </div>
          </div>

          {/* Critical / High Priority - Combined with divider */}
          <div className="bg-white p-4 rounded-lg border border-gray-200 shadow-sm">
            <div className="flex items-center justify-between divide-x divide-gray-200">
              <div className="flex-1 pr-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-gray-600">Critical</p>
                    <p className="text-xl font-bold text-red-600">{totalStats.critical}</p>
                  </div>
                  <AlertTriangle className="h-6 w-6 text-red-400" />
                </div>
              </div>
              <div className="flex-1 pl-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-gray-600">High Priority</p>
                    <p className="text-xl font-bold text-yellow-600">{totalStats.high}</p>
                  </div>
                  <AlertTriangle className="h-6 w-6 text-yellow-400" />
                </div>
              </div>
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
          <button
            onClick={() => handleQuickFilter('installing')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'installing'
                ? "bg-blue-100 border-blue-300 text-blue-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <Loader2 className="h-4 w-4 mr-1 inline" />
            Installing
          </button>
          <button
            onClick={() => handleQuickFilter('installed')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'installed'
                ? "bg-emerald-100 border-emerald-300 text-emerald-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <CheckCircle className="h-4 w-4 mr-1 inline" />
            Installed
          </button>
          <button
            onClick={() => handleQuickFilter('failed')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'failed'
                ? "bg-red-100 border-red-300 text-red-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <XCircle className="h-4 w-4 mr-1 inline" />
            Failed
          </button>
          <button
            onClick={() => handleQuickFilter('dependencies')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'pending_dependencies'
                ? "bg-amber-100 border-amber-300 text-amber-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <AlertTriangle className="h-4 w-4 mr-1 inline" />
            Dependencies
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

          {/* Command History button */}
          <button
            onClick={() => setActiveTab('commands')}
            className="btn btn-ghost"
          >
            <RotateCcw className="h-4 w-4 mr-2" />
            Command History
          </button>
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
            {debouncedSearchQuery || statusFilter || severityFilter || typeFilter || agentFilter
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
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('package_name')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Package
                      {renderSortIcon('package_name')}
                    </button>
                  </th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('package_type')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Type
                      {renderSortIcon('package_type')}
                    </button>
                  </th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('available_version')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Versions
                      {renderSortIcon('available_version')}
                    </button>
                  </th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('severity')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Severity
                      {renderSortIcon('severity')}
                    </button>
                  </th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('status')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Status
                      {renderSortIcon('status')}
                    </button>
                  </th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('agent_id')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Agent
                      {renderSortIcon('agent_id')}
                    </button>
                  </th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('created_at')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Discovered
                      {renderSortIcon('created_at')}
                    </button>
                  </th>
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
                        <div className="min-w-0 flex-1">
                          <div className="text-sm font-medium text-gray-900">
                            <button
                              onClick={() => navigate(`/updates/${update.id}`)}
                              className="hover:text-primary-600 truncate block max-w-xs"
                              title={update.package_name}
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
                        {update.status === 'checking_dependencies' ? (
                          <div className="flex items-center space-x-1">
                            <Loader2 className="h-3 w-3 animate-spin" />
                            <span>Checking dependencies...</span>
                          </div>
                        ) : (
                          update.status
                        )}
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

                        {update.status === 'checking_dependencies' && (
                          <div className="text-blue-500" title="Checking dependencies">
                            <Loader2 className="h-4 w-4 animate-spin" />
                          </div>
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
                          const pageNum = totalPages <= 5 ? i + 1 : currentPage <= 3 ? i + 1 : currentPage >= totalPages - 2 ? totalPages - 4 + i : currentPage - 2 + i;

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