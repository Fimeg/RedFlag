import React from 'react';
import { Clock } from 'lucide-react';
import { useSettingsStore } from '@/lib/store';
import { useTimezones, useTimezone, useUpdateTimezone } from '../hooks/useSettings';

const Settings: React.FC = () => {
  const { autoRefresh, refreshInterval, setAutoRefresh, setRefreshInterval } = useSettingsStore();

  const { data: timezones, isLoading: isLoadingTimezones } = useTimezones();
  const { data: currentTimezone, isLoading: isLoadingCurrentTimezone } = useTimezone();
  const updateTimezone = useUpdateTimezone();

  const [selectedTimezone, setSelectedTimezone] = React.useState('');

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
      // Revert on error
      if (currentTimezone?.timezone) {
        setSelectedTimezone(currentTimezone.timezone);
      }
    }
  };

  return (
    <div className="px-4 sm:px-6 lg:px-8 space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900">Settings</h1>
        <p className="mt-1 text-sm text-gray-600">Configure your RedFlag dashboard preferences</p>
      </div>

      {/* Timezone Settings */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <div className="flex items-center gap-3 mb-6">
          <div className="p-2 bg-gray-100 rounded-lg">
            <Clock className="w-5 h-5 text-gray-600" />
          </div>
          <div>
            <h2 className="text-xl font-semibold text-gray-900">Timezone Settings</h2>
            <p className="text-gray-600">Configure the timezone used for displaying timestamps</p>
          </div>
        </div>

        <div className="space-y-4">
          <div>
            <label htmlFor="timezone" className="block text-sm font-medium text-gray-700 mb-2">
              Display Timezone
            </label>
            <div className="relative">
              <select
                id="timezone"
                value={selectedTimezone}
                onChange={handleTimezoneChange}
                disabled={isLoadingTimezones || isLoadingCurrentTimezone || updateTimezone.isPending}
                className="w-full px-4 py-2 bg-white border border-gray-300 rounded-lg text-gray-900 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent appearance-none cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isLoadingTimezones ? (
                  <option>Loading timezones...</option>
                ) : (
                  timezones?.map((tz) => (
                    <option key={tz.value} value={tz.value}>
                      {tz.label}
                    </option>
                  ))
                )}
              </select>

              {/* Custom dropdown arrow */}
              <div className="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none">
                <svg className="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                </svg>
              </div>
            </div>

            {updateTimezone.isPending && (
              <p className="mt-2 text-sm text-yellow-600">Updating timezone...</p>
            )}

            {updateTimezone.isSuccess && (
              <p className="mt-2 text-sm text-green-600">Timezone updated successfully!</p>
            )}

            {updateTimezone.isError && (
              <p className="mt-2 text-sm text-red-600">
                Failed to update timezone. Please try again.
              </p>
            )}
          </div>

          <div className="pt-4 border-t border-gray-200">
            <p className="text-sm text-gray-600">
              This setting affects how timestamps are displayed throughout the dashboard, including agent
              last check-in times, scan times, and update timestamps.
            </p>
          </div>
        </div>
      </div>

      {/* Dashboard Settings */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <div className="flex items-center gap-3 mb-6">
          <div className="p-2 bg-gray-100 rounded-lg">
            <Clock className="w-5 h-5 text-gray-600" />
          </div>
          <div>
            <h2 className="text-xl font-semibold text-gray-900">Dashboard Settings</h2>
            <p className="text-gray-600">Configure how the dashboard behaves and displays information</p>
          </div>
        </div>

        <div className="space-y-6">
          {/* Auto Refresh */}
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-medium text-gray-900">Auto Refresh</h3>
              <p className="text-sm text-gray-600">
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
              className="w-full px-4 py-2 bg-white border border-gray-300 rounded-lg text-gray-900 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
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

      {/* Future Settings Sections */}
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6 opacity-60">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-2 bg-gray-100 rounded-lg">
            <Clock className="w-5 h-5 text-gray-400" />
          </div>
          <div>
            <h2 className="text-xl font-semibold text-gray-400">Additional Settings</h2>
            <p className="text-gray-500">More configuration options coming soon</p>
          </div>
        </div>

        <div className="space-y-3 text-sm text-gray-500">
          <div>• Notification preferences</div>
          <div>• Agent monitoring settings</div>
          <div>• Data retention policies</div>
          <div>• API access tokens</div>
        </div>
      </div>
    </div>
  );
};

export default Settings;