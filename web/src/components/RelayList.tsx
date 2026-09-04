import { useState } from 'react';
import { Upload, RefreshCw } from 'lucide-react';
import { agentApi } from '@/lib/api';
import { Agent } from '@/types';
import toast from 'react-hot-toast';

interface BulkAgentUpdateProps {
  agents: Agent[];
  onBulkUpdateComplete?: () => void;
}

export function BulkAgentUpdate({ agents, onBulkUpdateComplete }: BulkAgentUpdateProps) {
  const [updatingAgents, setUpdatingAgents] = useState<Set<string>>(new Set());
  const [checkingUpdates, setCheckingUpdates] = useState<Set<string>>(new Set());

  const handleBulkUpdate = async () => {
    if (agents.length === 0) {
      toast.error('No agents selected');
      return;
    }

    // Check each agent for available updates first
    let agentsNeedingUpdate: Agent[] = [];
    let availableVersion: string | undefined;

    // This will populate the checking state
    agents.forEach(agent => setCheckingUpdates(prev => new Set(prev).add(agent.id)));

    try {
      const checkPromises = agents.map(async (agent) => {
        try {
          const result = await agentApi.checkForUpdateAvailable(agent.id);

          if (result.hasUpdate && result.latestVersion) {
            agentsNeedingUpdate.push(agent);
            if (!availableVersion) {
              availableVersion = result.latestVersion;
            }
          }
        } catch (error) {
          console.error(`Failed to check updates for agent ${agent.id}:`, error);
        } finally {
          setCheckingUpdates(prev => {
            const newSet = new Set(prev);
            newSet.delete(agent.id);
            return newSet;
          });
        }
      });

      await Promise.all(checkPromises);

      if (agentsNeedingUpdate.length === 0) {
        toast('Selected agents are already up to date');
        return;
      }

      // Generate nonces for each agent that needs updating
      const noncePromises = agentsNeedingUpdate.map(async (agent) => {
        if (availableVersion) {
          try {
            const nonceData = await agentApi.generateUpdateNonce(agent.id, availableVersion);

            // Store nonce for use in update request
            return {
              agentId: agent.id,
              hostname: agent.hostname,
              nonce: nonceData.update_nonce,
              targetVersion: availableVersion
            };
          } catch (error) {
            console.error(`Failed to generate nonce for ${agent.hostname}:`, error);
            return null;
          }
        }
        return null;
      });

      const nonceResults = await Promise.all(noncePromises);
      const validUpdates = nonceResults.filter(item => item !== null);

      if (validUpdates.length === 0) {
        toast.error('Failed to generate update nonces for any agents');
        return;
      }

      // Perform bulk updates
      const firstAgent = agents.find(a => a.id === validUpdates[0].agentId);
      const detectedPlatform = firstAgent
        ? `${firstAgent.os_type || 'linux'}-${firstAgent.os_architecture || 'amd64'}`
        : 'linux-amd64';
      const updateData = {
        agent_ids: validUpdates.map(item => item.agentId),
        version: availableVersion || '',
        platform: detectedPlatform,
        nonces: validUpdates.map(item => item.nonce)
      };

      // Mark agents as updating
      validUpdates.forEach(item => {
        setUpdatingAgents(prev => new Set(prev).add(item.agentId));
      });

      const result = await agentApi.updateMultipleAgents(updateData);

      toast.success(`Initiated updates for ${result.updated.length} of ${agents.length} agents`);

      if (result.failed.length > 0) {
        toast.error(`Failed to update ${result.failed.length} agents`);
      }

      // Start polling for completion
      startBulkUpdatePolling(validUpdates);

      if (onBulkUpdateComplete) {
        onBulkUpdateComplete();
      }

    } catch (error) {
      console.error('Bulk update failed:', error);
      toast.error(`Bulk update failed: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  };

  const startBulkUpdatePolling = (agents: Array<{agentId: string, hostname: string}>) => {
    let attempts = 0;
    const maxAttempts = 60; // 5 minutes max

    const pollInterval = setInterval(async () => {
      attempts++;

      if (attempts >= maxAttempts || updatingAgents.size === 0) {
        clearInterval(pollInterval);
        setUpdatingAgents(new Set());
        return;
      }

      const statusPromises = agents.map(async (item) => {
        try {
          const status = await agentApi.getUpdateStatus(item.agentId);

          if (status.status === 'complete' || status.status === 'failed') {
            // Remove from updating set
            setUpdatingAgents(prev => {
              const newSet = new Set(prev);
              newSet.delete(item.agentId);
              return newSet;
            });

            if (status.status === 'complete') {
              toast.success(`${item.hostname} updated successfully`);
            } else {
              toast.error(`${item.hostname} update failed: ${status.error || 'Unknown error'}`);
            }
          }
        } catch (error) {
          console.error(`Failed to poll ${item.hostname}:`, error);
        }
      });

      await Promise.allSettled(statusPromises);

    }, 5000); // Check every 5 seconds

    return () => clearInterval(pollInterval);
  };

  const isAnyAgentUpdating = (): boolean => {
    return agents.some(agent => updatingAgents.has(agent.id));
  };

  const isAnyAgentChecking = (): boolean => {
    return agents.some(agent => checkingUpdates.has(agent.id));
  };

  const getButtonContent = () => {
    if (isAnyAgentUpdating() || isAnyAgentChecking()) {
      return (
        <>
          <RefreshCw className="h-4 w-4 animate-spin" />
          <span>{isAnyAgentChecking() ? "Checking..." : "Updating..."}</span>
        </>
      );
    }

    if (agents.length === 1) {
      return (
        <>
          <Upload className="h-4 w-4" />
          <span>Update 1 Agent</span>
        </>
      );
    }

    return (
      <>
        <Upload className="h-4 w-4" />
        <span>Update {agents.length} Agents</span>
      </>
    );
  };

  return (
    <button
      onClick={handleBulkUpdate}
      disabled={isAnyAgentUpdating() || isAnyAgentChecking()}
      className="btn btn-secondary"
    >
      {getButtonContent()}
    </button>
  );
}