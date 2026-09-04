import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useConfirm } from '@/components/primitives';
import {
  GitBranch,
  Plus,
  RefreshCw,
  Trash2,
  AlertOctagon,
  CheckCircle,
  ExternalLink,
  ChevronDown,
  ChevronRight,
  Computer,
} from 'lucide-react';
import {
  useTrackedSoftware,
  useAddTrackedSoftware,
  useRemoveTrackedSoftware,
  useSyncTrackedSoftware,
  useUpdateTrackedSoftwareSettings,
  useInstallations,
} from '@/hooks/useUpstream';
import { CreateTrackedSoftwareRequest, UpstreamSource } from '@/types';

// Curated seeds for "common stack" one-click adds. (source, source_ref) tuples
// follow the same mapping the API expects. Kept on endoflife.date because
// the seeded projects all have EOL coverage there — the chips exist to get
// new operators to a working drift panel in one click, not to demo every
// adapter the registry happens to support.
//
// To extend: just append. The set is deliberately small — operators add
// their own from there, and we don't ship an opinion about "what every server
// needs to track."
const COMMON_SEEDS: CreateTrackedSoftwareRequest[] = [
  { name: 'PostgreSQL',  ecosystem: 'system', source: 'endoflife', source_ref: 'postgresql',    repology_slug: 'postgresql' },
  { name: 'nginx',       ecosystem: 'system', source: 'endoflife', source_ref: 'nginx',         repology_slug: 'nginx' },
  { name: 'Node.js',     ecosystem: 'language', source: 'endoflife', source_ref: 'nodejs',       repology_slug: 'nodejs' },
  { name: 'Python',      ecosystem: 'language', source: 'endoflife', source_ref: 'python',       repology_slug: 'python' },
  { name: 'Redis',       ecosystem: 'system', source: 'endoflife', source_ref: 'redis',         repology_slug: 'redis' },
  { name: 'Docker',      ecosystem: 'system', source: 'endoflife', source_ref: 'docker-engine', repology_slug: 'moby' },
  { name: 'Go',          ecosystem: 'language', source: 'endoflife', source_ref: 'go',           repology_slug: 'golang' },
  { name: 'Kubernetes',  ecosystem: 'system', source: 'endoflife', source_ref: 'kubernetes',    repology_slug: 'kubernetes' },
  { name: 'Ubuntu',      ecosystem: 'system', source: 'endoflife', source_ref: 'ubuntu',        repology_slug: 'ubuntu' },
  { name: 'Debian',      ecosystem: 'system', source: 'endoflife', source_ref: 'debian',        repology_slug: 'debian' },
];

const sourceLabel = (s: UpstreamSource): string => {
  switch (s) {
    case 'repology':  return 'Repology';
    case 'endoflife': return 'endoflife.date';
    case 'anitya':    return 'Anitya';
    case 'github':    return 'GitHub Releases';
    case 'forgejo':   return 'Forgejo Releases';
    case 'gitea':     return 'Gitea Releases';
    case 'gitlab':    return 'GitLab Releases';
    case 'bitbucket': return 'Bitbucket Tags';
    case 'git':       return 'Git ls-remote';
    case 'npm':       return 'npm';
    case 'pypi':      return 'PyPI';
  }
};

// Per-source hint shown next to the source_ref input. Each source
// expects a different shape; surfacing the convention here cuts down
// on "why does it say 404" support questions.
const sourceRefHint = (s: UpstreamSource): { placeholder: string; help: string } => {
  switch (s) {
    case 'repology':  return { placeholder: 'nginx',                       help: 'Repology project slug (lowercase, kebab)' };
    case 'endoflife': return { placeholder: 'postgresql',                  help: 'endoflife.date product slug' };
    case 'anitya':    return { placeholder: 'nginx',                       help: 'Anitya project name' };
    case 'github':    return { placeholder: 'kubernetes/kubernetes',       help: 'GitHub owner/repo. Set REDFLAG_GITHUB_TOKEN for higher rate limit.' };
    case 'forgejo':   return { placeholder: 'codeberg.org/Fimeg/RedFlag',  help: 'Forgejo host/owner/repo, such as codeberg.org/Fimeg/RedFlag.' };
    case 'gitea':     return { placeholder: 'Fimeg/RedFlag',               help: 'Gitea owner/repo, or host/owner/repo. REDFLAG_GITEA_HOST used for two-part refs.' };
    case 'gitlab':    return { placeholder: 'gitlab-org/gitlab',           help: 'GitLab group/project path. REDFLAG_GITLAB_TOKEN for private projects.' };
    case 'bitbucket': return { placeholder: 'atlassian/atlassian-sdk',     help: 'Bitbucket workspace/repo. Highest tag picked via semver compare.' };
    case 'git':       return { placeholder: 'https://git.kernel.org/….git', help: 'Any Git clone URL. Anonymous ls-remote; private repos use the dedicated adapters.' };
    case 'npm':       return { placeholder: 'express',                     help: 'npm package name' };
    case 'pypi':      return { placeholder: 'django',                      help: 'PyPI package name' };
  }
};

