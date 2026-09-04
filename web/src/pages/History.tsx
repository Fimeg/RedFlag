import React, { useState } from 'react';

import { History, Filter } from 'lucide-react';
import ChatTimeline from '@/components/ChatTimeline';
import { SearchInput, FilterBar } from '@/components/primitives';
import { useDebounce } from '@/hooks/useDebounce';
import { useFilterUrl, buildFilterPills } from '@/hooks/useFilterUrl';

const EVENT_TYPES = [
  { value: '', label: 'All types' },
  { value: 'command', label: 'Commands' },
  { value: 'system_event', label: 'System events' },
  { value: 'package_event', label: 'Package events' },
  { value: 'install_transition', label: 'Install transitions' },
  { value: 'log', label: 'Logs' },
];

const SEVERITIES = [
  { value: '', label: 'All severity' },
  { value: 'info', label: 'Info' },
  { value: 'warning', label: 'Warning' },
  { value: 'error', label: 'Error' },
  { value: 'critical', label: 'Critical' },
];

const HistoryPage: React.FC = () => {
  const [searchQuery, setSearchQuery] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const debouncedSearch = useDebounce(searchQuery, 300);
  const filterConfig = {
    type: { urlParam: 'type', label: 'Type' },
    severity: { urlParam: 'severity', label: 'Severity' },
  };
  const filter = useFilterUrl(filterConfig);

  return (
    <div className="mb-6">
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center space-x-3">
            <History className="h-8 w-8 text-indigo-600" />
            <h1 className="text-2xl font-bold text-gray-900">History & Audit Log</h1>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setShowFilters(!showFilters)}
              className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-sm border rounded-md transition-colors ${
                showFilters || filter.activeCount > 0
                  ? 'bg-indigo-50 border-indigo-300 text-indigo-700'
                  : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
              }`}
            >
              <Filter className="h-3.5 w-3.5" />
              Filters
              {filter.activeCount > 0 && (
                <span className="ml-1 inline-flex items-center justify-center w-5 h-5 text-[10px] font-medium bg-indigo-600 text-white rounded-full">
                  {filter.activeCount}
                </span>
              )}
            </button>
            <SearchInput
              value={searchQuery}
              onChange={setSearchQuery}
              placeholder="Search events..."
              className="w-64"
            />
          </div>
        </div>
        <p className="text-gray-600">
          Command history, package events, system activity, and client errors across all agents
        </p>

      {/* Filter bar — toggled via the Filters button */}
      {showFilters && (
        <FilterBar
          filters={[
            { label: 'Type', value: filter.values.type, onChange: (v) => filter.setFilter('type', v), options: EVENT_TYPES, placeholder: 'All types' },
            { label: 'Severity', value: filter.values.severity, onChange: (v) => filter.setFilter('severity', v), options: SEVERITIES, placeholder: 'All severity' },
          ]}
          pills={buildFilterPills(filter, filterConfig)}
          onClearAll={() => filter.clearAll()}
          activeCount={filter.activeCount}
          className="mb-4 p-3 bg-gray-50 border border-gray-200 rounded-md"
        />
      )}

      {/* Timeline */}
      <ChatTimeline
        externalSearch={debouncedSearch}
        externalType={filter.values.type}
        externalSeverity={filter.values.severity}
      />
    </div>
  );
};

export default HistoryPage;
