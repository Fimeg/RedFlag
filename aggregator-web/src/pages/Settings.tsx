import React from 'react';
import { Link } from 'react-router-dom';
import {
  Clock,
  User,
  Shield,
  Server,
  Settings as SettingsIcon,
  ArrowRight,
  AlertTriangle,
  CheckCircle,
  Activity
} from 'lucide-react';
import { useSettingsStore } from '@/lib/store';
import { useTimezones, useTimezone, useUpdateTimezone } from '../hooks/useSettings';
import { useRegistrationTokenStats } from '../hooks/useRegistrationTokens';
import { useRateLimitSummary } from '../hooks/useRateLimits';
import { formatDateTime } from '@/lib/utils';

const Settings: React.FC = () => {
  const { autoRefresh, refreshInterval, setAutoRefresh, setRefreshInterval } = useSettingsStore();

  // Timezone settings
  const { data: timezones, isLoading: isLoadingTimezones } = useTimezones();
  const { data: currentTimezone, isLoading: isLoadingCurrentTimezone } = useTimezone();
  const updateTimezone = useUpdateTimezone();
  const [selectedTimezone, setSelectedTimezone] = React.useState('');

  // Statistics for overview
  const { data: tokenStats } = useRegistrationTokenStats();
  const { data: rateLimitSummary } = useRateLimitSummary();

  React.useEffect(() => {
    if (currentTimezone?.timezone) {
      setSelectedTimezone(currentTimezone.timezone);
    }
  }, [currentTimezone]);

  const handleTimezoneChange = async (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newTimezone = e.target.value;
    setSelectedTimezone(newTimezone);
    try {
      await updateTimezone.mutateAsync(newTimezone);
    } catch (error) {
      console.error('Failed to update timezone:', error);
    }
  };

  const overviewCards = [
    {
      title: 'Registration Tokens',
      description: 'Create and manage agent registration tokens',
      icon: Shield,
      href: '/settings/tokens',
      stats: tokenStats ? {
        total: tokenStats.total_tokens,
        active: tokenStats.active_tokens,
        used: tokenStats.used_tokens,
        color: 'blue'
      } : null,
      status: 'implemented'
    },
    {
      title: 'Rate Limiting',
      description: 'Configure API rate limits and monitor usage',
      icon: Activity,
      href: '/settings/rate-limiting',
      stats: rateLimitSummary ? {
        active: rateLimitSummary.active_endpoints,
        total: rateLimitSummary.total_endpoints,
        utilization: Math.round(rateLimitSummary.average_utilization),
        color: 'green'
      } : null,
      status: 'implemented'
    },
    {
      title: 'System Configuration',
      description: 'Server settings and performance tuning',
      icon: Server,
      href: '/settings/system',
      stats: null,
      status: 'not-implemented'
    },
    {
      title: 'Agent Management',
      description: 'Deploy and configure agents across platforms',
      icon: SettingsIcon,
      href: '/settings/agents',
      stats: null,
      status: 'implemented'
    }
  ];

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Settings</h1>
        <p className="mt-2 text-gray-600">Configure your RedFlag deployment and system preferences</p>
      </div>

      {/* Quick Actions */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <Link
          to="/settings/tokens"
          className="block p-6 bg-white border border-gray-200 rounded-lg hover:border-blue-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Shield className="w-8 h-8 text-blue-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Registration Tokens</h3>
          <p className="text-sm text-gray-600 mt-1">Manage agent registration tokens</p>
        </Link>

        <Link
          to="/settings/rate-limiting"
          className="block p-6 bg-white border border-gray-200 rounded-lg hover:border-green-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Activity className="w-8 h-8 text-green-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Rate Limiting</h3>
          <p className="text-sm text-gray-600 mt-1">Configure API rate limits</p>
        </Link>

        <div className="p-6 bg-gray-50 border border-gray-200 rounded-lg opacity-60">
          <div className="flex items-center justify-between mb-4">
            <Server className="w-8 h-8 text-gray-400" />
            <ArrowRight className="w-5 h-5 text-gray-300" />
          </div>
          <h3 className="font-semibold text-gray-500">System Configuration</h3>
          <p className="text-sm text-gray-400 mt-1">Coming soon</p>
        </div>

        <Link
          to="/settings/agents"
          className="block p-6 bg-white border border-gray-200 rounded-lg hover:border-purple-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <SettingsIcon className="w-8 h-8 text-purple-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Agent Management</h3>
          <p className="text-sm text-gray-600 mt-1">Deploy and configure agents</p>
        </Link>
      </div>

      {/* Overview Statistics */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
        {/* Token Overview */}
        <div className="bg-white border border-gray-200 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-gray-900">Token Overview</h2>
            <Link
              to="/settings/tokens"
              className="text-blue-600 hover:text-blue-800 text-sm font-medium"
            >
              Manage all →
            </Link>
          </div>
          {tokenStats ? (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-2xl font-bold text-gray-900">{tokenStats.total_tokens}</p>
                <p className="text-sm text-gray-600">Total Tokens</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-green-600">{tokenStats.active_tokens}</p>
                <p className="text-sm text-gray-600">Active</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-blue-600">{tokenStats.used_tokens}</p>
                <p className="text-sm text-gray-600">Used</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-gray-600">{tokenStats.expired_tokens}</p>
                <p className="text-sm text-gray-600">Expired</p>
              </div>
            </div>
          ) : (
            <div className="text-center py-4">
              <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600 mx-auto"></div>
              <p className="text-sm text-gray-500 mt-2">Loading token statistics...</p>
            </div>
          )}
        </div>

        {/* Rate Limiting Overview */}
        <div className="bg-white border border-gray-200 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-gray-900">Rate Limiting Status</h2>
            <Link
              to="/settings/rate-limiting"
              className="text-blue-600 hover:text-blue-800 text-sm font-medium"
            >
              Configure →
            </Link>
          </div>
          {rateLimitSummary ? (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-2xl font-bold text-gray-900">{rateLimitSummary.active_endpoints}</p>
                <p className="text-sm text-gray-600">Active Endpoints</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-green-600">
                  {rateLimitSummary.total_requests_per_minute}
                </p>
                <p className="text-sm text-gray-600">Requests/Min</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-blue-600">
                  {Math.round(rateLimitSummary.average_utilization)}%
                </p>
                <p className="text-sm text-gray-600">Avg Utilization</p>
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <CheckCircle className="w-5 h-5 text-green-600" />
                  <p className="text-lg font-bold text-green-600">Enabled</p>
                </div>
                <p className="text-sm text-gray-600">System Protected</p>
              </div>
            </div>
          ) : (
            <div className="text-center py-4">
              <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600 mx-auto"></div>
              <p className="text-sm text-gray-500 mt-2">Loading rate limit status...</p>
            </div>
          )}
        </div>
      </div>

      {/* Account Settings */}
      <div className="bg-white border border-gray-200 rounded-lg p-6 mb-8">
        <h2 className="text-xl font-semibold text-gray-900 mb-6 pb-2 border-b border-gray-200">Account Settings</h2>

        <div className="space-y-8">
          {/* Display Preferences */}
          <div>
            <h3 className="text-lg font-medium text-gray-900 mb-4">Display Preferences</h3>

            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Timezone
                <span className="ml-1 text-xs text-gray-500">(Note: Changes apply to current session only)</span>
              </label>
              <select
                value={selectedTimezone}
                onChange={handleTimezoneChange}
                disabled={isLoadingTimezones || updateTimezone.isPending}
                className="w-full md:w-64 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {isLoadingTimezones ? (
                  <option>Loading...</option>
                ) : (
                  timezones?.map((tz) => (
                    <option key={tz.value} value={tz.value}>{tz.label}</option>
                  ))
                )}
              </select>
              {updateTimezone.isPending && (
                <p className="mt-2 text-sm text-blue-600">Updating timezone...</p>
              )}
              {updateTimezone.isSuccess && (
                <p className="mt-2 text-sm text-green-600">Timezone updated successfully</p>
              )}
              {updateTimezone.isError && (
                <p className="mt-2 text-sm text-red-600">Failed to update timezone</p>
              )}
            </div>
          </div>

          {/* Dashboard Behavior */}
          <div>
            <h3 className="text-lg font-medium text-gray-900 mb-4">Dashboard Behavior</h3>
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium text-gray-900">Auto-refresh</div>
                  <div className="text-sm text-gray-600">Automatically refresh dashboard data</div>
                </div>
                <button
                  onClick={() => setAutoRefresh(!autoRefresh)}
                  className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors ${
                    autoRefresh ? 'bg-blue-600' : 'bg-gray-200'
                  }`}
                >
                  <span className={`translate-x-${autoRefresh ? '5' : '0'} inline-block h-5 w-5 transform rounded-full bg-white transition`} />
                </button>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Refresh Interval</label>
                <select
                  value={refreshInterval}
                  onChange={(e) => setRefreshInterval(Number(e.target.value))}
                  disabled={!autoRefresh}
                  className="w-full md:w-64 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50"
                >
                  <option value={10000}>10 seconds</option>
                  <option value={30000}>30 seconds</option>
                  <option value={60000}>1 minute</option>
                  <option value={300000}>5 minutes</option>
                  <option value={600000}>10 minutes</option>
                </select>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Implementation Status */}
      <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-6">
        <h2 className="text-lg font-semibold text-yellow-800 mb-4">Implementation Status</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div>
            <h3 className="font-medium text-yellow-800 mb-3">✅ Implemented Features</h3>
            <ul className="space-y-1 text-sm text-yellow-700">
              <li>• Registration token management (full CRUD)</li>
              <li>• API rate limiting configuration</li>
              <li>• Real-time usage monitoring</li>
              <li>• User preferences (timezone, dashboard)</li>
            </ul>
          </div>
          <div>
            <h3 className="font-medium text-yellow-800 mb-3">🚧 Planned Features</h3>
            <ul className="space-y-1 text-sm text-yellow-700">
              <li>• System configuration management</li>
              <li>• Integration with third-party services</li>
              <li>• Persistent settings storage</li>
            </ul>
          </div>
        </div>
        <p className="mt-4 text-xs text-yellow-600">
          This settings page reflects the current state of the RedFlag backend API.
        </p>
      </div>
    </div>
  );
};

export default Settings;