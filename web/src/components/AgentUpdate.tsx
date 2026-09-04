import React, { useState } from 'react';
import { Upload, CheckCircle, XCircle, RotateCw, Download, AlertTriangle } from 'lucide-react';
import { useAgentUpdate } from '@/hooks/useAgentUpdate';
import { Agent } from '@/types';
import { cn } from '@/lib/utils';
import { Modal } from '@/components/primitives';
import toast from 'react-hot-toast';

interface AgentUpdateProps {
  agent: Agent;
  onUpdateComplete?: () => void;
  className?: string;
}

export function AgentUpdate({ agent, onUpdateComplete, className }: AgentUpdateProps) {
  const {
    checkForUpdate,
    triggerAgentUpdate,
    updateStatus,
    updatingAgent,
    hasUpdate,
    availableVersion,
    currentVersion
  } = useAgentUpdate();

  const [isChecking, setIsChecking] = useState(false);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);
  const [hasChecked, setHasChecked] = useState(false);

  const handleCheckUpdate = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setIsChecking(true);

    try {
      await checkForUpdate(agent.id);
      setHasChecked(true);

      if (hasUpdate && availableVersion) {
        setShowConfirmDialog(true);
      } else if (!hasUpdate) {
        // We're already inside the post-check path, so no need to re-read
        // hasChecked (its closure value is still the pre-setState false).
        toast('Agent is already at latest version');
      }
    } catch (error) {
      console.error('[UI] Failed to check for updates:', error);
      toast.error('Failed to check for available updates');
    } finally {
      setIsChecking(false);
    }
  };

  const handleConfirmUpdate = async () => {
    if (!hasUpdate || !availableVersion) {
      toast.error('No update available');
      return;
    }

    setShowConfirmDialog(false);

    try {
      await triggerAgentUpdate(agent, availableVersion);

      if (onUpdateComplete) {
        onUpdateComplete();
      }

    } catch (error) {
      console.error('[UI] Update failed:', error);
    }
  };

  const buttonContent = () => {
    if (updatingAgent) {
      return (
        <>
          <RotateCw className="h-4 w-4 animate-spin" />
          <span>
            {updateStatus.status === 'downloading' && 'Downloading...'}
            {updateStatus.status === 'installing' && 'Installing...'}
            {updateStatus.status === 'pending' && 'Starting update...'}
          </span>
        </>
      );
    }

    if (agent.is_updating) {
      return (
        <>
          <RotateCw className="h-4 w-4 animate-pulse" />
          <span>Updating...</span>
        </>
      );
    }

    if (isChecking) {
      return (
        <>
          <RotateCw className="h-4 w-4" />
          <span>Checking...</span>
        </>
      );
    }

    if (hasChecked && hasUpdate) {
      return (
        <>
          <Download className="h-4 w-4" />
          <span>Update to {availableVersion}</span>
        </>
      );
    }

    return (
      <>
        <Upload className="h-4 w-4" />
        <span>Check for Update</span>
      </>
    );
  };

  return (
    <div className="inline-flex items-center">
      <button
        onClick={handleCheckUpdate}
        disabled={updatingAgent || agent.is_updating || isChecking}
        className={cn(
          "text-sm px-3 py-1 rounded border flex items-center space-x-2 transition-colors",
          {
            "bg-green-50 border-green-300 text-green-700 hover:bg-green-100": hasChecked && hasUpdate,
            "bg-amber-50 border-amber-300 text-amber-700": updatingAgent || agent.is_updating,
            "text-gray-600 hover:text-primary-600 border-gray-300 bg-white hover:bg-primary-50":
              !updatingAgent && !agent.is_updating && !hasUpdate
          },
          className
        )}
        title={updatingAgent ? "Updating agent..." : agent.is_updating ? "Agent is updating..." : "Check for available updates"}
      >
        {buttonContent()}
      </button>

      {/* Progress indicator */}
      {updatingAgent && updateStatus.progress && (
        <div className="ml-2 w-16 h-2 bg-gray-200 rounded">
          <div
            className="h-2 bg-green-500 rounded"
            style={{ width: `${updateStatus.progress}%` }}
          />
        </div>
      )}

      {/* Status icon */}
      {hasChecked && !updatingAgent && (
        <div className="ml-2">
          {hasUpdate ? (
            <CheckCircle className="h-4 w-4 text-green-600" />
          ) : (
            <XCircle className="h-4 w-4 text-gray-400" />
          )}
        </div>
      )}

      {/* Version info popup */}
      {hasChecked && (
        <div className="ml-2 text-xs text-gray-500">
          {currentVersion} → {hasUpdate ? availableVersion : 'Latest'}
        </div>
      )}

      {/* Confirmation Dialog — Modal primitive */}
      <Modal
        open={showConfirmDialog}
        onClose={() => setShowConfirmDialog(false)}
        title={`Update Agent: ${agent.hostname}`}
        maxWidth="md"
      >
        <Modal.Body>
          {/* Warning for same-version updates */}
          {currentVersion === availableVersion ? (
            <>
              <div className="mb-4 p-3 bg-amber-50 border border-amber-200 rounded">
                <p className="text-amber-800 font-medium mb-2 inline-flex items-center gap-1.5">
                  <AlertTriangle className="h-4 w-4" />
                  Version appears identical
                </p>
                <p className="text-sm text-amber-700 mb-2">
                  Current: <strong>{currentVersion}</strong> → Target: <strong>{availableVersion}</strong>
                </p>
                <p className="text-xs text-amber-600">
                  This will reinstall the current version. Useful if the binary was rebuilt or corrupted.
                </p>
              </div>
              <p className="mb-4 text-sm text-gray-600">
                The agent will be temporarily offline during reinstallation.
              </p>
            </>
          ) : (
            <>
              <p className="mb-4 text-gray-600">
                Update agent from <strong>{currentVersion}</strong> to <strong>{availableVersion}</strong>?
              </p>
              <p className="mb-4 text-sm text-gray-500">
                This will temporarily take the agent offline during the update process.
              </p>
            </>
          )}
        </Modal.Body>

        <Modal.Footer>
          <button
            onClick={() => setShowConfirmDialog(false)}
            className="px-4 py-2 text-gray-600 border border-gray-300 rounded hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirmUpdate}
            className={cn(
              "px-4 py-2 rounded hover:bg-primary-700",
              currentVersion === availableVersion
                ? "bg-amber-600 text-white hover:bg-amber-700"
                : "bg-primary-600 text-white"
            )}
          >
            {currentVersion === availableVersion ? 'Reinstall Agent' : 'Update Agent'}
          </button>
        </Modal.Footer>
      </Modal>
    </div>
  );
}