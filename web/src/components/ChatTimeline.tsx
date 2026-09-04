import React, { useState } from 'react';
import {
  CheckCircle,
  XCircle,
  AlertTriangle,
  Package,
  Search,
  RefreshCw,
  ChevronDown,
  ChevronRight,
  User,
  Clock,
  Activity,
  Copy,
  HardDrive,
  Cpu,
  Container,
} from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { logApi } from '@/lib/api';
import { useRetryCommand } from '@/hooks/useCommands';
import { cn, formatTimeOnly } from '@/lib/utils';
import toast from 'react-hot-toast';
import { Highlight, themes } from 'prism-react-renderer';
import { getCommandDisplay } from '@/lib/command-naming';

interface HistoryEntry {
  id: string;
  agent_id: string;
  type: string; // "command" or "log"
  action: string;
  status?: string;
  result: string;
  package_name?: string;
  package_type?: string;
  stdout?: string;
  stderr?: string;
  exit_code?: number;
  duration_seconds?: number;
  created_at: string;
  metadata?: Record<string, string>;
  params?: Record<string, any>;
  hostname?: string;
  narrative?: string; // server-supplied summary; falls back to action enum when absent
  is_retry?: boolean; // command re-attempts an earlier one (retried_from_id set)
  retried_from_id?: string; // links to the original command in the retry chain
}

interface ChatTimelineProps {
  agentId?: string;
  className?: string;
  isScopedView?: boolean; // true for agent-specific view, false for global view
  externalSearch?: string; // external search query from parent
  externalType?: string; // event type filter from parent
  externalSeverity?: string; // severity filter from parent
}

const entryPackageName = (entry: HistoryEntry): string | undefined => {
  if (entry.package_name) return entry.package_name;
  return typeof entry.params?.package_name === 'string' ? entry.params.package_name : undefined;
};

const entryPackageType = (entry: HistoryEntry): string | undefined => {
  if (entry.package_type) return entry.package_type;
  return typeof entry.params?.package_type === 'string' ? entry.params.package_type : undefined;
};

const entryUpdateLink = (entry: HistoryEntry): string | undefined => {
  if (typeof entry.params?.update_id === 'string' && entry.params.update_id) {
    return `/updates/${entry.params.update_id}`;
  }

  const packageName = entryPackageName(entry);
  if (!packageName) return undefined;

  const packageType = entryPackageType(entry);
  if (packageType) {
    return `/updates/package/${packageType}/${packageName}`;
  }

  return `/updates?search=${encodeURIComponent(packageName)}`;
};

// Helper function to create smart summaries for package operations
const createPackageOperationSummary = (entry: HistoryEntry): string => {
  const action = entry.action.replace(/_/g, ' ');
  const result = entry.result || 'unknown';

  // Extract package name from stdout or params
  let packageName = 'unknown package';
  if (entry.params?.package_name) {
    packageName = entry.params.package_name as string;
  } else if (entry.stdout) {
    // Look for package patterns in stdout
    const packageMatch = entry.stdout.match(/(?:Upgrading|Installing|Package):\s+(\S+)/i);
    if (packageMatch) {
      packageName = packageMatch[1];
    } else {
      // Look for "Packages installed: [pkg]" pattern
      const installedMatch = entry.stdout.match(/Packages installed:\s*\[([^\]]+)\]/i);
      if (installedMatch) {
        packageName = installedMatch[1];
      }
    }
  }

  // Extract duration if available
  let durationInfo = '';
  if (entry.created_at) {
    try {
      const loggedTime = new Date(entry.created_at).toLocaleTimeString('en-US', {
        hour: '2-digit',
        minute: '2-digit'
      });
      durationInfo = ` at ${loggedTime}`;

      if (entry.duration_seconds) {
        durationInfo += ` (${entry.duration_seconds}s)`;
      }
    } catch (e) {
      // Ignore date parsing errors
    }
  }

  // Create action-specific summaries
  switch (entry.action) {
    case 'upgrade':
    case 'install':
    case 'confirm_dependencies':
      if (result === 'success' || result === 'completed') {
        return `Successfully ${action}d ${packageName}${durationInfo}`;
      } else if (result === 'failed' || result === 'error') {
        return `Failed to ${action} ${packageName}${durationInfo}`;
      } else {
        return `${action.charAt(0).toUpperCase() + action.slice(1)} ${packageName}${durationInfo}`;
      }

    case 'dry_run_update':
      if (result === 'success' || result === 'completed') {
        return `Dry run completed for ${packageName}${durationInfo}`;
      } else {
        return `Dry run for ${packageName}${durationInfo}`;
      }

    default:
      return `${action} ${packageName}${durationInfo}`;
  }
};

