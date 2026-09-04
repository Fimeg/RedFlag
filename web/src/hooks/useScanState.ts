import { useState, useCallback } from 'react';
import { api } from '@/lib/api';
import toast from 'react-hot-toast';

interface ScanState {
  isScanning: boolean;
  commandId?: string;
  error?: string;
}

/**
 * Hook for managing scan button state and preventing duplicate scans
 * Integrates with backend deduplication (409 Conflict responses)
 */
export function useScanState(agentId: string, subsystem: string) {
  const [state, setState] = useState<ScanState>({
    isScanning: false,
  });

  const triggerScan = useCallback(async () => {
    if (state.isScanning) {
      toast('Scan already in progress');
      return;
    }

    setState({ isScanning: true, commandId: undefined, error: undefined });

    try {
      const result = await api.post(`/agents/${agentId}/subsystems/${subsystem}/trigger`);

      setState(prev => ({
        ...prev,
        commandId: result.data.command_id,
      }));

      // Poll for completion or wait for subscription update
      await waitForScanComplete(agentId, result.data.command_id);

      setState({ isScanning: false, commandId: result.data.command_id });

      toast.success(`${subsystem} scan completed`);
    } catch (error: any) {
      const isAlreadyRunning = error.response?.status === 409;

      if (isAlreadyRunning) {
        const existingCommandId = error.response?.data?.command_id;
        setState({
          isScanning: false,
          commandId: existingCommandId,
          error: 'Scan already in progress',
        });

        toast(`Scan already running (command: ${existingCommandId})`);
      } else {
        const errorMessage = error.response?.data?.error || error.message;
        setState({
          isScanning: false,
          error: errorMessage,
        });

        // The underlying api.post failure is already logged as api_error by the
        // response interceptor (lib/api.ts) — this is presentation only.
        toast.error(`Failed to trigger scan: ${errorMessage}`);
      }
    }
  }, [agentId, subsystem, state.isScanning]);

  const reset = useCallback(() => {
    setState({ isScanning: false, commandId: undefined, error: undefined });
  }, []);

  return {
    isScanning: state.isScanning,
    commandId: state.commandId,
    error: state.error,
    triggerScan,
    reset,
  };
}

/**
 * Wait for scan to complete by polling command status
 * Max wait: 5 minutes
 */
async function waitForScanComplete(agentId: string, commandId: string): Promise<void> {
  const maxWaitMs = 300000; // 5 minutes max
  const startTime = Date.now();
  const pollInterval = 2000; // Poll every 2 seconds

  return new Promise((resolve, reject) => {
    const interval = setInterval(async () => {
      try {
        const result = await api.get(`/agents/${agentId}/commands/${commandId}`);

        if (result.data.status === 'completed' || result.data.status === 'failed') {
          clearInterval(interval);
          resolve();
        }
      } catch (error) {
        clearInterval(interval);
        reject(error);
      }

      if (Date.now() - startTime > maxWaitMs) {
        clearInterval(interval);
        reject(new Error('Scan timeout'));
      }
    }, pollInterval);
  });
}
