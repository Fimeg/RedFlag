import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useConfirm, PageState } from '@/components/primitives';
import {
  Shield,
  RefreshCw,
  Save,
  RotateCcw,
  Activity,
} from 'lucide-react';
import {
  useRateLimitSettings,
  useRateLimitStats,
  useUpdateRateLimitSettings,
  useResetRateLimitSettings,
  useCleanupRateLimits,
} from '../hooks/useRateLimits';
import { RateLimitCategory, RateLimitSettings } from '@/types';

const NS_PER_SECOND = 1_000_000_000;

interface CategoryMeta {
  key: RateLimitCategory;
  title: string;
  description: string;
  routesNote: string;
}

const CATEGORIES: CategoryMeta[] = [
  {
    key: 'agent_registration',
    title: 'Agent Registration',
    description: 'New agents enrolling with the server. Low limit prevents enrollment floods.',
    routesNote: 'POST /agents/register',
  },
  {
    key: 'agent_checkin',
    title: 'Agent Check-In',
    description: 'Agents polling for queued commands. Raise this if you operate over high-latency links and want faster command pickup.',
    routesNote: 'GET /agents/:id/commands',
  },
  {
    key: 'agent_reports',
    title: 'Agent Reports',
    description: 'Telemetry uploads — update lists, system info, metrics, events, docker images.',
    routesNote: 'POST /agents/:id/{updates,system-info,metrics,events,...}',
  },
  {
    key: 'admin_token_generation',
    title: 'Admin: Token Generation',
    description: 'Creating new registration tokens. Low limit guards the enrollment-credential surface.',
    routesNote: 'POST /admin/registration-tokens',
  },
  {
    key: 'admin_operations',
    title: 'Admin: General Operations',
    description: 'All other admin API calls — viewing agents, dashboards, settings.',
    routesNote: 'most /admin/* routes',
  },
  {
    key: 'public_access',
    title: 'Public Access',
    description: 'Unauthenticated and web routes — login, public key, system info, agent downloads.',
    routesNote: '/auth/login, /public-key, /downloads/:platform, /install/:platform',
  },
];

