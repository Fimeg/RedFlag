import React, { useState } from 'react';
import {
  Activity,
  AlertTriangle,
  XCircle,
  Download,
  Filter,
  Search,
  RefreshCw,
  Pause,
  Play,
  ChevronDown,
  Copy,
  Server,
  User,
  Tag,
  FileText,
  Info
} from 'lucide-react';
import { useSecurityEvents, useSecurityWebSocket } from '@/hooks/useSecuritySettings';
import { SecurityEvent, EventFilters } from '@/types/security';
import { clientLogger } from '@/lib/client-logger';
import { securityEventSeverityColor } from '@/components/primitives/statusColors';
import Modal from '@/components/primitives/Modal';

const SecurityEvents: React.FC = () => {
  const [filters, setFilters] = useState<EventFilters>({});
  const [selectedEvent, setSelectedEvent] = useState<SecurityEvent | null>(null);
  const [showFilterPanel, setShowFilterPanel] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 20;

  // Fetch events
  const { data: eventsData, isLoading, error, refetch } = useSecurityEvents(
    currentPage,
    pageSize,
    filters
  );

  // WebSocket for real-time updates
  const { events: liveEvents, connected } = useSecurityWebSocket();
  const [liveUpdates, setLiveUpdates] = useState(true);

  // Combine live events with paginated events
  const allEvents = React.useMemo(() => {
    const staticEvents = eventsData?.events || [];
    if (liveUpdates && liveEvents.length > 0) {
      // Merge live events, avoiding duplicates
      const existingIds = new Set(staticEvents.map(e => e.id));
      const newLiveEvents = liveEvents.filter(e => !existingIds.has(e.id));
      return [...newLiveEvents, ...staticEvents].slice(0, pageSize);
    }
    return staticEvents;
  }, [eventsData, liveEvents, liveUpdates, pageSize]);


  const getSeverityIcon = (severity: string) => {
    switch (severity) {
      case 'critical':
      case 'error':
        return <XCircle className="w-4 h-4" />;
      case 'warn':
        return <AlertTriangle className="w-4 h-4" />;
      case 'info':
        return <Info className="w-4 h-4" />;
      default:
        return <Activity className="w-4 h-4" />;
    }
  };

  // toLocaleString() (no options) — produces date+time with seconds in the
  // browser's default locale. formatDateTime in utils pins en-US and drops
  // seconds, so they diverge. Keep local until the display format is aligned.
  const formatTimestamp = (timestamp: string) => new Date(timestamp).toLocaleString();

  // Copy event details to clipboard
  const copyEventDetails = (event: SecurityEvent) => {
    const details = JSON.stringify(event, null, 2);
    navigator.clipboard.writeText(details);
  };

  // Export events
  const exportEvents = async (format: 'json' | 'csv') => {
    // Implementation would call API to export events
    clientLogger.debug('Exporting events', { format });
  };

  // Clear filters
  const clearFilters = () => {
    setFilters({});
    setSearchTerm('');
    setCurrentPage(1);
  };

  // Apply filters
  const applyFilters = (newFilters: EventFilters) => {
    setFilters(newFilters);
    setCurrentPage(1);
    setShowFilterPanel(false);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="bg-white border border-gray-200 rounded-lg p-6">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-4">
            <h2 className="text-xl font-semibold text-gray-900">Security Events</h2>
            <div className="flex items-center gap-2">
              {connected ? (
                <div className="flex items-center gap-1 text-sm text-green-600">
                  <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
                  Live
                </div>
              ) : (
                <div className="flex items-center gap-1 text-sm text-gray-500">
                  <div className="w-2 h-2 bg-gray-400 rounded-full"></div>
                  Offline
                </div>
              )}
            </div>
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={() => setLiveUpdates(!liveUpdates)}
              className={`flex items-center gap-2 px-3 py-2 text-sm rounded-lg border ${
                liveUpdates
                  ? 'bg-green-50 text-green-700 border-green-200'
                  : 'bg-gray-50 text-gray-700 border-gray-200'
              }`}
            >
              {liveUpdates ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
              {liveUpdates ? 'Pause Updates' : 'Resume Updates'}
            </button>

            <button
              onClick={() => setShowFilterPanel(!showFilterPanel)}
              className={`flex items-center gap-2 px-3 py-2 text-sm rounded-lg border ${
                Object.keys(filters).length > 0
                  ? 'bg-blue-50 text-blue-700 border-blue-200'
                  : 'bg-gray-50 text-gray-700 border-gray-200'
              }`}
            >
              <Filter className="w-4 h-4" />
              Filters
              {Object.keys(filters).length > 0 && (
                <span className="badge badge-info badge-sm">
                  {Object.keys(filters).length}
                </span>
              )}
            </button>

            <div className="relative group">
              <button className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg border border-gray-200 bg-gray-50 text-gray-700 hover:bg-gray-100">
                <Download className="w-4 h-4" />
                Export
                <ChevronDown className="w-3 h-3" />
              </button>
              <div className="absolute right-0 mt-1 w-32 bg-white border border-gray-200 rounded-lg shadow-lg opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all z-10">
                <button
                  onClick={() => exportEvents('json')}
                  className="block w-full text-left px-3 py-2 text-sm hover:bg-gray-50 rounded-t-lg"
                >
                  Export as JSON
                </button>
                <button
                  onClick={() => exportEvents('csv')}
                  className="block w-full text-left px-3 py-2 text-sm hover:bg-gray-50 rounded-b-lg"
                >
                  Export as CSV
                </button>
              </div>
            </div>

            <button
              onClick={() => refetch()}
              disabled={isLoading}
              className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg border border-gray-200 bg-gray-50 text-gray-700 hover:bg-gray-100 disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>
        </div>

        {/* Search Bar */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            placeholder="Search events by message, agent ID, or user..."
            value={searchTerm}
            onChange={(e) => {
              setSearchTerm(e.target.value);
              if (e.target.value) {
                applyFilters({ ...filters, search: e.target.value });
              } else {
                const newFilters = { ...filters };
                delete newFilters.search;
                applyFilters(newFilters);
              }
            }}
            className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
      </div>

      {/* Filter Panel */}
      {showFilterPanel && (
        <div className="bg-white border border-gray-200 rounded-lg p-6">
          <h3 className="text-lg font-medium text-gray-900 mb-4">Filter Events</h3>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            {/* Severity Filter */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Severity
              </label>
              <div className="space-y-2">
                {['critical', 'error', 'warn', 'info'].map((severity) => (
                  <label key={severity} className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={filters.severity?.includes(severity) || false}
                      onChange={(e) => {
                        const current = filters.severity || [];
                        if (e.target.checked) {
                          applyFilters({
                            ...filters,
                            severity: [...current, severity],
                          });
                        } else {
                          applyFilters({
                            ...filters,
                            severity: current.filter(s => s !== severity),
                          });
                        }
                      }}
                      className="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                    />
                    <span className="text-sm capitalize">{severity}</span>
                  </label>
                ))}
              </div>
            </div>

            {/* Category Filter */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Category
              </label>
              <div className="space-y-2">
                {[
                  'command_signing',
                  'update_security',
                  'machine_binding',
                  'key_management',
                  'authentication',
                ].map((category) => (
                  <label key={category} className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={filters.category?.includes(category) || false}
                      onChange={(e) => {
                        const current = filters.category || [];
                        if (e.target.checked) {
                          applyFilters({
                            ...filters,
                            category: [...current, category],
                          });
                        } else {
                          applyFilters({
                            ...filters,
                            category: current.filter(c => c !== category),
                          });
                        }
                      }}
                      className="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
                    />
                    <span className="text-sm">
                      {category.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase())}
                    </span>
                  </label>
                ))}
              </div>
            </div>

            {/* Date Range Filter */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Date Range
              </label>
              <div className="space-y-2">
                <input
                  type="datetime-local"
                  value={filters.date_range?.start || ''}
                  onChange={(e) => {
                    applyFilters({
                      ...filters,
                      date_range: {
                        ...filters.date_range,
                        start: e.target.value,
                      },
                    });
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                />
                <input
                  type="datetime-local"
                  value={filters.date_range?.end || ''}
                  onChange={(e) => {
                    applyFilters({
                      ...filters,
                      date_range: {
                        start: filters.date_range?.start || '',
                        end: e.target.value,
                      },
                    });
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
                />
              </div>
            </div>

            {/* Agent/User Filter */}
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Agent / User
              </label>
              <input
                type="text"
                placeholder="Agent ID or User ID"
                value={filters.agent_id || filters.user_id || ''}
                onChange={(e) => {
                  applyFilters({
                    ...filters,
                    agent_id: e.target.value || undefined,
                    user_id: e.target.value || undefined,
                  });
                }}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
              />
            </div>
          </div>

          <div className="flex justify-end gap-2 mt-6">
            <button
              onClick={clearFilters}
              className="px-4 py-2 text-sm text-gray-700 bg-gray-100 rounded-lg hover:bg-gray-200"
            >
              Clear Filters
            </button>
            <button
              onClick={() => setShowFilterPanel(false)}
              className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700"
            >
              Apply Filters
            </button>
          </div>
        </div>
      )}

      {/* Events List */}
      <div className="bg-white border border-gray-200 rounded-lg">
        {isLoading && allEvents.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
          </div>
        ) : error ? (
          <div className="p-6 text-center">
            <AlertTriangle className="w-12 h-12 text-red-500 mx-auto mb-4" />
            <p className="text-red-600">Failed to load security events</p>
            <button
              onClick={() => refetch()}
              className="mt-2 text-blue-600 hover:text-blue-800"
            >
              Try again
            </button>
          </div>
        ) : allEvents.length === 0 ? (
          <div className="p-6 text-center">
            <Activity className="w-12 h-12 text-gray-400 mx-auto mb-4" />
            <p className="text-gray-600">No security events found</p>
            {Object.keys(filters).length > 0 && (
              <button
                onClick={clearFilters}
                className="mt-2 text-blue-600 hover:text-blue-800"
              >
                Clear filters
              </button>
            )}
          </div>
        ) : (
          <div className="divide-y divide-gray-200">
            {allEvents.map((event) => (
              <div
                key={event.id}
                className="p-4 hover:bg-gray-50 cursor-pointer"
                onClick={() => setSelectedEvent(event)}
              >
                <div className="flex items-start gap-4">
                  <div className={`p-2 rounded-lg border ${securityEventSeverityColor(event.severity)}`}>
                    {getSeverityIcon(event.severity)}
                  </div>

                  <div className="flex-1 min-w-0">
                    <div className="flex items-start justify-between mb-2">
                      <div>
                        <p className="text-sm font-medium text-gray-900 mb-1">
                          {event.event_type.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase())}
                        </p>
                        <p className="text-sm text-gray-600">{event.message}</p>
                      </div>
                      <span className="text-xs text-gray-500 whitespace-nowrap ml-4">
                        {formatTimestamp(event.timestamp)}
                      </span>
                    </div>

                    <div className="flex items-center gap-4 text-xs text-gray-500">
                      <span className="flex items-center gap-1">
                        <Tag className="w-3 h-3" />
                        {event.category.replace('_', ' ')}
                      </span>
                      {event.agent_id && (
                        <span className="flex items-center gap-1">
                          <Server className="w-3 h-3" />
                          {event.agent_id}
                        </span>
                      )}
                      {event.user_id && (
                        <span className="flex items-center gap-1">
                          <User className="w-3 h-3" />
                          {event.user_id}
                        </span>
                      )}
                      {event.trace_id && (
                        <span className="flex items-center gap-1">
                          <FileText className="w-3 h-3" />
                          {event.trace_id.substring(0, 8)}...
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Pagination */}
        {eventsData && eventsData.total > pageSize && (
          <div className="p-4 border-t border-gray-200 flex items-center justify-between">
            <p className="text-sm text-gray-600">
              Showing {(currentPage - 1) * pageSize + 1} to{' '}
              {Math.min(currentPage * pageSize, eventsData.total)} of {eventsData.total} events
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
                disabled={currentPage === 1}
                className="px-3 py-1 text-sm border rounded-lg disabled:opacity-50"
              >
                Previous
              </button>
              <button
                onClick={() => setCurrentPage(currentPage + 1)}
                disabled={currentPage * pageSize >= eventsData.total}
                className="px-3 py-1 text-sm border rounded-lg disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Event Detail Modal */}
      <Modal
        open={selectedEvent !== null}
        onClose={() => setSelectedEvent(null)}
        title="Event Details"
        maxWidth="2xl"
        maxHeight="80vh"
      >
        <Modal.Body scrollable>
          {selectedEvent && (
            <div className="space-y-4">
              {/* Event Header */}
              <div className="flex items-start gap-4 pb-4 border-b">
                <div className={`p-2 rounded-lg border ${securityEventSeverityColor(selectedEvent.severity)}`}>
                  {getSeverityIcon(selectedEvent.severity)}
                </div>
                <div className="flex-1">
                  <p className="font-medium text-gray-900 mb-1">
                    {selectedEvent.event_type.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase())}
                  </p>
                  <p className="text-sm text-gray-600">{selectedEvent.message}</p>
                  <p className="text-xs text-gray-500 mt-2">
                    {formatTimestamp(selectedEvent.timestamp)}
                  </p>
                </div>
              </div>

              {/* Event Information */}
              <div className="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <p className="font-medium text-gray-900">Severity</p>
                  <p className="capitalize">{selectedEvent.severity}</p>
                </div>
                <div>
                  <p className="font-medium text-gray-900">Category</p>
                  <p className="capitalize">{selectedEvent.category.replace('_', ' ')}</p>
                </div>
                {selectedEvent.agent_id && (
                  <div>
                    <p className="font-medium text-gray-900">Agent ID</p>
                    <p className="font-mono text-xs">{selectedEvent.agent_id}</p>
                  </div>
                )}
                {selectedEvent.user_id && (
                  <div>
                    <p className="font-medium text-gray-900">User ID</p>
                    <p className="font-mono text-xs">{selectedEvent.user_id}</p>
                  </div>
                )}
                {selectedEvent.trace_id && (
                  <div className="col-span-2">
                    <p className="font-medium text-gray-900">Trace ID</p>
                    <p className="font-mono text-xs">{selectedEvent.trace_id}</p>
                  </div>
                )}
              </div>

              {/* Event Details */}
              {Object.keys(selectedEvent.details).length > 0 && (
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <p className="font-medium text-gray-900">Additional Details</p>
                    <button
                      onClick={() => copyEventDetails(selectedEvent)}
                      className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-800"
                    >
                      <Copy className="w-3 h-3" />
                      Copy
                    </button>
                  </div>
                  <pre className="p-3 bg-gray-50 rounded border text-xs overflow-auto max-h-48">
                    {JSON.stringify(selectedEvent.details, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
        </Modal.Body>
      </Modal>
    </div>
  );
};

export default SecurityEvents;