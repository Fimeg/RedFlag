import React from 'react';
import { Link } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import {
  Computer,
  Package,
  CheckCircle,
  AlertTriangle,
  RefreshCw,
  Clock,
} from 'lucide-react';
import { useDashboardStats } from '@/hooks/useStats';
import { getPackageTypeIcon } from '@/lib/utils';

import { useServerKeySecurity } from '@/hooks/useSecurity';
import StackDriftPanel from '@/components/StackDriftPanel';
import AttentionPanel from '@/components/AttentionPanel';
import { StatCard, PageState } from '@/components/primitives';

const Dashboard: React.FC = () => {
  const { data: stats, isPending, error } = useDashboardStats();
  const { data: serverKeySecurity } = useServerKeySecurity();
  const queryClient = useQueryClient();

  // Stat card definitions for StatCard component
  const statCardDefs = [
    {
      title: 'Total Agents',
      value: stats?.total_agents ?? 0,
      icon: Computer,
      color: 'text-blue-600 bg-blue-100',
      to: '/agents',
    },
    {
      title: 'Online Agents',
      value: stats?.online_agents ?? 0,
      icon: CheckCircle,
      color: 'text-success-600 bg-success-100',
      to: '/agents?status=online',
    },
    {
      title: 'Pending Updates',
      value: stats?.pending_updates ?? 0,
      icon: Clock,
      color: 'text-warning-600 bg-warning-100',
      to: '/updates?status=pending',
    },
    {
      title: 'Failed Updates',
      value: stats?.failed_updates ?? 0,
      icon: Package,
      color: 'text-danger-600 bg-danger-100',
      to: '/updates?status=failed',
    },
  ];

  const severityBreakdown = [
    { label: 'Critical', value: stats?.critical_updates ?? 0, color: 'bg-danger-600' },
    { label: 'High', value: stats?.high_updates ?? 0, color: 'bg-warning-600' },
    { label: 'Medium', value: stats?.medium_updates ?? 0, color: 'bg-blue-600' },
    { label: 'Low', value: stats?.low_updates ?? 0, color: 'bg-gray-600' },
  ];

  // Bars show each severity's share of the actionable (non-terminal) severity
  // counts the server returns. Sizing against this sum — not total_updates,
  // which also counts installed/failed — keeps numerator and denominator in the
  // same scope so bars always sum to 100% and never overflow. UI-DASHBOARD-AUDIT #2.
  const severityTotal = severityBreakdown.reduce((sum, s) => sum + s.value, 0);

  const updateTypeBreakdown = Object.entries(stats?.updates_by_type ?? {}).map(([type, count]) => ({
    type: type.charAt(0).toUpperCase() + type.slice(1),
    value: count,
    icon: getPackageTypeIcon(type),
  }));

  return (
    <PageState
      loading={isPending}
      error={error ? 'Unable to fetch statistics from the server.' : null}
      empty={false}
    >
      {/* Page header */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-gray-900">Dashboard</h1>
        <p className="mt-1 text-sm text-gray-600">
          Overview of your infrastructure and update status
        </p>
      </div>

      {/* Aggregated attention surface — failed updates, EOL drift, upstream movement */}
      <AttentionPanel />

      {/* Important Messages / Security Alert */}
      {serverKeySecurity && !serverKeySecurity.has_private_key && (
        <div className="bg-yellow-50 border border-yellow-200 text-yellow-800 px-4 py-3 rounded-lg relative mb-8" role="alert">
          <div className="flex items-center">
            <AlertTriangle className="h-5 w-5 mr-3" />
            <div>
              <strong className="font-bold">Security Upgrade Required:</strong>
              <span className="block sm:inline"> Your server is missing a private key for secure agent updates. Please go to <Link to="/settings/agents" className="font-bold underline hover:text-yellow-900">Agents &amp; Enrollment</Link> to generate one.</span>
            </div>
          </div>
        </div>
      )}

      {/* Stats cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        {statCardDefs.map((stat) => (
          <StatCard
            key={stat.title}
            title={stat.title}
            value={stat.value}
            icon={stat.icon}
            color={stat.color}
            to={stat.to}
          />
        ))}
      </div>

      <div className="mb-8">
        <StackDriftPanel />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Severity breakdown */}
        <div className="card">
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
                          width: `${severityTotal > 0 ? (severity.value / severityTotal) * 100 : 0}%`
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
        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-medium text-gray-900">Updates by Type</h2>
            <Package className="h-5 w-5 text-gray-400" />
          </div>

          {updateTypeBreakdown.length > 0 ? (
            <div className="space-y-3">
              {updateTypeBreakdown.map((type) => (
                <div key={type.type} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                  <div className="flex items-center space-x-3">
                    <type.icon className="h-5 w-5 text-gray-500" />
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
            onClick={() => queryClient.invalidateQueries()}
            className="flex items-center space-x-3 p-3 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors"
          >
            <RefreshCw className="h-5 w-5 text-green-600" />
            <span className="text-sm font-medium text-gray-700">Refresh Data</span>
          </button>
        </div>
      </div>
    </PageState>
  );
};

export default Dashboard;