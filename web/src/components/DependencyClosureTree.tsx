import React from 'react';
import { Link } from 'react-router-dom';
import { GitBranch, Shield, AlertTriangle, Circle, Package } from 'lucide-react';
import { cn } from '@/lib/utils';

// Shared dependency-closure renderer. Used by the update detail view today and
// intended for reuse in the package-inventory and staging views (see
// docs/tasks/UI-DEPENDENCY-CLOSURE-TREE.md, UI-UPDATE-DETAIL-PANELS.md,
// UI-STAGING-WORKFLOW.md, UI-PACKAGE-INVENTORY.md — all four describe the same
// component). Build it once, mount it everywhere.
//
// Two inputs feed it:
//   - `dependencies`: the flat name list the agent reports for dnf/apt (no
//     transitive resolution, no hashes).
//   - `closure`: the resolved closure pinned at token-mint time, carrying
//     version + sha256 per entry.
// When `closure` is present it is authoritative and renders the pinned/floating
// indicators. When only `dependencies` exist we render the flat list with an
// honest note that transitive resolution wasn't reported.

export interface ClosureEntry {
  name: string;
  version: string;
  sha256: string;
  source?: string;
}

interface Props {
  dependencies: string[];
  closure?: ClosureEntry[];
  // Package names known to be vulnerable (from per-dep OSV results, when
  // available). Matched case-insensitively against entry/dep names.
  vulnerableNames?: string[];
  // Header action slot (e.g. the "Review" button on pending_dependencies).
  headerAction?: React.ReactNode;
}

type DepStatus = 'pinned' | 'floating' | 'vulnerable';

const statusMeta: Record<DepStatus, { Icon: typeof Shield; cls: string; title: string }> = {
  vulnerable: { Icon: AlertTriangle, cls: 'text-red-600', title: 'Known vulnerability in this dependency' },
  pinned: { Icon: Shield, cls: 'text-green-600', title: 'Pinned — SHA-256 hash verified' },
  floating: { Icon: Circle, cls: 'text-amber-500', title: 'Floating — no hash pinned yet' },
};

const DependencyClosureTree: React.FC<Props> = ({
  dependencies,
  closure = [],
  vulnerableNames = [],
  headerAction,
}) => {
  const vulnSet = new Set(vulnerableNames.map((n) => n.toLowerCase()));
  const hasClosure = closure.length > 0;
  const count = hasClosure ? closure.length : dependencies.length;

  if (count === 0) return null;

  const statusOf = (name: string, sha256?: string): DepStatus => {
    if (vulnSet.has(name.toLowerCase())) return 'vulnerable';
    return sha256 ? 'pinned' : 'floating';
  };

  return (
    <div className="card">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2">
          <GitBranch className="h-4 w-4 text-gray-500" />
          {hasClosure ? 'Dependency Closure' : 'Dependencies'}
          <span className="text-xs text-gray-500 font-normal">({count})</span>
        </h2>
        {headerAction}
      </div>

      {hasClosure ? (
        <ul className="border border-gray-100 rounded divide-y divide-gray-100">
          {closure.map((entry, i) => {
            const status = statusOf(entry.name, entry.sha256);
            const meta = statusMeta[status];
            return (
              <li
                key={`${entry.name}-${i}`}
                className="py-2 px-3 flex items-center gap-2 text-xs"
              >
                {/* Tree connector for nested reading; flat depth for now since
                    dnf/apt report a single resolved level. */}
                <span className="text-gray-300 font-mono select-none">
                  {i === closure.length - 1 ? '└─' : '├─'}
                </span>
                <span title={meta.title} className="inline-flex flex-shrink-0">
                  <meta.Icon className={cn('h-3.5 w-3.5', meta.cls)} />
                </span>
                <Link
                  to={`/updates?search=${encodeURIComponent(entry.name)}`}
                  className="text-gray-900 font-mono truncate flex-shrink min-w-0 hover:text-indigo-700 hover:underline"
                >
                  {entry.name}
                </Link>
                <span className="text-gray-500 font-mono flex-shrink-0">{entry.version}</span>
                <span
                  className="text-gray-400 font-mono ml-auto truncate flex-shrink-0"
                  title={entry.sha256}
                >
                  {entry.sha256 ? entry.sha256.slice(0, 12) + '…' : 'unpinned'}
                </span>
              </li>
            );
          })}
        </ul>
      ) : (
        <>
          <ul className="divide-y divide-gray-100 -my-2">
            {dependencies.map((dep, i) => {
              const status = statusOf(dep);
              const meta = statusMeta[status];
              return (
                <li key={`${dep}-${i}`} className="py-2 flex items-center gap-2 text-sm">
                  {status === 'vulnerable' ? (
                    <span title={meta.title} className="inline-flex flex-shrink-0">
                      <meta.Icon className={cn('h-3.5 w-3.5', meta.cls)} />
                    </span>
                  ) : (
                    <Package className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
                  )}
                  <Link
                    to={`/updates?search=${encodeURIComponent(dep)}`}
                    className="text-gray-900 font-mono truncate hover:text-indigo-700 hover:underline"
                  >
                    {dep}
                  </Link>
                </li>
              );
            })}
          </ul>
          <p className="text-xs text-gray-400 mt-3 italic">
            Transitive resolution not yet reported for this ecosystem — names only, no
            versions or hashes until a dry-run pins the closure.
          </p>
        </>
      )}
    </div>
  );
};

export default DependencyClosureTree;
