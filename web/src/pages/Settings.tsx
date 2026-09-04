import React from 'react';
import { Link } from 'react-router-dom';
import {
  Shield,
  Lock,
  SlidersHorizontal,
  ArrowRight,
  CheckCircle,
  Activity,
  Clock,
  GitBranch,
  RadioTower,
} from 'lucide-react';
import { useRegistrationTokenStats } from '../hooks/useRegistrationTokens';
import { useRateLimitStats } from '../hooks/useRateLimits';

const Settings: React.FC = () => {
  // Statistics for overview
  const { data: tokenStats } = useRegistrationTokenStats();
  const { data: rateLimitStats } = useRateLimitStats();

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
          to="/settings/general"
          className="card block hover:border-gray-400 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <SlidersHorizontal className="w-8 h-8 text-gray-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">General</h3>
          <p className="text-sm text-gray-600 mt-1">Display preferences and dashboard behavior</p>
        </Link>

        <Link
          to="/settings/agents"
          className="card block hover:border-blue-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Shield className="w-8 h-8 text-blue-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Agents &amp; Enrollment</h3>
          <p className="text-sm text-gray-600 mt-1">Deploy agents, manage keys and revocation</p>
        </Link>

        <Link
          to="/settings/rate-limiting"
          className="card block hover:border-green-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Activity className="w-8 h-8 text-green-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Rate Limiting</h3>
          <p className="text-sm text-gray-600 mt-1">Configure API rate limits</p>
        </Link>

        <Link
          to="/settings/polling"
          className="card block hover:border-amber-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <RadioTower className="w-8 h-8 text-amber-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Agent Polling</h3>
          <p className="text-sm text-gray-600 mt-1">Fleet check-in jitter and reconnect backoff</p>
        </Link>

        <Link
          to="/settings/security"
          className="card block hover:border-red-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Lock className="w-8 h-8 text-red-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Security</h3>
          <p className="text-sm text-gray-600 mt-1">Command signing, machine binding, key management</p>
        </Link>

        <Link
          to="/settings/maintenance-windows"
          className="card block hover:border-indigo-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Clock className="w-8 h-8 text-indigo-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Maintenance Windows</h3>
          <p className="text-sm text-gray-600 mt-1">Schedule update installation windows</p>
        </Link>

        <Link
          to="/settings/upstream"
          className="card block hover:border-teal-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <GitBranch className="w-8 h-8 text-teal-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Upstream Tracking</h3>
          <p className="text-sm text-gray-600 mt-1">Compare deployed versions to canonical upstream releases</p>
        </Link>

        <Link
          to="/settings/process-explorer"
          className="card block hover:border-cyan-300 hover:shadow-sm transition-all"
        >
          <div className="flex items-center justify-between mb-4">
            <Activity className="w-8 h-8 text-cyan-600" />
            <ArrowRight className="w-5 h-5 text-gray-400" />
          </div>
          <h3 className="font-semibold text-gray-900">Process Explorer</h3>
          <p className="text-sm text-gray-600 mt-1">Data collection caps for process drill-down scans</p>
        </Link>
      </div>

      {/* Overview Statistics */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
        {/* Token Overview */}
        <div className="bg-white border border-gray-200 rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-gray-900">Registration Keys</h2>
            <Link
              to="/settings/agents"
              className="text-blue-600 hover:text-blue-800 text-sm font-medium"
            >
              Manage all →
            </Link>
          </div>
          {tokenStats ? (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-2xl font-bold text-gray-900">{tokenStats.total_tokens}</p>
                <p className="text-sm text-gray-600">Total Keys</p>
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
              <p className="text-sm text-gray-500 mt-2">Loading key statistics...</p>
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
          {rateLimitStats ? (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <p className="text-2xl font-bold text-gray-900">{rateLimitStats.total_configured_limits}</p>
                <p className="text-sm text-gray-600">Categories</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-green-600">{rateLimitStats.enabled_limits}</p>
                <p className="text-sm text-gray-600">Enabled</p>
              </div>
              <div>
                <p className="text-2xl font-bold text-blue-600">
                  {rateLimitStats.total_requests_per_minute}
                </p>
                <p className="text-sm text-gray-600">Total Requests / Window</p>
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <CheckCircle className="w-5 h-5 text-green-600" />
                  <p className="text-lg font-bold text-green-600">Active</p>
                </div>
                <p className="text-sm text-gray-600">In-memory counters</p>
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

    </div>
  );
};

export default Settings;
