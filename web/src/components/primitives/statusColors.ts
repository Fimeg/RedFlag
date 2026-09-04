// Single source of truth for status/severity -> Tailwind color class mappings.
//
// Each domain is kept separate even when two domains share identical colors
// today — they are semantically distinct and may diverge independently.
//
// Return value convention: all functions return a string of Tailwind classes
// suitable for direct use in className. Badge and border classes are included
// where the original code included them; text-only variants return only text
// classes.

// ---------------------------------------------------------------------------
// DOMAIN: package/vuln severity  (critical / high / medium / low / unknown)
// Used by: StateBadge (SeverityBadge), AgentUpdatesEnhanced severity chips
// Source authority: lib/utils.ts getSeverityColor
// ---------------------------------------------------------------------------
export function packageSeverityColor(severity: string): string {
  switch (severity) {
    case 'critical':
      return 'text-danger-600 bg-danger-100';
    case 'important':
    case 'high':
      return 'text-warning-600 bg-warning-100';
    case 'moderate':
    case 'medium':
      return 'text-info-600 bg-info-100';
    case 'low':
    case 'none':
      return 'text-gray-600 bg-gray-100';
    default:
      return 'text-gray-600 bg-gray-100';
  }
}

// Inline text-only severity accent used in AgentUpdatesEnhanced severity
// count chips (e.g. "3 critical"). Only a text color — no bg.
export function packageSeverityTextColor(severity: string): string {
  switch (severity) {
    case 'critical':
      return 'text-red-600';
    case 'high':
      return 'text-orange-600';
    case 'medium':
      return 'text-yellow-600';
    case 'low':
      return 'text-blue-600';
    default:
      return 'text-gray-600';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: package lifecycle status
// (online/offline/pending/approved/installing/installed/failed/ignored + docker variants)
// Used by: StateBadge (StatusBadge)
// Source authority: lib/utils.ts getStatusColor
// ---------------------------------------------------------------------------
export function packageStatusColor(status: string): string {
  switch (status) {
    case 'online':
      return 'text-success-600 bg-success-100';
    case 'offline':
      return 'text-danger-600 bg-danger-100';
    case 'pending':
      return 'text-warning-600 bg-warning-100';
    case 'checking_dependencies':
      return 'text-info-500 bg-info-100';
    case 'pending_dependencies':
      return 'text-orange-600 bg-orange-100';
    case 'approved':
      return 'text-info-600 bg-info-100';
    case 'installing':
      return 'text-indigo-600 bg-indigo-100';
    case 'installed':
      return 'text-success-600 bg-success-100';
    case 'failed':
      return 'text-danger-600 bg-danger-100';
    case 'ignored':
      return 'text-gray-500 bg-gray-100';
    // Docker image lifecycle (docker_images.status)
    case 'up-to-date':
      return 'text-success-600 bg-success-100';
    case 'update-available':
      return 'text-info-600 bg-info-100';
    case 'update-approved':
      return 'text-orange-600 bg-orange-100';
    case 'update-scheduled':
      return 'text-purple-600 bg-purple-100';
    case 'update-installing':
      return 'text-indigo-600 bg-indigo-100';
    case 'update-failed':
      return 'text-danger-600 bg-danger-100';
    default:
      return 'text-gray-600 bg-gray-100';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: lifecycle history entry status
// (installed / failed / rollback — values from update_version_history rows)
// Used by: PackageDetail.tsx and Updates.tsx lifecycle history sections
// Includes border class — these are rendered as bordered mini-badges.
// ---------------------------------------------------------------------------
export function lifecycleHistoryStatusColor(status: string): string {
  switch (status) {
    case 'installed':
      return 'bg-green-50 text-green-700 border-green-200';
    case 'failed':
      return 'bg-red-50 text-red-700 border-red-200';
    case 'rollback':
      return 'bg-amber-50 text-amber-700 border-amber-200';
    default:
      return 'bg-gray-50 text-gray-600 border-gray-200';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: command status (inline badge, not CommandStatusBadge primitive)
// (completed / failed / timed_out / cancelled / pending / sent / running)
//
// 'completed' is green everywhere — ruled 2026-06-12 (Casey); the gray
// rendering CommandStatusBadge used to carry was the odd one out.
// ---------------------------------------------------------------------------
export function commandStatusInlineColor(status: string): string {
  switch (status) {
    case 'completed':
      return 'bg-green-50 text-green-700 border-green-200';
    case 'failed':
    case 'timed_out':
      return 'bg-red-50 text-red-700 border-red-200';
    case 'cancelled':
      return 'bg-gray-50 text-gray-700 border-gray-200';
    case 'pending':
    case 'sent':
    case 'running':
    default:
      return 'bg-blue-50 text-blue-700 border-blue-200';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: registration token status  (active / used / expired / revoked)
// Used by: TokenManagement.tsx
// Returns text-only color — used inside a badge element that supplies bg.
// ---------------------------------------------------------------------------
export function tokenStatusColor(status: string): string {
  switch (status) {
    case 'active':
      return 'text-green-600';
    case 'used':
      return 'text-yellow-600';
    case 'expired':
      return 'text-red-600';
    case 'revoked':
      return 'text-gray-500';
    default:
      return 'text-gray-500';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: history log result  (success / failed / started / running / partial)
// Used by: HistoryTimeline.tsx
// Includes border class — these are rendered as bordered badge spans.
// ---------------------------------------------------------------------------
export function historyResultColor(result: string): string {
  switch (result) {
    case 'success':
      return 'text-green-700 bg-green-100 border-green-200';
    case 'failed':
      return 'text-red-700 bg-red-100 border-red-200';
    case 'started':
    case 'running':
      return 'text-blue-700 bg-blue-100 border-blue-200';
    case 'partial':
      return 'text-amber-700 bg-amber-100 border-amber-200';
    default:
      return 'text-gray-700 bg-gray-100 border-gray-200';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: security event severity  (critical / error / warn / info)
// Used by: SecurityEvents.tsx
// These are audit-log severity levels — semantically distinct from package
// vulnerability severity even though 'critical' appears in both.
// Includes border class — icon wrapper uses a bordered rounded-lg container.
// ---------------------------------------------------------------------------
export function securityEventSeverityColor(severity: string): string {
  switch (severity) {
    case 'critical':
    case 'error':
      return 'text-red-600 bg-red-50 border-red-200';
    case 'warn':
      return 'text-yellow-600 bg-yellow-50 border-yellow-200';
    case 'info':
      return 'text-blue-600 bg-blue-50 border-blue-200';
    default:
      return 'text-gray-600 bg-gray-50 border-gray-200';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: security overall health  (healthy / warning / critical / unknown)
// Used by: SecurityStatusCard.tsx
// Includes border and text — applied to a bordered container.
// ---------------------------------------------------------------------------
export function securityHealthColor(overall: string): string {
  switch (overall) {
    case 'healthy':
      return 'bg-green-50 border-green-200 text-green-900';
    case 'warning':
      return 'bg-yellow-50 border-yellow-200 text-yellow-900';
    case 'critical':
      return 'bg-red-50 border-red-200 text-red-900';
    default:
      return 'bg-gray-50 border-gray-200 text-gray-900';
  }
}

// ---------------------------------------------------------------------------
// DOMAIN: security subsystem status  (healthy / enforced / degraded / unhealthy)
// Used by: AgentHealth.tsx security grid
// Includes border class — rendered as bordered pill badges.
// ---------------------------------------------------------------------------
export function securitySubsystemStatusColor(status: string): string {
  switch (status) {
    case 'healthy':
      return 'bg-green-100 text-green-700 border-green-200';
    case 'enforced':
      return 'bg-blue-100 text-blue-700 border-blue-200';
    case 'degraded':
      return 'bg-amber-100 text-amber-700 border-amber-200';
    case 'unhealthy':
    default:
      return 'bg-red-100 text-red-700 border-red-200';
  }
}
