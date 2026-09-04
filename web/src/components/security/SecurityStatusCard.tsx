import React from 'react';
import {
  Shield,
  ShieldCheck,
  AlertTriangle,
  XCircle,
  RefreshCw,
  CheckCircle,
  Clock,
  Activity,
  Eye,
  Info
} from 'lucide-react';
import { SecurityStatusCardProps } from '@/types/security';
import { securityHealthColor } from '@/components/primitives/statusColors';

const SecurityStatusCard: React.FC<SecurityStatusCardProps> = ({
  status,
  onRefresh,
  onViewLogs,
  onMonitorEvents,
  loading = false,
}) => {
  const getStatusIcon = () => {
    switch (status.overall) {
      case 'healthy':
        return <ShieldCheck className="w-6 h-6 text-green-600" />;
      case 'warning':
        return <AlertTriangle className="w-6 h-6 text-yellow-600" />;
      case 'critical':
        return <XCircle className="w-6 h-6 text-red-600" />;
      default:
        return <Shield className="w-6 h-6 text-gray-400" />;
    }
  };


  const getStatusText = () => {
    switch (status.overall) {
      case 'healthy':
        return 'All security features are operating normally';
      case 'warning':
        return 'Some security features require attention';
      case 'critical':
        return 'Critical security issues detected';
      default:
        return 'Security status unknown';
    }
  };

  const formatLastUpdate = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const minutes = Math.floor(diff / 60000);

    if (minutes < 1) return 'Just now';
    if (minutes < 60) return `${minutes} minute${minutes !== 1 ? 's' : ''} ago`;
    if (minutes < 1440) return `${Math.floor(minutes / 60)} hour${Math.floor(minutes / 60) !== 1 ? 's' : ''} ago`;
    return date.toLocaleDateString();
  };

  return (
    <div className="space-y-6">
      {/* Main Status Card */}
      <div className="bg-white border border-gray-200 rounded-lg p-6">
        <div className="flex items-start justify-between mb-6">
          <div className="flex items-start gap-4">
            <div className={`p-3 rounded-lg border ${securityHealthColor(status.overall)}`}>
              {getStatusIcon()}
            </div>
            <div>
              <h2 className="text-xl font-semibold text-gray-900">Security Overview</h2>
              <p className="text-gray-600 mt-1">{getStatusText()}</p>
            </div>
          </div>
          <button
            onClick={onRefresh}
            disabled={loading}
            className={`
              flex items-center gap-2 px-3 py-2 text-sm rounded-lg transition-colors
              ${loading
                ? 'bg-gray-100 text-gray-400 cursor-not-allowed'
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }
            `}
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            {loading ? 'Updating...' : 'Refresh'}
          </button>
        </div>

        {/* Feature Status Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {status.features.map((feature) => (
            <div
              key={feature.name}
              className="p-4 bg-gray-50 border border-gray-200 rounded-lg"
            >
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm font-medium text-gray-900">
                  {feature.name}
                </span>
                {feature.enabled ? (
                  <div className="flex items-center gap-1">
                    <CheckCircle className="w-4 h-4 text-green-500" />
                    <span className="text-xs text-green-600 font-medium">
                      {feature.status === 'healthy' ? 'Active' : feature.status}
                    </span>
                  </div>
                ) : (
                  <div className="flex items-center gap-1">
                    <XCircle className="w-4 h-4 text-gray-400" />
                    <span className="text-xs text-gray-500">Disabled</span>
                  </div>
                )}
              </div>
              {feature.details && (
                <p className="text-xs text-gray-500">{feature.details}</p>
              )}
              <p className="text-xs text-gray-400 mt-1 flex items-center gap-1">
                <Clock className="w-3 h-3" />
                {formatLastUpdate(feature.last_check)}
              </p>
            </div>
          ))}
        </div>
      </div>

      {/* Recent Events Summary */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-white border border-gray-200 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900">Recent Events</p>
              <p className="text-2xl font-bold text-gray-900 mt-1">{status.recent_events}</p>
              <p className="text-sm text-gray-600">Last 24 hours</p>
            </div>
            <Activity className="w-8 h-8 text-blue-600 opacity-20" />
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900">Enabled Features</p>
              <p className="text-2xl font-bold text-green-600 mt-1">
                {status.features.filter(f => f.enabled).length}/{status.features.length}
              </p>
              <p className="text-sm text-gray-600">Active</p>
            </div>
            <Shield className="w-8 h-8 text-green-600 opacity-20" />
          </div>
        </div>

        <div className="bg-white border border-gray-200 rounded-lg p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900">Last Updated</p>
              <p className="text-sm font-medium text-gray-700 mt-2">
                {formatLastUpdate(status.last_updated)}
              </p>
            </div>
            <Clock className="w-8 h-8 text-gray-600 opacity-20" />
          </div>
        </div>
      </div>

      {/* Alert Section */}
      {status.overall !== 'healthy' && (
        <div className={`alert ${securityHealthColor(status.overall)}`}>
          <div className="flex items-start gap-3">
            {status.overall === 'warning' ? (
              <AlertTriangle className="w-5 h-5 text-yellow-600 flex-shrink-0 mt-0.5" />
            ) : (
              <XCircle className="w-5 h-5 text-red-600 flex-shrink-0 mt-0.5" />
            )}
            <div>
              <h3 className="font-medium mb-1">
                {status.overall === 'warning' ? 'Security Warnings' : 'Security Alert'}
              </h3>
              <ul className="text-sm space-y-1">
                {status.features
                  .filter(f => f.status !== 'healthy' && f.enabled)
                  .map((feature, index) => (
                    <li key={index} className="flex items-center gap-2">
                      <span className="w-1.5 h-1.5 rounded-full bg-current opacity-60"></span>
                      {feature.name}: {feature.details || feature.status}
                    </li>
                  ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Quick Actions */}
      <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
        <p className="text-sm font-medium text-gray-900 mb-3 flex items-center gap-2">
          <Info className="w-4 h-4" />
          Quick Actions
        </p>
        <div className="flex flex-wrap gap-2">
          <button
            onClick={onViewLogs}
            disabled={!onViewLogs}
            className="px-3 py-1.5 text-sm bg-white border border-gray-300 rounded-lg hover:border-gray-400 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            <Eye className="w-3 h-3" />
            View Security Logs
          </button>
          <button
            onClick={onRefresh}
            disabled={!onRefresh || loading}
            className="px-3 py-1.5 text-sm bg-white border border-gray-300 rounded-lg hover:border-gray-400 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            <Shield className="w-3 h-3" />
            Run Security Check
          </button>
          <button
            onClick={onMonitorEvents}
            disabled={!onMonitorEvents}
            className="px-3 py-1.5 text-sm bg-white border border-gray-300 rounded-lg hover:border-gray-400 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            <Activity className="w-3 h-3" />
            Monitor Events
          </button>
        </div>
      </div>
    </div>
  );
};

export default SecurityStatusCard;