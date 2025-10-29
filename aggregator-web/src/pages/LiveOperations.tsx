import React, { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Activity,
  Clock,
  Package,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Loader2,
  RefreshCw,
  Filter,
  ChevronDown,
  Terminal,
  User,
  Calendar,
  Search,
  Computer,
  Eye,
  RotateCcw,
  X,
  Archive,
} from 'lucide-react';
import { useAgents, useUpdates } from '@/hooks/useAgents';
import { useActiveCommands, useRetryCommand, useCancelCommand, useClearFailedCommands } from '@/hooks/useCommands';
import { getStatusColor, formatRelativeTime, isOnline } from '@/lib/utils';
import { cn } from '@/lib/utils';
import toast from 'react-hot-toast';
import { logApi } from '@/lib/api';

interface LiveOperation {
  id: string;
  agentId: string;
  agentName: string;
  updateId: string;
  packageName: string;
  action: 'checking_dependencies' | 'installing' | 'pending_dependencies';
  status: 'running' | 'completed' | 'failed' | 'pending' | 'sent';
  startTime: Date;
  duration?: number;
  progress?: string;
  logOutput?: string;
  error?: string;
  commandId: string;
  commandStatus: string;
  isRetry?: boolean;
  hasBeenRetried?: boolean;
  retryCount?: number;
  retriedFromId?: string;
}

