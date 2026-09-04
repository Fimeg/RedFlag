/**
 * ClientLogger — sends debug/trace signals to the server-side client_errors table.
 *
 * ETHOS #1: Errors are History. Debug/trace signals are not errors, but they
 * belong in the system's logging infrastructure, not on console.log where they
 * spam operators. This module routes them through the same /logs/client-error
 * endpoint as real errors, just tagged with client_debug / client_trace so they
 * can be filtered separately.
 *
 * Toggle: set localStorage['redflag_debug'] = '1' to enable, or unset to
 * suppress. The UI could grow a toggle in Settings → General later.
 */

import { api } from './api';

const isDebug = (): boolean => {
  if (typeof window === 'undefined') return false;
  return localStorage.getItem('redflag_debug') === '1';
};

function getSubsystem(): string {
  if (typeof window === 'undefined') return 'unknown';
  const path = window.location.pathname;
  if (path.startsWith('/agents')) return 'agents';
  if (path.startsWith('/updates') || path.startsWith('/staging')) return 'updates';
  if (path.startsWith('/docker')) return 'docker';
  if (path.startsWith('/history')) return 'history';
  if (path.startsWith('/settings')) return 'settings';
  return 'ui';
}

export const clientLogger = {
  debug: (message: string, metadata?: Record<string, unknown>) => {
    if (!isDebug()) return;
    api.post('/logs/client-error', {
      subsystem: getSubsystem(),
      error_type: 'client_debug',
      message: message.substring(0, 5000),
      metadata: metadata ?? {},
      url: window.location.href,
    }).catch(() => {
      // Best effort — don't let logging failures cascade
    });
  },

  trace: (message: string, metadata?: Record<string, unknown>) => {
    if (!isDebug()) return;
    api.post('/logs/client-error', {
      subsystem: getSubsystem(),
      error_type: 'client_trace',
      message: message.substring(0, 5000),
      metadata: metadata ?? {},
      url: window.location.href,
    }).catch(() => {});
  },
};
