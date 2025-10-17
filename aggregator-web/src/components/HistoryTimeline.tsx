import React, { useState, useEffect } from 'react';
import {
  Activity,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Clock,
  Package,
  Computer,
  Calendar,
  ChevronDown,
  ChevronRight,
  Terminal,
  RefreshCw,
  Filter,
  Search,
} from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { logApi } from '@/lib/api';
import { cn } from '@/lib/utils';
import { formatRelativeTime } from '@/lib/utils';
import toast from 'react-hot-toast';

interface HistoryEntry {
  id: string;
  agent_id: string;
  update_package_id?: string;
  action: string;
  result: string;
  stdout?: string;
  stderr?: string;
  exit_code: number;
  duration_seconds: number;
  executed_at: string;
}

interface HistoryTimelineProps {
  agentId?: string; // Optional - if provided, filter to specific agent
  className?: string;
}

interface TimelineGroup {
  date: string;
  entries: HistoryEntry[];
}

const HistoryTimeline: React.FC<HistoryTimelineProps> = ({ agentId, className }) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [actionFilter, setActionFilter] = useState('all');
  const [resultFilter, setResultFilter] = useState('all');
  const [showFilters, setShowFilters] = useState(false);
  const [expandedEntries, setExpandedEntries] = useState<Set<string>>(new Set());
  const [expandedDates, setExpandedDates] = useState<Set<string>>(new Set());

  // Query parameters for API
  const [queryParams, setQueryParams] = useState({
    page: 1,
    page_size: 50,
    agent_id: agentId || '',
    action: actionFilter !== 'all' ? actionFilter : '',
    result: resultFilter !== 'all' ? resultFilter : '',
    search: searchQuery,
  });

  // Fetch history data
  const { data: historyData, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['history', queryParams],
    queryFn: async () => {
      try {
        const params: any = {
          page: queryParams.page,
          page_size: queryParams.page_size,
        };

        if (queryParams.agent_id) {
          params.agent_id = queryParams.agent_id;
        }

        if (queryParams.action) {
          params.action = queryParams.action;
        }

        if (queryParams.result) {
          params.result = queryParams.result;
        }

        const response = await logApi.getAllLogs(params);
        return response;
      } catch (error) {
        console.error('Failed to fetch history:', error);
        toast.error('Failed to fetch history');
        return { logs: [], total: 0, page: 1, page_size: 50 };
      }
    },
    refetchInterval: 30000, // Refresh every 30 seconds
  });

  // Group entries by date
  const groupEntriesByDate = (entries: HistoryEntry[]): TimelineGroup[] => {
    const groups: { [key: string]: HistoryEntry[] } = {};

    entries.forEach(entry => {
      const date = new Date(entry.executed_at);
      const today = new Date();
      const yesterday = new Date(today);
      yesterday.setDate(yesterday.getDate() - 1);

      let dateKey: string;
      if (date.toDateString() === today.toDateString()) {
        dateKey = 'Today';
      } else if (date.toDateString() === yesterday.toDateString()) {
        dateKey = 'Yesterday';
      } else {
        dateKey = date.toLocaleDateString('en-US', {
          year: 'numeric',
          month: 'long',
          day: 'numeric'
        });
      }

      if (!groups[dateKey]) {
        groups[dateKey] = [];
      }
      groups[dateKey].push(entry);
    });

    return Object.entries(groups).map(([date, entries]) => ({
      date,
      entries: entries.sort((a, b) =>
        new Date(b.executed_at).getTime() - new Date(a.executed_at).getTime()
      ),
    }));
  };

  const timelineGroups = groupEntriesByDate(historyData?.logs || []);

  // Toggle entry expansion
  const toggleEntry = (entryId: string) => {
    const newExpanded = new Set(expandedEntries);
    if (newExpanded.has(entryId)) {
      newExpanded.delete(entryId);
    } else {
      newExpanded.add(entryId);
    }
    setExpandedEntries(newExpanded);
  };

  // Toggle date expansion
  const toggleDate = (date: string) => {
    const newExpanded = new Set(expandedDates);
    if (newExpanded.has(date)) {
      newExpanded.delete(date);
    } else {
      newExpanded.add(date);
    }
    setExpandedDates(newExpanded);
  };

  // Get action icon
  const getActionIcon = (action: string) => {
    switch (action) {
      case 'install':
      case 'upgrade':
        return <Package className="h-4 w-4" />;
      case 'scan':
        return <Search className="h-4 w-4" />;
      case 'dry_run':
        return <Terminal className="h-4 w-4" />;
      default:
        return <Activity className="h-4 w-4" />;
    }
  };

  // Get result icon
  const getResultIcon = (result: string) => {
    switch (result) {
      case 'success':
        return <CheckCircle className="h-4 w-4 text-green-500" />;
      case 'failed':
        return <XCircle className="h-4 w-4 text-red-500" />;
      case 'running':
        return <RefreshCw className="h-4 w-4 text-blue-500 animate-spin" />;
      default:
        return <AlertTriangle className="h-4 w-4 text-yellow-500" />;
    }
  };

  // Get status color
  const getStatusColor = (result: string) => {
    switch (result) {
      case 'success':
        return 'text-green-700 bg-green-100 border-green-200';
      case 'failed':
        return 'text-red-700 bg-red-100 border-red-200';
      case 'running':
        return 'text-blue-700 bg-blue-100 border-blue-200';
      default:
        return 'text-gray-700 bg-gray-100 border-gray-200';
    }
  };

  // Format duration
  const formatDuration = (seconds: number) => {
    if (seconds < 60) {
      return `${seconds}s`;
    } else if (seconds < 3600) {
      const minutes = Math.floor(seconds / 60);
      const remainingSeconds = seconds % 60;
      return `${minutes}m ${remainingSeconds}s`;
    } else {
      const hours = Math.floor(seconds / 3600);
      const minutes = Math.floor((seconds % 3600) / 60);
      return `${hours}h ${minutes}m`;
    }
  };

  return (
    <div className={cn("space-y-6", className)}>
      {/* Header with search and filters */}
      <div className="bg-white rounded-lg border border-gray-200 shadow-sm p-4">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <Calendar className="h-5 w-5 text-gray-600" />
            <h3 className="text-lg font-medium text-gray-900">
              {agentId ? 'Agent History' : 'Universal Audit Log'}
            </h3>
          </div>
          <button
            onClick={() => refetch()}
            disabled={isFetching}
            className="flex items-center space-x-2 px-3 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 text-sm font-medium transition-colors disabled:opacity-50"
          >
            <RefreshCw className={cn("h-4 w-4", isFetching && "animate-spin")} />
            <span>Refresh</span>
          </button>
        </div>

        <div className="flex flex-col sm:flex-row gap-4">
          {/* Search */}
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search by action or result..."
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
            {(actionFilter !== 'all' || resultFilter !== 'all') && (
              <span className="bg-primary-100 text-primary-800 px-2 py-0.5 rounded-full text-xs">
                {[actionFilter, resultFilter].filter(f => f !== 'all').length}
              </span>
            )}
          </button>
        </div>

        {/* Filters */}
        {showFilters && (
          <div className="mt-4 pt-4 border-t border-gray-200 grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Action
              </label>
              <select
                value={actionFilter}
                onChange={(e) => setActionFilter(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              >
                <option value="all">All Actions</option>
                <option value="install">Install</option>
                <option value="upgrade">Upgrade</option>
                <option value="scan">Scan</option>
                <option value="dry_run">Dry Run</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Result
              </label>
              <select
                value={resultFilter}
                onChange={(e) => setResultFilter(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              >
                <option value="all">All Results</option>
                <option value="success">Success</option>
                <option value="failed">Failed</option>
                <option value="running">Running</option>
              </select>
            </div>
          </div>
        )}
      </div>

      {/* Loading state */}
      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <RefreshCw className="h-6 w-6 animate-spin text-gray-400" />
          <span className="ml-2 text-gray-600">Loading history...</span>
        </div>
      )}

      {/* Timeline */}
      {!isLoading && timelineGroups.length === 0 ? (
        <div className="text-center py-12">
          <Calendar className="mx-auto h-12 w-12 text-gray-400" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">No history found</h3>
          <p className="mt-1 text-sm text-gray-500">
            {searchQuery || actionFilter !== 'all' || resultFilter !== 'all'
              ? 'Try adjusting your search or filters.'
              : 'No activities have been recorded yet.'}
          </p>
        </div>
      ) : (
        <div className="space-y-6">
          {timelineGroups.map((group) => (
            <div key={group.date} className="bg-white rounded-lg border border-gray-200 shadow-sm overflow-hidden">
              {/* Date header */}
              <div
                className="px-4 py-3 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors"
                onClick={() => toggleDate(group.date)}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    {expandedDates.has(group.date) ? (
                      <ChevronDown className="h-4 w-4 text-gray-600" />
                    ) : (
                      <ChevronRight className="h-4 w-4 text-gray-600" />
                    )}
                    <h4 className="font-medium text-gray-900">{group.date}</h4>
                    <span className="text-sm text-gray-500">
                      ({group.entries.length} events)
                    </span>
                  </div>
                </div>
              </div>

              {/* Timeline entries */}
              {expandedDates.has(group.date) && (
                <div className="divide-y divide-gray-200">
                  {group.entries.map((entry) => (
                    <div key={entry.id} className="p-4">
                      <div className="flex items-start space-x-3">
                        {/* Timeline icon */}
                        <div className="flex-shrink-0 mt-1">
                          {getResultIcon(entry.result)}
                        </div>

                        {/* Entry content */}
                        <div className="flex-1 min-w-0">
                          <div
                            className="flex items-center justify-between cursor-pointer"
                            onClick={() => toggleEntry(entry.id)}
                          >
                            <div className="flex items-center space-x-2">
                              {getActionIcon(entry.action)}
                              <span className="font-medium text-gray-900 capitalize">
                                {entry.action}
                              </span>
                              <span className={cn(
                                "inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium border",
                                getStatusColor(entry.result)
                              )}>
                                {entry.result}
                              </span>
                            </div>
                            <div className="flex items-center space-x-4 text-sm text-gray-500">
                              <span>{formatRelativeTime(entry.executed_at)}</span>
                              <span>{formatDuration(entry.duration_seconds)}</span>
                            </div>
                          </div>

                          {/* Agent info */}
                          <div className="mt-1 flex items-center space-x-2 text-sm text-gray-600">
                            <Computer className="h-3 w-3" />
                            <span>Agent: {entry.agent_id}</span>
                          </div>

                          {/* Expanded details */}
                          {expandedEntries.has(entry.id) && (
                            <div className="mt-3 space-y-3">
                              {/* Metadata */}
                              <div className="grid grid-cols-2 gap-4 text-sm">
                                <div>
                                  <span className="font-medium text-gray-700">Exit Code:</span>
                                  <span className="ml-2">{entry.exit_code}</span>
                                </div>
                                <div>
                                  <span className="font-medium text-gray-700">Duration:</span>
                                  <span className="ml-2">{formatDuration(entry.duration_seconds)}</span>
                                </div>
                              </div>

                              {/* Output */}
                              {(entry.stdout || entry.stderr) && (
                                <div>
                                  <h5 className="text-sm font-medium text-gray-900 mb-2 flex items-center space-x-2">
                                    <Terminal className="h-4 w-4" />
                                    <span>Output</span>
                                  </h5>
                                  {entry.stdout && (
                                    <div className="bg-gray-900 text-green-400 p-3 rounded-md font-mono text-xs overflow-x-auto">
                                      <pre className="whitespace-pre-wrap">{entry.stdout}</pre>
                                    </div>
                                  )}
                                  {entry.stderr && (
                                    <div className="bg-gray-900 text-red-400 p-3 rounded-md font-mono text-xs overflow-x-auto mt-2">
                                      <pre className="whitespace-pre-wrap">{entry.stderr}</pre>
                                    </div>
                                  )}
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Pagination */}
      {historyData && historyData.total > historyData.page_size && (
        <div className="flex items-center justify-between bg-white px-4 py-3 border border-gray-200 rounded-lg shadow-sm">
          <div className="text-sm text-gray-700">
            Showing {((historyData.page - 1) * historyData.page_size) + 1} to{' '}
            {Math.min(historyData.page * historyData.page_size, historyData.total)} of{' '}
            {historyData.total} results
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setQueryParams(prev => ({ ...prev, page: Math.max(1, prev.page - 1) }))}
              disabled={historyData.page === 1}
              className="px-3 py-1 text-sm border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Previous
            </button>
            <span className="text-sm text-gray-700">
              Page {historyData.page} of {Math.ceil(historyData.total / historyData.page_size)}
            </span>
            <button
              onClick={() => setQueryParams(prev => ({ ...prev, page: prev.page + 1 }))}
              disabled={historyData.page >= Math.ceil(historyData.total / historyData.page_size)}
              className="px-3 py-1 text-sm border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export default HistoryTimeline;