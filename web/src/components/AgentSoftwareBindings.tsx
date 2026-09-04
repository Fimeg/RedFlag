import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { useConfirm } from '@/components/primitives';
import {
  GitBranch,
  Plus,
  AlertOctagon,
  CheckCircle,
  Trash2,
  ArrowRight,
  ExternalLink,
  Clock,
} from 'lucide-react';
import {
  useAgentBindings,
  useUpsertAgentBinding,
  useDeleteAgentBinding,
} from '@/hooks/useAgentBindings';
import { useTrackedSoftware } from '@/hooks/useUpstream';
import { CreateAgentBindingRequest, AgentTrackedSoftwareView, UpstreamSource } from '@/types';

interface AgentSoftwareBindingsProps {
  agentId: string;
}

// Per-source URL — matches the helper in pages/settings/UpstreamTracking.tsx
// so the same affordance is consistent across surfaces. (kept inline because
// extracting one helper for two callers is premature.)
const sourceHref = (source: UpstreamSource, ref: string): string | null => {
  switch (source) {
    case 'repology': return `https://repology.org/project/${ref}/versions`;
    case 'endoflife': return `https://endoflife.date/${ref}`;
    case 'github': return `https://github.com/${ref}/releases`;
    case 'gitlab': return `https://gitlab.com/${ref}/-/releases`;
    case 'bitbucket': return `https://bitbucket.org/${ref}/downloads/?tab=tags`;
    case 'git': return ref;
    default: return null;
  }
};

const driftBadge = (b: AgentTrackedSoftwareView): React.ReactNode => {
  if (b.past_eol) {
    return (
      <span className="chip chip-danger">
        <AlertOctagon className="w-3 h-3" />
        Past EOL
      </span>
    );
  }
  if (b.drifted) {
    return (
      <span className="chip chip-warning">
        Behind upstream
      </span>
    );
  }
  if (b.latest_version) {
    return (
      <span className="chip chip-success">
        <CheckCircle className="w-3 h-3" />
        Up to date
      </span>
    );
  }
  return (
    <span className="chip chip-neutral">
      <Clock className="w-3 h-3" />
      Awaiting sync
    </span>
  );
};

