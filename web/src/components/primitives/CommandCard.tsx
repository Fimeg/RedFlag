import React from 'react';
import { RefreshCw, Clock, CheckCircle, XCircle, Activity, Package, Search } from 'lucide-react';
import { cn, formatRelativeTime } from '@/lib/utils';
import { getCommandStatus } from './CommandStatusBadge';

function getCommandDisplayInfo(command: any): { icon: React.ReactNode; label: string } {
  const getPackageName = (cmd: any): string => {
    if (cmd.package_name) return cmd.package_name;
    if (cmd.params?.package_name) return cmd.params.package_name;
    if (cmd.params?.update_id && cmd.update_name) return cmd.update_name;
    return 'unknown package';
  };

  const actionMap: Record<string, { icon: React.ReactNode; label: string }> = {
    scan:                 { icon: <RefreshCw className="h-4 w-4" />, label: 'System scan' },
    install_updates:      { icon: <Package className="h-4 w-4" />,   label: `Installing ${getPackageName(command)}` },
    dry_run_update:       { icon: <Search className="h-4 w-4" />,    label: `Checking dependencies for ${getPackageName(command)}` },
    confirm_dependencies: { icon: <CheckCircle className="h-4 w-4" />, label: `Installing ${getPackageName(command)}` },
  };

  return actionMap[command.command_type] ?? {
    icon: <Activity className="h-4 w-4" />,
    label: command.command_type.replace('_', ' '),
  };
}

function commandTimestamp(createdAt: string): string {
  const createdTime = new Date(createdAt);
  const hoursOld = (Date.now() - createdTime.getTime()) / (1000 * 60 * 60);
  return hoursOld > 1 ? createdTime.toLocaleString() : formatRelativeTime(createdAt);
}

interface CommandCardProps {
  command: any;
  onCancel: (id: string) => void;
  onDismiss: (id: string) => void;
  disabled?: boolean;
}

const CommandCard: React.FC<CommandCardProps> = ({ command, onCancel, onDismiss, disabled }) => {
  const displayInfo = getCommandDisplayInfo(command);
  const statusInfo = getCommandStatus(command.status);
  const isActive = command.status === 'running' || command.status === 'sent' || command.status === 'pending';

  return (
    <div className="flex items-start space-x-2 p-2 bg-gray-50 rounded border border-gray-200">
      <div className="flex-shrink-0 mt-0.5">{displayInfo.icon}</div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-gray-900 truncate">
            <span className="flex items-center space-x-1">
              <span className={cn(
                'inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border',
                statusInfo.color
              )}>
                {command.status === 'running' && <RefreshCw className="h-3 w-3 animate-spin mr-1" />}
                {command.status === 'pending' && <Clock className="h-3 w-3 mr-1" />}
                {command.status === 'completed' && <CheckCircle className="h-3 w-3 mr-1" />}
                {command.status === 'failed' && <XCircle className="h-3 w-3 mr-1" />}
                {isActive ? command.status.replace('_', ' ') : statusInfo.text}
              </span>
              <span className="ml-1">{displayInfo.label}</span>
            </span>
          </span>
        </div>
        <div className="flex items-center justify-between mt-1">
          <span className="text-xs text-gray-500">{commandTimestamp(command.created_at)}</span>
          {(command.status === 'pending' || command.status === 'sent') && (
            <button
              onClick={() => onCancel(command.id)}
              disabled={disabled}
              className="text-xs text-red-600 hover:text-red-800 disabled:opacity-50"
            >
              Cancel
            </button>
          )}
          {(command.status === 'timed_out' || command.status === 'failed') && (
            <button
              onClick={() => onDismiss(command.id)}
              disabled={disabled}
              className="text-xs text-gray-600 hover:text-gray-800 disabled:opacity-50"
            >
              Dismiss
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

export default CommandCard;
