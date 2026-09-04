/**
 * Command Naming Service
 * Provides ETHOS-compliant, consistent command display names
 *
 * ETHOS Compliance:
 * - Section 5: No marketing fluff, clear honest naming
 * - Section 1: Transparent, consistent error logging
 */

export interface CommandDisplay {
  action: string;
  verb: string;
  noun: string;
  icon: string;
}

/**
 * Maps internal command types to user-friendly display names
 * Internal format: scan_storage, scan_system, scan_docker, scan_apt, scan_dnf, scan_winget, scan_windows
 * External format: Storage Scan, System Scan, etc.
 * Note: scan_updates was removed - use platform-specific scans instead
 */
export const getCommandDisplay = (commandType: string): CommandDisplay => {
  const commandMap: Record<string, CommandDisplay> = {
    'scan_storage': {
      action: 'Storage Scan',
      verb: 'Scan',
      noun: 'Disk Usage',
      icon: 'HardDrive',
    },
    'scan_system': {
      action: 'System Scan',
      verb: 'Scan',
      noun: 'System Metrics',
      icon: 'Cpu',
    },
    'scan_docker': {
      action: 'Docker Scan',
      verb: 'Scan',
      noun: 'Docker Images',
      icon: 'Container',
    },
    'scan_apt': {
      action: 'APT Scan',
      verb: 'Scan',
      noun: 'APT Packages',
      icon: 'Package',
    },
    'scan_dnf': {
      action: 'DNF Scan',
      verb: 'Scan',
      noun: 'DNF Packages',
      icon: 'Package',
    },
    'scan_winget': {
      action: 'Winget Scan',
      verb: 'Scan',
      noun: 'Winget Packages',
      icon: 'Package',
    },
    'scan_windows': {
      action: 'Windows Update Scan',
      verb: 'Scan',
      noun: 'Windows Updates',
      icon: 'Package',
    },
    'install_package': {
      action: 'Package Install',
      verb: 'Install',
      noun: 'Package',
      icon: 'Download',
    },
    'upgrade_package': {
      action: 'Package Upgrade',
      verb: 'Upgrade',
      noun: 'Package',
      icon: 'ArrowUp',
    },
    'update_agent': {
      action: 'Agent Update',
      verb: 'Update',
      noun: 'Agent',
      icon: 'RefreshCw',
    },
    'reboot_agent': {
      action: 'Agent Reboot',
      verb: 'Reboot',
      noun: 'Agent',
      icon: 'Power',
    },
    'enable_heartbeat': {
      action: 'Enable Heartbeat',
      verb: 'Enable',
      noun: 'Heartbeat',
      icon: 'Heart',
    },
    'disable_heartbeat': {
      action: 'Disable Heartbeat',
      verb: 'Disable',
      noun: 'Heartbeat',
      icon: 'HeartOff',
    },
    'toggle_auto_run': {
      action: 'Auto-Run Toggle',
      verb: 'Toggle',
      noun: 'Auto-Run',
      icon: 'ToggleLeft',
    },
  };

  // Return mapped command or create generic display
  if (commandMap[commandType]) {
    return commandMap[commandType];
  }

  // For unknown commands, create a generic display (ETHOS transparency)
  const parts = commandType.split('_');
  const action = parts.map(p => p.charAt(0).toUpperCase() + p.slice(1)).join(' ');

  return {
    action,
    verb: parts[0] || 'Operation',
    noun: parts.slice(1).join(' ') || 'Unknown',
    icon: 'Settings',
  };
};

/**
 * Gets the icon name for a command type
 * For use with lucide-react icons
 */
export const getCommandIcon = (commandType: string): string => {
  return getCommandDisplay(commandType).icon;
};

/**
 * Formats a command type for display in UI
 * Example: scan_updates → Updates Scan
 */
export const formatCommandAction = (commandType: string): string => {
  return getCommandDisplay(commandType).action;
};

/**
 * Creates a narrative sentence for a history entry
 * ETHOS compliant: honest, clear, no fluff
 */
export const createCommandSentence = (
  commandType: string,
  status: string,
  isInProgress: boolean,
  duration?: number,
  error?: string
): string => {
  const display = getCommandDisplay(commandType);
  const durationStr = duration ? ` (${duration.toFixed(1)}s)` : '';

  if (isInProgress) {
    return `${display.verb} in progress${durationStr}`;
  }

  switch (status) {
    case 'success':
    case 'completed':
      return `${display.action} completed successfully${durationStr}`;
    case 'failed':
    case 'error':
      return `${display.action} failed${durationStr}${error ? `: ${error}` : ''}`;
    case 'pending':
    case 'sent':
      return `${display.verb} initiated${durationStr}`;
    default:
      return `${display.action} results${durationStr}`;
  }
};

/**
 * Validates that a command type follows ETHOS naming conventions
 * Internal format: verb_noun (e.g., scan_updates, install_package)
 */
export const validateCommandType = (commandType: string): boolean => {
  // Must contain underscore
  if (!commandType.includes('_')) {
    return false;
  }

  const parts = commandType.split('_');

  // Must have at least 2 parts
  if (parts.length < 2) {
    return false;
  }

  // First part should be a valid verb
  const validVerbs = ['scan', 'install', 'upgrade', 'update', 'reboot', 'enable', 'disable', 'toggle', 'confirm'];
  if (!validVerbs.includes(parts[0])) {
    return false;
  }

  return true;
};

/**
 * Logs an ETHOS-compliant error for invalid command types
 */
export const logInvalidCommandType = (commandType: string, context: string): void => {
  // Use console.error for frontend logging with ETHOS format
  console.error(`[ERROR] [web] [command] invalid_command_type="${commandType}" context="${context}" timestamp=${new Date().toISOString()}`);
};
