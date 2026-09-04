// Centralized polling intervals. Every hook that polls uses these constants
// instead of hardcoded magic numbers. The tiers are based on how volatile the
// data is and how directly the operator is watching it.
//
// LIVE     — real-time operations the operator is actively watching (commands)
// DASHBOARD — primary operator view, needs to feel responsive (stats, updates)
// DETAIL   — single-entity pages the operator opened on purpose (agent, package)
// OVERVIEW — fleet-wide lists, less time-critical (agent list, settings)
// STATIC   — admin data that almost never changes (tokens, upstream tracking)
// HEALTH   — fallback connection check (only when disconnected + unauthenticated)

export const POLL = {
  LIVE:       5_000,
  DASHBOARD: 15_000,
  DETAIL:    30_000,
  OVERVIEW:  60_000,
  STATIC:   300_000,
  HEALTH:    10_000,
} as const;
