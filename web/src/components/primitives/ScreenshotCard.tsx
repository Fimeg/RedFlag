import React from 'react';
import { MonitorPlay, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * ScreenshotCard — screenshot thumbnail with sunshine overlay and capture state.
 *
 * Renders the agent's screenshot (or capture prompt), live/stream badges,
 * and timestamp. The image box keeps a 16:9 ratio at its natural size but
 * grows past it to absorb extra height — pass `className="grow"` from a
 * flex-col parent to make the card fill leftover vertical space.
 *
 * Handlers are passed in as props — this component doesn't own mutations.
 */
interface SunshineInfo {
  web_ui?: string;
  version?: string;
  state: string; // 'active' | 'running' | 'stopped' | ...
}

interface ScreenshotStatus {
  status: string;
  completed_at?: string;
}

interface ScreenshotCardProps {
  image?: string;               // base64 PNG
  isCapturing: boolean;
  isPolling: boolean;
  sunshine?: SunshineInfo;
  screenshotStatus?: ScreenshotStatus;
  onCapture: () => void;
  formatRelativeTime?: (t: string) => string;
  className?: string;
}

const ScreenshotCard: React.FC<ScreenshotCardProps> = ({
  image,
  isCapturing,
  isPolling,
  sunshine,
  screenshotStatus,
  onCapture,
  formatRelativeTime,
  className,
}) => {
  const live = sunshine?.state === 'active';
  const sunshineReady = live || sunshine?.state === 'running';
  const hasImage = !!image;
  const canClick = !isCapturing && !isPolling;

  return (
    <div className={cn('flex min-w-0 flex-col', className)}>
      <div
        className={cn(
          'relative aspect-video w-full grow overflow-hidden rounded border',
          'bg-gradient-to-br from-slate-800 to-slate-900 border-slate-700',
          'flex flex-col items-center justify-center gap-1 text-slate-400',
          canClick && 'cursor-pointer hover:from-slate-700 hover:to-slate-800 transition-colors'
        )}
        onClick={() => {
          if (sunshineReady && sunshine?.web_ui) {
            window.open(sunshine.web_ui, '_blank', 'noreferrer');
          } else if (canClick) {
            onCapture();
          }
        }}
      >
        {hasImage ? (
          <img
            src={`data:image/png;base64,${image}`}
            alt="Agent screenshot"
            className="absolute inset-0 w-full h-full object-contain"
          />
        ) : (
          <>
            <MonitorPlay className="h-6 w-6 opacity-70" />
            <span className="text-[10px]">
              {isCapturing ? 'Requesting…'
                : isPolling ? 'Capturing…'
                : sunshineReady ? 'Open stream'
                : 'Click to capture'}
            </span>
          </>
        )}
        {live && (
          <span className="absolute top-1 left-1 flex items-center gap-1 rounded bg-red-600/90 px-1 py-0.5 text-[9px] font-medium text-white">
            <span className="h-1.5 w-1.5 rounded-full bg-white animate-pulse" />
            LIVE
          </span>
        )}
        {sunshineReady && !live && (
          <span className="absolute top-1 right-1 rounded bg-black/50 px-1 py-0.5 text-[9px] text-white">
            Sunshine
          </span>
        )}
        {(isCapturing || isPolling) && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/30">
            <RefreshCw className="h-5 w-5 text-white animate-spin" />
          </div>
        )}
      </div>
      <div className="flex items-center justify-between mt-1 text-[10px] text-gray-500">
        {sunshine?.version ? <span>Sunshine v{sunshine.version}</span> : <span />}
        {screenshotStatus?.status === 'completed' && screenshotStatus?.completed_at && formatRelativeTime
          ? <span>{formatRelativeTime(screenshotStatus.completed_at)}</span>
          : screenshotStatus?.status === 'failed'
          ? <span className="text-red-500">Failed</span>
          : null}
      </div>
    </div>
  );
};

export default ScreenshotCard;
