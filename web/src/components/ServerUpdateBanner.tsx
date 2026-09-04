import React, { useState } from 'react';
import { RefreshCw, X } from 'lucide-react';

interface ServerUpdateBannerProps {
  version: string | null;
}

// ServerUpdateBanner is an inline banner (same pattern as AdvisoryBanner) that
// appears when the server version changes while the tab is open. The operator
// can click "Refresh" to reload or dismiss the banner to keep working with
// potentially stale data.
const ServerUpdateBanner: React.FC<ServerUpdateBannerProps> = ({ version }) => {
  const [dismissed, setDismissed] = useState(false);

  if (dismissed) return null;

  return (
    <div className="bg-blue-50 border-b border-blue-200 px-4 sm:px-6 lg:px-8 py-2">
      <div className="flex items-center gap-2 text-sm text-blue-900">
        <span className="font-medium">Server updated</span>
        <span className="text-blue-800">
          {version ? `v${version} is running.` : 'Server restarted.'}{' '}
          Refresh for the latest UI.
        </span>
        <button
          onClick={() => window.location.reload()}
          className="ml-auto inline-flex items-center gap-1.5 px-3 py-1 text-xs font-medium text-white bg-blue-600 rounded hover:bg-blue-700 transition-colors"
        >
          <RefreshCw className="w-3 h-3" />
          Refresh
        </button>
        <button
          onClick={() => setDismissed(true)}
          className="flex-shrink-0 text-blue-400 hover:text-blue-600 transition-colors p-1"
          aria-label="Dismiss"
        >
          <X className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
};

export default ServerUpdateBanner;
