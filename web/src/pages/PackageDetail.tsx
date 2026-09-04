import React, { useState } from 'react';
import { useParams, useNavigate, useSearchParams, Link } from 'react-router-dom';
import {
  Package,
  Computer,
  ChevronLeft,
  ChevronRight,
  AlertTriangle,
  Clock,
  Shield,
  ExternalLink,
  GitBranch,
  Activity,
  CheckCircle,
  XCircle,
  Loader2,
} from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import {
  usePackageSummary,
  usePackageAgents,
  usePackageVersionsByCoords,
  usePackageVulnerabilities,
  useUpdate,
  useUpdateLifecycle,
  useApproveUpdate,
  useInstallUpdate,
  useReopenUpdate,
} from '@/hooks/useUpdates';
import { useRecentCommands } from '@/hooks/useCommands';
import {
  formatBytes,
  formatRelativeTime,
} from '@/lib/utils';
import { cn } from '@/lib/utils';
import { lifecycleHistoryStatusColor, commandStatusInlineColor } from '@/components/primitives/statusColors';
import { StatusBadge, SeverityBadge } from '@/components/primitives';
import toast from 'react-hot-toast';
import DependencyClosureTree from '@/components/DependencyClosureTree';
import VulnerabilityList from '@/components/VulnerabilityList';
import { parseVulnerabilityList } from '@/lib/vulnerabilities';
import type { PackageFleetAgent } from '@/types';