const AgentSoftwareBindings: React.FC<AgentSoftwareBindingsProps> = ({ agentId }) => {
  const { data: bindings, isPending } = useAgentBindings(agentId);
  const { data: trackedData } = useTrackedSoftware();
  const upsert = useUpsertAgentBinding(agentId);
  const remove = useDeleteAgentBinding(agentId);
  const confirm = useConfirm();

  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState<CreateAgentBindingRequest>({
    tracked_software_id: '',
    installed_version: '',
    install_path: '',
    notes: '',
  });

  const allTracked = trackedData?.software ?? [];
  const boundIDs = new Set((bindings ?? []).map((b) => b.tracked_software_id));
  const bindable = allTracked.filter((s) => !boundIDs.has(s.id));

  const reset = () => {
    setForm({ tracked_software_id: '', installed_version: '', install_path: '', notes: '' });
    setShowAdd(false);
  };

  const handleAdd = () => {
    if (!form.tracked_software_id || !form.installed_version) return;
    upsert.mutate(
      {
        tracked_software_id: form.tracked_software_id,
        installed_version: form.installed_version,
        install_path: form.install_path || null,
        notes: form.notes || null,
      },
      { onSuccess: reset },
    );
  };

  const list = bindings ?? [];

  return (
    <div className="card">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <GitBranch className="h-5 w-5 text-gray-400" />
          <h2 className="text-lg font-medium text-gray-900">Tracked Software</h2>
          {list.length > 0 && (
            <span className="text-sm text-gray-500">({list.length})</span>
          )}
        </div>
        {allTracked.length > 0 && bindable.length > 0 && (
          <button
            onClick={() => setShowAdd((v) => !v)}
            className="inline-flex items-center gap-1 px-3 py-1.5 text-sm bg-primary-600 text-white rounded hover:bg-primary-700"
          >
            <Plus className="w-4 h-4" />
            Bind
          </button>
        )}
      </div>

      <p className="text-xs text-gray-500 mb-4">
        Link this agent to tracked-software entries so upstream drift becomes per-host actionable.
        The installed version you supply here feeds the global Stack Drift panel.
      </p>

      {showAdd && bindable.length > 0 && (
        <div className="border border-gray-200 rounded-lg p-4 mb-4 bg-gray-50">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="md:col-span-2">
              <label className="block text-xs font-medium text-gray-700 mb-1">Tracked software</label>
              <select
                value={form.tracked_software_id}
                onChange={(e) => setForm({ ...form, tracked_software_id: e.target.value })}
                className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-primary-500"
              >
                <option value="">— choose —</option>
                {bindable.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name} ({s.source}: {s.source_ref})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 mb-1">Installed version</label>
              <input
                value={form.installed_version}
                onChange={(e) => setForm({ ...form, installed_version: e.target.value })}
                placeholder="e.g. 15.4"
                className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded font-mono focus:outline-none focus:ring-2 focus:ring-primary-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-700 mb-1">
                Install path <span className="text-gray-400">(optional)</span>
              </label>
              <input
                value={form.install_path ?? ''}
                onChange={(e) => setForm({ ...form, install_path: e.target.value })}
                placeholder="/opt/postgres"
                className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded font-mono focus:outline-none focus:ring-2 focus:ring-primary-500"
              />
            </div>
            <div className="md:col-span-2">
              <label className="block text-xs font-medium text-gray-700 mb-1">
                Notes <span className="text-gray-400">(optional — build flags, branch, etc.)</span>
              </label>
              <input
                value={form.notes ?? ''}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
                placeholder="built from main; --with-openssl"
                className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-primary-500"
              />
            </div>
          </div>
          <div className="mt-3 flex gap-2 justify-end">
            <button
              onClick={reset}
              className="px-3 py-1.5 text-sm bg-gray-200 text-gray-700 rounded hover:bg-gray-300"
            >
              Cancel
            </button>
            <button
              onClick={handleAdd}
              disabled={!form.tracked_software_id || !form.installed_version || upsert.isPending}
              className="px-3 py-1.5 text-sm bg-primary-600 text-white rounded hover:bg-primary-700 disabled:opacity-50"
            >
              {upsert.isPending ? 'Saving…' : 'Save binding'}
            </button>
          </div>
        </div>
      )}

      {isPending ? (
        <div className="text-center py-6">
          <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
        </div>
      ) : list.length === 0 ? (
        <div className="text-center py-8 border border-dashed border-gray-200 rounded-lg">
          <GitBranch className="w-10 h-10 mx-auto text-gray-300 mb-2" />
          {allTracked.length === 0 ? (
            <>
              <p className="text-sm text-gray-600 mb-2">
                Nothing tracked yet — add software in settings first.
              </p>
              <Link
                to="/settings/upstream"
                className="inline-flex items-center gap-1 text-sm text-primary-600 hover:text-primary-800"
              >
                Open Upstream Tracking <ArrowRight className="w-4 h-4" />
              </Link>
            </>
          ) : (
            <>
              <p className="text-sm text-gray-600 mb-2">
                No bindings on this agent yet. Click <span className="font-medium">Bind</span> to
                link a tracked entry to what's installed on this host.
              </p>
            </>
          )}
        </div>
      ) : (
        <ul className="divide-y divide-gray-100">
          {list.map((b) => {
            const href = sourceHref(b.source, b.source_ref);
            return (
              <li key={b.binding_id} className="py-3 flex items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-medium text-gray-900">{b.name}</span>
                    <span className="text-xs text-gray-500">
                      ({b.source}: <span className="font-mono">{b.source_ref}</span>)
                    </span>
                    {driftBadge(b)}
                  </div>
                  <div className="mt-1 text-xs text-gray-600 flex items-center gap-3 flex-wrap">
                    <span className="font-mono">
                      {b.installed_version}
                      {b.latest_version && b.latest_version !== b.installed_version && (
                        <>
                          {' '}
                          <span className="text-gray-400">→</span>{' '}
                          <span className={b.past_eol ? 'text-red-700' : 'text-amber-700'}>
                            {b.latest_version}
                          </span>
                        </>
                      )}
                    </span>
                    {b.install_path && (
                      <span className="text-gray-500 font-mono truncate" title={b.install_path}>
                        {b.install_path}
                      </span>
                    )}
                    {b.eol_at && (
                      <span className={b.past_eol ? 'text-red-700' : 'text-gray-500'}>
                        EOL {new Date(b.eol_at).toISOString().slice(0, 10)}
                      </span>
                    )}
                  </div>
                  {b.notes && (
                    <div className="mt-1 text-xs text-gray-500 italic">{b.notes}</div>
                  )}
                  {b.last_error && (
                    <div className="mt-1 text-xs text-red-600 truncate" title={b.last_error}>
                      sync error: {b.last_error}
                    </div>
                  )}
                </div>
                <div className="flex items-center gap-1 flex-shrink-0">
                  {href && (
                    <a
                      href={href}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="inline-flex items-center gap-1 px-2 py-1 text-xs text-gray-600 hover:bg-gray-100 rounded"
                      title="Open upstream source"
                    >
                      <ExternalLink className="w-3 h-3" />
                    </a>
                  )}
                  <button
                    onClick={async () => {
                      if (!(await confirm({
                        title: 'Remove Binding',
                        body: `Remove binding for ${b.name}? The tracked entry itself stays.`,
                        confirmLabel: 'Remove',
                        danger: true,
                      }))) return;
                      remove.mutate(b.binding_id);
                    }}
                    className="inline-flex items-center gap-1 px-2 py-1 text-xs text-red-600 hover:bg-red-50 rounded"
                    title="Remove binding"
                  >
                    <Trash2 className="w-3 h-3" />
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
};

export default AgentSoftwareBindings;
