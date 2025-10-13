import React from 'react';
import { useSettingsStore } from '@/lib/store';

const Settings: React.FC = () => {
  const { autoRefresh, refreshInterval, setAutoRefresh, setRefreshInterval } = useSettingsStore();

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Settings</h1>
        <p className="mt-1 text-sm text-gray-600">
          Configure your dashboard preferences
        </p>
      </div>

      <div className="bg-white rounded-lg shadow-sm border border-gray-200">
        <div className="p-6 border-b border-gray-200">
          <h2 className="text-lg font-medium text-gray-900">Dashboard Settings</h2>
          <p className="mt-1 text-sm text-gray-600">
            Configure how the dashboard behaves and displays information
          </p>
        </div>

        <div className="p-6 space-y-6">
          {/* Auto Refresh */}
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-medium text-gray-900">Auto Refresh</h3>
              <p className="text-sm text-gray-500">
                Automatically refresh dashboard data at regular intervals
              </p>
            </div>
            <button
              onClick={() => setAutoRefresh(!autoRefresh)}
              className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
                autoRefresh ? 'bg-primary-600' : 'bg-gray-200'
              }`}
            >
              <span
                className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
                  autoRefresh ? 'translate-x-5' : 'translate-x-0'
                }`}
              />
            </button>
          </div>

          {/* Refresh Interval */}
          <div>
            <h3 className="text-sm font-medium text-gray-900 mb-3">Refresh Interval</h3>
            <select
              value={refreshInterval}
              onChange={(e) => setRefreshInterval(Number(e.target.value))}
              disabled={!autoRefresh}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <option value={10000}>10 seconds</option>
              <option value={30000}>30 seconds</option>
              <option value={60000}>1 minute</option>
              <option value={300000}>5 minutes</option>
              <option value={600000}>10 minutes</option>
            </select>
            <p className="mt-1 text-xs text-gray-500">
              How often to refresh dashboard data when auto-refresh is enabled
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default Settings;