const ChatTimeline: React.FC<ChatTimelineProps> = ({ agentId, className, isScopedView = false, externalSearch, externalType, externalSeverity }) => {
  const [statusFilter, _setStatusFilter] = useState('all'); // 'all', 'success', 'failed', 'pending', 'completed', 'running', 'timed_out'
  const [expandedEntries, setExpandedEntries] = useState<Set<string>>(new Set());
  const [selectedAgents, _setSelectedAgents] = useState<string[]>([]);

  // Retry command hook
  const retryCommandMutation = useRetryCommand();

  // Query parameters for API
  const [queryParams, setQueryParams] = useState({
    page: 1,
    page_size: 50,
    agent_id: agentId || '',
    result: statusFilter !== 'all' ? statusFilter : '',
    search: externalSearch || '',
    type: externalType || '',
    severity: externalSeverity || '',
  });

  // Update query params when external filters change
  React.useEffect(() => {
    setQueryParams(prev => ({
      ...prev,
      search: externalSearch || '',
      type: externalType || '',
      severity: externalSeverity || '',
    }));
  }, [externalSearch, externalType, externalSeverity]);

  // Fetch history data
  const { data: historyData, isLoading, refetch: _refetch, isFetching } = useQuery({
    queryKey: ['history', queryParams],
    queryFn: async () => {
      try {
        const params: any = {
          page: queryParams.page,
          page_size: queryParams.page_size,
        };

        if (queryParams.agent_id) {
          params.agent_id = queryParams.agent_id;
        }

        if (queryParams.result) {
          params.result = queryParams.result;
        }

        if (queryParams.search) {
          params.search = queryParams.search;
        }

        if (queryParams.type) {
          params.type = queryParams.type;
        }

        if (queryParams.severity) {
          params.severity = queryParams.severity;
        }

        const response = await logApi.getAllLogs(params);
        return response;
      } catch (error) {
        console.error('Failed to fetch history:', error);
        toast.error('Failed to fetch history');
        return { logs: [], total: 0, page: 1, page_size: 50 };
      }
    },
    refetchInterval: 30000,
  });

  const allEntries: HistoryEntry[] = historyData?.logs || [];

  // Filter entries based on selected agents
  const filteredEntries = allEntries.filter(entry => {
    // Agent filter
    if (selectedAgents.length > 0 && !selectedAgents.includes(entry.agent_id)) {
      return false;
    }

    return true;
  });

  // Group entries by date with timestamp dividers and timeline connector
  const createTimelineWithDividers = (entries: HistoryEntry[]) => {
    const timeline: JSX.Element[] = [];
    let lastDate: string | null = null;

    entries.forEach((entry) => {
      const entryDate = new Date(entry.created_at);
      const dateKey = entryDate.toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric'
      });

      // Add date divider if date changed
      if (dateKey !== lastDate) {
        timeline.push(
          <div key={`date-${dateKey}`} className="flex items-center justify-center my-6">
            <div className="flex-1 h-px bg-gray-300"></div>
            <span className="badge badge-lg text-sm bg-gray-100 text-gray-600">
              {dateKey}
            </span>
            <div className="flex-1 h-px bg-gray-300"></div>
          </div>
        );
        lastDate = dateKey;
      }

      // Add the event bubble
      timeline.push(
        <EventBubble
          key={entry.id}
          entry={entry}
          isExpanded={expandedEntries.has(entry.id)}
          isScopedView={isScopedView}
          onToggle={() => {
            const newExpanded = new Set(expandedEntries);
            if (newExpanded.has(entry.id)) {
              newExpanded.delete(entry.id);
            } else {
              newExpanded.add(entry.id);
            }
            setExpandedEntries(newExpanded);
          }}
        />
      );
    });

    return timeline;
  };

  // Interface for narrative event summary
  interface NarrativeSummary {
    sentence: string;
    statusType: 'success' | 'failed' | 'running' | 'started' | 'warning' | 'pending' | 'partial' | 'info';
    statusIcon: React.ReactNode;
    hoverColor: string;
    borderColor: string;
    subject: string;
  }

  // Create narrative event summary
  const getNarrativeSummary = (entry: HistoryEntry): NarrativeSummary => {
    const action = entry.action.replace(/_/g, ' ');
    const result = entry.result || entry.status || 'unknown';

    // Determine status type and corresponding colors/icons
    let statusType: 'success' | 'failed' | 'running' | 'started' | 'warning' | 'info' | 'pending' | 'partial';
    let statusIcon: React.ReactNode;
    let hoverColor: string;
    let borderColor: string;

    if (result === 'success' || result === 'completed') {
      statusType = 'success';
      statusIcon = <CheckCircle className="h-4 w-4" />;
      hoverColor = 'hover:bg-green-50';
      borderColor = 'border-l-green-300';
    } else if (result === 'failed' || result === 'error') {
      statusType = 'failed';
      statusIcon = <XCircle className="h-4 w-4" />;
      hoverColor = 'hover:bg-red-50';
      borderColor = 'border-l-red-300';
    } else if (result === 'started') {
      statusType = 'started';
      statusIcon = <RefreshCw className="h-4 w-4 animate-spin" />;
      hoverColor = 'hover:bg-blue-50';
      borderColor = 'border-l-blue-300';
    } else if (result === 'running') {
      statusType = 'running';
      statusIcon = <RefreshCw className="h-4 w-4 animate-spin" />;
      hoverColor = 'hover:bg-blue-50';
      borderColor = 'border-l-blue-300';
    } else if (result === 'pending' || result === 'sent') {
      statusType = 'pending';
      statusIcon = <Clock className="h-4 w-4" />;
      hoverColor = 'hover:bg-purple-50';
      borderColor = 'border-l-purple-300';
    } else if (result === 'timed_out' || result === 'rollback' || result === 'warning') {
      statusType = 'warning';
      statusIcon = <AlertTriangle className="h-4 w-4" />;
      hoverColor = 'hover:bg-amber-50';
      borderColor = 'border-l-amber-300';
    } else if (result === 'critical') {
      statusType = 'failed';
      statusIcon = <XCircle className="h-4 w-4" />;
      hoverColor = 'hover:bg-red-50';
      borderColor = 'border-l-red-500';
    } else if (result === 'partial') {
      statusType = 'partial';
      statusIcon = <AlertTriangle className="h-4 w-4" />;
      hoverColor = 'hover:bg-amber-50';
      borderColor = 'border-l-amber-300';
    } else {
      statusType = 'info';
      statusIcon = <Activity className="h-4 w-4" />;
      hoverColor = 'hover:bg-gray-50';
      borderColor = 'border-l-gray-300';
    }

    // Extract subject (package name or target)
    let subject = '';
    if (entry.stdout) {
      // Priority 1: Extract actual package/installation details from stdout
      const stdout = entry.stdout;

      // Pattern 1: "Packages installed: [Update Name]" (Windows Update success)
      const packagesInstalledMatch = stdout.match(/Packages installed:\s*\[([^\]]+)\]/i);
      if (packagesInstalledMatch) {
        subject = packagesInstalledMatch[1].trim();
      } else {
        // Pattern 2: Bullet point format "• Update Name" (Dry run results)
        const bulletMatch = stdout.match(/•\s*([^\n]+)/);
        if (bulletMatch) {
          subject = bulletMatch[1].trim();
        } else {
          // Pattern 3: Package line format
          const packageMatch = stdout.match(/Package:\s*([^\n]+)/i);
          if (packageMatch) {
            subject = packageMatch[1].trim();
          } else {
            // Pattern 4: Windows Update full name patterns
            // Look for Windows Update with KB numbers - more comprehensive pattern
            const windowsUpdateMatch = stdout.match(/([A-Z][^-\n]*\bUpdate\b[^-\n]*\bKB\d{7,8}\b[^\n]*)/);
            if (windowsUpdateMatch) {
              subject = windowsUpdateMatch[1].trim();
            } else {
              // Pattern 5: Generic update patterns (full line)
              const updateMatch = stdout.match(/([A-Z][^\n]*\bUpdate\b[^\n]*\bKB\d{7,8}\b[^\n]*)/);
              if (updateMatch) {
                subject = updateMatch[1].trim();
              } else {
                // Pattern 6: Look for Security Intelligence Update or similar specific patterns
                const securityUpdateMatch = stdout.match(/([A-Z][^-\n]*Security Intelligence Update[^-\n]*KB\d{7,8}[^\n]*)/);
                if (securityUpdateMatch) {
                  subject = securityUpdateMatch[1].trim();
                } else {
                  // Pattern 7: Extract from dependency confirmation broken sentences
                  // Fix: "Dependency check for 'Windows Updates installation initiated via wuauclt Packages installed'"
                  const dependencyBrokenMatch = stdout.match(/Packages installed:\s*\[([^\]]+)\]/i);
                  if (dependencyBrokenMatch) {
                    subject = dependencyBrokenMatch[1].trim();
                  } else {
                    // Pattern 8: Look for any line with "Update" and treat it as subject
                    const lines = stdout.split('\n');
                    for (const line of lines) {
                      if (line.includes('Update') && line.includes('KB') && line.length > 20) {
                        subject = line.trim();
                        break;
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }

      // Clean up common artifacts
      if (subject) {
        subject = subject
          .replace(/\s*-\s*Current Channel\s*\(Broad\)$/i, '') // Remove Windows Update channel info
          .replace(/\s*-\s*Version\s*[\d.]+$/i, '') // Remove version numbers for readability
          .replace(/\s*Method:\s*.*$/i, '') // Remove method info
          .replace(/\s*Requires:\s*.*$/i, '') // Remove requirement info
          .replace(/^Dry run\s*[-:]\s*/i, '') // Remove "Dry run -" prefix
          .replace(/^The following updates would be installed:\s*/i, '') // Remove generic dry run prefix
          .trim();
      }
    }

    // Fallback subject - provide better action labels
    if (!subject) {
      // Map action to more readable labels
      const actionLabels: Record<string, string> = {
        'scan updates': 'Package Updates',
        'scan storage': 'Disk Usage',
        'scan system': 'System Metrics',
        'scan docker': 'Docker Images',
        'update agent': 'Agent Update',
        'dry run update': 'Update Dry Run',
        'confirm dependencies': 'Dependency Check',
        'install update': 'Update Installation',
        'collect specs': 'System Specifications',
        'enable heartbeat': 'Heartbeat Enable',
        'disable heartbeat': 'Heartbeat Disable',
        'reboot': 'System Reboot',
        'process command': 'Command Processing'
      };

      // Prioritize metadata subsystem label for better descriptions
      subject = entry.metadata?.subsystem_label || entry.package_name || actionLabels[action] || action;
    }

    // Build narrative sentence - system thought style
    let sentence = '';
    const isInProgress = result === 'started' || result === 'running' || result === 'pending' || result === 'sent';


    if (entry.type === 'package_event') {
      const pkg = entry.package_name || 'unknown';
      switch (entry.action) {
        case 'discovered':
          sentence = `New update available for ${pkg}`;
          break;
        case 'updated':
          sentence = `${pkg} updated`;
          break;
        case 'failed':
          sentence = `${pkg} update failed`;
          break;
        case 'ignored':
          sentence = `${pkg} update ignored`;
          break;
        default:
          sentence = `${pkg}: ${entry.action}`;
      }
    } else if (entry.type === 'install_transition') {
      const pkg = entry.package_name || 'unknown';
      switch (result) {
        case 'success':
          sentence = `${pkg} installed successfully`;
          break;
        case 'failed':
          sentence = `${pkg} installation failed`;
          if (entry.stderr) sentence += `: ${entry.stderr.split('\n')[0]}`;
          break;
        case 'rollback':
          sentence = `${pkg} rolled back`;
          if (entry.stderr) sentence += `: ${entry.stderr.split('\n')[0]}`;
          break;
        default:
          sentence = `${pkg} install: ${result}`;
      }
    } else if (entry.type === 'system_event') {
      const eventAction = entry.action.replace(/_/g, ' ');
      const msg = entry.stdout?.split('\n')[0]?.trim();
      if (msg) {
        sentence = msg;
      } else {
        sentence = `${eventAction}: ${entry.status || 'info'}`;
      }
    } else if (entry.type === 'command') {
      if (entry.action.startsWith('scan_')) {
        const display = getCommandDisplay(entry.action);
        const verb = display.verb;
        const noun = display.noun;

        if (isInProgress) {
          sentence = `${verb} ${noun} initiated`;
        } else if (statusType === 'success') {
          sentence = `${verb} ${noun} completed`;
        } else if (statusType === 'failed') {
          sentence = `${verb} ${noun} failed`;
        } else {
          sentence = `${verb} ${noun} results`;
        }
      } else if (action === 'update agent') {
        if (isInProgress) {
          sentence = `Agent Update initiated to version ${subject}`;
        } else if (statusType === 'success') {
          sentence = `Agent updated to version ${subject}`;
        } else if (statusType === 'failed') {
          sentence = `Agent update failed for version ${subject}`;
        } else {
          sentence = `Agent update to version ${subject}`;
        }
      } else if (action === 'dry run update') {
        if (isInProgress) {
          sentence = `Dry run initiated for ${subject}`;
        } else if (statusType === 'success') {
          sentence = `Dry run completed: ${subject} available`;
        } else if (statusType === 'failed') {
          sentence = `Dry run failed for ${subject}`;
        } else {
          sentence = `Dry run results: ${subject} available`;
        }
      } else if (action === 'confirm dependencies') {
        if (isInProgress) {
          sentence = `Dependency confirmation initiated for '${subject}'`;
        } else if (statusType === 'success') {
          sentence = `Dependencies confirmed for '${subject}'`;
        } else if (statusType === 'failed') {
          sentence = `Dependency confirmation failed for '${subject}'`;
        } else {
          sentence = `Dependency check for '${subject}'`;
        }
      } else if (action === 'install update' || action === 'install') {
        if (isInProgress) {
          sentence = `${subject} installation initiated`;
        } else if (statusType === 'success') {
          sentence = `${subject} installed successfully`;
        } else if (statusType === 'failed') {
          sentence = `${subject} installation failed`;
        } else {
          sentence = `${subject} installation`;
        }
      } else {
        // Generic action - simplified system thought style
        if (isInProgress) {
          sentence = `${action} initiated for '${subject}'`;
        } else if (statusType === 'success') {
          sentence = `${action} completed for '${subject}'`;
        } else if (statusType === 'failed') {
          sentence = `${action} failed for '${subject}'`;
        } else {
          sentence = `${action} for '${subject}'`;
        }
      }
    } else {
      // Log entry - extract meaningful content (only if not already set by command processing)
      if (!sentence) {
        if (entry.stdout) {
          try {
            const parsed = JSON.parse(entry.stdout);
            if (parsed.message) {
              sentence = parsed.message;
            } else {
              sentence = `System log: ${entry.action}`;
            }
          } catch {
            // Create smart summary for package management operations
            if (['upgrade', 'install', 'confirm_dependencies', 'dry_run_update'].includes(entry.action)) {
              sentence = createPackageOperationSummary(entry);
            } else {
              const lines = entry.stdout.split('\n');
              const firstLine = lines[0]?.trim();
              // Clean up common prefixes for more elegant system thoughts
              if (firstLine) {
                sentence = firstLine
                  .replace(/^(INFO|WARN|ERROR|DEBUG):\s*/i, '')
                  .replace(/^Step \d+:\s*/i, '')
                  .replace(/^Command:\s*/i, '')
                  .replace(/^Output:\s*/i, '')
                  .trim() || `System log: ${entry.action}`;
              } else {
                sentence = `System log: ${entry.action}`;
              }
            }
          }
        } else {
          // No stdout to extract real data from. This is the ONLY place narrative
          // should override — when the UI has nothing better than a raw enum.
          // Carefully-crafted branches above (stdout extraction, package name
          // extraction, action-specific verbiage) take precedence and stay
          // untouched.
          sentence = entry.narrative || `System event: ${entry.action}`;
        }
      }
    }

    // A retried command carries the same action/result as its original; mark it
    // so the history reads as a re-attempt rather than a fresh action.
    if (entry.is_retry && sentence) {
      sentence = `Retry — ${sentence}`;
    }

    // Add agent location for global view
    if (!isScopedView && entry.hostname) {
      sentence += ` on ${entry.hostname}`;
    }

    // Add inline timestamp and duration
    const timeStr = formatTimeOnly(entry.created_at);
    const duration = entry.duration_seconds || 0;
    let durationStr = '';

    if (duration > 0) {
      // Format duration nicely
      if (duration < 60) {
        durationStr = ` (${duration}s)`;
      } else if (duration < 3600) {
        const minutes = Math.floor(duration / 60);
        const seconds = duration % 60;
        durationStr = ` (${minutes}m ${seconds}s)`;
      } else {
        const hours = Math.floor(duration / 3600);
        const minutes = Math.floor((duration % 3600) / 60);
        durationStr = ` (${hours}h ${minutes}m)`;
      }
    } else {
      // Show minimum 1s for null/zero duration to avoid empty parentheses
      durationStr = ' (1s)';
    }

    sentence += ` at ${timeStr}${durationStr}`;

    return {
      sentence,
      statusType,
      statusIcon,
      hoverColor,
      borderColor,
      subject,
    };
  };

  // Copy to clipboard utility
  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(`Copied ${label} to clipboard`);
    } catch (err) {
      console.error('Failed to copy:', err);
      toast.error('Failed to copy to clipboard');
    }
  };

  // Event Bubble Component with professional narrative design and pastel color-coding
  const EventBubble: React.FC<{
    entry: HistoryEntry;
    isExpanded: boolean;
    isScopedView: boolean;
    onToggle: () => void;
  }> = ({ entry, isExpanded, isScopedView: _isScopedView, onToggle }) => {
    const narrative = getNarrativeSummary(entry);
    const packageName = entryPackageName(entry);
    const updateLink = entryUpdateLink(entry);

    return (
      <div className="group rounded-lg transition-all duration-200">
        <div className="p-2 rounded-lg transition-all duration-200 bg-white">
          {/* Narrative content with inline status indicator */}
          <div
            className="flex items-center justify-between cursor-pointer group"
            onClick={onToggle}
          >
            {/* Narrative sentence with status indicator */}
            <div className="flex items-center space-x-3 text-gray-700 flex-1 min-w-0">
              {/* Status indicator */}
              <div className="flex items-center space-x-2 flex-shrink-0">
                {narrative.statusType === 'success' && (
                  <>
                    <CheckCircle className="h-3 w-3 text-green-600" />
                    <span className="code code-success">
                      SUCCESS
                    </span>
                  </>
                )}
                {narrative.statusType === 'failed' && (
                  <>
                    <XCircle className="h-3 w-3 text-red-600" />
                    <span className="code code-danger">
                      FAILED
                    </span>
                  </>
                )}
                {narrative.statusType === 'running' && (
                  <>
                    <RefreshCw className="h-3 w-3 text-blue-600 animate-spin" />
                    <span className="code code-info">
                      RUNNING
                    </span>
                  </>
                )}
                {narrative.statusType === 'started' && (
                  <>
                    <RefreshCw className="h-3 w-3 text-blue-600 animate-spin" />
                    <span className="code code-info">
                      STARTED
                    </span>
                  </>
                )}
                {narrative.statusType === 'pending' && (
                  <>
                    <Clock className="h-3 w-3 text-amber-600" />
                    <span className="code code-warning">
                      PENDING
                    </span>
                  </>
                )}
                {narrative.statusType === 'warning' && (
                  <>
                    <AlertTriangle className="h-3 w-3 text-amber-600" />
                    <span className="code code-warning">
                      {entry.result === 'rollback' ? 'ROLLBACK' :
                       entry.result === 'warning' ? 'WARNING' :
                       'TIMEOUT'}
                    </span>
                  </>
                )}
                {narrative.statusType === 'partial' && (
                  <>
                    <AlertTriangle className="h-3 w-3 text-amber-600" />
                    <span className="code code-warning">
                      PARTIAL
                    </span>
                  </>
                )}
                {narrative.statusType === 'info' && (
                  <>
                    <Activity className="h-3 w-3 text-gray-600" />
                    <span className="code code-neutral">
                      {entry.type === 'package_event' ? entry.action?.toUpperCase() || 'INFO' :
                       entry.type === 'system_event' ? 'SYSTEM' :
                       'INFO'}
                    </span>
                  </>
                )}
              </div>

              <span className="text-sm leading-relaxed flex-1 break-words">
                {narrative.sentence}
              </span>
            </div>

            {/* Expand/collapse icon - aligned inline */}
            <div className="flex-shrink-0 ml-3 text-gray-400 group-hover:text-gray-600 transition-colors">
              {isExpanded ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </div>
          </div>

          {/* Critical vitals - always visible in collapsed view */}
          <div className="mt-2 ml-8 text-xs text-gray-600 space-y-1">
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              {!_isScopedView && entry.agent_id && (
                <span>
                  <span className="font-medium">Agent:</span>{' '}
                  <Link
                    to={`/agents/${entry.agent_id}`}
                    className="text-blue-700 hover:text-blue-900 hover:underline"
                  >
                    {entry.hostname || `${entry.agent_id.slice(0, 8)}...`}
                  </Link>
                </span>
              )}
              <span>
                <span className="font-medium">Action:</span> {entry.action.replace(/_/g, ' ')}
              </span>
              <span>
                <span className="font-medium">Result:</span> {entry.result}
                {entry.exit_code !== undefined && (
                  <span className="text-gray-500"> (Exit Code: {entry.exit_code})</span>
                )}
              </span>
              {updateLink && (
                <span>
                  <span className="font-medium">Package:</span>{' '}
                  <Link
                    to={updateLink}
                    className="text-green-700 hover:text-green-900 hover:underline"
                  >
                    {packageName || 'View Update'}
                  </Link>
                </span>
              )}
              {narrative.subject && narrative.subject !== 'system operation' && narrative.subject !== entry.package_name && (
                <span>
                  <span className="font-medium">Target:</span> {narrative.subject.length > 50 ? narrative.subject.substring(0, 50) + '...' : narrative.subject}
                </span>
              )}
            </div>
          </div>

          {/* Expanded details with integrated frosted glass effect */}
          {isExpanded && (
            <div className="mt-2 ml-4">
              {/* Integrated frosted glass pane container */}
              <div className="relative bg-white/90 backdrop-blur-md rounded-lg shadow-xl transition-all duration-200">
                {/* Copy button */}
                <div className="absolute top-2 right-2 z-10">
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      const fullOutput = [
                        entry.stdout ? `STDOUT:\n${entry.stdout}` : '',
                        entry.stderr ? `STDERR:\n${entry.stderr}` : '',
                      ].filter(Boolean).join('\n\n');
                      copyToClipboard(fullOutput, 'output');
                    }}
                    className="p-1.5 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded transition-colors"
                    title="Copy output to clipboard"
                  >
                    <Copy className="h-3.5 w-3.5" />
                  </button>
                </div>

                <div className="p-4 space-y-4">
                  {/* System Information */}
                  <div className="bg-gray-50 rounded-lg p-3 border border-gray-200">
                    <h4 className="text-xs font-semibold text-gray-700 uppercase tracking-wide mb-3 flex items-center">
                      <Activity className="h-3 w-3 mr-1.5" />
                      System Information
                    </h4>
                    <div className="grid grid-cols-2 md:grid-cols-3 gap-3 text-xs">
                      <div className="flex flex-col">
                        <span className="text-gray-500 font-medium">Command ID</span>
                        <span className="font-mono text-gray-800 break-all">{entry.id}</span>
                      </div>
                      {packageName && (
                        <div className="flex flex-col">
                          <span className="text-gray-500 font-medium">Package</span>
                          <span className="text-gray-800 truncate" title={packageName}>
                            {packageName}
                          </span>
                        </div>
                      )}
                      <div className="flex flex-col">
                        <span className="text-gray-500 font-medium">Exit Code</span>
                        <span className={cn(
                          "font-mono",
                          entry.exit_code === 0 ? "text-green-600" :
                          entry.exit_code ? "text-red-600" : "text-gray-600"
                        )}>
                          {entry.exit_code !== undefined ? entry.exit_code : 'N/A'}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Parsed Details from stdout */}
                  {entry.stdout && (
                    <div className="alert alert-info">
                      <h4 className="text-xs font-semibold text-gray-700 uppercase tracking-wide mb-3 flex items-center">
                        {entry.action === 'scan_storage' ? (
                          <HardDrive className="h-3 w-3 mr-1.5" />
                        ) : entry.action === 'scan_system' ? (
                          <Cpu className="h-3 w-3 mr-1.5" />
                        ) : entry.action === 'scan_docker' ? (
                          <Container className="h-3 w-3 mr-1.5" />
                        ) : (
                          <Package className="h-3 w-3 mr-1.5" />
                        )}
                        {entry.action === 'scan_storage' ? 'Disk Usage Report' :
                         entry.action === 'scan_system' ? 'System Metrics Report' :
                         entry.action === 'scan_docker' ? 'Docker Image Analysis' :
                         entry.action === 'scan_apt' ? 'APT Package Scan' :
                         entry.action === 'scan_dnf' ? 'DNF Package Scan' :
                         entry.action === 'scan_winget' ? 'Winget Package Scan' :
                         entry.action === 'scan_windows' ? 'Windows Update Scan' :
                         'Operation Details'}
                      </h4>
                      <div className="space-y-2 text-xs">
                        {(() => {
                          const stdout = entry.stdout;
                          const details: Array<{label: string, value: string}> = [];

                          // Handle scan results specifically
                          if (entry.action === 'scan_apt' || entry.action === 'scan_dnf' ||
                            entry.action === 'scan_winget' || entry.action === 'scan_windows') {
                            // Extract update counts
                            const updateCountMatch = stdout.match(/Found\s+(\d+)\s+([^:\n]+)/i);
                            if (updateCountMatch) {
                              details.push({
                                label: "Updates Found",
                                value: `${updateCountMatch[1]} ${updateCountMatch[2].trim()}`
                              });
                            }

                            const totalUpdatesMatch = stdout.match(/Total Updates Found:\s*(\d+)/i);
                            if (totalUpdatesMatch) {
                              details.push({
                                label: "Total Updates",
                                value: totalUpdatesMatch[1]
                              });
                            }

                            // Extract scanner availability
                            const availableScanners: string[] = [];
                            const unavailableScanners: string[] = [];

                            const scannerLines = stdout.match(/([A-Z][a-z]+)\s+scanner\s+(not\s+available|available)/gi);
                            if (scannerLines) {
                              scannerLines.forEach(line => {
                                const match = line.match(/([A-Z][a-z]+)\s+scanner\s+(not\s+available|available)/i);
                                if (match) {
                                  if (match[2].toLowerCase().includes('not')) {
                                    unavailableScanners.push(match[1]);
                                  } else {
                                    availableScanners.push(match[1]);
                                  }
                                }
                              });
                            }

                            if (availableScanners.length > 0) {
                              details.push({
                                label: "Available Scanners",
                                value: availableScanners.join(", ")
                              });
                            }

                            // Extract scan errors
                            const scanErrorsMatch = stdout.match(/Scan Errors:\s*\n([\s\S]*?)(?=\n\n|\n[A-Z]|\n$)/);
                            if (scanErrorsMatch) {
                              details.push({
                                label: "Scan Errors",
                                value: scanErrorsMatch[1].replace(/\\n/g, ' ').trim()
                              });
                            }

                            // Extract individual scanner failures
                            const failureLines = stdout.match(/^([A-Z][a-z]+)\s+scan\s+failed:\s*([^\n]+)/gm);
                            if (failureLines) {
                              failureLines.forEach(line => {
                                const match = line.match(/([A-Z][a-z]+)\s+scan\s+failed:\s*([^\n]+)/);
                                if (match) {
                                  details.push({
                                    label: `${match[1]} Scanner`,
                                    value: `Failed: ${match[2].replace(/\\n/g, ' ').trim()}`
                                  });
                                }
                              });
                            }
                          } else if (entry.action === 'scan_storage') {
                            // Parse storage/disk usage information
                            // Look for disk metrics in the stdout
                            const diskLines = stdout.split('\n');
                            diskLines.forEach(line => {
                              // Match patterns like "Mount: /dev/sda1" or "Usage: 85%"
                              const mountMatch = line.match(/(?:Mount|Filesystem|Path):\s*([^\s]+)/i);
                              const usageMatch = line.match(/(?:Usage|Used):\s*(\d+\.?\d*%?)/i);
                              const sizeMatch = line.match(/(?:Size|Total):\s*([^\s]+)/i);
                              const availMatch = line.match(/(?:Available|Free):\s*([^\s]+)/i);

                              if (mountMatch || usageMatch || sizeMatch || availMatch) {
                                if (mountMatch) details.push({ label: "Mount Point", value: mountMatch[1] });
                                if (usageMatch) details.push({ label: "Usage", value: usageMatch[1] });
                                if (sizeMatch) details.push({ label: "Total Size", value: sizeMatch[1] });
                                if (availMatch) details.push({ label: "Available", value: availMatch[1] });
                              }
                            });
                          } else if (entry.action === 'scan_system') {
                            // Parse system metrics (CPU, memory, processes, uptime)
                            const cpuMatch = stdout.match(/(?:CPU|Processor):\s*([^\n]+)/i);
                            if (cpuMatch) {
                              details.push({ label: "CPU", value: cpuMatch[1].trim() });
                            }

                            const memoryMatch = stdout.match(/(?:Memory|RAM):\s*([^\n]+)/i);
                            if (memoryMatch) {
                              details.push({ label: "Memory", value: memoryMatch[1].trim() });
                            }

                            const processMatch = stdout.match(/(?:Processes|Process Count):\s*(\d+)/i);
                            if (processMatch) {
                              details.push({ label: "Running Processes", value: processMatch[1] });
                            }

                            const uptimeMatch = stdout.match(/(?:Uptime|Up Time):\s*([^\n]+)/i);
                            if (uptimeMatch) {
                              details.push({ label: "System Uptime", value: uptimeMatch[1].trim() });
                            }

                            const loadMatch = stdout.match(/(?:Load Average|Load):\s*([^\n]+)/i);
                            if (loadMatch) {
                              details.push({ label: "Load Average", value: loadMatch[1].trim() });
                            }
                          } else if (entry.action === 'scan_docker') {
                            // Parse Docker image/container information
                            const containerCountMatch = stdout.match(/(?:Containers|Container Count):\s*(\d+)/i);
                            if (containerCountMatch) {
                              details.push({ label: "Containers", value: containerCountMatch[1] });
                            }

                            const imageCountMatch = stdout.match(/(?:Images|Image Count):\s*(\d+)/i);
                            if (imageCountMatch) {
                              details.push({ label: "Images", value: imageCountMatch[1] });
                            }

                            const updateCountMatch = stdout.match(/(?:Updates Available|Updatable Images):\s*(\d+)/i);
                            if (updateCountMatch) {
                              details.push({ label: "Updates Available", value: updateCountMatch[1] });
                            }

                            const runningMatch = stdout.match(/(?:Running Containers):\s*(\d+)/i);
                            if (runningMatch) {
                              details.push({ label: "Running", value: runningMatch[1] });
                            }
                          }

                          // Extract "Packages installed" info
                          const packagesMatch = stdout.match(/Packages installed:\s*\[([^\]]+)\]/i);
                          if (packagesMatch) {
                            details.push({
                              label: "Installed Package",
                              value: packagesMatch[1].trim()
                            });
                          }

                          // Extract KB articles
                          const kbMatch = stdout.match(/KB(\d{7,8})/g);
                          if (kbMatch) {
                            details.push({
                              label: "KB Articles",
                              value: kbMatch.join(", ")
                            });
                          }

                          // Extract version info
                          const versionMatch = stdout.match(/Version\s*([\d.]+)/i);
                          if (versionMatch) {
                            details.push({
                              label: "Version",
                              value: versionMatch[1]
                            });
                          }

                          // Extract method info
                          const methodMatch = stdout.match(/Method:\s*([^\n]+)/i);
                          if (methodMatch) {
                            details.push({
                              label: "Method",
                              value: methodMatch[1].trim()
                            });
                          }

                          // Extract requirements
                          const requiresMatch = stdout.match(/Requires:\s*([^\n]+)/i);
                          if (requiresMatch) {
                            details.push({
                              label: "Requirements",
                              value: requiresMatch[1].trim()
                            });
                          }

                          return details.length > 0 ? (
                            details.map((detail, idx) => (
                              <div key={idx} className="flex flex-col sm:flex-row sm:gap-2">
                                <span className="text-gray-600 font-medium min-w-0 sm:min-w-[120px]">
                                  {detail.label}:
                                </span>
                                <span className="text-gray-800 break-all font-mono">
                                  {detail.value.replace(/\\n/g, ' ').trim()}
                                </span>
                              </div>
                            ))
                          ) : (
                            <div className="text-gray-500 italic">
                              No structured details found in output
                            </div>
                          );
                        })()}
                      </div>
                    </div>
                  )}

                  {/* Contextual Navigation Links */}
                  <div className="flex flex-wrap gap-2 text-xs">
                    <Link
                      to={`/agents/${entry.agent_id}`}
                      className="inline-flex items-center px-2.5 py-1.5 bg-blue-50 text-blue-700 rounded-md hover:bg-blue-100 transition-colors font-medium"
                    >
                      <User className="h-3 w-3 mr-1" />
                      {entry.hostname || 'View Agent'}
                    </Link>

                    {updateLink && (
                      <Link
                        to={updateLink}
                        className="inline-flex items-center px-2.5 py-1.5 bg-green-50 text-green-700 rounded-md hover:bg-green-100 transition-colors font-medium"
                      >
                        <Package className="h-3 w-3 mr-1" />
                        {packageName || 'View Update'}
                      </Link>
                    )}

                    {(entry.result === 'failed' || entry.result === 'timed_out') && (
                      <button
                        onClick={async (e) => {
                          e.stopPropagation();
                          try {
                            await retryCommandMutation.mutateAsync(entry.id);
                            toast.success(`Retry command sent to ${entry.hostname || 'agent'}`);
                          } catch (error: any) {
                            toast.error(`Failed to retry command: ${error.message || 'Unknown error'}`);
                          }
                        }}
                        disabled={retryCommandMutation.isPending}
                        className="inline-flex items-center px-2.5 py-1.5 bg-amber-50 text-amber-700 rounded-md hover:bg-amber-100 transition-colors font-medium"
                      >
                        {retryCommandMutation.isPending ? (
                          <RefreshCw className="h-3 w-3 mr-1 animate-spin" />
                        ) : (
                          <RefreshCw className="h-3 w-3 mr-1" />
                        )}
                        {retryCommandMutation.isPending ? 'Retrying...' : 'Retry Command'}
                      </button>
                    )}
                  </div>

                  {/* Output Section */}
                  {(entry.stdout || entry.stderr) && (
                    <div className="space-y-3">
                      {entry.stdout && (
                        <div className="space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-xs font-medium text-gray-600">Output</span>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                copyToClipboard(entry.stdout!, 'output');
                              }}
                              className="text-xs text-gray-500 hover:text-gray-700 transition-colors"
                            >
                              <Copy className="h-3 w-3 inline mr-1" />
                              Copy
                            </button>
                          </div>
                          <div className="bg-gray-900 rounded-md border border-gray-200">
                            <Highlight
                              theme={themes.vsDark}
                              code={entry.stdout}
                              language="bash"
                            >
                              {({ className, style, tokens, getLineProps, getTokenProps }) => (
                                <pre className={cn("p-3 text-xs overflow-x-auto font-mono leading-relaxed", className)} style={style}>
                                  {tokens.map((line, i) => (
                                    <div key={i} {...getLineProps({ line })} className="hover:bg-gray-800 px-1 -mx-1 rounded transition-colors">
                                      {line.map((token, key) => (
                                        <span key={key} {...getTokenProps({ token })} />
                                      ))}
                                    </div>
                                  ))}
                                </pre>
                              )}
                            </Highlight>
                          </div>
                        </div>
                      )}

                      {entry.stderr && (
                        <div className="space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-xs font-medium text-red-600">Error Output</span>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                copyToClipboard(entry.stderr!, 'error output');
                              }}
                              className="text-xs text-gray-500 hover:text-gray-700 transition-colors"
                            >
                              <Copy className="h-3 w-3 inline mr-1" />
                              Copy
                            </button>
                          </div>
                          <div className="bg-red-950/90 rounded-md border border-red-200">
                            <Highlight
                              theme={themes.vsDark}
                              code={entry.stderr}
                              language="bash"
                            >
                              {({ className, style, tokens, getLineProps, getTokenProps }) => (
                                <pre className={cn("p-3 text-xs overflow-x-auto font-mono leading-relaxed text-red-300", className)} style={style}>
                                  {tokens.map((line, i) => (
                                    <div key={i} {...getLineProps({ line })} className="hover:bg-red-900/30 px-1 -mx-1 rounded transition-colors">
                                      {line.map((token, key) => (
                                        <span key={key} {...getTokenProps({ token })} />
                                      ))}
                                    </div>
                                  ))}
                                </pre>
                              )}
                            </Highlight>
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className={cn("space-y-6", className)}>
  
      {/* Loading state */}
      {isLoading && (
        <div className="flex items-center justify-center py-12">
          <RefreshCw className="h-6 w-6 animate-spin text-gray-400" />
          <span className="ml-2 text-gray-600">Loading events...</span>
        </div>
      )}

      {/* Timeline */}
      {!isLoading && filteredEntries.length === 0 ? (
        <div className="text-center py-12">
          <Search className="mx-auto h-12 w-12 text-gray-400" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">No events found</h3>
          <p className="mt-1 text-sm text-gray-500">
            {externalSearch || statusFilter !== 'all' || selectedAgents.length > 0
              ? 'Try adjusting your search or filters.'
              : 'No events have been recorded yet.'}
          </p>
        </div>
      ) : (
        <div className={cn(
          isScopedView ? "bg-gray-50 rounded-lg border border-gray-200 p-4" : "",
          "space-y-4"
        )}>
          {createTimelineWithDividers(filteredEntries)}
        </div>
      )}

      {/* Load More */}
      {historyData && historyData.total > filteredEntries.length && (
        <div className="flex justify-center mt-6">
          <button
            onClick={() => setQueryParams(prev => ({ ...prev, page: prev.page + 1 }))}
            disabled={isFetching}
            className="flex items-center space-x-2 px-6 py-3 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {isFetching ? (
              <RefreshCw className="h-4 w-4 animate-spin" />
            ) : (
              <span>Load More Events</span>
            )}
          </button>
        </div>
      )}
    </div>
  );
};

export default ChatTimeline;
