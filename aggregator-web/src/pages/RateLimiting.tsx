import React, { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Shield,
  RefreshCw,
  Save,
  RotateCcw,
  Activity,
  AlertTriangle,
  TrendingUp,
  BarChart3,
  Settings as SettingsIcon,
  Eye,
  Users,
  Search,
  Filter
} from 'lucide-react';
import {
  useRateLimitConfigs,
  useRateLimitStats,
  useRateLimitUsage,
  useRateLimitSummary,
  useUpdateAllRateLimitConfigs,
  useResetRateLimitConfigs,
  useCleanupRateLimits
} from '../hooks/useRateLimits';
import { RateLimitConfig, RateLimitStats, RateLimitUsage } from '@/types';

// Helper function to format date/time strings
const formatDateTime = (dateString: string): string => {
  try {
    const date = new Date(dateString);
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch (error) {
    return dateString;
  }
};

const RateLimiting: React.FC = () => {
  const navigate = useNavigate();
  const [editingMode, setEditingMode] = useState(false);
  const [editingConfigs, setEditingConfigs] = useState<RateLimitConfig[]>([]);
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Search and filter state
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all');

  // Queries
  const { data: configs, isLoading: isLoadingConfigs, refetch: refetchConfigs } = useRateLimitConfigs();
  const { data: stats, isLoading: isLoadingStats } = useRateLimitStats();
  const { data: usage, isLoading: isLoadingUsage } = useRateLimitUsage();
  const { data: summary, isLoading: isLoadingSummary } = useRateLimitSummary();

  // Mutations
  const updateAllConfigs = useUpdateAllRateLimitConfigs();
  const resetConfigs = useResetRateLimitConfigs();
  const cleanupLimits = useCleanupRateLimits();

  React.useEffect(() => {
    if (configs && Array.isArray(configs)) {
      setEditingConfigs([...configs]);
    }
  }, [configs]);

  // Filtered configurations for display
  const filteredConfigs = useMemo(() => {
    if (!configs || !Array.isArray(configs)) return [];

    return configs.filter((config) => {
      const matchesSearch = searchTerm === '' ||
        config.endpoint.toLowerCase().includes(searchTerm.toLowerCase()) ||
        config.method.toLowerCase().includes(searchTerm.toLowerCase());

      const matchesStatus = statusFilter === 'all' ||
        (statusFilter === 'enabled' && config.enabled) ||
        (statusFilter === 'disabled' && !config.enabled);

      return matchesSearch && matchesStatus;
    });
  }, [configs, searchTerm, statusFilter]);

  const handleConfigChange = (index: number, field: keyof RateLimitConfig, value: any) => {
    const updatedConfigs = [...editingConfigs];
    updatedConfigs[index] = { ...updatedConfigs[index], [field]: value };
    setEditingConfigs(updatedConfigs);
  };

  const handleSaveAllConfigs = () => {
    updateAllConfigs.mutate(editingConfigs, {
      onSuccess: () => {
        setEditingMode(false);
        refetchConfigs();
      }
    });
  };

  const handleResetConfigs = () => {
    if (confirm('Reset all rate limit configurations to defaults? This will overwrite your custom settings.')) {
      resetConfigs.mutate(undefined, {
        onSuccess: () => {
          setEditingMode(false);
          refetchConfigs();
        }
      });
    }
  };

  const handleCleanup = () => {
    if (confirm('Clean up expired rate limit data?')) {
      cleanupLimits.mutate(undefined, {
        onSuccess: () => {
          // Refetch stats and usage after cleanup
        }
      });
    }
  };

  const getUsagePercentage = (endpoint: string) => {
    const endpointUsage = usage?.find(u => u.endpoint === endpoint);
    if (!endpointUsage) return 0;
    return (endpointUsage.current / endpointUsage.limit) * 100;
  };

  const getUsageColor = (percentage: number) => {
    if (percentage >= 90) return 'text-red-600 bg-red-100';
    if (percentage >= 70) return 'text-yellow-600 bg-yellow-100';
    return 'text-green-600 bg-green-100';
  };

  const formatEndpointName = (endpoint: string) => {
    return endpoint.split('/').pop()?.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase()) || endpoint;
  };

  return (
    <div className="max-w-7xl mx-auto px-6 py-8">
      <button
        onClick={() => navigate('/settings')}
        className="text-sm text-gray-500 hover:text-gray-700 mb-4"
      >
        ← Back to Settings
      </button>

      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900">Rate Limiting</h1>
            <p className="mt-2 text-gray-600">Configure API rate limits and monitor system usage</p>
          </div>
          <div className="flex gap-3">
            <button
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="inline-flex items-center gap-2 px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50"
            >
              <SettingsIcon className="w-4 h-4" />
              {showAdvanced ? 'Simple View' : 'Advanced View'}
            </button>
            <button
              onClick={handleCleanup}
              disabled={cleanupLimits.isPending}
              className="inline-flex items-center gap-2 px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 ${cleanupLimits.isPending ? 'animate-spin' : ''}`} />
              Cleanup Data
            </button>
            <button
              onClick={() => refetchConfigs()}
              className="inline-flex items-center gap-2 px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50"
            >
              <RefreshCw className="w-4 h-4" />
              Refresh
            </button>
          </div>
        </div>
      </div>

      {/* Summary Cards */}
      {summary && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4 mb-8">
          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Active Endpoints</p>
                <p className="text-2xl font-bold text-gray-900">{summary.active_endpoints}</p>
                <p className="text-xs text-gray-500">of {summary.total_endpoints} total</p>
              </div>
              <Shield className="w-8 h-8 text-blue-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Total Requests/Min</p>
                <p className="text-2xl font-bold text-gray-900">{summary.total_requests_per_minute}</p>
              </div>
              <Activity className="w-8 h-8 text-green-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Avg Utilization</p>
                <p className="text-2xl font-bold text-blue-600">
                  {Math.round(summary.average_utilization)}%
                </p>
              </div>
              <BarChart3 className="w-8 h-8 text-purple-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Most Active</p>
                <p className="text-lg font-bold text-gray-900 truncate">
                  {formatEndpointName(summary.most_active_endpoint)}
                </p>
              </div>
              <TrendingUp className="w-8 h-8 text-orange-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Status</p>
                <p className="text-lg font-bold text-green-600">Enabled</p>
              </div>
              <Shield className="w-8 h-8 text-green-600" />
            </div>
          </div>
        </div>
      )}

      {/* Controls */}
      {(editingMode || editingConfigs.length > 0) && (
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
          <div className="flex items-center justify-between">
            <p className="text-sm text-blue-800">
              You have unsaved changes. Click "Save All Changes" to apply them.
            </p>
            <div className="flex gap-2">
              <button
                onClick={handleSaveAllConfigs}
                disabled={updateAllConfigs.isPending}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
              >
                <Save className="w-4 h-4 inline mr-1" />
                {updateAllConfigs.isPending ? 'Saving...' : 'Save All Changes'}
              </button>
              <button
                onClick={() => {
                  if (configs && Array.isArray(configs)) {
                    setEditingConfigs([...configs]);
                  }
                  setEditingMode(false);
                }}
                className="px-4 py-2 bg-gray-200 text-gray-800 rounded-lg hover:bg-gray-300"
              >
                Discard Changes
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Rate Limit Configurations */}
      <div className="bg-white rounded-lg border border-gray-200 mb-8">
        <div className="px-6 py-4 border-b border-gray-200">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-gray-900">Rate Limit Configurations</h2>
            <div className="flex gap-2">
              {!editingMode && (
                <button
                  onClick={() => setEditingMode(true)}
                  className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
                >
                  <SettingsIcon className="w-4 h-4 inline mr-1" />
                  Edit All
                </button>
              )}
              <button
                onClick={handleResetConfigs}
                disabled={resetConfigs.isPending}
                className="px-4 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 disabled:opacity-50"
              >
                <RotateCcw className="w-4 h-4 inline mr-1" />
                Reset to Defaults
              </button>
            </div>
          </div>
        </div>

        {/* Search and Filter Controls */}
        <div className="bg-white border border-gray-200 rounded-lg p-4">
          <div className="flex flex-col lg:flex-row gap-4">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 w-4 h-4" />
                <input
                  type="text"
                  placeholder="Search by endpoint or method..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </div>

            <div className="flex gap-2">
              <button
                onClick={() => setStatusFilter('all')}
                className={`px-4 py-2 rounded-lg transition-colors ${
                  statusFilter === 'all'
                    ? 'bg-gray-100 text-gray-800 border border-gray-300'
                    : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
                }`}
              >
                All
              </button>
              <button
                onClick={() => setStatusFilter('enabled')}
                className={`px-4 py-2 rounded-lg transition-colors ${
                  statusFilter === 'enabled'
                    ? 'bg-green-100 text-green-800 border border-green-300'
                    : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
                }`}
              >
                Enabled
              </button>
              <button
                onClick={() => setStatusFilter('disabled')}
                className={`px-4 py-2 rounded-lg transition-colors ${
                  statusFilter === 'disabled'
                    ? 'bg-red-100 text-red-800 border border-red-300'
                    : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
                }`}
              >
                Disabled
              </button>
            </div>
          </div>

          {/* Filter results summary */}
          {configs && (
            <div className="mt-3 text-sm text-gray-600">
              Showing {filteredConfigs.length} of {configs.length} configurations
            </div>
          )}
        </div>

        {filteredConfigs.length > 0 ? (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Endpoint
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Current Usage
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Requests/Min
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Window (min)
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Max Requests
                  </th>
                  {showAdvanced && (
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                      Burst Allowance
                    </th>
                  )}
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {filteredConfigs.map((config) => {
                  const originalIndex = editingConfigs.findIndex(c => c.endpoint === config.endpoint);
                  const usagePercentage = getUsagePercentage(config.endpoint);
                  const endpointUsage = usage?.find(u => u.endpoint === config.endpoint);

                  return (
                    <tr key={config.endpoint} className="hover:bg-gray-50">
                      <td className="px-6 py-4 whitespace-nowrap">
                        <div className="text-sm font-medium text-gray-900">
                          {formatEndpointName(config.endpoint)}
                        </div>
                        <div className="text-xs text-gray-500">
                          {config.endpoint}
                        </div>
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap">
                        {endpointUsage && (
                          <div>
                            <div className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${getUsageColor(usagePercentage)}`}>
                              <div className={`w-2 h-2 rounded-full mr-1 ${
                                usagePercentage >= 90 ? 'bg-red-500' :
                                usagePercentage >= 70 ? 'bg-yellow-500' : 'bg-green-500'
                              }`}></div>
                              {endpointUsage.current} / {endpointUsage.limit}
                              ({Math.round(usagePercentage)}%)
                            </div>
                            <div className="w-full bg-gray-200 rounded-full h-2 mt-2">
                              <div
                                className={`h-2 rounded-full transition-all ${
                                  usagePercentage >= 90 ? 'bg-red-500' :
                                  usagePercentage >= 70 ? 'bg-yellow-500' : 'bg-green-500'
                                }`}
                                style={{ width: `${Math.min(usagePercentage, 100)}%` }}
                              ></div>
                            </div>
                            {endpointUsage && (
                              <div className="flex items-center gap-2 mt-1">
                                <Eye className="w-3 h-3 text-gray-400" />
                                <span className="text-xs text-gray-500">
                                  Window: {formatDateTime(endpointUsage.window_start)} - {formatDateTime(endpointUsage.window_end)}
                                </span>
                              </div>
                            )}
                          </div>
                        )}
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap">
                        {editingMode ? (
                          <input
                            type="number"
                            min="1"
                            value={config.requests_per_minute}
                            onChange={(e) => handleConfigChange(originalIndex, 'requests_per_minute', parseInt(e.target.value))}
                            className="w-24 px-3 py-1 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                          />
                        ) : (
                          <span className="text-sm text-gray-900">{config.requests_per_minute}</span>
                        )}
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap">
                        {editingMode ? (
                          <input
                            type="number"
                            min="1"
                            value={config.window_minutes}
                            onChange={(e) => handleConfigChange(originalIndex, 'window_minutes', parseInt(e.target.value))}
                            className="w-20 px-3 py-1 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                          />
                        ) : (
                          <span className="text-sm text-gray-900">{config.window_minutes}</span>
                        )}
                      </td>

                      <td className="px-6 py-4 whitespace-nowrap">
                        {editingMode ? (
                          <input
                            type="number"
                            min="1"
                            value={config.max_requests}
                            onChange={(e) => handleConfigChange(originalIndex, 'max_requests', parseInt(e.target.value))}
                            className="w-24 px-3 py-1 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                          />
                        ) : (
                          <span className="text-sm text-gray-900">{config.max_requests}</span>
                        )}
                      </td>

                      {showAdvanced && (
                        <td className="px-6 py-4 whitespace-nowrap">
                          {editingMode ? (
                            <input
                              type="number"
                              min="0"
                              value={config.burst_allowance}
                              onChange={(e) => handleConfigChange(originalIndex, 'burst_allowance', parseInt(e.target.value))}
                              className="w-24 px-3 py-1 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                            />
                          ) : (
                            <span className="text-sm text-gray-900">{config.burst_allowance}</span>
                          )}
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : configs && configs.length > 0 ? (
          <div className="p-12 text-center">
            <Activity className="w-16 h-16 text-gray-400 mx-auto mb-4" />
            <h3 className="text-lg font-medium text-gray-900 mb-2">No configurations found</h3>
            <p className="text-gray-600">
              {searchTerm || statusFilter !== 'all'
                ? 'Try adjusting your search or filter criteria'
                : 'No rate limit configurations available'}
            </p>
          </div>
        ) : (
          <div className="p-8 text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
            <p className="mt-2 text-gray-600">Loading rate limit configurations...</p>
          </div>
        )}
      </div>

      {/* Rate Limit Statistics */}
      {stats && Array.isArray(stats) && stats.length > 0 && (
        <div className="bg-white rounded-lg border border-gray-200">
          <div className="px-6 py-4 border-b border-gray-200">
            <h2 className="text-lg font-semibold text-gray-900">Rate Limit Statistics</h2>
          </div>
          <div className="p-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {stats.map((stat) => (
                <div key={stat.endpoint} className="border border-gray-200 rounded-lg p-4">
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-medium text-gray-900">
                      {formatEndpointName(stat.endpoint)}
                    </h4>
                    <Activity className="w-4 h-4 text-yellow-500" />
                  </div>

                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span className="text-gray-600">Current Requests:</span>
                      <span className="font-medium">{stat.current_requests}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Limit:</span>
                      <span className="font-medium">{stat.limit}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Blocked:</span>
                      <span className="font-medium text-red-600">{stat.blocked_requests}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-gray-600">Window:</span>
                      <span className="font-medium text-xs">
                        {new Date(stat.window_start).toLocaleTimeString()} - {new Date(stat.window_end).toLocaleTimeString()}
                      </span>
                    </div>
                  </div>

                  {stat.top_clients && Array.isArray(stat.top_clients) && stat.top_clients.length > 0 && (
                    <div className="mt-4 pt-3 border-t border-gray-200">
                      <p className="text-xs text-gray-600 mb-2">Top Clients:</p>
                      <div className="space-y-1">
                        {stat.top_clients.slice(0, 3).map((client, index) => (
                          <div key={index} className="flex justify-between text-xs">
                            <span className="text-gray-500 truncate mr-2">{client.identifier}</span>
                            <span className="font-medium">{client.request_count}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Usage Monitoring */}
      {usage && Array.isArray(usage) && usage.length > 0 && (
        <div className="bg-white rounded-lg border border-gray-200">
          <div className="px-6 py-4 border-b border-gray-200">
            <h2 className="text-lg font-semibold text-gray-900">Usage Monitoring</h2>
          </div>
          <div className="p-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {usage.map((endpointUsage) => (
                <div key={endpointUsage.endpoint} className="border border-gray-200 rounded-lg p-4">
                  <div className="flex items-center justify-between mb-3">
                    <h4 className="font-medium text-gray-900">
                      {formatEndpointName(endpointUsage.endpoint)}
                    </h4>
                    <BarChart3 className="w-4 h-4 text-blue-500" />
                  </div>

                  <div className="space-y-3">
                    <div>
                      <div className="flex justify-between text-sm mb-1">
                        <span className="text-gray-600">Usage</span>
                        <span className="font-medium">
                          {endpointUsage.current} / {endpointUsage.limit}
                        </span>
                      </div>
                      <div className="w-full bg-gray-200 rounded-full h-3">
                        <div
                          className={`h-3 rounded-full transition-all ${
                            (endpointUsage.current / endpointUsage.limit) * 100 >= 90 ? 'bg-red-500' :
                            (endpointUsage.current / endpointUsage.limit) * 100 >= 70 ? 'bg-yellow-500' : 'bg-green-500'
                          }`}
                          style={{ width: `${Math.min((endpointUsage.current / endpointUsage.limit) * 100, 100)}%` }}
                        ></div>
                      </div>
                    </div>

                    <div className="text-xs text-gray-600 space-y-1">
                      <div>Remaining: {endpointUsage.remaining} requests</div>
                      <div>Reset: {formatDateTime(endpointUsage.reset_time)}</div>
                      <div>Window: {endpointUsage.window_minutes} minutes</div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default RateLimiting;