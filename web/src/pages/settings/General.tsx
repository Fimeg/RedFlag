import React from 'react';
import { useSettingsStore } from '@/lib/store';
import { useTimezones, useTimezone, useUpdateTimezone } from '../../hooks/useSettings';

const General: React.FC = () => {
  const { autoRefresh, refreshInterval, setAutoRefresh, setRefreshInterval } = useSettingsStore();

  // Timezone settings
  const { data: timezones, isLoading: isLoadingTimezones } = useTimezones();
  const { data: currentTimezone } = useTimezone();
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
    }
  };

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">General</h1>
        <p className="mt-2 text-gray-600">Display preferences and dashboard behavior for this session</p>
      </div>

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
                  className={`toggle ${
                    autoRefresh ? 'toggle-on' : 'toggle-off'
                  }`}
                >
                  <span className={`toggle-knob ${autoRefresh ? 'toggle-knob-on' : 'toggle-knob-off'}`} />
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
    </div>
  );
};

export default General;
