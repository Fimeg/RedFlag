import React from 'react';
import { Link } from 'react-router-dom';
import { GitBranch, AlertOctagon, ArrowRight } from 'lucide-react';
import { useDriftedSoftware } from '@/hooks/useUpstream';
import { TrackedSoftware } from '@/types';

// StackDriftPanel surfaces software whose deployed version doesn't match
// the canonical upstream release that the syncer pulled from Repology /
// endoflife.date. Past-EOL rows surface first, then everything else by
// most-recently-updated.
const StackDriftPanel: React.FC = () => {
  const { data: drifted, isPending } = useDriftedSoftware();

  const drift: TrackedSoftware[] = drifted ?? [];
  const eolCount = drift.filter((s) => s.eol_at && new Date(s.eol_at) < new Date()).length;
  const redflagDrift = drift.find(
    (s) => s.source === 'forgejo' && s.source_ref === 'codeberg.org/Fimeg/RedFlag',
  );
  const worst3 = drift.slice(0, 3);

  return (
    <div className="card">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-medium text-gray-900">Stack Drift</h2>
        <GitBranch className="h-5 w-5 text-gray-400" />
      </div>

      {isPending ? (
        <div className="text-center py-6">
          <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
        </div>
      ) : drift.length === 0 ? (
        <div className="text-center py-6">
          <p className="text-sm text-gray-600">
            Nothing being tracked yet, or everything matches upstream.
          </p>
          <Link
            to="/settings/upstream"
            className="mt-3 inline-flex items-center gap-1 text-sm text-primary-600 hover:text-primary-800"
          >
            Track something <ArrowRight className="w-4 h-4" />
          </Link>
        </div>
      ) : (
        <>
          <div className="flex items-baseline gap-6 mb-4">
            <div>
              <p className="text-3xl font-bold text-gray-900">{drift.length}</p>
              <p className="text-xs text-gray-600">behind upstream</p>
            </div>
            {eolCount > 0 && (
              <div>
                <p className="text-3xl font-bold text-red-600">{eolCount}</p>
                <p className="text-xs text-red-600">past EOL</p>
              </div>
            )}
          </div>

          {redflagDrift && (
            <div className="mb-4 rounded border border-amber-200 bg-amber-50 p-3">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-medium text-amber-900">RedFlag update available</p>
                  <p className="text-xs text-amber-800 font-mono mt-0.5">
                    {redflagDrift.current_version ?? '?'} → {redflagDrift.latest_version ?? '?'}
                  </p>
                </div>
                <Link
                  to="/settings/upstream"
                  className="text-xs text-amber-800 hover:text-amber-950 font-medium whitespace-nowrap"
                >
                  View row
                </Link>
              </div>
              <code className="mt-2 block overflow-x-auto rounded bg-white/80 px-2 py-1 text-xs text-amber-950">
                git checkout {redflagDrift.latest_version ?? 'v...'} && docker compose build server && docker compose up -d server
              </code>
            </div>
          )}

          <ul className="space-y-2">
            {worst3.map((s) => {
              const eolPassed = s.eol_at && new Date(s.eol_at) < new Date();
              return (
                <li
                  key={s.id}
                  className={`flex items-center justify-between p-2 rounded ${
                    eolPassed ? 'bg-red-50 border border-red-200' : 'bg-gray-50'
                  }`}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      {eolPassed && <AlertOctagon className="w-4 h-4 text-red-600 flex-shrink-0" />}
                      <span className="text-sm font-medium text-gray-900 truncate">{s.name}</span>
                      <span className="text-xs text-gray-400">({s.source})</span>
                    </div>
                    <p className="text-xs text-gray-600 mt-0.5 font-mono">
                      {s.current_version ?? '?'} → {s.latest_version ?? '?'}
                    </p>
                  </div>
                </li>
              );
            })}
          </ul>

          {drift.length > 3 && (
            <Link
              to="/settings/upstream"
              className="mt-3 inline-flex items-center gap-1 text-sm text-primary-600 hover:text-primary-800"
            >
              View all {drift.length} <ArrowRight className="w-4 h-4" />
            </Link>
          )}
        </>
      )}
    </div>
  );
};

export default StackDriftPanel;
