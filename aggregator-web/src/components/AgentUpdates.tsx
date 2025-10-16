import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Search, Filter, Package, Clock, AlertTriangle } from 'lucide-react';
import { formatRelativeTime } from '@/lib/utils';
import { updateApi } from '@/lib/api';
import type { UpdatePackage } from '@/types';

interface AgentUpdatesProps {
  agentId: string;
}

interface AgentUpdateResponse {
  updates: UpdatePackage[];
  total: number;
}

export function AgentSystemUpdates({ agentId }: AgentUpdatesProps) {
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [searchTerm, setSearchTerm] = useState('');
  const { data: updateData, isLoading, error } = useQuery<AgentUpdateResponse>({
    queryKey: ['agent-updates', agentId, currentPage, pageSize, searchTerm],
    queryFn: async () => {
      const params = {
        page: currentPage,
        page_size: pageSize,
        agent: agentId,
        type: 'system', // Only show system updates in AgentUpdates
        ...(searchTerm && { search: searchTerm }),
      };

      const response = await updateApi.getUpdates(params);
      return response;
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

  const getPackageTypeIcon = (packageType: string) => {
    switch (packageType.toLowerCase()) {
      case 'system': return '📦';
      default: return '📋';
    }
  };

  if (isLoading) {
    return (
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <div className="animate-pulse">
          <div className="h-6 bg-gray-200 rounded w-1/4 mb-4"></div>
          <div className="space-y-3">
            {[...Array(5)].map((_, i) => (
              <div key={i} className="h-4 bg-gray-200 rounded"></div>
            ))}
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <div className="text-red-600 text-sm">Error loading updates: {(error as Error).message}</div>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200">
      {/* Header */}
      <div className="p-6 border-b border-gray-200">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900">System Updates</h2>
          <div className="text-sm text-gray-500">
            {totalCount} update{totalCount !== 1 ? 's' : ''} available
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className="p-4 border-b border-gray-200 bg-gray-50">
        <div className="flex flex-col sm:flex-row gap-4">
          {/* Search */}
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="text"
                placeholder="Search packages..."
                value={searchTerm}
                onChange={(e) => {
                  setSearchTerm(e.target.value);
                  setCurrentPage(1);
                }}
                className="pl-10 pr-4 py-2 w-full border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              />
            </div>
          </div>

          {/* Page Size */}
          <div className="sm:w-32">
            <select
              value={pageSize}
              onChange={(e) => {
                setPageSize(Number(e.target.value));
                setCurrentPage(1);
              }}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            >
              <option value={10}>10</option>
              <option value={20}>20</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
          </div>
        </div>
      </div>

      {/* Updates List */}
      <div className="divide-y divide-gray-200">
        {updates.length === 0 ? (
          <div className="p-8 text-center text-gray-500">
            <Package className="h-12 w-12 mx-auto mb-4 text-gray-300" />
            <p>No updates found</p>
            <p className="text-sm mt-2">This agent is up to date!</p>
          </div>
        ) : (
          updates.map((update) => (
            <div key={update.id} className="p-4 hover:bg-gray-50 transition-colors">
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-lg">{getPackageTypeIcon(update.package_type)}</span>
                    <h3 className="text-sm font-medium text-gray-900 truncate">
                      {update.package_name}
                    </h3>
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${getSeverityColor(update.severity)}`}>
                      {update.severity}
                    </span>
                  </div>

                  <div className="flex items-center gap-4 text-xs text-gray-500 mb-2">
                    <span>Type: {update.package_type}</span>
                    {update.repository_source && (
                      <span>Source: {update.repository_source}</span>
                    )}
                    <div className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      {formatRelativeTime(update.created_at)}
                    </div>
                  </div>

                  <div className="flex items-center gap-2 text-xs">
                    <span className="text-gray-600">From:</span>
                    <span className="font-mono bg-gray-100 px-1 py-0.5 rounded">
                      {update.current_version || 'N/A'}
                    </span>
                    <span className="text-gray-600">→</span>
                    <span className="font-mono bg-green-50 text-green-700 px-1 py-0.5 rounded">
                      {update.available_version}
                    </span>
                  </div>
                </div>

                <div className="flex items-center gap-2 ml-4">
                  <button
                    className="text-green-600 hover:text-green-800 text-sm font-medium"
                    onClick={() => {
                      // TODO: Implement install single update functionality
                      console.log('Install update:', update.id);
                    }}
                  >
                    Install
                  </button>
                  <button
                    className="text-blue-600 hover:text-blue-800 text-sm"
                    onClick={() => {
                      // TODO: Implement view logs functionality
                      console.log('View logs for update:', update.id);
                    }}
                  >
                    Logs
                  </button>
                </div>
              </div>
            </div>
          ))
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="p-4 border-t border-gray-200 bg-gray-50">
          <div className="flex items-center justify-between">
            <div className="text-sm text-gray-700">
              Showing {((currentPage - 1) * pageSize) + 1} to {Math.min(currentPage * pageSize, totalCount)} of {totalCount} results
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
                disabled={currentPage === 1}
                className="px-3 py-1 text-sm border border-gray-300 rounded-md hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Previous
              </button>
              <span className="px-3 py-1 text-sm text-gray-700">
                Page {currentPage} of {totalPages}
              </span>
              <button
                onClick={() => setCurrentPage(Math.min(totalPages, currentPage + 1))}
                disabled={currentPage === totalPages}
                className="px-3 py-1 text-sm border border-gray-300 rounded-md hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Next
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}