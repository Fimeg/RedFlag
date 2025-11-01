import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Search,
  Package,
  Download,
  CheckCircle,
  RefreshCw,
  Terminal,
  Filter,
  ChevronDown,
  ChevronRight,
  Check,
  X,
} from 'lucide-react';
import { formatRelativeTime, formatBytes } from '@/lib/utils';
import { updateApi, agentApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';
import type { UpdatePackage } from '@/types';

interface AgentUpdatesEnhancedProps {
  agentId: string;
}

interface AgentUpdateResponse {
  updates: UpdatePackage[];
  total: number;
}

interface CommandResponse {
  command_id: string;
  status: string;
  message: string;
}

interface LogResponse {
  stdout: string;
  stderr: string;
  exit_code: number;
  duration_seconds: number;
  result: string;
}

type StatusTab = 'pending' | 'approved' | 'installing' | 'installed';

export function AgentUpdatesEnhanced({ agentId }: AgentUpdatesEnhancedProps) {
  const [activeStatus, setActiveStatus] = useState<StatusTab>('pending');
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedSeverity, setSelectedSeverity] = useState('all');
  const [showLogsModal, setShowLogsModal] = useState(false);
  const [logsData, setLogsData] = useState<LogResponse | null>(null);
  const [isLoadingLogs, setIsLoadingLogs] = useState(false);
  const [expandedUpdates, setExpandedUpdates] = useState<Set<string>>(new Set());
  const [selectedUpdates, setSelectedUpdates] = useState<string[]>([]);

  const queryClient = useQueryClient();

  // Fetch updates with status filter
  const { data: updateData, isLoading, error, refetch } = useQuery<AgentUpdateResponse>({
    queryKey: ['agent-updates', agentId, activeStatus, currentPage, pageSize, searchTerm, selectedSeverity],
    queryFn: async () => {
      const params = {
        page: currentPage,
        page_size: pageSize,
        agent_id: agentId,
        status: activeStatus,
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
      const response = await agentApi.installUpdate(agentId, updateId);
      return response;
    },
    onSuccess: () => {
      toast.success('Installation started');
      setTimeout(() => {
        refetch();
        queryClient.invalidateQueries({ queryKey: ['active-commands'] });
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

  const getLogsMutation = useMutation({
    mutationFn: async (commandId: string) => {
      setIsLoadingLogs(true);
      const response = await agentApi.getCommandLogs(agentId, commandId);
      return response;
    },
    onSuccess: (data: LogResponse) => {
      setLogsData(data);
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

  const getSeverityColor = (severity: string) => {
    switch (severity.toLowerCase()) {
      case 'critical': return 'text-red-600 bg-red-50';
      case 'important':
      case 'high': return 'text-orange-600 bg-orange-50';
      case 'moderate':
      case 'medium': return 'text-yellow-600 bg-yellow-50';
      case 'low':
      case 'none': return 'text-blue-600 bg-blue-50';
      default: return 'text-gray-600 bg-gray-50';
    }
  };

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

  const handleApprove = async (updateId: string) => {
    approveMutation.mutate(updateId);
  };

  const handleInstall = async (updateId: string) => {
    installMutation.mutate(updateId);
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
      <div className="p-4 bg-red-50 border border-red-200 rounded text-sm text-red-600">
        Error loading updates: {(error as Error).message}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Tabs */}
      <div className="flex items-center space-x-1 border-b border-gray-200 text-sm">
        {[
          { key: 'pending', label: 'Pending' },
          { key: 'approved', label: 'Approved' },
          { key: 'installing', label: 'Installing' },
          { key: 'installed', label: 'Installed' },
        ].map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveStatus(tab.key as StatusTab)}
            className={cn(
              'px-4 py-2 border-b-2 transition-colors',
              activeStatus === tab.key
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
                <span className={cn(
                  'font-medium',
                  severity === 'critical' ? 'text-red-600' :
                  severity === 'high' ? 'text-orange-600' :
                  severity === 'medium' ? 'text-yellow-600' : 'text-blue-600'
                )}>{count}</span> {severity}
              </span>
            );
          })}
        </div>

        {selectedUpdates.length > 0 && activeStatus === 'pending' && (
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
          {activeStatus === 'installed' ? (
            <div>
              <p className="mb-2">Installed updates are shown in History</p>
              <button
                onClick={() => window.location.href = `/agents/${agentId}?tab=history`}
                className="text-gray-600 hover:text-gray-900 underline"
              >
                View History
              </button>
            </div>
          ) : (
            `No ${activeStatus} updates`
          )}
        </div>
      ) : (
        <div className="space-y-px">
          {updates.map((update) => {
            const isExpanded = expandedUpdates.has(update.id);
            return (
              <div key={update.id} className="bg-white border-b border-gray-100 last:border-0">
                <div className="flex items-center p-2 gap-3">
                  {/* Checkbox for pending */}
                  {activeStatus === 'pending' && (
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
                      <span className={cn('px-2 py-0.5 rounded text-xs font-medium', getSeverityColor(update.severity))}>
                        {update.severity.toUpperCase()}
                      </span>
                      <span className="text-sm text-gray-900 truncate">{update.package_name}</span>
                      <span className="text-xs text-gray-500">{update.current_version} → {update.available_version}</span>
                    </div>

                    <div className="flex items-center space-x-2 flex-shrink-0">
                      {activeStatus === 'pending' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleApprove(update.id); }}
                          className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                        >
                          Approve
                        </button>
                      )}
                      {activeStatus === 'approved' && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleInstall(update.id); }}
                          className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                        >
                          Install
                        </button>
                      )}
                      {update.recent_command_id && (
                        <button
                          onClick={(e) => { e.stopPropagation(); handleViewLogs(update); }}
                          className="text-xs text-gray-600 hover:text-gray-900 px-2 py-1"
                        >
                          Logs
                        </button>
                      )}
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

      {/* Logs Modal */}
      {showLogsModal && logsData && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-lg max-w-4xl w-full max-h-[80vh] overflow-hidden">
            <div className="p-4 border-b border-gray-200 flex items-center justify-between">
              <h3 className="text-sm font-medium text-gray-900 flex items-center space-x-2">
                <Terminal className="h-4 w-4" />
                <span>Installation Logs</span>
              </h3>
              <button
                onClick={() => setShowLogsModal(false)}
                className="text-gray-400 hover:text-gray-600"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <div className="p-4 overflow-y-auto max-h-[60vh] space-y-3 text-xs">
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <span className="font-medium text-gray-700">Result:</span>
                  <span className={cn(
                    'ml-2 px-2 py-0.5 rounded',
                    logsData.result === 'success' ? 'bg-green-100 text-green-800' :
                    logsData.result === 'failed' ? 'bg-red-100 text-red-800' :
                    'bg-gray-100 text-gray-800'
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
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
