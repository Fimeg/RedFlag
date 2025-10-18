import React, { useState } from 'react';
import {
  History,
  Search,
  RefreshCw,
} from 'lucide-react';
import ChatTimeline from '@/components/ChatTimeline';
import { useQuery } from '@tanstack/react-query';
import { logApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn } from '@/lib/utils';

const HistoryPage: React.FC = () => {
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');

  // Debounce search query
  React.useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery);
    }, 300);

    return () => {
      clearTimeout(timer);
    };
  }, [searchQuery]);

  const { data: historyData, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['history', { search: debouncedSearchQuery }],
    queryFn: async () => {
      try {
        const params: any = {
          page: 1,
          page_size: 50,
        };

        if (debouncedSearchQuery) {
          params.search = debouncedSearchQuery;
        }

        const response = await logApi.getAllLogs(params);
        return response;
      } catch (error) {
        console.error('Failed to fetch history:', error);
        toast.error('Failed to fetch history');
        return { logs: [], total: 0, page: 1, page_size: 50 };
      }
    },
    refetchInterval: 30000,
  });

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center space-x-3">
            <History className="h-8 w-8 text-indigo-600" />
            <h1 className="text-2xl font-bold text-gray-900">History & Audit Log</h1>
          </div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search events..."
                className="pl-10 pr-4 py-2 w-64 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              />
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
        </div>
        <p className="text-gray-600">
          Complete chronological timeline of all system activities across all agents
        </p>
      </div>

      {/* Timeline */}
      <ChatTimeline isScopedView={false} externalSearch={debouncedSearchQuery} />
    </div>
  );
};

export default HistoryPage;