import { useState } from 'react';
import {
  Download,
  CheckCircle,
  AlertCircle,
  RefreshCw,
  Info,
  Users,
  Package,
  Hash,
} from 'lucide-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { agentApi, updateApi } from '@/lib/api';
import toast from 'react-hot-toast';
import { cn, versionCompare } from '@/lib/utils';
import { Modal } from '@/components/primitives';
import { Agent } from '@/types';
import { clientLogger } from '@/lib/client-logger';

interface AgentUpdatesModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedAgentIds: string[];
  onAgentsUpdated: () => void;
}

export function AgentUpdatesModal({
  isOpen,
  onClose,
  selectedAgentIds,
  onAgentsUpdated,
}: AgentUpdatesModalProps) {
  const [selectedVersion, setSelectedVersion] = useState('');
  const [selectedPlatform] = useState('');
  const [isUpdating, setIsUpdating] = useState(false);

  // Fetch selected agents details
  const { data: agents = [] } = useQuery<Agent[]>({
    queryKey: ['agents-details', selectedAgentIds],
    queryFn: async (): Promise<Agent[]> => {
      const promises = selectedAgentIds.map(id => agentApi.getAgent(id));
      const results = await Promise.all(promises);
      return results;
    },
    enabled: isOpen && selectedAgentIds.length > 0,
  });

  // Fetch available update packages
  const { data: packagesResponse, isLoading: packagesLoading } = useQuery({
    queryKey: ['update-packages'],
    queryFn: () => updateApi.getPackages(),
    enabled: isOpen,
  });

  const allPackages = packagesResponse?.packages || [];

  // Get unique platform/arch for selected agents (simplified - assumes all agents same platform)
  const agentPlatform = agents[0]?.os_type || 'linux';
  const agentArchitecture = agents[0]?.os_architecture || 'amd64';

  // Restrict the candidate set to packages that actually match the selected
  // agents' platform+architecture. The server enforces this on POST too (it
  // returns 400 for an incompatible combo) — surface it here so the operator
  // never sees a row they can't actually install.
  const compatiblePackages = allPackages.filter(
    pkg => pkg.platform === agentPlatform && pkg.architecture === agentArchitecture
  );

  // Dedupe by (version, platform, architecture) — the server signs binaries
  // at every boot, which historically left multiple rows per tuple. Keep the
  // most recent row so the operator picks from one entry per real artifact.
  const dedupedPackages = Object.values(
    compatiblePackages.reduce<Record<string, typeof compatiblePackages[number]>>((acc, pkg) => {
      const key = `${pkg.version}|${pkg.platform}|${pkg.architecture}`;
      const existing = acc[key];
      if (!existing || new Date(pkg.created_at) > new Date(existing.created_at)) {
        acc[key] = pkg;
      }
      return acc;
    }, {})
  ).sort((a, b) => versionCompare(b.version, a.version));

  // No-downgrades policy. The floor is the highest current_version across all
  // selected agents, so a multi-select can't silently roll one of them back.
  // An agent with no reported current_version (fresh install / not yet checked
  // in) sets no floor — operator can install anything for it.
  const versionFloor = agents.reduce<string>((floor, agent) => {
    const v = agent.current_version || '';
    if (!v) return floor;
    if (!floor) return v;
    return versionCompare(v, floor) > 0 ? v : floor;
  }, '');

  const upgradeOnlyPackages = versionFloor
    ? dedupedPackages.filter(pkg => versionCompare(pkg.version, versionFloor) > 0)
    : dedupedPackages;

  const versions = [...new Set(upgradeOnlyPackages.map(pkg => pkg.version))].sort(
    (a, b) => versionCompare(b, a)
  );

  const availablePackages = upgradeOnlyPackages.filter(
    pkg => !selectedVersion || pkg.version === selectedVersion
  );

  // selectedPlatform is now driven entirely by agent platform; kept in state
  // for future multi-arch agent selection but unused in current filtering.
  void selectedPlatform;

  // Update agents mutation
  const updateAgentsMutation = useMutation({
    mutationFn: async (packageId: string) => {
      const pkg = dedupedPackages.find(p => p.id === packageId);
      if (!pkg) throw new Error('Package not found');
      if (versionFloor && versionCompare(pkg.version, versionFloor) <= 0) {
        throw new Error(
          `Downgrades are not allowed (target ${pkg.version} ≤ current ${versionFloor})`
        );
      }

      // For single agent updates, use individual update with nonce for security
      if (selectedAgentIds.length === 1) {
        const agentId = selectedAgentIds[0];

        // Generate nonce for security
        const nonceData = await agentApi.generateUpdateNonce(agentId, pkg.version);
        clientLogger.debug('Update nonce generated for single agent', { agentId, version: pkg.version });

        // Use individual update endpoint with nonce
        return agentApi.updateAgent(agentId, {
          version: pkg.version,
          platform: pkg.platform,
          nonce: nonceData.update_nonce
        });
      }

      // For multiple agents, generate nonces for each then bulk update
      const noncePromises = selectedAgentIds.map(async (agentId) => {
        try {
          const nonceData = await agentApi.generateUpdateNonce(agentId, pkg.version);
          return { agentId, nonce: nonceData.update_nonce };
        } catch {
          return null;
        }
      });
      const nonceResults = (await Promise.all(noncePromises)).filter(Boolean) as { agentId: string; nonce: string }[];

      if (nonceResults.length === 0) {
        throw new Error('Failed to generate nonces for any agents');
      }

      return agentApi.updateMultipleAgents({
        agent_ids: nonceResults.map(n => n.agentId),
        version: pkg.version,
        platform: pkg.platform,
        nonces: nonceResults.map(n => n.nonce),
      });
    },
    onSuccess: () => {
      const count = selectedAgentIds.length;
      const message = count === 1 ? 'Update initiated for agent' : `Update initiated for ${count} agents`;
      toast.success(message);
      setIsUpdating(false);
      onAgentsUpdated();
      onClose();
    },
    onError: (error: any) => {
      toast.error(`Failed to update agents: ${error.message}`);
      setIsUpdating(false);
    },
  });

  const handleUpdateAgents = async (packageId: string) => {
    setIsUpdating(true);
    updateAgentsMutation.mutate(packageId);
  };

  const canUpdate = selectedAgentIds.length > 0 && availablePackages.length > 0 && !isUpdating;
  const hasUpdatingAgents = agents.some(agent => agent.is_updating);

  if (!isOpen) return null;

  const modalTitle = (
    <div className="flex items-center space-x-3">
      <Download className="h-6 w-6 text-primary-600" />
      <div>
        <h3 className="text-lg font-semibold text-gray-900">Agent Updates</h3>
        <p className="text-sm text-gray-500">
          Update {selectedAgentIds.length} agent{selectedAgentIds.length !== 1 ? 's' : ''}
        </p>
      </div>
    </div>
  );

  return (
    <Modal open={isOpen} onClose={onClose} title={modalTitle} maxWidth="4xl">
      <Modal.Body>
            {/* Selected Agents */}
            <div className="mb-6">
              <h4 className="text-sm font-medium text-gray-900 mb-3 flex items-center">
                <Users className="h-4 w-4 mr-2" />
                Selected Agents
              </h4>
              <div className="max-h-32 overflow-y-auto space-y-2">
                {agents.map((agent) => (
                  <div key={agent.id} className="flex items-center justify-between p-2 bg-gray-50 rounded">
                    <div className="flex items-center space-x-3">
                      <CheckCircle className={cn(
                        "h-4 w-4",
                        agent.status === 'online' ? "text-green-500" : "text-gray-400"
                      )} />
                      <div>
                        <div className="text-sm font-medium text-gray-900">{agent.hostname}</div>
                        <div className="text-xs text-gray-500">
                          {agent.os_type}/{agent.os_architecture} • Current: {agent.current_version || 'Unknown'}
                        </div>
                      </div>
                    </div>
                    {agent.is_updating && (
                      <div className="flex items-center text-amber-600 text-xs">
                        <RefreshCw className="h-3 w-3 mr-1 animate-spin" />
                        Updating to {agent.updating_to_version}
                      </div>
                    )}
                  </div>
                ))}
              </div>
              {hasUpdatingAgents && (
                <div className="mt-2 text-xs text-amber-600 flex items-center">
                  <AlertCircle className="h-3 w-3 mr-1" />
                  Some agents are currently updating
                </div>
              )}
            </div>

            {/* Package Selection */}
            <div className="mb-6">
              <h4 className="text-sm font-medium text-gray-900 mb-3 flex items-center">
                <Package className="h-4 w-4 mr-2" />
                Update Package Selection
              </h4>

              {/* Filters — platform/arch fixed by selected agents; only version varies */}
              <div className="mb-4">
                <label className="block text-xs font-medium text-gray-700 mb-1">Version</label>
                <select
                  value={selectedVersion}
                  onChange={(e) => setSelectedVersion(e.target.value)}
                  className="w-full rounded-md border-gray-300 shadow-sm text-sm"
                >
                  <option value="">All Versions</option>
                  {versions.map(version => (
                    <option key={version} value={version}>{version}</option>
                  ))}
                </select>
              </div>

              {/* Available Packages */}
              <div className="space-y-2 max-h-48 overflow-y-auto">
                {packagesLoading ? (
                  <div className="text-center py-4 text-sm text-gray-500">
                    Loading packages...
                  </div>
                ) : availablePackages.length === 0 ? (
                  <div className="text-center py-4 text-sm text-gray-500">
                    {versionFloor && dedupedPackages.length > 0 && upgradeOnlyPackages.length === 0
                      ? `No newer packages available — agent${agents.length > 1 ? 's are' : ' is'} already at ${versionFloor}. Downgrades are not allowed.`
                      : 'No packages available for the selected criteria'}
                  </div>
                ) : (
                  availablePackages.map((pkg) => (
                    <div
                      key={pkg.id}
                      className={cn(
                        "flex items-center justify-between p-3 border rounded-lg cursor-pointer transition-colors",
                        "hover:bg-gray-50 border-gray-200"
                      )}
                      onClick={() => handleUpdateAgents(pkg.id)}
                    >
                      <div className="flex items-center space-x-3">
                        <Package className="h-4 w-4 text-primary-600" />
                        <div>
                          <div className="text-sm font-medium text-gray-900">
                            Version {pkg.version}
                          </div>
                          <div className="text-xs text-gray-500">
                            {pkg.platform}/{pkg.architecture} • {(pkg.file_size / 1024 / 1024).toFixed(1)} MB
                          </div>
                        </div>
                      </div>
                      <div className="flex items-center space-x-2">
                        <div className="text-xs text-gray-400">
                          <Hash className="h-3 w-3 inline mr-1" />
                          {pkg.checksum.slice(0, 8)}...
                        </div>
                        <button
                          disabled={!canUpdate}
                          className={cn(
                            "px-3 py-1 text-xs rounded-md font-medium transition-colors",
                            canUpdate
                              ? "bg-primary-600 text-white hover:bg-primary-700"
                              : "bg-gray-100 text-gray-400 cursor-not-allowed"
                          )}
                        >
                          {isUpdating ? (
                            <RefreshCw className="h-3 w-3 animate-spin" />
                          ) : (
                            'Update'
                          )}
                        </button>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>

            {/* Platform Compatibility Info */}
            <div className="text-xs text-gray-500 flex items-start">
              <Info className="h-3 w-3 mr-1 mt-0.5 flex-shrink-0" />
              <span>
                Detected platform: <strong>{agentPlatform}/{agentArchitecture}</strong>.
                Only compatible packages will be shown.
                {versionFloor && (
                  <>
                    {' '}Floor: <strong>&gt; {versionFloor}</strong> (no downgrades).
                  </>
                )}
              </span>
            </div>
      </Modal.Body>

      <Modal.Footer>
        <button
          onClick={onClose}
          disabled={isUpdating}
          className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50"
        >
          Cancel
        </button>
      </Modal.Footer>
    </Modal>
  );
}