import { useEffect, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router-dom';
import {
  Search,
  RefreshCw,
  Terminal,
  ChevronDown,
  ChevronRight,
  Check,
  ExternalLink,
  X,
  Clock,
  AlertTriangle,
  Loader2,
  CheckCircle,
  XCircle,
} from 'lucide-react';
import { formatRelativeTime, formatBytes } from '@/lib/utils';
import { packageSeverityColor, packageSeverityTextColor } from '@/components/primitives/statusColors';
import Modal from '@/components/primitives/Modal';
import { updateApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';
import type { UpdatePackage } from '@/types';

interface AgentUpdatesEnhancedProps {
  agentId: string;
  onNavigateToHistory?: () => void;
}

interface AgentUpdateResponse {
  updates: UpdatePackage[];
  total: number;
}

interface LogResponse {
  stdout: string;
  stderr: string;
  exit_code: number;
  duration_seconds: number;
  result: string;
}

const TAB_GROUPS: { key: string; label: string; statuses: string }[] = [
  { key: 'needs-review', label: 'Needs Review', statuses: 'pending' },
  { key: 'in-progress', label: 'In Progress', statuses: 'approved,checking_dependencies,pending_dependencies,installing' },
  { key: 'installed', label: 'Installed', statuses: 'installed' },
  { key: 'failed-ignored', label: 'Failed / Ignored', statuses: 'failed,ignored' },
];

const parseAgentUpdatesTab = (tab: string | null) => {
  return TAB_GROUPS.some(group => group.key === tab) ? tab || TAB_GROUPS[0].key : TAB_GROUPS[0].key;
};

const STATUS_META: Record<string, { label: string; icon: React.ReactNode; class: string }> = {
  pending:               { label: 'Pending',          icon: <Clock className="h-3 w-3" />, class: 'text-gray-600 bg-gray-100' },
  approved:              { label: 'Approved',         icon: <Check className="h-3 w-3" />, class: 'text-blue-600 bg-blue-100' },
  checking_dependencies: { label: 'Checking Deps',    icon: <Loader2 className="h-3 w-3 animate-spin" />, class: 'text-yellow-600 bg-yellow-100' },
  pending_dependencies:  { label: 'Deps Pending',     icon: <AlertTriangle className="h-3 w-3" />, class: 'text-orange-600 bg-orange-100' },
  installing:            { label: 'Installing',       icon: <RefreshCw className="h-3 w-3 animate-spin" />, class: 'text-purple-600 bg-purple-100' },
  installed:             { label: 'Installed',        icon: <CheckCircle className="h-3 w-3" />, class: 'text-green-600 bg-green-100' },
  failed:                { label: 'Failed',           icon: <XCircle className="h-3 w-3" />, class: 'text-red-600 bg-red-100' },
  ignored:               { label: 'Ignored',          icon: <X className="h-3 w-3" />, class: 'text-gray-500 bg-gray-50' },
};

export function AgentUpdatesEnhanced({ agentId, onNavigateToHistory }: AgentUpdatesEnhancedProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = parseAgentUpdatesTab(searchParams.get('updates_tab'));
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize] = useState(50);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedSeverity, setSelectedSeverity] = useState('all');
  const [showLogsModal, setShowLogsModal] = useState(false);
  const [logsData, setLogsData] = useState<LogResponse | null>(null);
  const [isLoadingLogs, setIsLoadingLogs] = useState(false);
  const [expandedUpdates, setExpandedUpdates] = useState<Set<string>>(new Set());
  const [selectedUpdates, setSelectedUpdates] = useState<string[]>([]);
  const [confirmDepsUpdateId, setConfirmDepsUpdateId] = useState<string | null>(null);
  const [confirmDepsData, setConfirmDepsData] = useState<string[] | null>(null);

  const queryClient = useQueryClient();
  const activeGroup = TAB_GROUPS.find(g => g.key === activeTab) || TAB_GROUPS[0];

  useEffect(() => {
    setCurrentPage(1);
    setSelectedUpdates([]);
  }, [activeTab]);

  const selectActiveTab = (tab: string) => {
    const params = new URLSearchParams(searchParams);
    if (tab === TAB_GROUPS[0].key) {
      params.delete('updates_tab');
    } else {
      params.set('updates_tab', tab);
    }
    setSearchParams(params, { replace: true });
  };

  // Fetch updates with status filter
  const { data: updateData, isLoading, error, refetch } = useQuery<AgentUpdateResponse>({
    queryKey: ['agent-updates', agentId, activeGroup.statuses, currentPage, pageSize, searchTerm, selectedSeverity],
    queryFn: async () => {
      const params = {
        page: currentPage,
        page_size: pageSize,
        agent_id: agentId,
        status: activeGroup.statuses,
        ...(searchTerm && { search: searchTerm }),
        ...(selectedSeverity !== 'all' && { severity: selectedSeverity }),
      };

      const response = await updateApi.getUpdates(params);
      return response;
    },
    refetchInterval: 30000,
  });

  // Mutations
  const approveMutation = useMutation({
    mutationFn: async (updateId: string) => {
      const response = await updateApi.approveUpdate(updateId);
      return response;
    },
    onSuccess: () => {
      toast.success('Update approved');
      refetch();
      queryClient.invalidateQueries({ queryKey: ['agent-updates'] });
    },
    onError: (error: any) => {
      toast.error(`Failed to approve: ${error.message || 'Unknown error'}`);
    },
  });

  const installMutation = useMutation({
    mutationFn: async (updateId: string) => {
      const response = await updateApi.installUpdate(updateId);
      return response;
    },
    onSuccess: () => {
      toast.success('Installation started');
      setTimeout(() => {
        refetch();
        queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
      }, 2000);
    },
    onError: (error: any) => {
      toast.error(`Failed to install: ${error.message || 'Unknown error'}`);
    },
  });

  const bulkApproveMutation = useMutation({
    mutationFn: async (updateIds: string[]) => {
      const response = await updateApi.approveMultiple(updateIds);
      return response;
    },
    onSuccess: () => {
      toast.success(`${selectedUpdates.length} updates approved`);
      setSelectedUpdates([]);
      refetch();
    },
    onError: (error: any) => {
      toast.error(`Failed to approve: ${error.message || 'Unknown error'}`);
    },
  });

  const rejectMutation = useMutation({
    mutationFn: async (updateId: string) => {
      const response = await updateApi.rejectUpdate(updateId);
      return response;
    },
    onSuccess: () => {
      toast.success('Update rejected');
      refetch();
      queryClient.invalidateQueries({ queryKey: ['agent-updates'] });
    },
    onError: (error: any) => {
      toast.error(`Failed to reject: ${error.message || 'Unknown error'}`);
    },
  });

  const confirmDepsMutation = useMutation({
    mutationFn: async (updateId: string) => {
      const response = await updateApi.confirmDependencies(updateId);
      return response;
    },
    onSuccess: () => {
      toast.success('Dependencies confirmed, installing');
      setConfirmDepsUpdateId(null);
      setConfirmDepsData(null);
      refetch();
      queryClient.invalidateQueries({ queryKey: ['agent-updates'] });
    },
    onError: (error: any) => {
      toast.error(`Failed to confirm dependencies: ${error.message || 'Unknown error'}`);
    },
  });

  const getLogsMutation = useMutation({
    mutationFn: async (commandId: string) => {
      setIsLoadingLogs(true);
      const response = await updateApi.getUpdateLogs(commandId);
      return response;
    },
    onSuccess: (data) => {
      if (data.logs && data.logs.length > 0) {
        const log = data.logs[0];
        setLogsData({
          stdout: log.stdout || '',
          stderr: log.stderr || '',
          exit_code: log.exit_code ?? 0,
          duration_seconds: log.duration_seconds ?? 0,
          result: log.result || '',
        });
      }
      setShowLogsModal(true);
    },
    onError: (error: any) => {
      toast.error(`Failed to fetch logs: ${error.message || 'Unknown error'}`);
    },
    onSettled: () => {
      setIsLoadingLogs(false);
    },
  });

  const updates = updateData?.updates || [];
  const totalCount = updateData?.total || 0;
  const totalPages = Math.ceil(totalCount / pageSize);

  const handleSelectUpdate = (updateId: string, checked: boolean) => {
    if (checked) {
      setSelectedUpdates([...selectedUpdates, updateId]);
    } else {
      setSelectedUpdates(selectedUpdates.filter(id => id !== updateId));
    }
  };

  const handleApprove = async (updateId: string) => {
    approveMutation.mutate(updateId);
  };

  const handleInstall = async (updateId: string) => {
    installMutation.mutate(updateId);
  };

  const handleReject = async (updateId: string) => {
    rejectMutation.mutate(updateId);
  };

  const handleBulkApprove = async () => {
    if (selectedUpdates.length === 0) {
      toast.error('Select at least one update');
      return;
    }
    bulkApproveMutation.mutate(selectedUpdates);
  };

  const handleViewLogs = async (update: UpdatePackage) => {
    const recentCommand = update.recent_command_id;
    if (recentCommand) {
      getLogsMutation.mutate(recentCommand);
    } else {
      toast.error('No recent command logs available for this package');
    }
  };

  const handleConfirmDeps = async (update: UpdatePackage) => {
    const deps: string[] = Array.isArray(update.metadata?.dependencies)
      ? update.metadata.dependencies
      : [];
    setConfirmDepsUpdateId(update.id);
    setConfirmDepsData(deps);
  };

  const handleExecuteConfirmDeps = async () => {
    if (confirmDepsUpdateId) {
      confirmDepsMutation.mutate(confirmDepsUpdateId);
    }
  };

  const handleCancelConfirmDeps = () => {
    setConfirmDepsUpdateId(null);
    setConfirmDepsData(null);
  };

  const toggleExpanded = (updateId: string) => {
    const newExpanded = new Set(expandedUpdates);
    if (newExpanded.has(updateId)) {
      newExpanded.delete(updateId);
    } else {
      newExpanded.add(updateId);
    }
    setExpandedUpdates(newExpanded);
  };

  if (isLoading) {
    return (
      <div className="space-y-3">
        <div className="animate-pulse space-y-2">
          {[...Array(5)].map((_, i) => (
            <div key={i} className="p-3 bg-white rounded border border-gray-100">
              <div className="h-4 bg-gray-200 rounded w-1/3 mb-2"></div>
              <div className="h-3 bg-gray-200 rounded w-2/3"></div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="alert alert-danger rounded text-sm">
        Error loading updates: {(error as Error).message}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Tabs */}
      <div className="flex items-center space-x-1 border-b border-gray-200 text-sm">
        {TAB_GROUPS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => selectActiveTab(tab.key)}
            className={cn(
              'px-4 py-2 border-b-2 transition-colors',
              activeTab === tab.key
                ? 'border-gray-900 text-gray-900'
                : 'border-transparent text-gray-500 hover:text-gray-700'
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Filters and Actions */}
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-3 text-sm">
          <span className="text-gray-600">
            {totalCount} update{totalCount !== 1 ? 's' : ''}
          </span>
          {['critical', 'high', 'medium', 'low'].map((severity) => {
            const count = updates.filter(u => u.severity?.toLowerCase() === severity).length;
            if (count === 0) return null;
            return (
              <span key={severity} className="text-gray-500">
                <span className={cn('font-medium', packageSeverityTextColor(severity))}>{count}</span> {severity}
              </span>
            );
          })}
        </div>

        {selectedUpdates.length > 0 && activeTab === 'needs-review' && (
          <button
            onClick={handleBulkApprove}
            disabled={bulkApproveMutation.isPending}
            className="text-sm text-gray-600 hover:text-gray-900 flex items-center space-x-1"
          >
            {bulkApproveMutation.isPending ? (
              <>
                <RefreshCw className="h-4 w-4 animate-spin" />
                <span>Approving...</span>
              </>
            ) : (
              <>
                <Check className="h-4 w-4" />
                <span>Approve {selectedUpdates.length}</span>
              </>
            )}
          </button>
        )}
      </div>

      {/* Search and Filters */}
      <div className="flex items-center space-x-3 text-sm">
        <div className="flex-1 max-w-xs">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
            <input
              type="text"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder="Search packages..."
              className="pl-9 pr-3 py-1.5 w-full border border-gray-300 rounded text-sm"
            />
          </div>
        </div>

        <select
          value={selectedSeverity}
          onChange={(e) => setSelectedSeverity(e.target.value)}
          className="px-3 py-1.5 border border-gray-300 rounded text-sm"
        >
          <option value="all">All Severities</option>
          <option value="critical">Critical</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
      </div>

      {/* Updates List */}
      {updates.length === 0 ? (
        <div className="text-center py-12 text-sm text-gray-500">
          {activeTab === 'installed' ? (
            <div>
              <p className="mb-2">Installed updates are shown in History</p>
              {onNavigateToHistory && (
                <button
                  onClick={onNavigateToHistory}
                  className="text-gray-600 hover:text-gray-900 underline"
                >
                  View History
                </button>
              )}
            </div>
          ) : (
            `No ${activeGroup.label.toLowerCase()} updates`
          )}
        </div>
      ) : (
        <div className="space-y-px">
          {updates.map((update) => {
            const isExpanded = expandedUpdates.has(update.id);
            const statusMeta = STATUS_META[update.status] || { label: update.status, icon: null, class: 'text-gray-600 bg-gray-100' };
            const deps: string[] = Array.isArray(update.metadata?.dependencies)
              ? update.metadata.dependencies
              : [];
            return (
              <div key={update.id} className="bg-white border-b border-gray-100 last:border-0">
                <div className="flex items-center p-2 gap-3">
                  {/* Checkbox for bulk actions */}
                  {activeTab === 'needs-review' && (
                    <input
                      type="checkbox"
                      checked={selectedUpdates.includes(update.id)}
                      onChange={(e) => handleSelectUpdate(update.id, e.target.checked)}
                      onClick={(e) => e.stopPropagation()}
                      className="h-4 w-4 rounded border-gray-300"
                    />
                  )}

                  {/* Main content */}
                  <div
                    className="flex-1 flex items-center justify-between gap-3 cursor-pointer"
                    onClick={() => toggleExpanded(update.id)}
                  >
                    <div className="flex items-center space-x-3 flex-1 min-w-0">
                      {/* Status badge */}
                      <span className={cn(
                        'chip whitespace-nowrap',
                        statusMeta.class
                      )}>
                        {statusMeta.icon}
                        {statusMeta.label}
                      </span>

                      <span className={cn('chip', packageSeverityColor(update.severity))}>
                        {update.severity.toUpperCase()}
                      </span>
                      <span className="text-sm text-gray-900 truncate">{update.package_name}</span>
                      <span className="text-xs text-gray-500">{update.current_version} → {update.available_version}</span>
                    </div>

                    <div className="flex items-center space-x-2 flex-shrink-0">
                      {update.status === 'pending' && (
                        <>
                          <button
                            onClick={(e) => { e.stopPropagation(); handleApprove(update.id); }}
                            className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                          >
                            Approve
                          </button>
                          <button
                            onClick={(e) => { e.stopPropagation(); handleReject(update.id); }}
                            className="text-xs text-red-600 hover:text-red-800 px-2 py-1"
                          >
                            Reject
                          </button>
                        </>
                      )}
                      {update.status === 'approved' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleInstall(update.id); }}
                          className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                        >
                          Install
                        </button>
                      )}
                      {update.status === 'pending_dependencies' && deps.length > 0 && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleConfirmDeps(update); }}
                          className="text-xs text-orange-600 hover:text-orange-800 px-2 py-1 border border-orange-200 rounded"
                        >
                          Review Dependencies
                        </button>
                      )}
                      {update.status === 'failed' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleInstall(update.id); }}
                          className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                        >
                          Retry
                        </button>
                      )}
                      {update.recent_command_id && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleViewLogs(update); }}
                          disabled={isLoadingLogs}
                          className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1 disabled:opacity-50"
                        >
                          {isLoadingLogs ? 'Loading...' : 'Logs'}
                        </button>
                      )}
                      <Link
                        to={`/updates/${update.id}`}
                        onClick={(e) => e.stopPropagation()}
                        className="inline-flex items-center text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                        title="Open update detail"
                      >
                        <ExternalLink className="h-3 w-3 mr-1" />
                        Details
                      </Link>
                      {isExpanded ? (
                        <ChevronDown className="h-4 w-4 text-gray-400" />
                      ) : (
                        <ChevronRight className="h-4 w-4 text-gray-400" />
                      )}
                    </div>
                  </div>
                </div>

                {/* Expanded Details */}
                {isExpanded && (
                  <div className="px-2 pb-3 ml-8">
                    <div className="bg-white/90 backdrop-blur-md rounded border border-gray-200 p-3 text-xs space-y-2">
                      {update.metadata?.description && (
                        <p className="text-gray-700">{update.metadata.description}</p>
                      )}
                      <div className="grid grid-cols-2 gap-2 text-gray-600">
                        <div><span className="font-medium">Type:</span> {update.package_type}</div>
                        <div><span className="font-medium">Severity:</span> {update.severity}</div>
                        {update.metadata?.size_bytes && (
                          <div><span className="font-medium">Size:</span> {formatBytes(update.metadata.size_bytes)}</div>
                        )}
                        {update.last_discovered_at && (
                          <div><span className="font-medium">Discovered:</span> {formatRelativeTime(update.last_discovered_at)}</div>
                        )}
                        {update.approved_at && (
                          <div><span className="font-medium">Approved:</span> {formatRelativeTime(update.approved_at)}</div>
                        )}
                        {update.installed_at && (
                          <div><span className="font-medium">Installed:</span> {formatRelativeTime(update.installed_at)}</div>
                        )}
                      </div>

                      {/* Dependency inline panel for pending_dependencies */}
                      {update.status === 'pending_dependencies' && deps.length > 0 && (
                        <div className="mt-2 pt-2 border-t border-gray-200">
                          <p className="font-medium text-gray-700 mb-1">
                            Dependencies ({deps.length})
                          </p>
                          <ul className="list-disc list-inside text-gray-600 space-y-0.5 mb-2">
                            {deps.map((dep, i) => (
                              <li key={i}>{dep}</li>
                            ))}
                          </ul>
                          <div className="flex items-center space-x-2">
                            <button
                              onClick={(e) => { e.stopPropagation(); handleConfirmDeps(update); }}
                              className="text-xs text-orange-600 hover:text-orange-800 px-2 py-1 border border-orange-200 rounded"
                            >
                              Confirm & Install
                            </button>
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm text-gray-600">
          <span>
            {Math.min((currentPage - 1) * pageSize + 1, totalCount)} - {Math.min(currentPage * pageSize, totalCount)} of {totalCount}
          </span>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
              disabled={currentPage === 1}
              className="px-3 py-1 border border-gray-300 rounded disabled:opacity-50"
            >
              Previous
            </button>
            <span>Page {currentPage} of {totalPages}</span>
            <button
              onClick={() => setCurrentPage(Math.min(totalPages, currentPage + 1))}
              disabled={currentPage === totalPages}
              className="px-3 py-1 border border-gray-300 rounded disabled:opacity-50"
            >
              Next
            </button>
          </div>
        </div>
      )}

      {/* Dependency Confirmation Modal */}
      <Modal
        open={confirmDepsUpdateId !== null && confirmDepsData !== null}
        onClose={handleCancelConfirmDeps}
        title={
          <span className="flex items-center space-x-2">
            <AlertTriangle className="h-4 w-4 text-orange-500" />
            <span>Confirm Dependencies</span>
          </span>
        }
        maxWidth="lg"
      >
        <Modal.Body>
          {confirmDepsData !== null && (
            confirmDepsData.length === 0 ? (
              <p className="text-sm text-gray-600">No additional dependencies detected. Proceed with installation.</p>
            ) : (
              <>
                <p className="text-sm text-gray-600">
                  The following additional packages will be installed:
                </p>
                <ul className="list-disc list-inside text-sm text-gray-700 space-y-1 mt-2">
                  {confirmDepsData.map((dep, i) => (
                    <li key={i}>{dep}</li>
                  ))}
                </ul>
              </>
            )
          )}
        </Modal.Body>
        <Modal.Footer>
          <button
            onClick={handleExecuteConfirmDeps}
            disabled={confirmDepsMutation.isPending}
            className="inline-flex items-center px-3 py-1.5 text-sm bg-orange-600 text-white border border-orange-700 rounded hover:bg-orange-700 disabled:opacity-50"
          >
            {confirmDepsMutation.isPending ? 'Confirming...' : 'Confirm & Install'}
          </button>
          <button
            onClick={handleCancelConfirmDeps}
            className="inline-flex items-center px-3 py-1.5 text-sm text-gray-600 bg-white border border-gray-300 rounded hover:bg-gray-50"
          >
            Cancel
          </button>
        </Modal.Footer>
      </Modal>

      {/* Logs Modal */}
      <Modal
        open={showLogsModal && logsData !== null}
        onClose={() => setShowLogsModal(false)}
        title={
          <span className="flex items-center space-x-2">
            <Terminal className="h-4 w-4" />
            <span>Installation Logs</span>
          </span>
        }
        maxWidth="4xl"
        maxHeight="80vh"
      >
        <Modal.Body scrollable className="space-y-3 text-xs">
          {logsData && (
            <>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <span className="font-medium text-gray-700">Result:</span>
                  <span className={cn(
                    'ml-2 chip',
                    logsData.result === 'success' ? 'chip-success' :
                    logsData.result === 'failed' ? 'chip-danger' :
                    'chip-neutral'
                  )}>
                    {logsData.result || 'Unknown'}
                  </span>
                </div>
                <div>
                  <span className="font-medium text-gray-700">Exit Code:</span>
                  <span className="ml-2">{logsData.exit_code}</span>
                </div>
                <div>
                  <span className="font-medium text-gray-700">Duration:</span>
                  <span className="ml-2">{logsData.duration_seconds}s</span>
                </div>
              </div>

              {logsData.stdout && (
                <div>
                  <h4 className="font-medium text-gray-900 mb-1">Standard Output</h4>
                  <pre className="bg-gray-50 border border-gray-200 rounded p-2 text-xs overflow-x-auto whitespace-pre-wrap">
                    {logsData.stdout}
                  </pre>
                </div>
              )}

              {logsData.stderr && (
                <div>
                  <h4 className="font-medium text-gray-900 mb-1">Standard Error</h4>
                  <pre className="bg-red-50 border border-red-200 rounded p-2 text-xs overflow-x-auto whitespace-pre-wrap">
                    {logsData.stderr}
                  </pre>
                </div>
              )}
            </>
          )}
        </Modal.Body>
      </Modal>
    </div>
  );
}