const RateLimiting: React.FC = () => {
  const navigate = useNavigate();
  const confirm = useConfirm();

  const { data: settings, refetch: refetchSettings, isLoading } = useRateLimitSettings();
  const { data: stats } = useRateLimitStats();
  const updateSettings = useUpdateRateLimitSettings();
  const resetSettings = useResetRateLimitSettings();
  const cleanup = useCleanupRateLimits();

  const [editing, setEditing] = useState<RateLimitSettings | null>(null);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (settings) {
      setEditing(JSON.parse(JSON.stringify(settings)));
      setDirty(false);
    }
  }, [settings]);

  const handleField = (
    cat: RateLimitCategory,
    field: 'requests' | 'window' | 'enabled',
    value: number | boolean,
  ) => {
    if (!editing) return;
    const next = { ...editing, [cat]: { ...editing[cat], [field]: value } };
    setEditing(next);
    setDirty(true);
  };

  const handleSave = () => {
    if (!editing) return;
    updateSettings.mutate(editing, { onSuccess: () => setDirty(false) });
  };

  const handleReset = async () => {
    if (!(await confirm({
      title: 'Reset to Defaults',
      body: 'Reset all rate limits to defaults? Any custom values will be lost.',
      confirmLabel: 'Reset',
      danger: true,
    }))) return;
    resetSettings.mutate();
  };

  const handleCleanup = async () => {
    if (!(await confirm({
      title: 'Cleanup Counters',
      body: 'Clean up expired rate-limit counters? Active limits are unaffected.',
      confirmLabel: 'Clean Up',
    }))) return;
    cleanup.mutate();
  };

  const handleDiscard = () => {
    if (!settings) return;
    setEditing(JSON.parse(JSON.stringify(settings)));
    setDirty(false);
  };

  if (isLoading || !editing) {
    return (
      <div className="max-w-6xl mx-auto px-6 py-8">
        <PageState loading={true} empty={false} loadingTitle="Loading rate limit settings..." />
      </div>
    );
  }

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      <button
        onClick={() => navigate('/settings')}
        className="text-sm text-gray-500 hover:text-gray-700 mb-4"
      >
        ← Back to Settings
      </button>

      <div className="mb-8">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900">Rate Limiting</h1>
            <p className="mt-2 text-gray-600">
              Six request-rate categories. Each route on the server falls under exactly one category; tune the limit and the time window per category.
            </p>
          </div>
          <div className="flex gap-3">
            <button
              onClick={handleCleanup}
              disabled={cleanup.isPending}
              className="inline-flex items-center gap-2 px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 ${cleanup.isPending ? 'animate-spin' : ''}`} />
              Cleanup Counters
            </button>
            <button
              onClick={() => refetchSettings()}
              className="inline-flex items-center gap-2 px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50"
            >
              <RefreshCw className="w-4 h-4" />
              Refresh
            </button>
          </div>
        </div>
      </div>

      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Configured Categories</p>
                <p className="text-2xl font-bold text-gray-900">{stats.total_configured_limits}</p>
              </div>
              <Shield className="w-8 h-8 text-blue-600" />
            </div>
          </div>
          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Enabled</p>
                <p className="text-2xl font-bold text-green-600">{stats.enabled_limits}</p>
              </div>
              <Shield className="w-8 h-8 text-green-600" />
            </div>
          </div>
          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Total Requests / Window</p>
                <p className="text-2xl font-bold text-gray-900">{stats.total_requests_per_minute}</p>
                <p className="text-xs text-gray-500">summed across all categories</p>
              </div>
              <Activity className="w-8 h-8 text-purple-600" />
            </div>
          </div>
        </div>
      )}

      {dirty && (
        <div className="alert alert-info mb-6">
          <div className="flex items-center justify-between">
            <p className="text-sm text-blue-800">You have unsaved changes.</p>
            <div className="flex gap-2">
              <button
                onClick={handleSave}
                disabled={updateSettings.isPending}
                className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
              >
                <Save className="w-4 h-4" />
                {updateSettings.isPending ? 'Saving...' : 'Save Changes'}
              </button>
              <button
                onClick={handleDiscard}
                className="px-4 py-2 bg-gray-200 text-gray-800 rounded-lg hover:bg-gray-300"
              >
                Discard
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="bg-white rounded-lg border border-gray-200 mb-6">
        <div className="px-6 py-4 border-b border-gray-200 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900">Categories</h2>
          <button
            onClick={handleReset}
            disabled={resetSettings.isPending}
            className="inline-flex items-center gap-2 px-4 py-2 bg-gray-600 text-white rounded-lg hover:bg-gray-700 disabled:opacity-50"
          >
            <RotateCcw className="w-4 h-4" />
            Reset to Defaults
          </button>
        </div>

        <div className="divide-y divide-gray-200">
          {CATEGORIES.map((cat) => {
            const cfg = editing[cat.key];
            if (!cfg) return null;
            const windowSeconds = Math.round(cfg.window / NS_PER_SECOND);
            return (
              <div key={cat.key} className="px-6 py-5">
                <div className="flex items-start justify-between gap-6">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-1">
                      <h3 className="font-semibold text-gray-900">{cat.title}</h3>
                      <label className="inline-flex items-center gap-2 text-xs">
                        <input
                          type="checkbox"
                          checked={cfg.enabled}
                          onChange={(e) => handleField(cat.key, 'enabled', e.target.checked)}
                          className="rounded border-gray-300"
                        />
                        <span className={cfg.enabled ? 'text-green-700' : 'text-gray-500'}>
                          {cfg.enabled ? 'enabled' : 'disabled'}
                        </span>
                      </label>
                    </div>
                    <p className="text-sm text-gray-600 mb-2">{cat.description}</p>
                    <p className="text-xs text-gray-400 font-mono">{cat.routesNote}</p>
                  </div>

                  <div className="flex items-end gap-4 flex-shrink-0">
                    <div>
                      <label className="block text-xs font-medium text-gray-500 mb-1">
                        Requests
                      </label>
                      <input
                        type="number"
                        min={1}
                        max={1000}
                        value={cfg.requests}
                        onChange={(e) =>
                          handleField(cat.key, 'requests', parseInt(e.target.value || '0', 10))
                        }
                        className="w-24 px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                      />
                    </div>
                    <div>
                      <label className="block text-xs font-medium text-gray-500 mb-1">
                        Window (seconds)
                      </label>
                      <input
                        type="number"
                        min={1}
                        max={86400}
                        value={windowSeconds}
                        onChange={(e) =>
                          handleField(
                            cat.key,
                            'window',
                            Math.max(1, parseInt(e.target.value || '0', 10)) * NS_PER_SECOND,
                          )
                        }
                        className="w-28 px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                      />
                    </div>
                    <div className="text-xs text-gray-500 pb-2 whitespace-nowrap">
                      = {(cfg.requests / Math.max(windowSeconds, 1)).toFixed(2)} req/s
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <div className="alert alert-warning text-sm text-yellow-800">
        <p className="font-medium mb-1">Limits are in-memory.</p>
        <p>
          The server keeps rate-limit counters in memory and reloads defaults on restart. Changes
          made here apply immediately to live traffic but are not persisted across restarts in this
          version.
        </p>
      </div>
    </div>
  );
};

export default RateLimiting;
