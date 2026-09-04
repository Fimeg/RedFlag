// Integration framework.
//
// Agents report locally-detected integrations under `agent.metadata.integrations`.
// The architecture is poll-only: the agent observes its own host and folds what it
// finds into its regular report. The dashboard reads what the agent already knows —
// nothing in the web UI reaches into the host. An integration can be *observed*; it
// is not *commanded* from here.
//
// Today no agent reports this block yet, so the UI renders honest "not reported /
// not detected" states. The shape below is the contract the agent will fill.

export type IntegrationId = 'sunshine';

export type IntegrationState =
  | 'active' // present, running, and a session is live
  | 'running' // present and running, no active session
  | 'detected' // installed but not currently running
  | 'not_detected' // agent looked and did not find it
  | 'unknown'; // agent has not reported integration data yet

// Per-integration block the agent reports. All fields best-effort / optional —
// the agent reports what it can observe, nothing is required.
export interface ReportedIntegration {
  state?: IntegrationState;
  version?: string;
  last_observed?: string;
  // Sunshine-specific, observed-only:
  session_active?: boolean;
  session_started_at?: string;
  client_name?: string;
  web_ui?: string; // local Sunshine web UI, e.g. https://<host>:47990
}

export type ReportedIntegrations = Partial<Record<IntegrationId, ReportedIntegration>>;

// Static display metadata — never carries live state.
export interface IntegrationDescriptor {
  id: IntegrationId;
  name: string;
  vendor: string;
  blurb: string;
  docsUrl?: string;
}

export const INTEGRATION_CATALOG: Record<IntegrationId, IntegrationDescriptor> = {
  sunshine: {
    id: 'sunshine',
    name: 'Sunshine / Moonlight',
    vendor: 'LizardByte',
    blurb: 'Low-latency remote desktop, observed by the agent. A session without a capability token is an anomaly RedFlag can flag.',
    docsUrl: 'https://docs.lizardbyte.dev/projects/sunshine/',
  },
};

// Order the catalog renders in.
export const INTEGRATION_ORDER: IntegrationId[] = ['sunshine'];

export function readIntegrations(metadata?: Record<string, unknown>): ReportedIntegrations {
  const raw = metadata?.integrations;
  if (!raw || typeof raw !== 'object') return {};
  return raw as ReportedIntegrations;
}

export function resolveState(reported?: ReportedIntegration): IntegrationState {
  if (!reported || !reported.state) return 'unknown';
  if (reported.state === 'running' && reported.session_active) return 'active';
  return reported.state;
}
