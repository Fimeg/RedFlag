import React from 'react';

const Logs: React.FC = () => {
  return (
    <div className="px-4 sm:px-6 lg:px-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Logs</h1>
        <p className="mt-1 text-sm text-gray-600">
          View system logs and update history
        </p>
      </div>

      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-8 text-center">
        <div className="text-gray-400 mb-2">📋</div>
        <h3 className="text-lg font-medium text-gray-900 mb-2">Coming Soon</h3>
        <p className="text-sm text-gray-600">
          Logs and history tracking will be available in a future update.
        </p>
      </div>
    </div>
  );
};

export default Logs;