const PackageDetail: React.FC = () => {
  const { type: pkgType = '', name: pkgName = '' } = useParams<{ type: string; name: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const agentFilter = searchParams.get('agent');
  const queryClient = useQueryClient();

  const [approvingId, setApprovingId] = useState<string | null>(null);
  const [installingId, setInstallingId] = useState<string | null>(null);
  const [retryingId, setRetryingId] = useState<string | null>(null);

  const { data: summary, isLoading: summaryLoading, error: summaryError } = usePackageSummary(pkgType, pkgName);
  const { data: agentsData, isLoading: agentsLoading } = usePackageAgents(pkgType, pkgName);
  const { data: versionsData } = usePackageVersionsByCoords(pkgType, pkgName);
  const { data: vulnsData } = usePackageVulnerabilities(pkgType, pkgName);

  // Agent-filtered: find the update_id for the selected agent
  const agentRow: PackageFleetAgent | undefined = agentsData?.agents?.find(
    (a) => a.agent_id === agentFilter
  );
  const agentUpdateId = agentRow?.update_id ?? '';

  const { data: agentUpdate } = useUpdate(agentUpdateId, !!agentFilter && !!agentUpdateId);
  const { data: lifecycleData } = useUpdateLifecycle(agentUpdateId, !!agentFilter && !!agentUpdateId);
  const { data: recentCommandsData } = useRecentCommands(50);

  const approveMutation = useApproveUpdate();
  const installMutation = useInstallUpdate();
  const reopenMutation = useReopenUpdate();

  const handleApprove = async (updateId: string, hostname: string) => {
    setApprovingId(updateId);
    try {
      await approveMutation.mutateAsync({ id: updateId });
      toast.success(`Approved update for ${hostname}`);
      queryClient.invalidateQueries({ queryKey: ['package-agents', pkgType, pkgName] });
    } catch {
      toast.error(`Failed to approve update for ${hostname}`);
    } finally {
      setApprovingId(null);
    }
  };

  const handleInstall = async (updateId: string, hostname: string) => {
    setInstallingId(updateId);
    try {
      await installMutation.mutateAsync(updateId);
      toast.success(`Install queued for ${hostname}`);
      queryClient.invalidateQueries({ queryKey: ['package-agents', pkgType, pkgName] });
    } catch {
      toast.error(`Failed to queue install for ${hostname}`);
    } finally {
      setInstallingId(null);
    }
  };

  const handleRetry = async (updateId: string, hostname: string) => {
    setRetryingId(updateId);
    try {
      await reopenMutation.mutateAsync(updateId);
      toast.success(`Reopened ${hostname} for retry`);
      queryClient.invalidateQueries({ queryKey: ['package-agents', pkgType, pkgName] });
    } catch {
      toast.error(`Failed to reopen update for ${hostname}`);
    } finally {
      setRetryingId(null);
    }
  };

  if (summaryLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-6 w-6 animate-spin text-gray-400" />
      </div>
    );
  }

  if (summaryError || !summary) {
    return (
      <div className="p-6">
        <button onClick={() => navigate('/updates')} className="btn btn-secondary mb-4 inline-flex items-center gap-1">
          <ChevronLeft className="h-3.5 w-3.5" /> Updates
        </button>
        <p className="text-sm text-red-600">Package not found: {pkgType}/{pkgName}</p>
      </div>
    );
  }

  const agents = agentsData?.agents ?? [];
  const versions = versionsData?.versions ?? [];
  const vulns = parseVulnerabilityList(vulnsData?.vulnerabilities ?? summary.vulnerabilities ?? []);
  const pendingAgents = agents.filter((a) => a.can_approve);
  const approvedAgents = agents.filter((a) => a.can_install);
  const failedAgents = agents.filter((a) => a.can_retry);

  return (
    <div className="space-y-4 px-4 sm:px-6 lg:px-8 max-w-5xl">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-xs text-gray-500">
        <button onClick={() => navigate('/updates')} className="hover:text-gray-700 inline-flex items-center gap-1">
          <ChevronLeft className="h-3 w-3" /> Updates
        </button>
        <span>/</span>
        <span className="text-gray-700 font-medium">{pkgName}</span>
        {agentFilter && agentRow && (
          <>
            <span>/</span>
            <button
              onClick={() => navigate(`/updates/package/${pkgType}/${pkgName}`)}
              className="hover:text-gray-700"
            >
              fleet
            </button>
            <span>/</span>
            <span className="text-gray-700">{agentRow.hostname}</span>
          </>
        )}
      </div>

      {/* Package header */}
      <div className="card">
        <div className="flex items-start gap-3">
          <Package className="h-5 w-5 text-gray-400 flex-shrink-0 mt-0.5" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-base font-semibold text-gray-900">{pkgName}</h1>
              <span className="text-xs text-gray-500 font-mono bg-gray-100 rounded px-1.5 py-0.5">{pkgType}</span>
              {summary.severity && summary.severity !== 'unknown' && (
                <SeverityBadge severity={summary.severity} />
              )}
              {vulns.length > 0 && (
                <span className="badge bg-amber-50 text-amber-700 border border-amber-200 text-xs inline-flex items-center gap-1">
                  <AlertTriangle className="h-3 w-3" />
                  {vulns.length} {vulns.length === 1 ? 'vuln' : 'vulns'}
                </span>
              )}
            </div>
            <div className="flex items-center gap-3 mt-1 flex-wrap text-xs text-gray-500">
              <span className="font-mono">
                latest: <span className="text-gray-700">{summary.latest_available || '—'}</span>
              </span>
              {summary.latest_installed && (
                <span className="font-mono">
                  installed: <span className="text-gray-700">{summary.latest_installed}</span>
                </span>
              )}
              {summary.size_bytes > 0 && (
                <span>{formatBytes(summary.size_bytes)}</span>
              )}
              {summary.homepage_url && (
                <a
                  href={summary.homepage_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 hover:text-gray-700"
                >
                  homepage <ExternalLink className="h-3 w-3" />
                </a>
              )}
            </div>
            {summary.package_description && (
              <p className="text-xs text-gray-600 mt-1.5">{summary.package_description}</p>
            )}
          </div>
        </div>
      </div>

      {/* Agent-filtered view */}
      {agentFilter ? (
        <AgentDetailPane
          agentRow={agentRow}
          agentUpdate={agentUpdate}
          lifecycleData={lifecycleData}
          recentCommands={recentCommandsData?.commands ?? []}
          pkgType={pkgType}
          pkgName={pkgName}
        />
      ) : (
        <>
          {/* Fleet Status */}
          <div className="card">
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2">
                <Computer className="h-4 w-4 text-gray-500" />
                Fleet Status
                <span className="text-xs text-gray-500 font-normal">({summary.total_agents})</span>
              </h2>
              <div className="flex items-center gap-1.5 flex-wrap">
                {summary.installed_count > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 rounded border bg-green-50 text-green-700 border-green-200">
                    {summary.installed_count} installed
                  </span>
                )}
                {summary.pending_count > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 rounded border bg-blue-50 text-blue-700 border-blue-200">
                    {summary.pending_count} pending
                  </span>
                )}
                {summary.approved_count > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 rounded border bg-indigo-50 text-indigo-700 border-indigo-200">
                    {summary.approved_count} approved
                  </span>
                )}
                {summary.active_count > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 rounded border bg-amber-50 text-amber-700 border-amber-200">
                    {summary.active_count} active
                  </span>
                )}
                {summary.failed_count > 0 && (
                  <span className="text-[10px] px-1.5 py-0.5 rounded border bg-red-50 text-red-700 border-red-200">
                    {summary.failed_count} failed
                  </span>
                )}
              </div>
            </div>

            {/* Bulk action bar */}
            {(pendingAgents.length > 0 || approvedAgents.length > 0 || failedAgents.length > 0) && (
              <div className="flex items-center gap-2 mb-3 pb-3 border-b border-gray-100">
                {pendingAgents.length > 1 && (
                  <button
                    className="btn btn-secondary text-xs"
                    onClick={() => {
                      pendingAgents.forEach((a) => handleApprove(a.update_id, a.hostname));
                    }}
                  >
                    Approve all pending ({pendingAgents.length})
                  </button>
                )}
                {approvedAgents.length > 1 && (
                  <button
                    className="btn btn-secondary text-xs"
                    onClick={() => {
                      approvedAgents.forEach((a) => handleInstall(a.update_id, a.hostname));
                    }}
                  >
                    Install all approved ({approvedAgents.length})
                  </button>
                )}
              </div>
            )}

            {agentsLoading ? (
              <div className="flex items-center justify-center h-16">
                <Loader2 className="h-4 w-4 animate-spin text-gray-400" />
              </div>
            ) : agents.length === 0 ? (
              <p className="text-sm text-gray-500 italic">No agents tracking this package.</p>
            ) : (
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-gray-100 text-gray-500 text-left">
                    <th className="pb-2 font-normal">Agent</th>
                    <th className="pb-2 font-normal">Current</th>
                    <th className="pb-2 font-normal">Available</th>
                    <th className="pb-2 font-normal">Status</th>
                    <th className="pb-2 font-normal text-right">Action</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {agents.map((a) => (
                    <tr key={a.agent_id} className="group">
                      <td className="py-2">
                        <button
                          onClick={() =>
                            navigate(`/updates/package/${pkgType}/${pkgName}?agent=${a.agent_id}`)
                          }
                          className="inline-flex items-center gap-1 text-gray-900 hover:text-indigo-700 group-hover:underline"
                        >
                          <Computer className="h-3 w-3 text-gray-400 flex-shrink-0" />
                          {a.hostname}
                        </button>
                      </td>
                      <td className="py-2 font-mono text-gray-600">{a.current_version}</td>
                      <td className="py-2 font-mono text-gray-700">
                        {a.selected_version ? (
                          <span title={`pinned to ${a.selected_version}`}>
                            {a.available_version}{' '}
                            <span className="text-[10px] text-indigo-600 ml-1">[{a.selected_version}]</span>
                          </span>
                        ) : (
                          a.available_version
                        )}
                      </td>
                      <td className="py-2">
                        <StatusBadge status={a.status} className="text-[10px]" label={a.status.replace(/_/g, ' ')} />
                      </td>
                      <td className="py-2 text-right">
                        {a.can_approve && (
                          <button
                            className="btn btn-secondary text-[10px] py-0.5 px-2"
                            disabled={approvingId === a.update_id}
                            onClick={() => handleApprove(a.update_id, a.hostname)}
                          >
                            {approvingId === a.update_id ? (
                              <Loader2 className="h-3 w-3 animate-spin inline" />
                            ) : (
                              'Approve'
                            )}
                          </button>
                        )}
                        {a.can_install && (
                          <button
                            className="btn btn-secondary text-[10px] py-0.5 px-2"
                            disabled={installingId === a.update_id}
                            onClick={() => handleInstall(a.update_id, a.hostname)}
                          >
                            {installingId === a.update_id ? (
                              <Loader2 className="h-3 w-3 animate-spin inline" />
                            ) : (
                              'Install'
                            )}
                          </button>
                        )}
                        {a.can_retry && (
                          <button
                            className="btn btn-secondary text-[10px] py-0.5 px-2"
                            disabled={retryingId === a.update_id}
                            onClick={() => handleRetry(a.update_id, a.hostname)}
                          >
                            {retryingId === a.update_id ? (
                              <Loader2 className="h-3 w-3 animate-spin inline" />
                            ) : (
                              'Retry'
                            )}
                          </button>
                        )}
                        {!a.can_approve && !a.can_install && !a.can_retry && (
                          <button
                            className="btn btn-secondary text-[10px] py-0.5 px-2 opacity-60"
                            onClick={() =>
                              navigate(`/updates/package/${pkgType}/${pkgName}?agent=${a.agent_id}`)
                            }
                          >
                            Details
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {/* Version Timeline */}
          {versions.length > 0 && (
            <div className="card">
              <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
                <Clock className="h-4 w-4 text-gray-500" />
                Version Timeline
                <span className="text-xs text-gray-500 font-normal">({versions.length})</span>
              </h2>
              <ul className="divide-y divide-gray-100">
                {versions.map((v) => {
                  const osv = v.osv_status;
                  const osvCls =
                    osv === 'vulnerable'
                      ? 'bg-amber-50 text-amber-700 border-amber-200'
                      : osv === 'clean'
                      ? 'bg-green-50 text-green-700 border-green-200'
                      : 'bg-gray-50 text-gray-500 border-gray-200';
                  const isLatest = v.version === summary.latest_available;
                  const isInstalled = v.version === summary.latest_installed;
                  return (
                    <li key={v.id} className="py-2.5 flex items-center gap-3 flex-wrap">
                      <span className="text-sm font-mono text-gray-900 flex-shrink-0">{v.version}</span>
                      {isInstalled && (
                        <span className="text-[10px] font-medium text-gray-600 bg-gray-100 rounded px-1.5 py-0.5">
                          installed
                        </span>
                      )}
                      {isLatest && !isInstalled && (
                        <span className="text-[10px] font-medium text-blue-700 bg-blue-50 rounded px-1.5 py-0.5">
                          available
                        </span>
                      )}
                      {osv && (
                        <span className={cn('text-[10px] font-medium border rounded px-1.5 py-0.5', osvCls)}>{osv}</span>
                      )}
                      {v.sha256 && (
                        <span
                          className="text-[10px] text-gray-400 font-mono flex-shrink-0"
                          title={v.sha256}
                        >
                          <Shield className="h-3 w-3 inline mr-0.5 text-green-500" />
                          {v.sha256.slice(0, 8)}…
                        </span>
                      )}
                      <span className="text-xs text-gray-500 ml-auto flex-shrink-0">
                        {v.published_at
                          ? `published ${formatRelativeTime(v.published_at)}`
                          : `seen ${formatRelativeTime(v.first_scanned_at)}`}
                      </span>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}

          <VulnerabilityList vulnerabilities={vulns} />
        </>
      )}
    </div>
  );
};

// AgentDetailPane renders the agent-scoped lifecycle, history, and commands
// for a single agent row within this package's fleet.
interface AgentDetailPaneProps {
  agentRow?: PackageFleetAgent;
  agentUpdate: any;
  lifecycleData: any;
  recentCommands: any[];
  pkgType: string;
  pkgName: string;
}

const AgentDetailPane: React.FC<AgentDetailPaneProps> = ({
  agentRow,
  agentUpdate,
  lifecycleData,
  recentCommands,
  pkgType,
  pkgName,
}) => {
  if (!agentRow) {
    return <p className="text-sm text-gray-500 italic">Agent not found in fleet for this package.</p>;
  }

  const agentCommands = recentCommands.filter(
    (cmd: any) =>
      cmd.agent_id === agentRow.agent_id &&
      cmd.package_name === pkgName &&
      cmd.package_type === pkgType
  );

  const deps: string[] = agentUpdate?.metadata?.dependencies ?? [];
  const closure = (() => {
    try {
      const raw = agentUpdate?.metadata?.resolved_closure;
      if (raw && typeof raw === 'string') return JSON.parse(raw);
      if (Array.isArray(raw)) return raw;
    } catch {
      // ignore
    }
    return undefined;
  })();

  return (
    <div className="space-y-4">
      {/* Agent context strip */}
      <div className="card flex items-center gap-3 text-sm text-gray-700">
        <Computer className="h-4 w-4 text-gray-400 flex-shrink-0" />
        <span className="font-medium">{agentRow.hostname}</span>
        <StatusBadge status={agentRow.status} label={agentRow.status.replace(/_/g, ' ')} />
        <span className="text-xs text-gray-500 font-mono ml-auto">
          {agentRow.current_version}
          <ChevronRight className="inline h-3 w-3 mx-0.5 text-gray-400" />
          {agentRow.available_version}
        </span>
        <Link
          to={`/updates/${agentRow.update_id}`}
          className="text-xs text-indigo-600 hover:underline ml-2 flex-shrink-0"
        >
          full detail
        </Link>
      </div>

      {/* Dependency closure */}
      {(deps.length > 0 || closure) && (
        <div className="card">
          <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
            <GitBranch className="h-4 w-4 text-gray-500" />
            Dependencies
          </h2>
          <DependencyClosureTree dependencies={deps} closure={closure} />
        </div>
      )}

      {/* Lifecycle history */}
      {lifecycleData?.history?.length > 0 && (
        <div className="card">
          <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
            <GitBranch className="h-4 w-4 text-gray-500" />
            Lifecycle History
            <span className="text-xs text-gray-500 font-normal">({lifecycleData.count})</span>
          </h2>
          <ul className="divide-y divide-gray-100">
            {lifecycleData.history.map((h: any) => {
              const cls = lifecycleHistoryStatusColor(h.update_status);
              const reason =
                h.update_status === 'failed'
                  ? h.failure_reason || h.metadata?.failure_reason
                  : null;
              return (
                <li key={h.id} className="py-2.5">
                  <div className="flex items-center gap-3 flex-wrap">
                    <span className={cn('text-[10px] font-medium border rounded px-1.5 py-0.5', cls)}>
                      {h.update_status}
                    </span>
                    <span className="text-sm font-mono text-gray-900">
                      {h.version_from} → {h.version_to}
                    </span>
                    {reason && (
                      <span className="text-xs text-red-600 truncate max-w-xs" title={reason}>
                        {reason}
                      </span>
                    )}
                    <span className="text-xs text-gray-500 ml-auto flex-shrink-0">
                      {formatRelativeTime(h.update_completed_at)}
                    </span>
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      {/* Recent commands */}
      {agentCommands.length > 0 && (
        <div className="card">
          <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
            <Activity className="h-4 w-4 text-gray-500" />
            Commands
            <span className="text-xs text-gray-500 font-normal">({agentCommands.length})</span>
          </h2>
          <ul className="space-y-1.5">
            {agentCommands.slice(0, 10).map((cmd: any) => (
              <li
                key={cmd.id}
                className="flex items-center gap-3 text-xs py-1.5 border-b border-gray-50 last:border-0"
              >
                {cmd.status === 'completed' ? (
                  <CheckCircle className="h-3.5 w-3.5 text-green-500 flex-shrink-0" />
                ) : cmd.status === 'failed' ? (
                  <XCircle className="h-3.5 w-3.5 text-red-500 flex-shrink-0" />
                ) : (
                  <Clock className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
                )}
                <span className="font-mono text-gray-700 flex-shrink-0">{cmd.command_type}</span>
                <span
                  className={cn('text-[10px] border rounded px-1 py-0.5 flex-shrink-0', commandStatusInlineColor(cmd.status))}
                >
                  {cmd.status}
                </span>
                <span className="text-gray-400 ml-auto flex-shrink-0">
                  {cmd.created_at ? formatRelativeTime(cmd.created_at) : '—'}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
};

export default PackageDetail;
