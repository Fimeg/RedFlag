import React, { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import {
  Shield,
  Plus,
  RefreshCw,
  Trash2,
  Copy,
  AlertTriangle,
  CheckCircle,
  Clock,
  Users,
  Key,
  Terminal,
  KeyRound,
  Server,
  Monitor,
  Laptop,
} from 'lucide-react';
import { SearchInput, useConfirm } from '@/components/primitives';
import { tokenStatusColor } from '@/components/primitives/statusColors';
import {
  useRegistrationTokens,
  useCreateRegistrationToken,
  useRevokeRegistrationToken,
  useDeleteRegistrationToken,
  useRegistrationTokenStats,
  useCleanupRegistrationTokens,
  useBoundAgents,
  useRevokeAgent,
} from '@/hooks/useRegistrationTokens';
import { useServerKeySecurity } from '@/hooks/useSecurity';
import type { RegistrationToken, CreateRegistrationTokenRequest, BoundAgent } from '@/types';
import { formatDateTime, formatRelativeTime, isOnline, cn } from '@/lib/utils';
import toast from 'react-hot-toast';

// Install one-liner generation — matches what /api/v1/install/{linux,windows,macos}
// expects. Token travels in the X-Registration-Token header, never the URL
// (SEC-002: query strings land in shell history, process lists, access logs).
const PLATFORMS = [
  {
    id: 'linux',
    name: 'Linux',
    icon: Server,
    installScript: '/api/v1/install/linux',
    available: true,
    description: 'Ubuntu, Debian, RHEL, Fedora, Rocky, Alma (AMD64 + ARM64)',
  },
  {
    id: 'windows',
    name: 'Windows',
    icon: Monitor,
    installScript: '/api/v1/install/windows',
    available: true,
    description: 'Windows 10/11, Server 2019/2022 (AMD64 + ARM64)',
  },
  {
    id: 'macos',
    name: 'macOS',
    icon: Laptop,
    installScript: '/api/v1/install/macos',
    available: false,
    description: 'macOS 12+ (Apple Silicon + Intel) — coming soon',
  },
] as const;

// Expiry choices offered in the create-key form. Ceiling raised from 168h
// (7d) to 2160h (90d) 2026-06-30 — matches server/internal/api/handlers/
// registration_tokens.go's maxRegistrationTokenDuration. See
// docs/tasks/UI-REGISTRATION-ENROLLMENT-UNIFY.md for the reasoning: 90 days
// mirrors the refresh-token TTL already trusted elsewhere in this system,
// and there's still no "never expires" option — expires_at is a required
// column, and an unbounded bearer credential is a bigger step than a longer
// bound one.
const EXPIRY_OPTIONS = [
  { value: '24h', label: '24 hours' },
  { value: '72h', label: '3 days' },
  { value: '168h', label: '7 days (1 week)' },
  { value: '720h', label: '30 days' },
  { value: '2160h', label: '90 days (maximum)' },
] as const;

function getServerUrl(): string {
  // The host:port the browser is on is always reachable by the agent machine.
  const { protocol, hostname, port } = window.location;
  return `${protocol}//${hostname}${port ? `:${port}` : ''}`;
}

function generateInstallCommand(platformId: string, token: string | undefined): string {
  if (!token) return '';
  const serverUrl = getServerUrl();
  const script = PLATFORMS.find((p) => p.id === platformId)?.installScript;
  if (!script) return '';
  if (platformId === 'windows') {
    return `irm "${serverUrl}${script}" -Headers @{'X-Registration-Token'='${token}'} | iex`;
  }
  return `curl -sfL -H "X-Registration-Token: ${token}" "${serverUrl}${script}" | sudo bash`;
}

type StatusFilter = 'all' | 'active' | 'used' | 'expired' | 'revoked';

const STATUS_DOT: Record<string, string> = {
  active: 'bg-green-500',
  used: 'bg-primary-500',
  expired: 'bg-amber-500',
  revoked: 'bg-gray-400',
};

const getStatusText = (token: RegistrationToken): string => {
  const s = String(token.status);
  return s.charAt(0).toUpperCase() + s.slice(1);
};

const tokenLabel = (t: RegistrationToken): string => t.label || `token ${t.id.slice(0, 8)}`;

// PlatformPicker + InstallCommandBox are shared between the "use an existing
// key" and "key just created" branches of the enrollment flow below, so the
// platform choice and the resulting one-liner look and behave identically
// regardless of how the operator got there.
const PlatformPicker: React.FC<{ value: string; onChange: (id: string) => void }> = ({ value, onChange }) => (
  <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
    {PLATFORMS.map((p) => {
      const Icon = p.icon;
      const selected = value === p.id;
      return (
        <button
          key={p.id}
          type="button"
          onClick={() => onChange(p.id)}
          disabled={!p.available}
          className={cn(
            'p-4 border-2 rounded-lg text-left transition-all disabled:opacity-50 disabled:cursor-not-allowed',
            selected ? 'border-primary-500 bg-primary-50' : 'border-gray-200 hover:border-gray-300',
          )}
        >
          <div className="flex items-center justify-between mb-2">
            <Icon className={cn('w-6 h-6', p.id === 'linux' ? 'text-orange-600' : p.id === 'windows' ? 'text-primary-600' : 'text-gray-600')} />
            {selected && <CheckCircle className="w-4 h-4 text-primary-600" />}
          </div>
          <div className="font-medium text-gray-900">{p.name}</div>
          <div className="text-xs text-gray-500 mt-1">{p.description}</div>
        </button>
      );
    })}
  </div>
);

const InstallCommandBox: React.FC<{
  command: string;
  platform: string;
  copied: boolean;
  onCopy: () => void;
}> = ({ command, platform, copied, onCopy }) => (
  <div>
    <label className="block text-sm font-medium text-gray-700 mb-2">
      Installation command
      {platform === 'windows' && <span className="text-primary-600"> (Run in PowerShell as Administrator)</span>}
    </label>
    <div className="relative">
      <pre className="bg-gray-900 text-gray-100 p-4 rounded-lg overflow-x-auto">
        <code>{command}</code>
      </pre>
      <button
        onClick={onCopy}
        className="absolute top-2 right-2 p-2 bg-gray-700 text-white rounded hover:bg-gray-600"
        title="Copy command"
      >
        {copied ? <CheckCircle className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
      </button>
    </div>
  </div>
);

const AgentsEnrollment: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const confirm = useConfirm();

  // Key list — fetch all (incl. revoked/expired) so the operator sees the full
  // roster; status chips filter client-side.
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const { data: tokensData, isLoading, refetch } = useRegistrationTokens({
    page: 1,
    page_size: 100,
    label: searchTerm || undefined,
  });

  const { data: stats } = useRegistrationTokenStats();
  const createToken = useCreateRegistrationToken();
  const revokeToken = useRevokeRegistrationToken();
  const deleteToken = useDeleteRegistrationToken();
  const cleanupTokens = useCleanupRegistrationTokens();

  // Key-list selection (right-hand detail pane, independent of enrollment)
  const [selectedTokenId, setSelectedTokenId] = useState<string>('');
  const [revealToken, setRevealToken] = useState(false);

  // Enrollment flow state — one card, two ways in ("use existing key" vs.
  // "create new key"), converging on the same platform picker + one-liner.
  // Replaces the old showInstall/showCreate toggle-panel pair.
  const [enrollTab, setEnrollTab] = useState<'use' | 'create'>('use');
  const enrollTabTouched = React.useRef(false);
  const [installPlatform, setInstallPlatform] = useState<string>('linux');
  const [installTokenId, setInstallTokenId] = useState<string>('');
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);
  const [createdToken, setCreatedToken] = useState<{ token: string; label: string } | null>(null);

  // Create-key form
  const [formData, setFormData] = useState<CreateRegistrationTokenRequest>({
    label: '',
    expires_in: '168h',
    max_seats: 1,
  });

  // Signing keys (preserved from the old Agent Management page — now its own
  // section instead of being repeated under whichever key happens to be
  // selected, since it isn't actually per-key).
  const { data: serverKeySecurity, isLoading: isLoadingServerKeySecurity, refetch: refetchServerKeySecurity } =
    useServerKeySecurity();
  const [generatingKeys, setGeneratingKeys] = useState(false);

  const allTokens = tokensData?.tokens || [];

  // Client-side status filter
  const filteredTokens = allTokens.filter((t) => {
    if (statusFilter !== 'all' && t.status !== statusFilter) return false;
    return true;
  });

  // Auto-select the first key for the detail pane once the list lands, but never
  // override an explicit operator selection.
  React.useEffect(() => {
    if (!selectedTokenId && filteredTokens.length > 0) {
      setSelectedTokenId(filteredTokens[0].id);
    }
  }, [filteredTokens, selectedTokenId]);

  // Drop a stale selection if the token list changes underneath us.
  React.useEffect(() => {
    if (selectedTokenId && !allTokens.some((t) => t.id === selectedTokenId)) {
      setSelectedTokenId(allTokens[0]?.id ?? '');
    }
  }, [allTokens, selectedTokenId]);

  const selectedToken = allTokens.find((t) => t.id === selectedTokenId) || null;

  // Active tokens with available seats — what "use existing key" offers.
  const availableTokens = React.useMemo(
    () =>
      allTokens.filter(
        (t) => !t.revoked && t.status === 'active' && t.seats_used < t.max_seats,
      ),
    [allTokens],
  );

  // If the operator hasn't touched the tab and there's nothing to enroll
  // with yet, default straight to "create new key" instead of showing an
  // empty dropdown.
  React.useEffect(() => {
    if (!enrollTabTouched.current && !isLoading && availableTokens.length === 0) {
      setEnrollTab('create');
    }
  }, [availableTokens, isLoading]);

  const installToken = availableTokens.find((t) => t.id === installTokenId) ?? null;
  React.useEffect(() => {
    if (installTokenId && !availableTokens.some((t) => t.id === installTokenId)) {
      setInstallTokenId('');
    }
  }, [availableTokens, installTokenId]);

  // Deep-link from elsewhere (e.g. the Agents page): `?install=1` lands on the
  // "use existing key" tab; `?install=<tokenId>` pre-seeds it with that key so
  // the operator lands on the exact key they came to enroll with. The guard
  // effect above drops the seed if the key turns out unavailable. Strip the
  // param afterward so a refresh or back-nav doesn't re-trigger it.
  const deepLinkHandled = React.useRef(false);
  React.useEffect(() => {
    if (deepLinkHandled.current) return;
    const inst = searchParams.get('install');
    if (!inst) return;
    deepLinkHandled.current = true;
    enrollTabTouched.current = true;
    setEnrollTab('use');
    if (inst !== '1') setInstallTokenId(inst);
    searchParams.delete('install');
    setSearchParams(searchParams, { replace: true });
  }, [searchParams, setSearchParams]);

  const { data: boundAgentsData, isLoading: boundAgentsLoading } = useBoundAgents(selectedTokenId || '');
  const revokeAgentMutation = useRevokeAgent(selectedTokenId || '');

  const handleCreateToken = (e: React.FormEvent) => {
    e.preventDefault();
    createToken.mutate(formData, {
      onSuccess: (data: any) => {
        const label = formData.label || data.label;
        setFormData({ label: '', expires_in: '168h', max_seats: 1 });
        setCreatedToken({ token: data.token, label });
        refetch();
      },
    });
  };

  const handleRevokeToken = async (tokenId: string, label: string) => {
    // No cascade: revoking a key only stops new enrollments. Enrolled agents
    // keep working — revoke them individually. (Test-enforced in the server.)
    if (
      !(await confirm({
        title: 'Revoke registration key',
        body: `Revoke key "${label}"? No new agents can enroll with it. Agents already enrolled with this key keep working — revoke them individually below if their access must end.`,
        confirmLabel: 'Revoke key',
        danger: true,
      }))
    )
      return;
    revokeToken.mutate(tokenId, { onSuccess: () => refetch() });
  };

  const handleDeleteToken = async (tokenId: string, label: string) => {
    if (
      !(await confirm({
        title: 'PERMANENTLY DELETE key',
        body: `PERMANENTLY DELETE key "${label}"? This cannot be undone.`,
        confirmLabel: 'Delete',
        danger: true,
      }))
    )
      return;
    deleteToken.mutate(tokenId, { onSuccess: () => refetch() });
  };

  const handleRevokeAgent = async (agentId: string, hostname: string) => {
    if (
      !(await confirm({
        title: 'Revoke agent',
        body: `Revoke agent "${hostname}"? Its refresh tokens are invalidated so it can no longer check in. The agent record and its history are kept.`,
        confirmLabel: 'Revoke agent',
        danger: true,
      }))
    )
      return;
    revokeAgentMutation.mutate({ agentId });
  };

  const handleCleanup = async () => {
    if (
      !(await confirm({
        title: 'Cleanup expired keys',
        body: 'Clean up all expired keys? This cannot be undone.',
        confirmLabel: 'Clean Up',
        danger: true,
      }))
    )
      return;
    cleanupTokens.mutate(undefined, { onSuccess: () => refetch() });
  };

  // Jump straight from a selected key into the enrollment flow, pre-seeded
  // with that key — no re-picking it in the dropdown.
  const installWithKey = (tokenId: string) => {
    enrollTabTouched.current = true;
    setCreatedToken(null);
    setEnrollTab('use');
    setInstallTokenId(tokenId);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const copyToClipboard = async (text: string, id: string) => {
    if (!text || !text.trim()) {
      toast.error('Nothing to copy');
      return;
    }
    try {
      await navigator.clipboard.writeText(text);
      setCopiedCommand(id);
      toast.success('Copied to clipboard');
      setTimeout(() => setCopiedCommand(null), 2000);
    } catch {
      toast.error('Failed to copy. Copy manually.');
    }
  };

  const generateKeys = async () => {
    setGeneratingKeys(true);
    try {
      const response = await fetch('/api/setup/generate-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });
      if (!response.ok) throw new Error('Failed to generate keys');
      toast.success('Signing keys generated. Restart the server.');
      refetchServerKeySecurity();
    } catch (error: any) {
      toast.error(error.message || 'Failed to generate keys');
    } finally {
      setGeneratingKeys(false);
    }
  };

  // The token driving the platform picker + one-liner: whichever key was
  // just created, or (in "use existing key" mode) whichever key is selected.
  const enrollTokenValue = createdToken?.token ?? installToken?.token;
  const installCommand = generateInstallCommand(installPlatform, enrollTokenValue);
  const boundAgents = boundAgentsData?.agents || [];

  return (
    <div className="max-w-7xl mx-auto px-6 py-8">
      <button onClick={() => navigate('/settings')} className="text-sm text-gray-500 hover:text-gray-700 mb-4">
        ← Back to Settings
      </button>

      {/* Header */}
      <div className="mb-6">
        <h1 className="text-3xl font-bold text-gray-900">Agents &amp; Enrollment</h1>
        <p className="mt-2 text-gray-600">
          Enroll new agents and manage the registration keys that let them join.
        </p>
      </div>

      {/* Enroll a new agent — single flow: pick or create a key, pick a
          platform, copy the one-liner. Replaces the old install/create
          toggle panels. */}
      <div className="card mb-6">
        <div className="flex items-center gap-2 mb-1">
          <Terminal className="w-5 h-5 text-primary-600" />
          <h2 className="text-lg font-semibold text-gray-900">Enroll a new agent</h2>
        </div>
        <p className="text-sm text-gray-500 mb-4">
          Pick a registration key (or make one), choose a platform, and copy the one-liner.
        </p>

        {createdToken ? (
          <div className="space-y-4">
            <div className="alert alert-success flex items-start justify-between gap-3">
              <p className="text-sm text-success-800">
                <span className="font-medium">Key "{createdToken.label}" created.</span> Copy the
                token now — it cannot be retrieved again, only a hash is stored.
              </p>
              <button
                onClick={() => setCreatedToken(null)}
                className="text-xs text-success-700 hover:text-success-900 shrink-0 whitespace-nowrap"
              >
                Enroll another
              </button>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">Token</label>
              <div className="flex items-center gap-2">
                <code className="flex-1 font-mono text-sm bg-gray-50 border border-gray-200 px-3 py-2 rounded select-all">
                  {createdToken.token}
                </code>
                <button
                  onClick={() => copyToClipboard(createdToken.token, 'created-token')}
                  className="p-2 text-gray-500 hover:text-gray-700 border border-gray-200 rounded"
                  title="Copy token"
                >
                  {copiedCommand === 'created-token' ? <CheckCircle className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                </button>
              </div>
            </div>
            <PlatformPicker value={installPlatform} onChange={setInstallPlatform} />
            <InstallCommandBox
              command={installCommand}
              platform={installPlatform}
              copied={copiedCommand === 'install'}
              onCopy={() => copyToClipboard(installCommand, 'install')}
            />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="inline-flex rounded-lg border border-gray-200 p-0.5 bg-gray-50">
              <button
                type="button"
                onClick={() => {
                  enrollTabTouched.current = true;
                  setEnrollTab('use');
                }}
                className={cn(
                  'px-3 py-1.5 text-sm rounded-md transition-colors',
                  enrollTab === 'use' ? 'bg-white shadow-sm text-primary-700 font-medium' : 'text-gray-500 hover:text-gray-700',
                )}
              >
                Use existing key
              </button>
              <button
                type="button"
                onClick={() => {
                  enrollTabTouched.current = true;
                  setEnrollTab('create');
                }}
                className={cn(
                  'px-3 py-1.5 text-sm rounded-md transition-colors inline-flex items-center gap-1',
                  enrollTab === 'create' ? 'bg-white shadow-sm text-primary-700 font-medium' : 'text-gray-500 hover:text-gray-700',
                )}
              >
                <Plus className="w-3.5 h-3.5" />
                Create new key
              </button>
            </div>

            {enrollTab === 'use' ? (
              availableTokens.length === 0 ? (
                <div className="text-sm text-gray-600 bg-gray-50 border border-gray-200 rounded-lg p-4">
                  No registration keys with available seats.{' '}
                  <button
                    onClick={() => {
                      enrollTabTouched.current = true;
                      setEnrollTab('create');
                    }}
                    className="text-primary-600 hover:text-primary-800 underline"
                  >
                    Create one
                  </button>{' '}
                  — existing agents are unaffected.
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="text-sm text-gray-600">Key:</span>
                    <select
                      value={installTokenId}
                      onChange={(e) => setInstallTokenId(e.target.value)}
                      className="form-input bg-white min-w-[320px] w-auto"
                    >
                      <option value="">— Select a key ({availableTokens.length} available) —</option>
                      {availableTokens.map((t) => (
                        <option key={t.id} value={t.id}>
                          {(t.token ?? t.id).slice(0, 12)}…{t.label ? ` · ${t.label}` : ''} · {t.seats_used}/{t.max_seats} seats
                        </option>
                      ))}
                    </select>
                  </div>

                  <PlatformPicker value={installPlatform} onChange={setInstallPlatform} />

                  {installToken ? (
                    <InstallCommandBox
                      command={installCommand}
                      platform={installPlatform}
                      copied={copiedCommand === 'install'}
                      onCopy={() => copyToClipboard(installCommand, 'install')}
                    />
                  ) : (
                    <div className="text-sm text-gray-500 bg-gray-50 border border-gray-200 rounded-lg p-4">
                      Select a key above to generate the one-liner.
                    </div>
                  )}
                </div>
              )
            ) : (
              <form onSubmit={handleCreateToken} className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">Label *</label>
                    <input
                      type="text"
                      required
                      value={formData.label}
                      onChange={(e) => setFormData({ ...formData, label: e.target.value })}
                      placeholder="e.g., Production Servers"
                      className="form-input"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">Expires In</label>
                    <select
                      value={formData.expires_in}
                      onChange={(e) => setFormData({ ...formData, expires_in: e.target.value })}
                      className="form-input bg-white"
                    >
                      {EXPIRY_OPTIONS.map((opt) => (
                        <option key={opt.value} value={opt.value}>
                          {opt.label}
                        </option>
                      ))}
                    </select>
                    <p className="mt-1 text-xs text-gray-500">Maximum 90 days per key — revoke anytime before then.</p>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-2">Max Seats (Agents)</label>
                    <input
                      type="number"
                      min="1"
                      max="100"
                      value={formData.max_seats || 1}
                      onChange={(e) => setFormData({ ...formData, max_seats: parseInt(e.target.value) || 1 })}
                      className="form-input"
                    />
                    <p className="mt-1 text-xs text-gray-500">Number of agents that can enroll with this key</p>
                  </div>
                </div>
                <button type="submit" disabled={createToken.isPending} className="btn-primary">
                  {createToken.isPending ? 'Creating...' : 'Create key'}
                </button>
              </form>
            )}
          </div>
        )}
      </div>

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
          <StatCard label="Total keys" value={stats.total_tokens} icon={Shield} />
          <StatCard label="Active" value={stats.active_tokens} valueClass="text-green-600" icon={CheckCircle} />
          <StatCard label="Used" value={stats.used_tokens} valueClass="text-primary-600" icon={Users} />
          <StatCard label="Expired" value={stats.expired_tokens} valueClass="text-gray-600" icon={Clock} />
          <StatCard
            label="Seats used"
            value={`${stats.total_seats_used}/${stats.total_seats_available || '∞'}`}
            valueClass="text-purple-600"
            icon={Users}
          />
        </div>
      )}

      {/* Registration keys — master-detail roster + audit view */}
      <div className="mb-3">
        <h2 className="text-lg font-semibold text-gray-900">Registration keys</h2>
        <p className="text-sm text-gray-500">All keys, active and historical. Select one to see who enrolled with it.</p>
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-[320px_1fr] gap-6 mb-6">
        {/* LEFT: key list */}
        <div className="bg-white rounded-lg border border-gray-200 flex flex-col max-h-[70vh]">
          <div className="p-4 border-b border-gray-200 space-y-3">
            <SearchInput value={searchTerm} onChange={setSearchTerm} placeholder="Search by label..." />
            <div className="flex flex-wrap gap-1">
              {(['all', 'active', 'used', 'expired', 'revoked'] as StatusFilter[]).map((s) => (
                <button
                  key={s}
                  onClick={() => setStatusFilter(s)}
                  className={cn(
                    'px-2.5 py-1 rounded-md text-xs capitalize transition-colors border',
                    statusFilter === s
                      ? 'bg-gray-100 text-gray-800 border-gray-300'
                      : 'bg-white text-gray-500 border-gray-200 hover:bg-gray-50',
                  )}
                >
                  {s}
                </button>
              ))}
            </div>
            <div className="flex items-center justify-between">
              <span className="text-xs text-gray-500">{filteredTokens.length} keys</span>
              <button
                onClick={handleCleanup}
                disabled={cleanupTokens.isPending}
                className="text-xs text-orange-600 hover:text-orange-800 inline-flex items-center gap-1 disabled:opacity-50"
                title="Clean up expired keys"
              >
                <RefreshCw className={cn('w-3 h-3', cleanupTokens.isPending && 'animate-spin')} />
                Cleanup expired
              </button>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto">
            {isLoading ? (
              <div className="p-8 text-center">
                <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
                <p className="mt-2 text-sm text-gray-500">Loading keys...</p>
              </div>
            ) : filteredTokens.length === 0 ? (
              <div className="p-8 text-center">
                <Shield className="w-10 h-10 text-gray-300 mx-auto mb-2" />
                <p className="text-sm text-gray-500">
                  {searchTerm || statusFilter !== 'all' ? 'No keys match.' : 'No keys yet. Create one to enroll agents.'}
                </p>
              </div>
            ) : (
              filteredTokens.map((t) => {
                const selected = t.id === selectedTokenId;
                return (
                  <button
                    key={t.id}
                    onClick={() => {
                      setSelectedTokenId(t.id);
                      setRevealToken(false);
                    }}
                    className={cn(
                      'w-full text-left px-4 py-3 border-b border-gray-100 transition-colors',
                      selected ? 'bg-primary-50 border-l-2 border-l-primary-500' : 'hover:bg-gray-50 border-l-2 border-l-transparent',
                    )}
                  >
                    <div className={cn('font-medium truncate', t.status === 'revoked' ? 'text-gray-400 line-through' : 'text-gray-900')}>
                      {tokenLabel(t)}
                    </div>
                    <div className="mt-1 flex items-center gap-1.5 text-xs text-gray-500">
                      <span className={cn('inline-block w-2 h-2 rounded-full', STATUS_DOT[t.status] || 'bg-gray-400')} />
                      <span>{getStatusText(t)}</span>
                      <span>·</span>
                      <span>
                        {t.seats_used}/{t.max_seats} seats
                      </span>
                      {t.status === 'active' && t.expires_at && (
                        <>
                          <span>·</span>
                          <span>
                            {Math.round((new Date(t.expires_at).getTime() - Date.now()) / 86400000)}d left
                          </span>
                        </>
                      )}
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </div>

        {/* RIGHT: detail */}
        <div className="bg-white rounded-lg border border-gray-200 p-6">
          {!selectedToken ? (
            <div className="text-center py-16">
              <Shield className="w-12 h-12 text-gray-300 mx-auto mb-3" />
              <p className="text-gray-500">Select a key to see its details and enrolled agents.</p>
            </div>
          ) : (
            <div className="space-y-5">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <h2 className={cn('text-xl font-semibold truncate', selectedToken.status === 'revoked' ? 'text-gray-400 line-through' : 'text-gray-900')}>
                    {tokenLabel(selectedToken)}
                  </h2>
                  <div className="mt-1">
                    <span className={cn('badge badge-lg', tokenStatusColor(selectedToken.status))}>
                      {getStatusText(selectedToken)}
                    </span>
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  {selectedToken.status === 'active' && selectedToken.seats_used < selectedToken.max_seats && (
                    <button
                      onClick={() => installWithKey(selectedToken.id)}
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-primary-700 bg-primary-50 border border-primary-200 rounded-md hover:bg-primary-100"
                      title="Jump to the enrollment flow pre-seeded with this key"
                    >
                      <Terminal className="w-4 h-4" />
                      Install with this key
                    </button>
                  )}
                  {selectedToken.status === 'active' && (
                    <button
                      onClick={() => handleRevokeToken(selectedToken.id, tokenLabel(selectedToken))}
                      disabled={revokeToken.isPending}
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-orange-700 bg-orange-50 border border-orange-200 rounded-md hover:bg-orange-100 disabled:opacity-50"
                      title="Revoke key — stops new enrollments, enrolled agents keep working"
                    >
                      <AlertTriangle className="w-4 h-4" />
                      Revoke key
                    </button>
                  )}
                  <button
                    onClick={() => handleDeleteToken(selectedToken.id, tokenLabel(selectedToken))}
                    disabled={deleteToken.isPending}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-red-700 bg-red-50 border border-red-200 rounded-md hover:bg-red-100 disabled:opacity-50"
                    title="Permanently delete key"
                  >
                    <Trash2 className="w-4 h-4" />
                    Delete
                  </button>
                </div>
              </div>

              <dl className="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 text-sm">
                <div>
                  <dt className="text-gray-500">Token</dt>
                  <dd className="mt-0.5 font-mono text-gray-900 break-all">
                    {selectedToken.token ? (
                      <span className="inline-flex items-center gap-2">
                        <code>{revealToken ? selectedToken.token : `rk_${'•'.repeat(20)}${selectedToken.token.slice(-4)}`}</code>
                        <button
                          onClick={() => setRevealToken((v) => !v)}
                          className="text-xs text-primary-600 hover:text-primary-800"
                        >
                          {revealToken ? 'hide' : 'reveal'}
                        </button>
                        <button
                          onClick={() => copyToClipboard(selectedToken.token!, 'detail-token')}
                          className="text-gray-400 hover:text-gray-600"
                          title="Copy token"
                        >
                          {copiedCommand === 'detail-token' ? <CheckCircle className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                        </button>
                      </span>
                    ) : (
                      <span className="text-gray-400 italic">
                        Not retrievable ({selectedToken.status} keys aren't decryptable)
                      </span>
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="text-gray-500">Seats</dt>
                  <dd className="mt-0.5 text-gray-900">
                    {selectedToken.seats_used} used / {selectedToken.max_seats}
                    {selectedToken.seats_used >= selectedToken.max_seats && (
                      <span className="ml-2 text-xs text-red-600">(Full)</span>
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="text-gray-500">Created</dt>
                  <dd className="mt-0.5 text-gray-900">
                    {formatDateTime(selectedToken.created_at)}
                    {selectedToken.created_by && <span className="text-gray-500"> by {selectedToken.created_by}</span>}
                  </dd>
                </div>
                <div>
                  <dt className="text-gray-500">Expires</dt>
                  <dd className="mt-0.5 text-gray-900">{formatDateTime(selectedToken.expires_at)}</dd>
                </div>
                <div>
                  <dt className="text-gray-500">Last used</dt>
                  <dd className="mt-0.5 text-gray-900">
                    {selectedToken.used_at ? formatDateTime(selectedToken.used_at) : 'Never'}
                  </dd>
                </div>
              </dl>

              <div className="alert alert-warning">
                <div className="flex items-start gap-3">
                  <AlertTriangle className="w-5 h-5 text-amber-600 mt-0.5 shrink-0" />
                  <p className="text-sm text-amber-800">
                    Revoking this key stops new enrollments. Agents already enrolled with it keep working — revoke them individually below if access must end.
                  </p>
                </div>
              </div>

              {/* Bound agents */}
              <div>
                <div className="flex items-center justify-between mb-2">
                  <h3 className="text-sm font-semibold text-gray-900">
                    Agents enrolled with this key
                    {boundAgentsData && <span className="ml-1 text-gray-500">({boundAgentsData.count})</span>}
                  </h3>
                  <button onClick={() => refetch()} className="text-xs text-gray-400 hover:text-gray-600 inline-flex items-center gap-1">
                    <RefreshCw className="w-3 h-3" />
                    Refresh
                  </button>
                </div>

                {boundAgentsLoading ? (
                  <div className="py-6 text-center">
                    <div className="inline-block animate-spin rounded-full h-5 w-5 border-b-2 border-primary-600"></div>
                  </div>
                ) : boundAgents.length === 0 ? (
                  <div className="py-8 text-center bg-gray-50 rounded-lg border border-gray-200">
                    <Users className="w-8 h-8 text-gray-300 mx-auto mb-2" />
                    <p className="text-sm text-gray-500">No agents have enrolled with this key yet.</p>
                  </div>
                ) : (
                  <div className="overflow-x-auto border border-gray-200 rounded-lg">
                    <table className="min-w-full divide-y divide-gray-200">
                      <thead className="bg-gray-50">
                        <tr>
                          <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Hostname</th>
                          <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">OS</th>
                          <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                          <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Last seen</th>
                          <th className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase">Enrolled</th>
                          <th className="px-4 py-2"></th>
                        </tr>
                      </thead>
                      <tbody className="bg-white divide-y divide-gray-100">
                        {boundAgents.map((ba: BoundAgent) => {
                          const online = isOnline(ba.last_seen);
                          return (
                            <tr key={ba.agent_id} className="hover:bg-gray-50">
                              <td className="px-4 py-2.5">
                                <Link to={`/agents/${ba.agent_id}`} className="font-mono text-sm text-primary-600 hover:text-primary-800">
                                  {ba.hostname}
                                </Link>
                              </td>
                              <td className="px-4 py-2.5 text-sm text-gray-600 capitalize">{ba.os_type}</td>
                              <td className="px-4 py-2.5">
                                <span className="inline-flex items-center gap-1.5 text-sm text-gray-700">
                                  <span className={cn('inline-block w-2 h-2 rounded-full', online ? 'bg-green-500' : 'bg-gray-400')} />
                                  {online ? 'online' : 'offline'}
                                </span>
                              </td>
                              <td className="px-4 py-2.5 text-sm text-gray-600">{formatRelativeTime(ba.last_seen)}</td>
                              <td className="px-4 py-2.5 text-sm text-gray-600">{formatDateTime(ba.used_at)}</td>
                              <td className="px-4 py-2.5 text-right">
                                <button
                                  onClick={() => handleRevokeAgent(ba.agent_id, ba.hostname)}
                                  disabled={revokeAgentMutation.isPending}
                                  className="inline-flex items-center gap-1 px-2.5 py-1 text-xs text-red-700 bg-red-50 border border-red-200 rounded hover:bg-red-100 disabled:opacity-50"
                                  title="Revoke agent — invalidates refresh tokens"
                                >
                                  <AlertTriangle className="w-3 h-3" />
                                  Revoke agent
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Server signing key — not per-key, so it lives here rather than
          repeated inside every token's detail pane. */}
      <div className="card">
        <h2 className="text-lg font-semibold text-gray-900 mb-1 inline-flex items-center gap-2">
          <KeyRound className="w-5 h-5 text-gray-500" />
          Server signing key
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Signs agent update packages so agents can verify what they're installing.
        </p>
        {isLoadingServerKeySecurity ? (
          <div className="py-3 text-center">
            <div className="inline-block animate-spin rounded-full h-5 w-5 border-b-2 border-primary-600"></div>
          </div>
        ) : serverKeySecurity?.has_private_key ? (
          <div className="flex flex-col sm:flex-row sm:items-center gap-3">
            <div className="alert alert-success rounded-md p-2.5 flex-1">
              <p className="text-sm text-green-800 inline-flex items-center gap-2">
                <CheckCircle className="w-4 h-4" />
                Server has a private key for signing agent updates.
              </p>
            </div>
            <code className="text-xs text-gray-600 bg-gray-100 px-3 py-2 rounded font-mono">
              {serverKeySecurity.public_key_fingerprint}
            </code>
          </div>
        ) : (
          <div className="flex flex-col sm:flex-row sm:items-center gap-3">
            <div className="alert alert-warning rounded-md p-2.5 flex-1">
              <p className="text-sm text-amber-800">
                Server is missing a private key — generate one to enable secure agent updates.
              </p>
            </div>
            <button
              onClick={generateKeys}
              disabled={generatingKeys}
              className="inline-flex items-center justify-center gap-2 px-4 py-2 text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 rounded-md disabled:opacity-50"
            >
              {generatingKeys ? (
                <>
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
                  Generating...
                </>
              ) : (
                <>
                  <Key className="w-4 h-4" />
                  Generate signing keys
                </>
              )}
            </button>
          </div>
        )}
      </div>
    </div>
  );
};

const StatCard: React.FC<{
  label: string;
  value: React.ReactNode;
  valueClass?: string;
  icon: React.ComponentType<{ className?: string }>;
}> = ({ label, value, valueClass, icon: Icon }) => (
  <div className="card card-sm">
    <div className="flex items-center justify-between">
      <div>
        <p className="text-sm text-gray-600">{label}</p>
        <p className={cn('text-2xl font-bold text-gray-900', valueClass)}>{value}</p>
      </div>
      <Icon className="w-7 h-7 text-gray-400" />
    </div>
  </div>
);

export default AgentsEnrollment;