const LiveOperations: React.FC = () => {
  const [expandedOperations, setExpandedOperations] = useState<Set<string>>(new Set());
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [showFilters, setShowFilters] = useState(false);
  const [showCleanupDialog, setShowCleanupDialog] = useState(false);
  const [cleanupOptions, setCleanupOptions] = useState({
    olderThanDays: 7,
    onlyRetried: false,
    allFailed: false
  });

  // Fetch active commands from API
  const { data: activeCommandsData, refetch: refetchCommands } = useActiveCommands(autoRefresh);

  // Retry, cancel, and cleanup mutations
  const retryMutation = useRetryCommand();
  const cancelMutation = useCancelCommand();
  const clearFailedMutation = useClearFailedCommands();

  // Fetch agents for mapping
  const { data: agentsData } = useAgents();
  const agents = agentsData?.agents || [];

  // Transform API data to LiveOperation format
  const activeOperations: LiveOperation[] = React.useMemo(() => {
    if (!activeCommandsData?.commands) {
      return [];
    }

    return activeCommandsData.commands.map((cmd: any) => {
      const agent = agents.find(a => a.id === cmd.agent_id);
      let action: LiveOperation['action'];
      let status: LiveOperation['status'];

      // Map command status to operation status
      if (cmd.status === 'failed' || cmd.status === 'timed_out') {
        status = 'failed';
      } else if (cmd.status === 'pending') {
        status = 'pending';
      } else if (cmd.status === 'sent') {
        status = 'sent';
      } else if (cmd.status === 'completed') {
        status = 'completed';
      } else {
        status = 'running';
      }

      // Map command type to action
      switch (cmd.command_type) {
        case 'dry_run_update':
          action = 'checking_dependencies';
          break;
        case 'install_updates':
        case 'confirm_dependencies':
          action = 'installing';
          break;
        default:
          action = 'checking_dependencies';
      }

      return {
        id: cmd.id,
        agentId: cmd.agent_id,
        agentName: cmd.agent_hostname || 'Unknown Agent',
        updateId: cmd.id,
        packageName: cmd.package_name !== 'N/A' ? cmd.package_name : cmd.command_type,
        action,
        status,
        startTime: cmd.created_at ? new Date(cmd.created_at) : new Date(),
        progress: getStatusText(cmd.command_type, cmd.status),
        commandId: cmd.id,
        commandStatus: cmd.status,
        logOutput: cmd.result?.stdout || cmd.result?.stderr,
        error: cmd.result?.error_message,
        isRetry: cmd.is_retry || false,
        hasBeenRetried: cmd.has_been_retried || false,
        retryCount: cmd.retry_count || 0,
        retriedFromId: cmd.retried_from_id,
      };
    });
  }, [activeCommandsData, agents]);

  // Manual refresh function
  const handleManualRefresh = () => {
    refetchCommands();
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

  // Handle cleanup failed commands
  const handleClearFailedCommands = async () => {
    try {
      const result = await clearFailedMutation.mutateAsync(cleanupOptions);
      toast.success(result.message);
      if (result.cheeky_warning) {
        // Optional: Show a secondary toast with the cheeky warning
        setTimeout(() => {
          toast(result.cheeky_warning, {
            icon: '⚠️',
            style: {
              background: '#fef3c7',
              color: '#92400e',
            },
          });
        }, 1000);
      }
      setShowCleanupDialog(false);
    } catch (error: any) {
      toast.error(`Failed to clear failed commands: ${error.message || 'Unknown error'}`);
    }
  };

  // Count failed operations for display
  const failedCount = activeOperations.filter(op => op.status === 'failed').length;

  function getStatusText(commandType: string, status: string): string {
  if (commandType === 'dry_run_update') {
    return status === 'pending' ? 'Pending dependency check...' : 'Checking for required dependencies...';
  }
  if (commandType === 'install_updates') {
    return status === 'pending' ? 'Pending installation...' : 'Installing package and dependencies...';
  }
  if (commandType === 'confirm_dependencies') {
    return status === 'pending' ? 'Pending dependency confirmation...' : 'Installing confirmed dependencies...';
  }
  return status === 'pending' ? 'Pending operation...' : 'Processing command...';
}

  function getActionIcon(action: LiveOperation['action']) {
    switch (action) {
      case 'checking_dependencies':
        return <Search className="h-4 w-4" />;
      case 'installing':
        return <Package className="h-4 w-4" />;
      case 'pending_dependencies':
        return <AlertTriangle className="h-4 w-4" />;
      default:
        return <Activity className="h-4 w-4" />;
    }
  }

  function getStatusIcon(status: LiveOperation['status']) {
    switch (status) {
      case 'running':
        return <Loader2 className="h-4 w-4 animate-spin" />;
      case 'completed':
        return <CheckCircle className="h-4 w-4" />;
      case 'failed':
        return <XCircle className="h-4 w-4" />;
      case 'pending':
      case 'sent':
        return <Clock className="h-4 w-4" />;
      default:
        return <Activity className="h-4 w-4" />;
    }
  }

  function getDuration(startTime: Date): string {
    const now = new Date();
    const diff = now.getTime() - startTime.getTime();
    const seconds = Math.floor(diff / 1000);
    const minutes = Math.floor(seconds / 60);

    if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`;
    }
    return `${seconds}s`;
  }

  const filteredOperations = activeOperations.filter(op => {
    const matchesSearch = !searchQuery ||
      op.packageName.toLowerCase().includes(searchQuery.toLowerCase()) ||
      op.agentName.toLowerCase().includes(searchQuery.toLowerCase());

    const matchesStatus = statusFilter === 'all' || op.status === statusFilter;

    return matchesSearch && matchesStatus;
  });

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold text-gray-900 flex items-center space-x-2">
              <Activity className="h-6 w-6" />
              <span>Live Operations</span>
            </h1>
            <p className="mt-1 text-sm text-gray-600">
              Real-time monitoring of ongoing update operations
            </p>
          </div>
          <div className="flex items-center space-x-4">
            <button
              onClick={() => setAutoRefresh(!autoRefresh)}
              className={cn(
                "flex items-center space-x-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors",
                autoRefresh
                  ? "bg-green-100 text-green-700 hover:bg-green-200"
                  : "bg-gray-100 text-gray-700 hover:bg-gray-200"
              )}
            >
              <RefreshCw className={cn("h-4 w-4", autoRefresh && "animate-spin")} />
              <span>Auto Refresh</span>
            </button>
            <button
              onClick={handleManualRefresh}
              className="flex items-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 text-sm font-medium transition-colors"
            >
              <RefreshCw className="h-4 w-4" />
              <span>Refresh Now</span>
            </button>
            {failedCount > 0 && (
              <button
                onClick={() => setShowCleanupDialog(true)}
                className="flex items-center space-x-2 px-3 py-2 bg-red-100 text-red-700 rounded-lg hover:bg-red-200 text-sm font-medium transition-colors"
              >
                <Archive className="h-4 w-4" />
                <span>Archive Failed ({failedCount})</span>
              </button>
            )}
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <div className="bg-white p-4 rounded-lg border border-gray-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Total Active</p>
                <p className="text-2xl font-bold text-gray-900">{activeOperations.length}</p>
              </div>
              <Activity className="h-8 w-8 text-blue-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-blue-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Running</p>
                <p className="text-2xl font-bold text-blue-600">
                  {activeOperations.filter(op => op.status === 'running').length}
                </p>
              </div>
              <Loader2 className="h-8 w-8 text-blue-400 animate-spin" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-amber-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Pending</p>
                <p className="text-2xl font-bold text-amber-600">
                  {activeOperations.filter(op => op.status === 'pending' || op.status === 'sent').length}
                </p>
              </div>
              <Clock className="h-8 w-8 text-amber-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-red-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Failed</p>
                <p className="text-2xl font-bold text-red-600">
                  {activeOperations.filter(op => op.status === 'failed').length}
                </p>
              </div>
              <XCircle className="h-8 w-8 text-red-400" />
            </div>
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
                  placeholder="Search by package name or agent..."
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
              {statusFilter !== 'all' && (
                <span className="bg-primary-100 text-primary-800 px-2 py-0.5 rounded-full text-xs">1</span>
              )}
            </button>
          </div>

          {/* Filters */}
          {showFilters && (
            <div className="bg-white p-4 rounded-lg border border-gray-200">
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
                  <option value="running">Running</option>
                  <option value="pending">Pending</option>
                  <option value="sent">Sent</option>
                  <option value="completed">Completed</option>
                  <option value="failed">Failed</option>
                </select>
              </div>
            </div>
          )}
        </div>

        {/* Operations list */}
        {filteredOperations.length === 0 ? (
          <div className="text-center py-12">
            <Activity className="mx-auto h-12 w-12 text-gray-400" />
            <h3 className="mt-2 text-sm font-medium text-gray-900">No active operations</h3>
            <p className="mt-1 text-sm text-gray-500">
              {searchQuery || statusFilter !== 'all'
                ? 'Try adjusting your search or filters.'
                : 'All operations are completed. Check the Updates page to start new operations.'}
            </p>
          </div>
        ) : (
          <div className="space-y-4">
            {filteredOperations.map((operation) => (
              <div
                key={operation.id}
                className="bg-white rounded-lg border border-gray-200 shadow-sm overflow-hidden"
              >
                {/* Operation header */}
                <div className="p-4 border-b border-gray-200">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-3">
                      <div className="flex items-center space-x-2">
                        {getActionIcon(operation.action)}
                        <span className="text-lg font-medium text-gray-900">
                          {operation.packageName}
                        </span>
                        <span className={cn('badge', getStatusColor(operation.status))}>
                          {getStatusIcon(operation.status)}
                          <span className="ml-1">{operation.status}</span>
                        </span>
                        {operation.isRetry && operation.retryCount && operation.retryCount > 0 && (
                          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 border border-purple-200">
                            <RotateCcw className="h-3 w-3 mr-1" />
                            Retry #{operation.retryCount}
                          </span>
                        )}
                        {operation.hasBeenRetried && (
                          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-700 border border-gray-300">
                            Retried
                          </span>
                        )}
                      </div>
                      <div className="text-sm text-gray-600 flex items-center space-x-1">
                        <Computer className="h-4 w-4" />
                        <span>{operation.agentName}</span>
                        <span>•</span>
                        <span>{getDuration(operation.startTime)}</span>
                      </div>
                    </div>

                    <div className="flex items-center space-x-2">
                      <button
                        onClick={() => {
                          const newExpanded = new Set(expandedOperations);
                          if (newExpanded.has(operation.id)) {
                            newExpanded.delete(operation.id);
                          } else {
                            newExpanded.add(operation.id);
                          }
                          setExpandedOperations(newExpanded);
                        }}
                        className="flex items-center space-x-1 px-3 py-1 text-sm text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-md transition-colors"
                      >
                        <Eye className="h-4 w-4" />
                        <span>Details</span>
                        <ChevronDown
                          className={cn(
                            "h-4 w-4 transition-transform",
                            expandedOperations.has(operation.id) && "rotate-180"
                          )}
                        />
                      </button>
                    </div>
                  </div>

                  <div className="mt-2 text-sm text-gray-600">
                    {operation.progress}
                  </div>
                </div>

                {/* Expanded details */}
                {expandedOperations.has(operation.id) && (
                  <div className="p-4 bg-gray-50 border-t border-gray-200">
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                      <div>
                        <h4 className="text-sm font-medium text-gray-900 mb-2">Operation Details</h4>
                        <div className="space-y-2 text-sm">
                          <div className="flex justify-between">
                            <span className="text-gray-600">Action:</span>
                            <span className="font-medium capitalize">{operation.action.replace('_', ' ')}</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-gray-600">Started:</span>
                            <span className="font-medium">{formatRelativeTime(operation.startTime)}</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-gray-600">Duration:</span>
                            <span className="font-medium">{getDuration(operation.startTime)}</span>
                          </div>
                          <div className="flex justify-between">
                            <span className="text-gray-600">Agent:</span>
                            <span className="font-medium">{operation.agentName}</span>
                          </div>
                        </div>
                      </div>

                      <div>
                        <h4 className="text-sm font-medium text-gray-900 mb-2">Quick Actions</h4>
                        <div className="space-y-2">
                          <button
                            onClick={() => window.open(`/updates/${operation.updateId}`, '_blank')}
                            className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-blue-100 text-blue-700 rounded-md hover:bg-blue-200 text-sm font-medium transition-colors"
                          >
                            <Eye className="h-4 w-4" />
                            <span>View Update Details</span>
                          </button>
                          <button
                            onClick={() => window.open(`/agents/${operation.agentId}`, '_blank')}
                            className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-md hover:bg-gray-200 text-sm font-medium transition-colors"
                          >
                            <Computer className="h-4 w-4" />
                            <span>View Agent</span>
                          </button>

                          {/* Command control buttons */}
                          {operation.commandStatus === 'pending' || operation.commandStatus === 'sent' ? (
                            <button
                              onClick={() => handleCancelCommand(operation.commandId)}
                              disabled={cancelMutation.isPending}
                              className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-red-100 text-red-700 rounded-md hover:bg-red-200 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                              <X className="h-4 w-4" />
                              <span>{cancelMutation.isPending ? 'Cancelling...' : 'Cancel Command'}</span>
                            </button>
                          ) : null}

                          {/* Retry button for failed/timed_out commands */}
                          {operation.commandStatus === 'failed' || operation.commandStatus === 'timed_out' ? (
                            operation.hasBeenRetried ? (
                              <div className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-purple-50 text-purple-700 rounded-md border border-purple-200 text-sm font-medium">
                                <RotateCcw className="h-4 w-4" />
                                <span>Already Retried</span>
                              </div>
                            ) : (
                              <button
                                onClick={() => handleRetryCommand(operation.commandId)}
                                disabled={retryMutation.isPending}
                                className="w-full flex items-center justify-center space-x-2 px-3 py-2 bg-green-100 text-green-700 rounded-md hover:bg-green-200 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                              >
                                <RotateCcw className="h-4 w-4" />
                                <span>{retryMutation.isPending ? 'Retrying...' : 'Retry Command'}</span>
                              </button>
                            )
                          ) : null}
                        </div>
                      </div>
                    </div>

                    {/* Log output placeholder */}
                    <div className="mt-4">
                      <h4 className="text-sm font-medium text-gray-900 mb-2 flex items-center space-x-2">
                        <Terminal className="h-4 w-4" />
                        <span>Live Output</span>
                      </h4>
                      <div className="bg-gray-900 text-green-400 p-3 rounded-md font-mono text-xs min-h-32 max-h-48 overflow-y-auto">
                        {operation.status === 'running' ? (
                          <div className="flex items-center space-x-2">
                            <Loader2 className="h-3 w-3 animate-spin" />
                            <span>Waiting for log stream...</span>
                          </div>
                        ) : operation.logOutput ? (
                          <pre>{operation.logOutput}</pre>
                        ) : operation.error ? (
                          <div className="text-red-400">Error: {operation.error}</div>
                        ) : (
                          <div className="text-gray-500">No log output available</div>
                        )}
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Cleanup Confirmation Dialog */}
      {showCleanupDialog && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 max-w-md w-full mx-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-4">Archive Failed Operations</h3>

            <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded-md">
              <p className="text-sm text-blue-800">
                <strong>INFO:</strong> This will remove failed commands from the active operations view, but all history will be preserved in the database for audit trails and continuity.
              </p>
            </div>

            <div className="mb-4 p-3 bg-yellow-50 border border-yellow-200 rounded-md">
              <p className="text-sm text-yellow-800">
                <strong>WARNING:</strong> This shouldn't be necessary if the retry logic is working properly - you might want to check what's causing commands to fail in the first place!
              </p>
            </div>

            <div className="space-y-4 mb-6">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Clear operations older than
                </label>
                <div className="flex items-center space-x-2">
                  <input
                    type="number"
                    min="0"
                    value={cleanupOptions.olderThanDays}
                    onChange={(e) => setCleanupOptions(prev => ({
                      ...prev,
                      olderThanDays: parseInt(e.target.value) || 0
                    }))}
                    className="w-20 px-3 py-2 border border-gray-300 rounded-md text-sm"
                  />
                  <span className="text-sm text-gray-600">days</span>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Cleanup scope
                </label>
                <div className="space-y-2">
                  <label className="flex items-center">
                    <input
                      type="radio"
                      name="cleanupScope"
                      checked={!cleanupOptions.onlyRetried && !cleanupOptions.allFailed}
                      onChange={() => setCleanupOptions(prev => ({
                        ...prev,
                        onlyRetried: false,
                        allFailed: false
                      }))}
                      className="mr-2"
                    />
                    <span className="text-sm text-gray-700">All failed commands older than specified days</span>
                  </label>
                  <label className="flex items-center">
                    <input
                      type="radio"
                      name="cleanupScope"
                      checked={cleanupOptions.onlyRetried}
                      onChange={() => setCleanupOptions(prev => ({
                        ...prev,
                        onlyRetried: true,
                        allFailed: false
                      }))}
                      className="mr-2"
                    />
                    <span className="text-sm text-gray-700">Only failed commands that have been retried</span>
                  </label>
                  <label className="flex items-center">
                    <input
                      type="radio"
                      name="cleanupScope"
                      checked={cleanupOptions.allFailed}
                      onChange={() => setCleanupOptions(prev => ({
                        ...prev,
                        onlyRetried: false,
                        allFailed: true
                      }))}
                      className="mr-2"
                    />
                    <span className="text-sm text-red-700 font-medium">All failed commands (most aggressive)</span>
                  </label>
                </div>
              </div>
            </div>

            <div className="flex justify-end space-x-3">
              <button
                onClick={() => setShowCleanupDialog(false)}
                className="px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 rounded-md hover:bg-gray-200 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleClearFailedCommands}
                disabled={clearFailedMutation.isPending}
                className="px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-md hover:bg-red-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {clearFailedMutation.isPending ? 'Archiving...' : 'Archive Failed Commands'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default LiveOperations;