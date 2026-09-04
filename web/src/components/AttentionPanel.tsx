import React from 'react';
import { Link } from 'react-router-dom';
import {
  AlertTriangle,
  AlertOctagon,
  ShieldAlert,
  XCircle,
  GitBranch,
  ChevronRight,
} from 'lucide-react';
import { useDashboardStats } from '@/hooks/useStats';
import { useDriftedSoftware, useRecentDriftEvents } from '@/hooks/useUpstream';
import { advisoryUrl, cveSeverityBadge, cn } from '@/lib/utils';
import type { TopThreat } from '@/types';

// Severity ranks: eol (100) > vuln (90) > failed (80) > major drift (70) > minor drift (40) > patch (20) > metadata (10)
const SEVERITY_RANK: Record<string, number> = {
  eol:      100,
  vuln:     90,
  failed:   80,
  major:    70,
  minor:    40,
  patch:    20,
  metadata: 10,
};

interface AlertItem {
  key: string;
  severity: string;        // 'eol' | 'major' | 'failed' | 'minor' | ...
  title: string;
  detail?: string;
  href?: string;
  icon: React.ComponentType<{ className?: string }>;
  extra?: React.ReactNode; // rendered below the detail line, inside the link
}

// TopThreatLines renders the worst open advisories inline so the operator
// sees what the threat count actually means without clicking through.
const TopThreatLines: React.FC<{ threats: TopThreat[]; total: number }> = ({ threats, total }) => (
  <div className="mt-1.5 space-y-1">
    {threats.map((t) => {
      const sev = cveSeverityBadge(t.severity);
      return (
        <div key={t.id} className="flex items-center gap-2 text-xs flex-wrap">
          <span className={cn('badge border text-[10px] font-bold px-1.5 py-0.5', sev.cls)}>
            {sev.label}
          </span>
          {t.known_exploited && (
            <span className="badge border text-[10px] font-bold px-1.5 py-0.5 bg-red-100 text-red-800 border-red-300">
              KEV
            </span>
          )}
          {t.cvss_score !== undefined && t.cvss_score > 0 && (
            <span className="text-orange-800 font-medium">CVSS {t.cvss_score.toFixed(1)}</span>
          )}
          <a
            href={advisoryUrl(t.id)}
            target="_blank"
            rel="noopener noreferrer"
            className="font-mono font-semibold text-orange-900 hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            {t.id}
          </a>
          <span className="text-orange-700 truncate">{t.packages}</span>
        </div>
      );
    })}
    {total > threats.length && (
      <p className="text-[11px] text-orange-700">+ {total - threats.length} more advisories</p>
    )}
  </div>
);

