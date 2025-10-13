import React from 'react';
import { Link } from 'react-router-dom';
import {
  Computer,
  Package,
  CheckCircle,
  AlertTriangle,
  XCircle,
  RefreshCw,
  Activity,
  TrendingUp,
  Clock,
} from 'lucide-react';
import { useDashboardStats } from '@/hooks/useStats';
import { formatRelativeTime } from '@/lib/utils';

const Dashboard: React.FC = () => {
  const { data: stats, isLoading, error } = useDashboardStats();

  if (isLoading) {
    return (
      <div className="px-4 sm:px-6 lg:px-8">
        <div className="animate-pulse">
          <div className="h-8 bg-gray-200 rounded w-1/4 mb-8"></div>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-32 bg-gray-200 rounded-lg"></div>
            ))}
          </div>
        </div>
      </div>
    );
  }

  if (error || !stats) {
    return (
      <div className="px-4 sm:px-6 lg:px-8">
        <div className="text-center py-12">
          <XCircle className="mx-auto h-12 w-12 text-danger-500" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">Failed to load dashboard</h3>
          <p className="mt-1 text-sm text-gray-500">Unable to fetch statistics from the server.</p>
        </div>
      </div>
    );
  }

  const statCards = [
    {
      title: 'Total Agents',
      value: stats.total_agents,
      icon: Computer,
      color: 'text-blue-600 bg-blue-100',
      link: '/agents',
    },
    {
      title: 'Online Agents',
      value: stats.online_agents,
      icon: CheckCircle,
      color: 'text-success-600 bg-success-100',
      link: '/agents?status=online',
    },
    {
      title: 'Pending Updates',
      value: stats.pending_updates,
      icon: Clock,
      color: 'text-warning-600 bg-warning-100',
      link: '/updates?status=pending',
    },
    {
      title: 'Failed Updates',
      value: stats.failed_updates,
      icon: XCircle,
      color: 'text-danger-600 bg-danger-100',
      link: '/updates?status=failed',
    },
  ];

  const severityBreakdown = [
    { label: 'Critical', value: stats.critical_updates, color: 'bg-danger-600' },
    { label: 'High', value: stats.high_updates, color: 'bg-warning-600' },
    { label: 'Medium', value: stats.medium_updates, color: 'bg-blue-600' },
    { label: 'Low', value: stats.low_updates, color: 'bg-gray-600' },
  ];

  const updateTypeBreakdown = Object.entries(stats.updates_by_type).map(([type, count]) => ({
    type: type.charAt(0).toUpperCase() + type.slice(1),
    value: count,
    icon: type === 'apt' ? '📦' : type === 'docker' ? '🐳' : '📋',
  }));

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Page header */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-gray-900">Dashboard</h1>
        <p className="mt-1 text-sm text-gray-600">
          Overview of your infrastructure and update status
        </p>
      </div>

      {/* Stats cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        {statCards.map((stat) => {
          const Icon = stat.icon;
          return (
            <Link
              key={stat.title}
              to={stat.link}
              className="group block p-6 bg-white rounded-lg shadow-sm border border-gray-200 hover:shadow-md transition-shadow"
            >
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-medium text-gray-600 group-hover:text-gray-900">
                    {stat.title}
                  </p>
                  <p className="mt-2 text-3xl font-bold text-gray-900">
                    {stat.value.toLocaleString()}
                  </p>
                </div>
                <div className={`p-3 rounded-lg ${stat.color}`}>
                  <Icon className="h-6 w-6" />
                </div>
              </div>
            </Link>
          );
        })}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Severity breakdown */}
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-medium text-gray-900">Update Severity</h2>
            <AlertTriangle className="h-5 w-5 text-gray-400" />
          </div>

          {severityBreakdown.some(item => item.value > 0) ? (
            <div className="space-y-3">
              {severityBreakdown.map((severity) => (
                <div key={severity.label} className="flex items-center justify-between">
                  <div className="flex items-center space-x-3">
                    <div className={`w-3 h-3 rounded-full ${severity.color}`}></div>
                    <span className="text-sm font-medium text-gray-700">
                      {severity.label}
                    </span>
                  </div>
                  <span className="text-sm text-gray-900 font-semibold">
                    {severity.value}
                  </span>
                </div>
              ))}

              {/* Visual bar chart */}
              <div className="mt-4 space-y-2">
                {severityBreakdown.map((severity) => (
                  <div key={severity.label} className="relative">
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs text-gray-600">{severity.label}</span>
                      <span className="text-xs text-gray-900">{severity.value}</span>
                    </div>
                    <div className="w-full bg-gray-200 rounded-full h-2">
                      <div
                        className={`h-2 rounded-full ${severity.color}`}
                        style={{
                          width: `${stats.pending_updates > 0 ? (severity.value / stats.pending_updates) * 100 : 0}%`
                        }}
                      ></div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="text-center py-8">
              <CheckCircle className="mx-auto h-8 w-8 text-success-500" />
              <p className="mt-2 text-sm text-gray-600">No pending updates</p>
            </div>
          )}
        </div>

        {/* Update type breakdown */}
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-medium text-gray-900">Updates by Type</h2>
            <Package className="h-5 w-5 text-gray-400" />
          </div>

          {updateTypeBreakdown.length > 0 ? (
            <div className="space-y-3">
              {updateTypeBreakdown.map((type) => (
                <div key={type.type} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                  <div className="flex items-center space-x-3">
                    <span className="text-2xl">{type.icon}</span>
                    <span className="text-sm font-medium text-gray-700">
                      {type.type}
                    </span>
                  </div>
                  <span className="text-sm text-gray-900 font-semibold">
                    {type.value.toLocaleString()}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-center py-8">
              <Package className="mx-auto h-8 w-8 text-gray-400" />
              <p className="mt-2 text-sm text-gray-600">No updates found</p>
            </div>
          )}
        </div>
      </div>

      {/* Quick actions */}
      <div className="mt-8 bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <h2 className="text-lg font-medium text-gray-900 mb-4">Quick Actions</h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Link
            to="/agents"
            className="flex items-center space-x-3 p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors"
          >
            <Computer className="h-5 w-5 text-blue-600" />
            <span className="text-sm font-medium text-gray-700">View All Agents</span>
          </Link>

          <Link
            to="/updates"
            className="flex items-center space-x-3 p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors"
          >
            <Package className="h-5 w-5 text-warning-600" />
            <span className="text-sm font-medium text-gray-700">Manage Updates</span>
          </Link>

          <button
            onClick={() => window.location.reload()}
            className="flex items-center space-x-3 p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors"
          >
            <RefreshCw className="h-5 w-5 text-green-600" />
            <span className="text-sm font-medium text-gray-700">Refresh Data</span>
          </button>
        </div>
      </div>
    </div>
  );
};

export default Dashboard;