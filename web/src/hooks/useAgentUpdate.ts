import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'react-hot-toast';
import { agentApi } from '@/lib/api';
import { Agent } from '@/types';
import { clientLogger } from '@/lib/client-logger';

interface UseAgentUpdateReturn {
  checkForUpdate: (agentId: string) => Promise<void>;
  triggerAgentUpdate: (agent: Agent, targetVersion: string) => Promise<void>;
  updateStatus: UpdateStatus;
  checkingUpdate: boolean;
  updatingAgent: boolean;
  hasUpdate: boolean;
  availableVersion?: string;
  currentVersion?: string;
}

interface UpdateStatus {
  status: 'idle' | 'checking' | 'pending' | 'downloading' | 'installing' | 'complete' | 'failed';
  progress?: number;
  error?: string;
}

export function useAgentUpdate(): UseAgentUpdateReturn {
  const queryClient = useQueryClient();
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus>({ status: 'idle' });
  const [hasUpdate, setHasUpdate] = useState(false);
  const [availableVersion, setAvailableVersion] = useState<string>();
  const [currentVersion, setCurrentVersion] = useState<string>();

  // Check if update available for agent
  const checkMutation = useMutation({
    mutationFn: agentApi.checkForUpdateAvailable,
    onSuccess: (data) => {
      setHasUpdate(data.hasUpdate);
      setAvailableVersion(data.latestVersion);
      setCurrentVersion(data.currentVersion);

      if (!data.hasUpdate) {
        toast('Agent is already at latest version');
      }
    },
    onError: (error) => {
      console.error('Failed to check for updates:', error);
      toast.error(`Failed to check for updates: ${error instanceof Error ? error.message : String(error)}`);
    }
  });

  // Check for update available
  const checkForUpdate = async (agentId: string) => {
    try {
      await checkMutation.mutateAsync(agentId);
    } catch (error) {
      console.error('Error checking for update:', error);
    }
  };

  // Trigger agent update with nonce generation
  const triggerAgentUpdate = async (agent: Agent, targetVersion: string) => {
    try {
      // Step 1: Check for update availability (already done by checkmutation)
      if (!hasUpdate) {
        await checkForUpdate(agent.id);
        if (!hasUpdate) {
          toast('No updates available');
          return;
        }
      }

      // Step 2: Generate nonce for authorized update
      const nonceData = await agentApi.generateUpdateNonce(agent.id, targetVersion);
      clientLogger.debug('Update nonce generated', { agentId: agent.id, targetVersion });

      // Step 3: Trigger the actual update
      await agentApi.updateAgent(agent.id, {
        version: targetVersion,
        platform: `${agent.os_type}-${agent.os_architecture}`,
        // Include nonce in request for security
        nonce: nonceData.update_nonce
      });

      setUpdateStatus({ status: 'pending', progress: 0 });

      // Step 4: Start polling for progress
      startUpdatePolling(agent.id);

      // Step 5: Refresh agent data in cache
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      clientLogger.debug('Update initiated successfully', { agentId: agent.id, targetVersion });


    } catch (error) {
      console.error('[UI] Update failed:', error);
      const errorMessage = error instanceof Error ? error.message : String(error);
      toast.error(`Update failed: ${errorMessage}`);
      setUpdateStatus({ status: 'failed', error: errorMessage });
    }
  };

  // Poll for update progress
  const startUpdatePolling = (agentId: string) => {
    let attempts = 0;
    const maxAttempts = 60; // 5 minutes with 5 second intervals

    const pollInterval = setInterval(async () => {
      attempts++;

      if (attempts >= maxAttempts) {
        clearInterval(pollInterval);
        setUpdateStatus({ status: 'failed', error: 'Update timeout' });
        toast.error('Update timed out after 5 minutes');
        return;
      }

      try {
        const status = await agentApi.getUpdateStatus(agentId);

        switch (status.status) {
          case 'complete':
            clearInterval(pollInterval);
            setUpdateStatus({ status: 'complete' });
            toast.success('Agent updated successfully!');
            setHasUpdate(false);
            setAvailableVersion(undefined);
            break;
          case 'failed':
            clearInterval(pollInterval);
            setUpdateStatus({ status: 'failed', error: status.error || 'Update failed' });
            toast.error(`Update failed: ${status.error || 'Unknown error'}`);
            break;
          case 'downloading':
            setUpdateStatus({ status: 'downloading', progress: status.progress });
            break;
          case 'installing':
            setUpdateStatus({ status: 'installing', progress: status.progress });
            break;
          default:
            setUpdateStatus({ status: 'idle' });
        }
      } catch (error) {
        console.error('[UI] Failed to get update status:', error);
        clearInterval(pollInterval);
        setUpdateStatus({ status: 'failed', error: 'Failed to get update status' });
      }
    }, 5000); // Poll every 5 seconds

    return () => clearInterval(pollInterval);
  };

  return {
    checkForUpdate,
    triggerAgentUpdate,
    updateStatus,
    checkingUpdate: checkMutation.isPending,
    updatingAgent: updateStatus.status === 'downloading' || updateStatus.status === 'installing' || updateStatus.status === 'pending',
    hasUpdate,
    availableVersion,
    currentVersion
  };
}