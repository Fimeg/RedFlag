import React from 'react';
import { cn } from '@/lib/utils';

type CommandStatus = 'pending' | 'sent' | 'running' | 'completed' | 'failed' | 'timed_out' | string;

interface StatusInfo {
  text: string;
  color: string;
}

export function getCommandStatus(status: CommandStatus): StatusInfo {
  switch (status) {
    case 'pending':   return { text: 'Pending',      color: 'text-amber-600 bg-amber-50 border-amber-200' };
    case 'sent':      return { text: 'Sent to agent', color: 'text-blue-600 bg-blue-50 border-blue-200' };
    case 'running':   return { text: 'Running',       color: 'text-green-600 bg-green-50 border-green-200' };
    case 'completed': return { text: 'Completed',     color: 'text-green-700 bg-green-50 border-green-200' };
    case 'failed':    return { text: 'Failed',        color: 'text-red-600 bg-red-50 border-red-200' };
    case 'timed_out': return { text: 'Timed out',     color: 'text-red-600 bg-red-50 border-red-200' };
    default:          return { text: status,          color: 'text-gray-600 bg-gray-50 border-gray-200' };
  }
}

interface CommandStatusBadgeProps {
  status: CommandStatus;
  className?: string;
}

const CommandStatusBadge: React.FC<CommandStatusBadgeProps> = ({ status, className }) => {
  const { text, color } = getCommandStatus(status);
  return (
    <span className={cn('inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium border', color, className)}>
      {text}
    </span>
  );
};

export default CommandStatusBadge;
