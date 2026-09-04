import React from 'react';
import { WifiOff, RefreshCw } from 'lucide-react';

interface ServerStatusOverlayProps {
  onRetryNow: () => void;
}

// ServerStatusOverlay is a floating card that appears when the backend is
// unreachable. Positioned bottom-right so it doesn't block critical UI but is
// always visible. Rendered in App.tsx above the router so it persists across
// page navigations.
const ServerStatusOverlay: React.FC<ServerStatusOverlayProps> = ({
  onRetryNow,
}) => {
  return (
    <div className="fixed bottom-6 right-6 z-[100] animate-in fade-in slide-in-from-bottom-2 duration-300">
      <div className="bg-white rounded-xl shadow-2xl border border-red-200 p-4 w-72">
        <div className="flex items-start gap-3">
          <div className="flex-shrink-0 mt-0.5">
            <div className="relative">
              <WifiOff className="w-5 h-5 text-red-500" />
              <span className="absolute -top-1 -right-1 flex h-2.5 w-2.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500" />
              </span>
            </div>
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold text-gray-900">
              Backend unreachable
            </p>
            <p className="text-xs text-gray-500 mt-0.5">
              Retrying automatically…
            </p>
            <button
              onClick={onRetryNow}
              className="mt-2 inline-flex items-center gap-1.5 text-xs font-medium text-primary-600 hover:text-primary-700 transition-colors"
            >
              <RefreshCw className="w-3 h-3" />
              Retry now
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ServerStatusOverlay;