// Map a tracked-software row to its canonical web URL for the
// "Source" link in the actions column. Falls back to the row's
// stored source_url when an adapter doesn't have a predictable URL
// pattern (or when host is env-configurable).
const forgejoHref = (ref: string): string | null => {
  const clean = ref.trim().replace(/\/+$/, '');
  if (/^https?:\/\//.test(clean)) return `${clean}/releases`;
  const parts = clean.split('/');
  if (parts.length === 3 && parts.every(Boolean)) {
    return `https://${parts[0]}/${parts[1]}/${parts[2]}/releases`;
  }
  return null;
};

const sourceHref = (s: { source: UpstreamSource; source_ref: string }): string | null => {
  switch (s.source) {
    case 'repology':  return `https://repology.org/project/${s.source_ref}/versions`;
    case 'endoflife': return `https://endoflife.date/${s.source_ref}`;
    case 'github':    return `https://github.com/${s.source_ref}/releases`;
    case 'forgejo':   return forgejoHref(s.source_ref);
    case 'gitlab':    return `https://gitlab.com/${s.source_ref}/-/releases`;
    case 'bitbucket': return `https://bitbucket.org/${s.source_ref}/downloads/?tab=tags`;
    case 'git':       return s.source_ref; // already a URL
    case 'gitea':     return forgejoHref(s.source_ref);
    case 'anitya':
    case 'npm':
    case 'pypi':
      return null; // host varies per deployment; rely on operator knowing
  }
};

const supportsPrereleaseToggle = (source: UpstreamSource): boolean =>
  source === 'forgejo' || source === 'gitea';

// InstallationsRow lazy-fetches the agent list for a tracked_software entry
// when the operator expands the row. Kept inside this file because it's only
// the table's expanded-row affordance, not a reusable component.
const InstallationsRow: React.FC<{ softwareId: string }> = ({ softwareId }) => {
  const { data, isPending } = useInstallations(softwareId);
  if (isPending) {
    return (
      <div className="text-xs text-gray-500 py-2">Loading installations…</div>
    );
  }
  if (!data || data.length === 0) {
    return (
      <div className="text-xs text-gray-500 py-2">
        No agents have this bound. Open an agent's
        <span className="font-medium"> Tracked Software</span> tab to add a binding.
      </div>
    );
  }
  return (
    <ul className="space-y-1 py-2">
      {data.map((i) => (
        <li
          key={i.binding_id}
          className="flex items-center gap-3 text-xs"
        >
          <Computer className="w-3 h-3 text-gray-400 flex-shrink-0" />
          <Link
            to={`/agents/${i.agent_id}`}
            className="text-blue-700 hover:underline font-medium truncate"
          >
            {i.hostname}
          </Link>
          <span className="font-mono text-gray-700">{i.installed_version}</span>
          {i.install_path && (
            <span className="text-gray-500 font-mono truncate" title={i.install_path}>
              {i.install_path}
            </span>
          )}
          <span className="text-gray-400 ml-auto flex-shrink-0">
            seen {new Date(i.last_observed_at).toLocaleDateString()}
          </span>
        </li>
      ))}
    </ul>
  );
};

const UpstreamTracking: React.FC = () => {
  const navigate = useNavigate();
  const { data, isPending } = useTrackedSoftware();
  const add = useAddTrackedSoftware();
  const remove = useRemoveTrackedSoftware();
  const sync = useSyncTrackedSoftware();
  const updateSettings = useUpdateTrackedSoftwareSettings();
  const confirm = useConfirm();

  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const toggleExpand = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const [showAddForm, setShowAddForm] = useState(false);
  const [form, setForm] = useState<CreateTrackedSoftwareRequest>({
    name: '',
    ecosystem: 'system',
    source: 'endoflife',
    source_ref: '',
    current_version: '',
    track_prereleases: false,
  });

  const software = data?.software ?? [];
  const availableSources = (data?.sources ?? ['repology', 'endoflife']) as string[];

  const handleAdd = () => {
    if (!form.name || !form.source_ref) return;
    add.mutate({ ...form, current_version: form.current_version || undefined }, {
      onSuccess: () => {
        setForm({ name: '', ecosystem: 'system', source: 'endoflife', source_ref: '', current_version: '', track_prereleases: false });
        setShowAddForm(false);
      },
    });
  };

  const handleSeed = (seed: CreateTrackedSoftwareRequest) => {
    add.mutate(seed);
  };

  const trackedRefs = new Set(software.map((s) => `${s.source}:${s.source_ref}`));
  const unseededCommon = COMMON_SEEDS.filter((s) => !trackedRefs.has(`${s.source}:${s.source_ref}`));

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      <button
        onClick={() => navigate('/settings')}
        className="text-sm text-gray-500 hover:text-gray-700 mb-4"
      >
        ← Back to Settings
      </button>

      <div className="mb-8">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-3xl font-bold text-gray-900">Upstream Version Tracking</h1>
            <p className="mt-2 text-gray-600">
              Compare your deployed versions against canonical upstream releases across package indexes,
              lifecycle feeds, forge releases, and repository tags.
            </p>
          </div>
          <button
            onClick={() => setShowAddForm((v) => !v)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
          >
            <Plus className="w-4 h-4" />
            Track Software
          </button>
        </div>
      </div>

      {showAddForm && (
        <div className="bg-white border border-gray-200 rounded-lg p-6 mb-6">
          <h2 className="text-lg font-semibold text-gray-900 mb-4">Track a new piece of software</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Display name</label>
              <input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="e.g. PostgreSQL"
                className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Ecosystem</label>
              <select
                value={form.ecosystem}
                onChange={(e) => setForm({ ...form, ecosystem: e.target.value })}
                className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                <option value="system">system</option>
                <option value="container">container</option>
                <option value="language">language</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Source</label>
              <select
                value={form.source}
                onChange={(e) => {
                  const nextSource = e.target.value as UpstreamSource;
                  setForm({
                    ...form,
                    source: nextSource,
                    track_prereleases: supportsPrereleaseToggle(nextSource) ? form.track_prereleases : false,
                  });
                }}
                className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {availableSources.map((s) => (
                  <option key={s} value={s}>{sourceLabel(s as UpstreamSource)}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Source reference
                <span className="ml-1 text-xs text-gray-500">{sourceRefHint(form.source).help}</span>
              </label>
              <input
                value={form.source_ref}
                onChange={(e) => setForm({ ...form, source_ref: e.target.value })}
                placeholder={sourceRefHint(form.source).placeholder}
                className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono"
              />
            </div>
            <div className="md:col-span-2">
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Current deployed version
                <span className="ml-1 text-xs text-gray-500">(optional — operator-supplied for now)</span>
              </label>
              <input
                value={form.current_version || ''}
                onChange={(e) => setForm({ ...form, current_version: e.target.value })}
                placeholder="e.g. 15.4"
                className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono"
              />
            </div>
            {supportsPrereleaseToggle(form.source) && (
              <label className="md:col-span-2 inline-flex items-center gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={!!form.track_prereleases}
                  onChange={(e) => setForm({ ...form, track_prereleases: e.target.checked })}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                Track prereleases
              </label>
            )}
          </div>
          <div className="mt-4 flex gap-2 justify-end">
            <button
              onClick={() => setShowAddForm(false)}
              className="px-4 py-2 bg-gray-200 text-gray-800 rounded hover:bg-gray-300"
            >
              Cancel
            </button>
            <button
              onClick={handleAdd}
              disabled={!form.name || !form.source_ref || add.isPending}
              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {add.isPending ? 'Adding...' : 'Track'}
            </button>
          </div>
        </div>
      )}

      {unseededCommon.length > 0 && (
        <div className="alert alert-info mb-6">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1">
              <h3 className="font-medium text-blue-900 mb-1">Common stack — one-click add</h3>
              <p className="text-sm text-blue-700">
                Track popular projects with their endoflife.date slugs already mapped. Click an item to track it.
              </p>
            </div>
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {unseededCommon.map((s) => (
              <button
                key={`${s.source}:${s.source_ref}`}
                onClick={() => handleSeed(s)}
                disabled={add.isPending}
                className="inline-flex items-center gap-1 px-3 py-1 bg-white border border-blue-300 text-blue-700 rounded hover:bg-blue-100 text-sm disabled:opacity-50"
              >
                <Plus className="w-3 h-3" />
                {s.name}
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-lg font-semibold text-gray-900">Tracked software ({software.length})</h2>
        </div>

        {isPending ? (
          <div className="p-12 text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
          </div>
        ) : software.length === 0 ? (
          <div className="p-12 text-center text-gray-600">
            <GitBranch className="w-12 h-12 mx-auto text-gray-400 mb-3" />
            <p>Nothing tracked yet. Use the form above or the common-stack chips to start.</p>
          </div>
        ) : (
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Software</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Source</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Current</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Latest</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">EOL</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Last sync</th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {software.map((s) => {
                const drift =
                  s.current_version && s.latest_version && s.current_version !== s.latest_version;
                const eolPassed = s.eol_at && new Date(s.eol_at) < new Date();
                const isExpanded = expanded.has(s.id);
                return (
                  <React.Fragment key={s.id}>
                  <tr className={eolPassed ? 'bg-red-50' : drift ? 'bg-yellow-50' : ''}>
                    <td className="px-6 py-3">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => toggleExpand(s.id)}
                          className="text-gray-400 hover:text-gray-700"
                          title={isExpanded ? 'Collapse installations' : 'Show installations'}
                        >
                          {isExpanded ? (
                            <ChevronDown className="w-4 h-4" />
                          ) : (
                            <ChevronRight className="w-4 h-4" />
                          )}
                        </button>
                        {eolPassed && <AlertOctagon className="w-4 h-4 text-red-600" />}
                        {!drift && !eolPassed && s.latest_version && (
                          <CheckCircle className="w-4 h-4 text-green-600" />
                        )}
                        <span className="font-medium text-gray-900">{s.name}</span>
                      </div>
                      <span className="text-xs text-gray-400 ml-6">{s.ecosystem}</span>
                    </td>
                    <td className="px-6 py-3 text-sm text-gray-600">
                      {sourceLabel(s.source)}
                      <div className="text-xs text-gray-400 font-mono">{s.source_ref}</div>
                      {s.track_prereleases && (
                        <div className="text-xs text-blue-600">prereleases included</div>
                      )}
                    </td>
                    <td className="px-6 py-3 font-mono text-sm">{s.current_version ?? <span className="text-gray-400">—</span>}</td>
                    <td className="px-6 py-3 font-mono text-sm">{s.latest_version ?? <span className="text-gray-400">—</span>}</td>
                    <td className="px-6 py-3 text-sm">
                      {s.eol_at ? (
                        <span className={eolPassed ? 'text-red-700 font-medium' : 'text-gray-700'}>
                          {new Date(s.eol_at).toISOString().slice(0, 10)}
                        </span>
                      ) : (
                        <span className="text-gray-400">—</span>
                      )}
                    </td>
                    <td className="px-6 py-3 text-xs text-gray-500">
                      {s.last_synced_at ? new Date(s.last_synced_at).toLocaleString() : (
                        <span className="text-gray-400">never</span>
                      )}
                      {s.last_error && (
                        <div className="text-red-600 truncate max-w-xs inline-flex items-center gap-1" title={s.last_error}>
                          <AlertOctagon className="h-3 w-3 flex-shrink-0" />
                          {s.last_error}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-3 text-right space-x-2 whitespace-nowrap">
                      {supportsPrereleaseToggle(s.source) && (
                        <label className="inline-flex items-center gap-1 px-2 py-1 text-xs text-gray-600">
                          <input
                            type="checkbox"
                            checked={s.track_prereleases}
                            disabled={updateSettings.isPending}
                            onChange={(e) => updateSettings.mutate({
                              id: s.id,
                              input: { track_prereleases: e.target.checked },
                            })}
                            className="h-3 w-3 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                          />
                          Prereleases
                        </label>
                      )}
                      <button
                        onClick={() => sync.mutate(s.id)}
                        disabled={sync.isPending}
                        className="inline-flex items-center gap-1 px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 rounded"
                      >
                        <RefreshCw className={`w-3 h-3 ${sync.isPending ? 'animate-spin' : ''}`} />
                        Sync
                      </button>
                      {sourceHref(s) && (
                        <a
                          href={sourceHref(s)!}
                          target="_blank"
                          rel="noreferrer noopener"
                          className="inline-flex items-center gap-1 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 rounded"
                        >
                          <ExternalLink className="w-3 h-3" />
                          Source
                        </a>
                      )}
                      <button
                        onClick={async () => {
                          if (!(await confirm({
                            title: 'Stop Tracking',
                            body: `Stop tracking ${s.name}? Drift events are kept for audit.`,
                            confirmLabel: 'Stop Tracking',
                            danger: true,
                          }))) return;
                          remove.mutate(s.id);
                        }}
                        className="inline-flex items-center gap-1 px-2 py-1 text-xs text-red-600 hover:bg-red-50 rounded"
                      >
                        <Trash2 className="w-3 h-3" />
                        Remove
                      </button>
                    </td>
                  </tr>
                  {isExpanded && (
                    <tr className="bg-gray-50">
                      <td colSpan={7} className="px-12 py-2">
                        <InstallationsRow softwareId={s.id} />
                      </td>
                    </tr>
                  )}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};

export default UpstreamTracking;