const AttentionPanel: React.FC = () => {
  const { data: stats } = useDashboardStats();
  const { data: drifted } = useDriftedSoftware();
  const { data: driftEvents } = useRecentDriftEvents();

  const alerts: AlertItem[] = [];

  // 1. Failed updates (requires investigation)
  if (stats && stats.failed_updates > 0) {
    alerts.push({
      key: 'failed-updates',
      severity: 'failed',
      title: `${stats.failed_updates} failed update${stats.failed_updates === 1 ? '' : 's'}`,
      detail: 'Inspect agent logs to determine cause',
      href: '/updates?status=failed',
      icon: XCircle,
    });
  }

  // 1b. Open threats — advisories affecting installed versions.
  if (stats && stats.open_threat_count > 0) {
    const n = stats.open_threat_count;
    alerts.push({
      key: 'open-threats',
      severity: 'vuln',
      title: `${n} open threat${n === 1 ? '' : 's'} in installed packages`,
      detail: 'Installed versions have active security advisories — patch to remediate',
      href: '/updates?vuln=true',
      icon: ShieldAlert,
      extra: stats.top_threats?.length ? (
        <TopThreatLines threats={stats.top_threats} total={n} />
      ) : undefined,
    });
  }

  // 1c. Available fixes — advisories on available versions (remediation).
  if (stats && stats.available_fix_count > 0) {
    const n = stats.available_fix_count;
    alerts.push({
      key: 'available-fixes',
      severity: 'patch',
      title: `${n} security fix${n === 1 ? '' : 'es'} available to apply`,
      detail: 'Updates carry security advisories — review and approve to apply',
      href: '/updates?vuln=true',
      icon: ShieldAlert,
    });
  }

  // 2. EOL software (drifted + past EOL date) - highest priority
  if (drifted) {
    const eolPassed = drifted.filter((s) => s.eol_at && new Date(s.eol_at) < new Date());
    for (const s of eolPassed) {
      alerts.push({
        key: `eol-${s.id}`,
        severity: 'eol',
        title: `${s.name} is past upstream EOL`,
        detail: `deployed ${s.current_version ?? '?'} — upstream ${s.latest_version ?? '?'} (EOL ${s.eol_at?.slice(0, 10)})`,
        href: '/settings/upstream',
        icon: AlertOctagon,
      });
    }
  }

  // 3. Recent drift events (latest_version moved on something we're tracking)
  if (driftEvents) {
    for (const ev of driftEvents.slice(0, 5)) {
      if (ev.drift_severity === 'metadata') continue;
      alerts.push({
        key: `drift-${ev.id}`,
        severity: ev.drift_severity,
        title: `Upstream moved: ${ev.from_version ?? '?'} → ${ev.to_version ?? '?'}`,
        detail: ev.note ?? `${ev.drift_severity} version change observed ${new Date(ev.observed_at).toLocaleDateString()}`,
        href: '/settings/upstream',
        icon: GitBranch,
      });
    }
  }

  alerts.sort((a, b) => (SEVERITY_RANK[b.severity] ?? 0) - (SEVERITY_RANK[a.severity] ?? 0));

  if (alerts.length === 0) {
    return null;  // calm dashboards stay calm
  }

  const top = alerts.slice(0, 6);

  return (
    <div className="bg-amber-50 border border-amber-200 rounded-lg p-5 mb-8">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <AlertTriangle className="h-5 w-5 text-amber-600" />
          <h2 className="text-base font-semibold text-amber-900">Attention</h2>
          <span className="text-xs text-amber-700">({alerts.length})</span>
        </div>
      </div>

      <ul className="space-y-2">
        {top.map((a) => {
          const Icon = a.icon;
          const eol = a.severity === 'eol';
          const vuln = a.severity === 'vuln';
          return (
            <li key={a.key}>
              <Link
                to={a.href ?? '#'}
                className={`flex items-start gap-3 p-2 rounded hover:bg-amber-100 ${eol ? 'bg-red-50' : vuln ? 'bg-orange-50' : ''}`}
              >
                <Icon className={`w-5 h-5 flex-shrink-0 mt-0.5 ${eol ? 'text-red-600' : vuln ? 'text-orange-600' : 'text-amber-700'}`} />
                <div className="flex-1 min-w-0">
                  <p className={`text-sm font-medium ${eol ? 'text-red-900' : vuln ? 'text-orange-900' : 'text-amber-900'}`}>
                    {a.title}
                  </p>
                  {a.detail && (
                    <p className={`text-xs ${eol ? 'text-red-700' : vuln ? 'text-orange-700' : 'text-amber-700'} truncate`}>
                      {a.detail}
                    </p>
                  )}
                  {a.extra}
                </div>
                <ChevronRight className={`w-4 h-4 flex-shrink-0 mt-1 ${eol ? 'text-red-400' : vuln ? 'text-orange-400' : 'text-amber-400'}`} />
              </Link>
            </li>
          );
        })}
      </ul>

      {alerts.length > top.length && (
        <p className="text-xs text-amber-700 mt-2">
          + {alerts.length - top.length} more — open the linked pages for details.
        </p>
      )}
    </div>
  );
};

export default AttentionPanel;
