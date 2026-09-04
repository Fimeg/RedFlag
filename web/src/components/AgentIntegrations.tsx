import React from 'react';
import { MonitorPlay, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import { formatRelativeTime } from '@/lib/utils';
import {
  INTEGRATION_CATALOG,
  INTEGRATION_ORDER,
  readIntegrations,
  resolveState,
  type IntegrationId,
  type IntegrationState,
  type ReportedIntegration,
} from '@/types/integrations';

interface AgentIntegrationsProps {
  metadata?: Record<string, unknown>;
}

const STATE_BADGE: Record<IntegrationState, { label: string; className: string }> = {
  active: { label: 'Session live', className: 'text-green-700 bg-green-50' },
  running: { label: 'Running', className: 'text-info-600 bg-info-50' },
  detected: { label: 'Installed · idle', className: 'text-amber-700 bg-amber-50' },
  not_detected: { label: 'Not detected', className: 'text-gray-500 bg-gray-100' },
  unknown: { label: 'Not reported', className: 'text-gray-500 bg-gray-100' },
};

// The Sunshine tile is image-forward: a framed "screen" that doubles as the launch
// affordance. We have no live thumbnail yet, so the frame shows a monitor glyph;
// when a session is live it gets a LIVE marker. The whole frame is the button.
const SunshineTile: React.FC<{ reported?: ReportedIntegration }> = ({ reported }) => {
  const descriptor = INTEGRATION_CATALOG.sunshine;
  const state = resolveState(reported);
  const badge = STATE_BADGE[state];
  const live = state === 'active';
  const launchable = state === 'active' || state === 'running';
  const webUi = reported?.web_ui;

  const frame = (
    <div
      className={cn(
        'relative aspect-video w-full overflow-hidden rounded border',
        'bg-gradient-to-br from-slate-800 to-slate-900 border-slate-700',
        'flex flex-col items-center justify-center gap-2 text-slate-400',
        launchable && webUi && 'cursor-pointer hover:from-slate-700 hover:to-slate-800 transition-colors'
      )}
    >
      <MonitorPlay className="h-10 w-10 opacity-70" />
      <span className="text-xs">
        {launchable ? 'Open stream host' : 'No active stream'}
      </span>
      {live && (
        <span className="absolute top-2 left-2 flex items-center gap-1 rounded bg-red-600/90 px-1.5 py-0.5 text-[10px] font-medium text-white">
          <span className="h-1.5 w-1.5 rounded-full bg-white animate-pulse" />
          LIVE
        </span>
      )}
      {launchable && webUi && (
        <span className="absolute bottom-2 right-2">
          <ExternalLink className="h-3.5 w-3.5" />
        </span>
      )}
    </div>
  );

  return (
    <div className="rounded-md border border-gray-200 p-3 space-y-3">
      {launchable && webUi ? (
        <a href={webUi} target="_blank" rel="noreferrer" aria-label="Open Sunshine host">
          {frame}
        </a>
      ) : (
        frame
      )}

      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium text-gray-900 truncate">{descriptor.name}</p>
          <p className="text-xs text-gray-500">{descriptor.vendor}</p>
        </div>
        <span className={cn('shrink-0 rounded-md px-2 py-0.5 text-xs font-medium', badge.className)}>
          {badge.label}
        </span>
      </div>

      <p className="text-xs text-gray-500 leading-relaxed">{descriptor.blurb}</p>

      <div className="space-y-1 text-xs text-gray-500 pt-2 border-t border-gray-100">
        {reported?.version && (
          <div className="flex justify-between">
            <span>Version</span>
            <span className="font-medium text-gray-700">{reported.version}</span>
          </div>
        )}
        {live && reported?.client_name && (
          <div className="flex justify-between">
            <span>Client</span>
            <span className="font-medium text-gray-700">{reported.client_name}</span>
          </div>
        )}
        {live && reported?.session_started_at && (
          <div className="flex justify-between">
            <span>Session started</span>
            <span className="font-medium text-gray-700">{formatRelativeTime(reported.session_started_at)}</span>
          </div>
        )}
        {reported?.last_observed && (
          <div className="flex justify-between">
            <span>Last observed</span>
            <span className="font-medium text-gray-700">{formatRelativeTime(reported.last_observed)}</span>
          </div>
        )}
        {state === 'unknown' && (
          <p className="text-gray-400">
            This agent has not reported integration data. Detection lands when the agent ships its
            Sunshine poller.
          </p>
        )}
      </div>
    </div>
  );
};

const TILES: Record<IntegrationId, React.FC<{ reported?: ReportedIntegration }>> = {
  sunshine: SunshineTile,
};

export const AgentIntegrations: React.FC<AgentIntegrationsProps> = ({ metadata }) => {
  const reported = readIntegrations(metadata);

  return (
    <div className="card">
      <h2 className="text-lg font-medium text-gray-900 mb-4">Integrations</h2>
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {INTEGRATION_ORDER.map((id) => {
          const Tile = TILES[id];
          return <Tile key={id} reported={reported[id]} />;
        })}
      </div>
    </div>
  );
};

export default AgentIntegrations;
