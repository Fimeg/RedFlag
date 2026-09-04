import React, { useState, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams, Link } from 'react-router-dom';
import {
  Package,
  CheckCircle,
  XCircle,
  Computer,
  ExternalLink,
  ChevronLeft,
  ChevronRight,
  AlertTriangle,
  Clock,
  X,
  Loader2,
  RotateCcw,
  ArrowUpDown,
  ArrowUp,
  ArrowDown,
  Shield,
  ShieldAlert,
  GitBranch,
  Activity,
  HardDrive,
  FileText,
} from 'lucide-react';
import { FilterBar, StatCard, StatCardGroup, PageState, Pagination, Modal, StatusBadge, SeverityBadge, CommandStatusBadge } from '@/components/primitives';
import { useFilterUrl, buildFilterPills } from '@/hooks/useFilterUrl';
import { useDebounce } from '@/hooks/useDebounce';
import { useQueryClient } from '@tanstack/react-query';
import { useUpdates, useUpdate, usePackages, usePackageFleet, usePackageVersions, useUpdateLifecycle, useApproveUpdate, useRejectUpdate, useInstallUpdate, useApproveMultipleUpdates, useRetryCommand, useReopenUpdate, useResolveUpdate, useCancelCommand } from '@/hooks/useUpdates';
import { useRecentCommands } from '@/hooks/useCommands';
import type { UpdatePackage } from '@/types';
import { getPackageTypeIcon, formatBytes, formatRelativeTime } from '@/lib/utils';
import { cn } from '@/lib/utils';
import { lifecycleHistoryStatusColor, commandStatusInlineColor } from '@/components/primitives/statusColors';
import toast from 'react-hot-toast';
import { updateApi } from '@/lib/api';
import DependencyClosureTree from '@/components/DependencyClosureTree';
import VulnerabilityList from '@/components/VulnerabilityList';
import { parseVulnerabilityList } from '@/lib/vulnerabilities';

type UpdatesTab = 'updates' | 'commands';

const parseUpdatesTab = (tab: string | null): UpdatesTab => {
  return tab === 'commands' ? 'commands' : 'updates';
};

const Updates: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Filter state synced to URL (via useFilterUrl)
  const filterConfig = {
    status: { urlParam: 'status', label: 'Status' },
    severity: { urlParam: 'severity', label: 'Severity' },
    type: { urlParam: 'type', label: 'Type' },
    agent: { urlParam: 'agent', label: 'Agent' },
    vuln: { urlParam: 'vuln', label: 'Vulnerable' },
  };
  const filter = useFilterUrl(filterConfig);

  // Search query — kept separate because the input needs instant feedback
  // while the API call uses the debounced value
  const [searchQuery, setSearchQuery] = useState(searchParams.get('search') || '');
  const debouncedSearchQuery = useDebounce(searchQuery, 300);
  const [sortBy, setSortBy] = useState(searchParams.get('sort_by') || '');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(searchParams.get('sort_order') as 'asc' | 'desc' || 'desc');
  const [selectedUpdates, setSelectedUpdates] = useState<string[]>([]);
  const [currentPage, setCurrentPage] = useState(parseInt(searchParams.get('page') || '1'));
  const [pageSize, setPageSize] = useState(100);
  const [showLogModal, setShowLogModal] = useState(false);
  const [logs, setLogs] = useState<any[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [showDependencyModal, setShowDependencyModal] = useState(false);
  const [pendingDependencies, setPendingDependencies] = useState<string[]>([]);
  const [dependencyUpdateId, setDependencyUpdateId] = useState<string | null>(null);
  const [dependencyLoading, setDependencyLoading] = useState(false);
  const activeTab = parseUpdatesTab(searchParams.get('tab'));

  // Store non-filter state in URL (search is separate because it needs debounce)
  useEffect(() => {
    const params = new URLSearchParams(searchParams);
    if (activeTab !== 'updates') params.set('tab', activeTab);
    if (debouncedSearchQuery) params.set('search', debouncedSearchQuery);
    if (sortBy) params.set('sort_by', sortBy);
    if (sortOrder) params.set('sort_order', sortOrder);
    if (currentPage > 1) params.set('page', currentPage.toString());
    if (pageSize !== 100) params.set('page_size', pageSize.toString());

    setSearchParams(params, { replace: true });
  }, [activeTab, debouncedSearchQuery, sortBy, sortOrder, currentPage, pageSize, setSearchParams]);

  const selectActiveTab = (tab: UpdatesTab) => {
    const params = new URLSearchParams(searchParams);
    if (tab === 'updates') {
      params.delete('tab');
    } else {
      params.set('tab', tab);
    }
    setSearchParams(params, { replace: true });
  };

  // Fetch updates list (still used for the summary stat cards + detail fallback)
  const { data: updatesData } = useUpdates({
    search: debouncedSearchQuery || undefined,
    status: filter.values.status || undefined,
    severity: filter.values.severity || undefined,
    type: filter.values.type || undefined,
    agent: filter.values.agent || undefined,
    sort_by: sortBy || undefined,
    sort_order: sortOrder || undefined,
    page: currentPage,
    page_size: pageSize,
  });

  // Fetch the package-centric list (one row per package across the fleet).
  // Search, type, and status are server-side; severity filters client-side below.
  const { data: packagesData, isPending: packagesPending, error: packagesError } = usePackages({
    search: debouncedSearchQuery || undefined,
    type: filter.values.type || undefined,
    status: filter.values.status || undefined,
    sort_by: sortBy || undefined,
    sort_order: sortOrder || undefined,
    page: currentPage,
    page_size: pageSize,
  });

  // Fetch single update if ID is provided
  const { data: selectedUpdateData } = useUpdate(id || '', !!id);

  // Fetch the fleet view (which agents run this same package) for the detail pane
  const { data: packageFleetData } = usePackageFleet(id || '', !!id);

  // Fetch the version timeline catalog for the detail pane
  const { data: packageVersionsData } = usePackageVersions(id || '', !!id);

  // Fetch lifecycle history (state transitions) for the detail pane
  const { data: lifecycleData } = useUpdateLifecycle(id || '', !!id);

  // Fetch recent commands for retry functionality
  const { data: recentCommandsData } = useRecentCommands(50);

  const queryClient = useQueryClient();
  const approveMutation = useApproveUpdate();
  const rejectMutation = useRejectUpdate();
  const installMutation = useInstallUpdate();
  const bulkApproveMutation = useApproveMultipleUpdates();
  const retryMutation = useRetryCommand();
  const reopenMutation = useReopenUpdate();
  const resolveMutation = useResolveUpdate();
  const cancelMutation = useCancelCommand();

  const updates = updatesData?.updates || [];
  const totalCount = updatesData?.total || 0;
  const selectedUpdate = selectedUpdateData || updates.find((u: UpdatePackage) => u.id === id);

  // Package-centric list rows. Search + type are server-side; severity/status chips
  // filter the loaded rows against the aggregate counts so the chips stay meaningful.
  const packages = packagesData?.packages || [];
  const packageTotal = packagesData?.total || 0;
  const displayedPackages = packages.filter((p) => {
    if (filter.values.severity && p.max_severity !== filter.values.severity) return false;
    if (filter.values.vuln && !p.has_vulns) return false;
    return true;
  });
  const packageTotalPages = Math.ceil(packageTotal / pageSize);
  const packageTypes = [...new Set(packages.map((p) => p.package_type))];

  // Handle update actions
  const handleApproveUpdate = async (updateId: string) => {
    try {
      await approveMutation.mutateAsync({ id: updateId });
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleRejectUpdate = async (updateId: string) => {
    try {
      await rejectMutation.mutateAsync(updateId);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleInstallUpdate = async (updateId: string) => {
    try {
      await installMutation.mutateAsync(updateId);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  const handleBulkApprove = async () => {
    if (selectedUpdates.length === 0) {
      toast.error('Please select at least one update');
      return;
    }

    try {
      await bulkApproveMutation.mutateAsync({ update_ids: selectedUpdates });
      setSelectedUpdates([]);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  // Handle retry command
  const handleRetryCommand = async (commandId: string) => {
    try {
      await retryMutation.mutateAsync(commandId);
      toast.success('Command retry initiated successfully');
    } catch (error: any) {
      toast.error(`Failed to retry command: ${error.message || 'Unknown error'}`);
    }
  };

  // Re-open a failed update for another lifecycle attempt (failed → pending).
  const handleReopenUpdate = async (updateId: string) => {
    try {
      await reopenMutation.mutateAsync(updateId);
      toast.success('Update re-opened — will re-enter the approval flow');
    } catch (error: any) {
      const msg = error?.response?.data?.error || error?.message || 'Unknown error';
      toast.error(`Failed to re-open update: ${msg}`);
    }
  };

  // Resolve a failed update as installed (update no longer applies).
  const handleResolveUpdate = async (updateId: string) => {
    try {
      await resolveMutation.mutateAsync(updateId);
      toast.success('Update marked as resolved');
    } catch (error: any) {
      const msg = error?.response?.data?.error || error?.message || 'Unknown error';
      toast.error(`Failed to resolve update: ${msg}`);
    }
  };

  // Handle cancel command
  const handleCancelCommand = async (commandId: string) => {
    try {
      await cancelMutation.mutateAsync(commandId);
      toast.success('Command cancelled successfully');
    } catch (error: any) {
      toast.error(`Failed to cancel command: ${error.message || 'Unknown error'}`);
    }
  };

  // Handle dependency confirmation
  const handleConfirmDependencies = async (updateId: string) => {
    setDependencyLoading(true);
    try {
      // Route through the API layer so auth headers are injected (was a raw fetch).
      await updateApi.confirmDependencies(updateId);

      toast.success('Dependency installation confirmed');
      setShowDependencyModal(false);
      setPendingDependencies([]);
      setDependencyUpdateId(null);

      // Refresh the update data
      queryClient.invalidateQueries({ queryKey: ['updates'] });
      queryClient.invalidateQueries({ queryKey: ['update', updateId] });
      queryClient.invalidateQueries({ queryKey: ['activeCommands'] });
      queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
    } catch (error) {
      toast.error('Failed to confirm dependencies');
      console.error('Failed to confirm dependencies:', error);
    } finally {
      setDependencyLoading(false);
    }
  };

  // Handle dependency cancellation
  const handleCancelDependencies = async () => {
    setShowDependencyModal(false);
    setPendingDependencies([]);
    setDependencyUpdateId(null);
    toast('Dependency installation cancelled');
  };

  // Handle viewing logs
  const handleViewLogs = async (updateId: string) => {
    setLogsLoading(true);
    try {
      const result = await updateApi.getUpdateLogs(updateId, 50);
      setLogs(result.logs || []);
      setShowLogModal(true);
    } catch (error) {
      toast.error('Failed to load installation logs');
      console.error('Failed to load logs:', error);
    } finally {
      setLogsLoading(false);
    }
  };

  // Filter dropdown options — the canonical status/severity sets from the
  // package state machine, not values scraped off the current page (which
  // collapse to a single option as soon as a filter is applied).
  const statuses = ['pending', 'approved', 'checking_dependencies', 'pending_dependencies', 'installing', 'installed', 'failed', 'ignored'];
  const severities = ['critical', 'high', 'medium', 'low'];

  // Quick filter functions
  const handleQuickFilter = (quick: string) => {
    switch (quick) {
      case 'critical':
        filter.setFilter('severity', 'critical');
        filter.setFilter('status', 'pending');
        break;
      case 'pending':
        filter.setFilter('status', 'pending');
        filter.setFilter('severity', '');
        break;
      case 'approved':
        filter.setFilter('status', 'approved');
        filter.setFilter('severity', '');
        break;
      case 'installing':
        filter.setFilter('status', 'installing');
        filter.setFilter('severity', '');
        break;
      case 'installed':
        filter.setFilter('status', 'installed');
        filter.setFilter('severity', '');
        break;
      case 'failed':
        filter.setFilter('status', 'failed');
        filter.setFilter('severity', '');
        break;
      case 'dependencies':
        filter.setFilter('status', 'pending_dependencies');
        filter.setFilter('severity', '');
        break;
      case 'vuln':
        filter.setFilter('vuln', 'true');
        filter.setFilter('status', '');
        filter.setFilter('severity', '');
        break;
      default:
        // Clear all filters
        filter.clearAll();
        break;
    }
    setCurrentPage(1);
  };

  // Handle column sorting
  const handleSort = (column: string) => {
    if (sortBy === column) {
      // Toggle sort order if clicking the same column
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      // Set new column with default desc order
      setSortBy(column);
      setSortOrder('desc');
    }
    setCurrentPage(1);
  };

  // Render sort icon for column headers
  const renderSortIcon = (column: string) => {
    if (sortBy !== column) {
      return <ArrowUpDown className="h-4 w-4 ml-1 text-gray-400" />;
    }
    return sortOrder === 'asc' ? (
      <ArrowUp className="h-4 w-4 ml-1 text-primary-600" />
    ) : (
      <ArrowDown className="h-4 w-4 ml-1 text-primary-600" />
    );
  };


  // Get total statistics from API (not just current page)
  const totalStats = {
    total: totalCount,
    pending: updatesData?.stats?.pending_updates || 0,
    approved: updatesData?.stats?.approved_updates || 0,
    critical: updatesData?.stats?.critical_updates || 0,
    high: updatesData?.stats?.high_updates || 0,
  };

  // Update detail view
  if (id && selectedUpdate) {
    const vulns = parseVulnerabilityList(selectedUpdate.metadata?.supply_chain_vulns);

    const dependencies: string[] = Array.isArray(selectedUpdate.metadata?.dependencies)
      ? selectedUpdate.metadata.dependencies
      : [];

    const description: string | undefined = typeof selectedUpdate.metadata?.description === 'string'
      ? selectedUpdate.metadata.description
      : undefined;

    const sizeBytes: number | undefined = typeof selectedUpdate.metadata?.size_bytes === 'number'
      ? selectedUpdate.metadata.size_bytes
      : undefined;

    const checkedAt: string | undefined = typeof selectedUpdate.metadata?.supply_chain_checked_at === 'string'
      ? selectedUpdate.metadata.supply_chain_checked_at
      : undefined;

    // Supply-chain provenance — what RedFlag has collected and pinned for this package.
    const expectedSha: string | undefined = selectedUpdate.expected_sha256 || undefined;
    const publishedAt: string | undefined = typeof selectedUpdate.metadata?.package_published_at === 'string'
      ? selectedUpdate.metadata.package_published_at
      : undefined;
    const ageHours: number | undefined = typeof selectedUpdate.metadata?.package_age_hours === 'number'
      ? selectedUpdate.metadata.package_age_hours
      : undefined;
    const ageCheck = (selectedUpdate.metadata?.supply_chain_age_check && typeof selectedUpdate.metadata.supply_chain_age_check === 'object')
      ? selectedUpdate.metadata.supply_chain_age_check as { min_age_hours?: number; enforcement?: string; source?: string; blocked?: boolean }
      : undefined;
    const resolvedClosure: Array<{ name: string; version: string; sha256: string; source?: string }> =
      Array.isArray(selectedUpdate.metadata?.resolved_closure) ? selectedUpdate.metadata.resolved_closure : [];
    const hasSupplyChainData = !!expectedSha || !!publishedAt || ageCheck !== undefined || resolvedClosure.length > 0;

    const fleetAgents = packageFleetData?.agents || [];
    const versionTimeline = packageVersionsData?.versions || [];

    // Hostname for this update's agent — the fleet data already fetched on
    // this page carries it, so don't show a bare truncated UUID when we can
    // name the machine.
    const agentHostname = fleetAgents.find((fa) => fa.agent_id === selectedUpdate.agent_id)?.hostname;
    const agentLabel = agentHostname || `${selectedUpdate.agent_id.slice(0, 8)}…`;

    // Recent commands narrowed to this update — agent + package_name + package_type
    // is unique per current_package_state row, so this matches without server changes.
    const allRecent = recentCommandsData?.commands || [];
    const updateCommands = allRecent.filter((cmd: any) =>
      cmd.agent_id === selectedUpdate.agent_id &&
      cmd.package_name === selectedUpdate.package_name &&
      cmd.package_type === selectedUpdate.package_type
    );

    // Lifecycle steps. Failed branches off the "Installing" step.
    type StepKey = 'pending' | 'approved' | 'installing' | 'installed';
    const stepOrder: StepKey[] = ['pending', 'approved', 'installing', 'installed'];
    const stepLabels: Record<StepKey, string> = {
      pending: 'Discovered',
      approved: 'Approved',
      installing: 'Installing',
      installed: 'Installed',
    };
    const stepTimestamps: Record<StepKey, string | null> = {
      pending: selectedUpdate.last_discovered_at || selectedUpdate.created_at,
      approved: selectedUpdate.approved_at,
      installing: selectedUpdate.status === 'installing' ? selectedUpdate.last_updated_at : null,
      installed: selectedUpdate.installed_at,
    };
    const currentStepIndex = (() => {
      if (selectedUpdate.status === 'installed') return 3;
      if (selectedUpdate.status === 'installing') return 2;
      if (selectedUpdate.status === 'failed') return 2; // failed during install
      if (['approved', 'checking_dependencies', 'pending_dependencies'].includes(selectedUpdate.status)) return 1;
      if (selectedUpdate.status === 'ignored') return -1; // terminal, not highlighted
      return 0;
    })();
    const isFailed = selectedUpdate.status === 'failed';
    const TypeIcon = getPackageTypeIcon(selectedUpdate.package_type);

    return (
      <div className="px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="mb-6">
          <button
            onClick={() => navigate('/updates')}
            className="text-sm text-gray-500 hover:text-gray-700 mb-4 inline-flex items-center"
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            Back to Updates
          </button>
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <div className="min-w-0 flex-1">
              <div className="flex items-center flex-wrap gap-2 mb-1">
                <TypeIcon className="h-6 w-6 text-gray-500 flex-shrink-0" />
                <h1 className="text-2xl font-bold text-gray-900 truncate">
                  {selectedUpdate.package_name}
                </h1>
                <SeverityBadge severity={selectedUpdate.severity} />
                <StatusBadge
                  status={selectedUpdate.status}
                  label={
                    selectedUpdate.status === 'checking_dependencies' ? (
                      <span className="inline-flex items-center gap-1">
                        <Loader2 className="h-3 w-3 animate-spin" />
                        checking dependencies
                      </span>
                    ) : (
                      selectedUpdate.status.replace(/_/g, ' ')
                    )
                  }
                />
                {vulns.length > 0 && (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-amber-700 bg-amber-50 border border-amber-200 rounded-full px-2.5 py-0.5">
                    <Shield className="w-3.5 h-3.5" />
                    {vulns.length} security {vulns.length === 1 ? 'advisory' : 'advisories'}
                  </span>
                )}
                {/* Supply Chain Gate indicator: show when this update uses capability-token path */}
                {(selectedUpdate.metadata?.capability_token_id || selectedUpdate.metadata?.capability_decision) && (
                  <span className="inline-flex items-center gap-1 text-xs font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-full px-2.5 py-0.5">
                    <Shield className="w-3.5 h-3.5" />
                    Supply Chain Gate
                  </span>
                )}
              </div>
              <p className="text-sm text-gray-500 font-mono">
                {selectedUpdate.current_version} <ChevronRight className="inline h-3 w-3 mx-0.5 text-gray-400" /> {selectedUpdate.available_version}
              </p>
            </div>
          </div>
        </div>

        {/* Install Lifecycle */}
        <div className="card mb-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2">
              <Activity className="h-4 w-4 text-gray-500" />
              Install Lifecycle
            </h2>
            {isFailed && (
              <span className="inline-flex items-center gap-1 text-xs font-medium text-red-700 bg-red-50 border border-red-200 rounded-full px-2.5 py-0.5">
                <XCircle className="w-3.5 h-3.5" />
                Failed during install
              </span>
            )}
          </div>
          <div className="flex items-center">
            {stepOrder.map((step, idx) => {
              const reached = idx <= currentStepIndex;
              const isCurrent = idx === currentStepIndex && !isFailed && selectedUpdate.status !== 'installed';
              const stepFailed = isFailed && idx === 2;
              const ts = stepTimestamps[step];
              return (
                <React.Fragment key={step}>
                  <div className="flex flex-col items-center flex-1 min-w-0">
                    <div
                      className={cn(
                        'h-9 w-9 rounded-full flex items-center justify-center text-xs font-semibold border-2',
                        stepFailed
                          ? 'bg-red-50 border-red-300 text-red-700'
                          : isCurrent
                            ? 'bg-red-50 border-red-500 text-red-700'
                            : reached
                              ? 'bg-green-50 border-green-400 text-green-700'
                              : 'bg-gray-50 border-gray-200 text-gray-400'
                      )}
                    >
                      {stepFailed ? (
                        <XCircle className="h-4 w-4" />
                      ) : reached && !isCurrent ? (
                        <CheckCircle className="h-4 w-4" />
                      ) : isCurrent ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        idx + 1
                      )}
                    </div>
                    <p className={cn(
                      'text-xs font-medium mt-2 truncate max-w-full',
                      stepFailed ? 'text-red-700' : reached ? 'text-gray-900' : 'text-gray-400'
                    )}>
                      {stepLabels[step]}
                    </p>
                    <p className="text-[10px] text-gray-500 mt-0.5 truncate max-w-full">
                      {ts ? formatRelativeTime(ts) : '—'}
                    </p>
                  </div>
                  {idx < stepOrder.length - 1 && (
                    <div
                      className={cn(
                        'h-0.5 flex-1 mx-1 -mt-7',
                        idx < currentStepIndex
                          ? (isFailed && idx === 1 ? 'bg-red-300' : 'bg-green-300')
                          : 'bg-gray-200'
                      )}
                    />
                  )}
                </React.Fragment>
              );
            })}
          </div>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          {/* Main column */}
          <div className="lg:col-span-2 space-y-6">
            {description && (
              <div className="card">
                <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
                  <FileText className="h-4 w-4 text-gray-500" />
                  Description
                </h2>
                <p className="text-sm text-gray-700 leading-relaxed break-words whitespace-pre-line">{description}</p>
              </div>
            )}

            {/* Update Summary — replaces Version Information + Additional Information */}
            <div className="card">
              <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-4">
                <Package className="h-4 w-4 text-gray-500" />
                Update Summary
              </h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
                <div>
                  <p className="text-xs text-gray-500 mb-1">Package Type</p>
                  <p className="text-sm font-medium text-gray-900 uppercase">{selectedUpdate.package_type}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-500 mb-1">Current</p>
                  <p className="text-sm font-mono text-gray-900 truncate" title={selectedUpdate.current_version}>{selectedUpdate.current_version}</p>
                </div>
                <div>
                  <p className="text-xs text-gray-500 mb-1">Available</p>
                  <p className="text-sm font-mono text-gray-900 truncate" title={selectedUpdate.available_version}>{selectedUpdate.available_version}</p>
                </div>
                {sizeBytes !== undefined && (
                  <div>
                    <p className="text-xs text-gray-500 mb-1 inline-flex items-center gap-1">
                      <HardDrive className="h-3 w-3" /> Size
                    </p>
                    <p className="text-sm font-medium text-gray-900">{formatBytes(sizeBytes)}</p>
                  </div>
                )}
                <div>
                  <p className="text-xs text-gray-500 mb-1 inline-flex items-center gap-1">
                    <Clock className="h-3 w-3" /> Discovered
                  </p>
                  <p className="text-sm font-medium text-gray-900">{formatRelativeTime(selectedUpdate.last_discovered_at)}</p>
                </div>
                {selectedUpdate.approved_at && (
                  <div>
                    <p className="text-xs text-gray-500 mb-1 inline-flex items-center gap-1">
                      <CheckCircle className="h-3 w-3" /> Approved
                    </p>
                    <p className="text-sm font-medium text-gray-900">{formatRelativeTime(selectedUpdate.approved_at)}</p>
                  </div>
                )}
                {selectedUpdate.installed_at && (
                  <div>
                    <p className="text-xs text-gray-500 mb-1 inline-flex items-center gap-1">
                      <CheckCircle className="h-3 w-3 text-green-600" /> Installed
                    </p>
                    <p className="text-sm font-medium text-gray-900">{formatRelativeTime(selectedUpdate.installed_at)}</p>
                  </div>
                )}
              </div>
            </div>

            {/* Dependency closure — shared component (update detail + future inventory/staging) */}
            {(dependencies.length > 0 || resolvedClosure.length > 0) && (
              <DependencyClosureTree
                dependencies={dependencies}
                closure={resolvedClosure}
                headerAction={
                  selectedUpdate.status === 'pending_dependencies' ? (
                    <button
                      onClick={() => {
                        setPendingDependencies(dependencies);
                        setDependencyUpdateId(selectedUpdate.id);
                        setShowDependencyModal(true);
                      }}
                      className="text-xs text-amber-700 hover:text-amber-900 font-medium inline-flex items-center gap-1"
                    >
                      <AlertTriangle className="h-3 w-3" />
                      Review
                    </button>
                  ) : undefined
                }
              />
            )}

            <VulnerabilityList vulnerabilities={vulns} checkedAt={checkedAt} />

            {/* Supply Chain — what RedFlag collected and pinned */}
            {hasSupplyChainData && (
              <div className="card">
                <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-4">
                  <Shield className="h-4 w-4 text-gray-500" />
                  Supply Chain
                </h2>
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 mb-4">
                  {expectedSha && (
                    <div className="col-span-2 sm:col-span-3">
                      <p className="text-xs text-gray-500 mb-1 inline-flex items-center gap-1">
                        <HardDrive className="h-3 w-3" /> Pinned artifact hash (SHA256)
                      </p>
                      <p className="text-xs font-mono text-gray-900 break-all" title={expectedSha}>{expectedSha}</p>
                    </div>
                  )}
                  {publishedAt && (
                    <div>
                      <p className="text-xs text-gray-500 mb-1 inline-flex items-center gap-1">
                        <Clock className="h-3 w-3" /> Published
                      </p>
                      <p className="text-sm font-medium text-gray-900">{formatRelativeTime(publishedAt)}</p>
                    </div>
                  )}
                  {ageHours !== undefined && (
                    <div>
                      <p className="text-xs text-gray-500 mb-1">Age at check</p>
                      <p className="text-sm font-medium text-gray-900">{Math.round(ageHours)}h</p>
                    </div>
                  )}
                  {ageCheck && (
                    <div>
                      <p className="text-xs text-gray-500 mb-1">Age gate</p>
                      <p className={cn(
                        'text-sm font-medium',
                        ageCheck.blocked ? 'text-red-700' : 'text-green-700'
                      )}>
                        {ageCheck.blocked ? 'blocked' : 'passed'}
                        {typeof ageCheck.min_age_hours === 'number' && (
                          <span className="text-xs text-gray-500 font-normal"> (min {Math.round(ageCheck.min_age_hours)}h)</span>
                        )}
                      </p>
                    </div>
                  )}
                </div>
                {/* Resolved closure now renders in the shared DependencyClosureTree above. */}
                {/* Capability token execution result */}
                {selectedUpdate.metadata?.capability_token_id && (
                  <div className="mt-4 pt-4 border-t border-gray-200">
                    <p className="text-xs text-gray-500 mb-2 inline-flex items-center gap-1">
                      <Shield className="h-3 w-3" /> Token execution
                    </p>
                    <div className="grid grid-cols-2 gap-3 text-xs">
                      <div>
                        <span className="text-gray-500">Token ID:</span>
                        <span className="ml-1 font-mono text-gray-900">
                          {selectedUpdate.metadata.capability_token_id.slice(0, 12)}...
                        </span>
                      </div>
                      <div>
                        <span className="text-gray-500">Decision:</span>
                        <span className={cn(
                          'ml-1 font-medium',
                          selectedUpdate.metadata.capability_decision === 'executed' ? 'text-green-700' : 'text-red-700'
                        )}>
                          {selectedUpdate.metadata.capability_decision || 'unknown'}
                        </span>
                      </div>
                      {selectedUpdate.metadata.capability_reason && (
                        <div className="col-span-2">
                          <span className="text-gray-500">Reason:</span>
                          <span className="ml-1 text-gray-700">{selectedUpdate.metadata.capability_reason}</span>
                        </div>
                      )}
                      {selectedUpdate.metadata.capability_exit_code !== undefined && (
                        <div>
                          <span className="text-gray-500">Exit code:</span>
                          <span className={cn(
                            'ml-1 font-mono',
                            selectedUpdate.metadata.capability_exit_code === 0 ? 'text-green-700' : 'text-red-700'
                          )}>
                            {selectedUpdate.metadata.capability_exit_code}
                          </span>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* Affected agents — fleet view for this package */}
            {fleetAgents.length > 0 && (
              <div className="card">
                <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
                  <Computer className="h-4 w-4 text-gray-500" />
                  Affected Agents
                  <span className="text-xs text-gray-500 font-normal">({fleetAgents.length})</span>
                </h2>
                <ul className="divide-y divide-gray-100">
                  {fleetAgents.map((fa) => (
                    <li key={fa.agent_id}>
                      <button
                        onClick={() => navigate(`/agents/${fa.agent_id}`)}
                        className="w-full text-left py-2.5 flex items-center gap-3 hover:bg-gray-50 -mx-2 px-2 rounded transition-colors group"
                      >
                        <Computer className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
                        <span className="text-sm text-gray-900 truncate flex-shrink min-w-0">{fa.hostname}</span>
                        <span className="text-xs text-gray-500 font-mono flex-shrink-0">
                          {fa.current_version} <ChevronRight className="inline h-3 w-3 text-gray-400" /> {fa.available_version}
                        </span>
                        <StatusBadge status={fa.status} className="ml-auto flex-shrink-0" label={fa.status.replace(/_/g, ' ')} />
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {/* Version Timeline — the catalog of versions RedFlag has seen */}
            {versionTimeline.length > 0 && (
              <div className="card">
                <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
                  <Clock className="h-4 w-4 text-gray-500" />
                  Version Timeline
                  <span className="text-xs text-gray-500 font-normal">({versionTimeline.length})</span>
                </h2>
                <ul className="divide-y divide-gray-100">
                  {versionTimeline.map((v) => {
                    const osv = v.osv_status;
                    const osvCls =
                      osv === 'vulnerable' ? 'bg-amber-50 text-amber-700 border-amber-200' :
                      osv === 'clean' ? 'bg-green-50 text-green-700 border-green-200' :
                      'bg-gray-50 text-gray-500 border-gray-200';
                    const isCurrent = v.version === selectedUpdate.current_version;
                    const isAvailable = v.version === selectedUpdate.available_version;
                    return (
                      <li key={v.id} className="py-2.5 flex items-center gap-3 flex-wrap">
                        <span className="text-sm font-mono text-gray-900 flex-shrink-0">{v.version}</span>
                        {isCurrent && (
                          <span className="text-[10px] font-medium text-gray-600 bg-gray-100 rounded px-1.5 py-0.5">installed</span>
                        )}
                        {isAvailable && !isCurrent && (
                          <span className="text-[10px] font-medium text-blue-700 bg-blue-50 rounded px-1.5 py-0.5">available</span>
                        )}
                        {osv && (
                          <span className={cn('text-[10px] font-medium border rounded px-1.5 py-0.5', osvCls)}>
                            {osv}
                          </span>
                        )}
                        {v.sha256 && (
                          <span className="text-xs font-mono text-gray-400 truncate" title={v.sha256}>
                            {v.sha256.slice(0, 12)}…
                          </span>
                        )}
                        <span className="text-xs text-gray-500 ml-auto flex-shrink-0">
                          {v.published_at ? `published ${formatRelativeTime(v.published_at)}` : `seen ${formatRelativeTime(v.first_scanned_at)}`}
                        </span>
                      </li>
                    );
                  })}
                </ul>
                <p className="text-xs text-gray-500 mt-3">
                  Accumulated from scans and approvals. Hashes and OSV posture fill in as versions are reviewed.
                </p>
              </div>
            )}

            {/* Lifecycle History — state transitions scoped to this update's agent */}
            {lifecycleData && lifecycleData.history && lifecycleData.history.length > 0 && (
              <div className="card">
                <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
                  <GitBranch className="h-4 w-4 text-gray-500" />
                  Lifecycle History
                  <span className="text-xs text-gray-500 font-normal">
                    ({lifecycleData.count} on {agentLabel})
                  </span>
                </h2>
                <ul className="divide-y divide-gray-100">
                  {lifecycleData.history.map((h: any) => {
                    const cls = lifecycleHistoryStatusColor(h.update_status);
                    const reason = h.update_status === 'failed'
                      ? (h.failure_reason || h.metadata?.failure_reason)
                      : null;
                    return (
                      <li key={h.id} className="py-2.5">
                        <div className="flex items-center gap-3 flex-wrap">
                          <span className={cn('text-[10px] font-medium border rounded px-1.5 py-0.5', cls)}>
                            {h.update_status}
                          </span>
                          <span className="text-sm font-mono text-gray-900">
                            {h.version_from} → {h.version_to}
                          </span>
                          {reason && (
                            <span className="text-xs text-red-600 truncate max-w-xs" title={reason}>
                              {reason}
                            </span>
                          )}
                          <span className="text-xs text-gray-500 ml-auto flex-shrink-0">
                            {formatRelativeTime(h.update_completed_at)}
                          </span>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              </div>
            )}

            {/* Command Timeline */}
            <div className="card">
              <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
                <Activity className="h-4 w-4 text-gray-500" />
                Recent Activity
                {updateCommands.length > 0 && (
                  <span className="text-xs text-gray-500 font-normal">({updateCommands.length})</span>
                )}
              </h2>
              {updateCommands.length === 0 ? (
                <p className="text-sm text-gray-500 italic">No commands recorded for this update yet.</p>
              ) : (
                <ul className="space-y-2">
                  {updateCommands.map((cmd: any) => {
                    return (
                      <li key={cmd.id} className="flex items-center gap-3 py-2 px-3 bg-white border border-gray-100 rounded">
                        <span className={cn('text-xs font-medium border rounded px-2 py-0.5', commandStatusInlineColor(cmd.status))}>
                          {cmd.status}
                        </span>
                        <span className="text-sm text-gray-900 font-mono truncate flex-1">
                          {cmd.command_type}
                        </span>
                        <Link
                          to={`/agents/${cmd.agent_id}`}
                          className="text-xs text-blue-600 hover:text-blue-800 hover:underline flex-shrink-0"
                        >
                          Agent
                        </Link>
                        {cmd.params?.update_id && (
                          <Link
                            to={`/updates/${cmd.params.update_id}`}
                            className="text-xs text-green-600 hover:text-green-800 hover:underline flex-shrink-0"
                          >
                            Update
                          </Link>
                        )}
                        {cmd.is_retry && (
                          <span className="text-xs text-amber-700 inline-flex items-center gap-1">
                            <RotateCcw className="h-3 w-3" />
                            retry
                          </span>
                        )}
                        <span className="text-xs text-gray-500 flex-shrink-0">
                          {formatRelativeTime(cmd.created_at)}
                        </span>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>

            {/* Raw metadata — escape hatch */}
            {selectedUpdate.metadata && Object.keys(selectedUpdate.metadata).length > 0 && (
              <details className="card">
                <summary className="text-sm font-medium text-gray-700 cursor-pointer inline-flex items-center gap-2">
                  <FileText className="h-4 w-4 text-gray-500" />
                  Raw metadata
                </summary>
                <pre className="bg-gray-50 p-3 rounded-md text-xs overflow-x-auto mt-3">
                  {JSON.stringify(selectedUpdate.metadata, null, 2)}
                </pre>
              </details>
            )}
          </div>

          {/* Side column */}
          <div className="space-y-6">
            {/* Actions */}
            <div className="card">
              <h2 className="text-sm font-medium text-gray-700 mb-3">Actions</h2>
              <div className="space-y-2">
                {selectedUpdate.status === 'pending' && (
                  <>
                    <button
                      onClick={() => handleApproveUpdate(selectedUpdate.id)}
                      disabled={approveMutation.isPending}
                      className="w-full btn btn-success"
                    >
                      <CheckCircle className="h-4 w-4 mr-2" />
                      Approve Update
                    </button>
                    <button
                      onClick={() => handleRejectUpdate(selectedUpdate.id)}
                      disabled={rejectMutation.isPending}
                      className="w-full btn btn-secondary"
                    >
                      <XCircle className="h-4 w-4 mr-2" />
                      Reject Update
                    </button>
                  </>
                )}

                {selectedUpdate.status === 'approved' && (
                  <button
                    onClick={() => handleInstallUpdate(selectedUpdate.id)}
                    disabled={installMutation.isPending}
                    className="w-full btn btn-primary"
                  >
                    <Package className="h-4 w-4 mr-2" />
                    Install Now
                  </button>
                )}

                {selectedUpdate.status === 'checking_dependencies' && (
                  <div className="w-full btn btn-secondary opacity-75 cursor-not-allowed">
                    <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    Checking Dependencies...
                  </div>
                )}

                {selectedUpdate.status === 'pending_dependencies' && (
                  <button
                    onClick={() => {
                      const deps = selectedUpdate.metadata?.dependencies || [];
                      setPendingDependencies(Array.isArray(deps) ? deps : []);
                      setDependencyUpdateId(selectedUpdate.id);
                      setShowDependencyModal(true);
                    }}
                    className="w-full btn btn-warning"
                  >
                    <AlertTriangle className="h-4 w-4 mr-2" />
                    Review Dependencies
                  </button>
                )}

                {/* 'installed', not 'completed' — the package state machine has no
                    'completed' status, so logs were unreachable after a successful
                    install (UI-SCOPE sweep 2026-07-02). */}
                {['installing', 'installed', 'failed'].includes(selectedUpdate.status) && (
                  <button
                    onClick={() => handleViewLogs(selectedUpdate.id)}
                    disabled={logsLoading}
                    className="w-full btn btn-ghost"
                  >
                    <FileText className="h-4 w-4 mr-2" />
                    {logsLoading ? 'Loading...' : 'View Log'}
                  </button>
                )}

                {selectedUpdate.status === 'failed' && (
                  <>
                    {selectedUpdate.metadata?.failure_reason && (
                      <div className="w-full text-xs text-red-700 bg-red-50 border border-red-200 rounded p-2 mb-2">
                        {selectedUpdate.metadata.failure_reason}
                      </div>
                    )}
                    <button
                      onClick={() => handleReopenUpdate(selectedUpdate.id)}
                      disabled={reopenMutation.isPending}
                      className="w-full btn btn-warning"
                    >
                      <RotateCcw className="h-4 w-4 mr-2" />
                      {reopenMutation.isPending ? 'Re-opening...' : 'Reopen for Retry'}
                    </button>
                    <button
                      onClick={() => handleResolveUpdate(selectedUpdate.id)}
                      disabled={resolveMutation.isPending}
                      className="w-full btn btn-ghost text-xs"
                    >
                      <CheckCircle className="h-4 w-4 mr-2" />
                      {resolveMutation.isPending ? 'Resolving...' : 'Mark Resolved'}
                    </button>
                  </>
                )}
              </div>
            </div>

            {/* Agent context */}
            <div className="card">
              <h2 className="text-sm font-medium text-gray-700 mb-3">Agent</h2>
              <button
                onClick={() => navigate(`/agents/${selectedUpdate.agent_id}`)}
                className="w-full text-left bg-gray-50 hover:bg-gray-100 border border-gray-200 rounded p-3 transition-colors group"
              >
                <div className="flex items-center gap-2 mb-1">
                  <Computer className="h-4 w-4 text-gray-500" />
                  <span className="text-sm font-medium text-gray-900 truncate">
                    {agentLabel}
                  </span>
                  <ChevronRight className="h-4 w-4 text-gray-400 ml-auto group-hover:text-gray-700 transition-colors" />
                </div>
                <p className="text-xs text-gray-500">
                  Open agent detail
                </p>
              </button>
            </div>
          </div>
        </div>

        {/* Dependency Confirmation Modal — Modal primitive */}
        <Modal open={showDependencyModal} onClose={handleCancelDependencies} title="Dependencies Required" maxWidth="2xl">
          <Modal.Body>
            <div className="space-y-4">
              <div className="flex items-start space-x-3">
                <div className="flex-shrink-0">
                  <AlertTriangle className="h-6 w-6 text-amber-500" />
                </div>
                <div className="flex-1">
                  <h4 className="text-base font-medium text-gray-900">
                    Additional packages are required
                  </h4>
                  <p className="mt-1 text-sm text-gray-600">
                    To install <span className="font-medium text-gray-900">{selectedUpdate?.package_name}</span>, the following additional packages will also be installed:
                  </p>
                </div>
              </div>

              {pendingDependencies.length > 0 && (
                <div className="bg-gray-50 rounded-lg p-4">
                  <h5 className="text-sm font-medium text-gray-700 mb-3">Required Dependencies:</h5>
                  <ul className="space-y-2">
                    {pendingDependencies.map((dep, index) => (
                      <li key={index} className="flex items-center space-x-2 text-sm">
                        <Package className="h-4 w-4 text-gray-400" />
                        <span className="font-medium text-gray-700">{dep}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              <div className="alert alert-warning rounded-md p-3">
                <div className="flex">
                  <AlertTriangle className="h-4 w-4 text-amber-500 mr-2 flex-shrink-0" />
                  <div className="text-sm text-amber-800">
                    <p className="font-medium">Please review the dependencies before proceeding.</p>
                    <p className="mt-1">These additional packages will be installed alongside your requested package.</p>
                  </div>
                </div>
              </div>
            </div>
          </Modal.Body>

          <Modal.Footer>
            <button
              type="button"
              className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-primary-600 text-base font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm"
              onClick={() => handleConfirmDependencies(dependencyUpdateId!)}
              disabled={dependencyLoading}
            >
              {dependencyLoading ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                  Approving & Installing...
                </>
              ) : (
                <>
                  <CheckCircle className="h-4 w-4 mr-2" />
                  Approve & Install All
                </>
              )}
            </button>
            <button
              type="button"
              className="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
              onClick={handleCancelDependencies}
              disabled={dependencyLoading}
            >
              Cancel
            </button>
          </Modal.Footer>
        </Modal>

        {/* Log Modal — Modal primitive */}
        <Modal open={showLogModal} onClose={() => setShowLogModal(false)} title={`Installation Logs — ${selectedUpdate?.package_name}`} maxWidth="4xl">
          <Modal.Body className="p-0">
            <div className="bg-gray-900 text-green-400 p-4 max-h-96 overflow-y-auto terminal">
              {logsLoading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-5 w-5 animate-spin text-green-400 mr-2" />
                  <span className="text-green-400">Loading logs...</span>
                </div>
              ) : logs.length === 0 ? (
                <div className="text-gray-500 text-center py-8">
                  No installation logs available for this update.
                </div>
              ) : (
                <div className="space-y-3">
                  {logs.map((log, index) => (
                    <div key={index} className="border-b border-gray-700 pb-3 last:border-b-0">
                      <div className="flex items-center space-x-3 mb-2 text-xs">
                        <span className="text-gray-500">
                          {new Date(log.executedAt).toLocaleString()}
                        </span>
                        <span className={cn(
                          "px-2 py-1 rounded font-medium",
                          log.action === 'install' ? "bg-blue-900/50 text-blue-300" :
                          log.action === 'configure' ? "bg-yellow-900/50 text-yellow-300" :
                          log.action === 'cleanup' ? "bg-gray-700 text-gray-300" :
                          "bg-gray-700 text-gray-300"
                        )}>
                          {log.action?.toUpperCase() || 'UNKNOWN'}
                        </span>
                        {log.exit_code !== undefined && (
                          <span className={cn(
                            "px-2 py-1 rounded font-medium",
                            log.exit_code === 0 ? "bg-green-900/50 text-green-300" : "bg-red-900/50 text-red-300"
                          )}>
                            Exit: {log.exit_code}
                          </span>
                        )}
                        {log.duration_seconds && (
                          <span className="text-gray-500">{log.duration_seconds}s</span>
                        )}
                      </div>
                      {log.stdout && <div className="text-sm text-gray-300 whitespace-pre-wrap mb-2 font-mono">{log.stdout}</div>}
                      {log.stderr && <div className="text-sm text-red-400 whitespace-pre-wrap font-mono">{log.stderr}</div>}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </Modal.Body>

          <Modal.Footer>
            <button
              type="button"
              className="w-full inline-flex justify-center rounded-md border border-transparent shadow-sm px-4 py-2 bg-primary-600 text-base font-medium text-white hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm"
              onClick={() => setShowLogModal(false)}
            >
              Close
            </button>
            <button
              type="button"
              className="mt-3 w-full inline-flex justify-center rounded-md border border-gray-300 shadow-sm px-4 py-2 bg-white text-base font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:mt-0 sm:ml-3 sm:w-auto sm:text-sm"
              onClick={() => {
                navigator.clipboard.writeText(logs.map(log =>
                  `${log.action?.toUpperCase() || 'UNKNOWN'} - ${new Date(log.executedAt).toLocaleString()}\n${log.stdout || ''}\n${log.stderr || ''}`
                ).join('\n\n'));
              }}
            >
              Copy Logs
            </button>
          </Modal.Footer>
        </Modal>
      </div>
    );
  }

  // Pagination handlers
  const handlePageChange = (page: number) => {
    setCurrentPage(page);
  };

  const handlePageSizeChange = (newPageSize: number) => {
    setPageSize(newPageSize);
    setCurrentPage(1); // Reset to first page when changing page size
  };

  // Commands view
  if (activeTab === 'commands') {
    const commands = recentCommandsData?.commands || [];

    return (
      <div className="px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="mb-6">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Command History</h1>
              <p className="mt-1 text-sm text-gray-600">
                Review and retry failed or cancelled commands
              </p>
            </div>
            <button
              onClick={() => selectActiveTab('updates')}
              className="btn btn-ghost"
            >
              ← Back to Updates
            </button>
          </div>
        </div>

        {/* Commands list */}
        {commands.length === 0 ? (
          <div className="text-center py-12">
            <Package className="mx-auto h-12 w-12 text-gray-400" />
            <h3 className="mt-2 text-sm font-medium text-gray-900">No commands found</h3>
            <p className="mt-1 text-sm text-gray-500">
              No command history available yet.
            </p>
          </div>
        ) : (
          <div className="card overflow-hidden">
            <div className="overflow-x-auto">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="table-header">Command</th>
                    <th className="table-header">Package</th>
                    <th className="table-header">Agent</th>
                    <th className="table-header">Status</th>
                    <th className="table-header">Created</th>
                    <th className="table-header">Actions</th>
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {commands.map((command: any) => (
                    <tr key={command.id} className="hover:bg-gray-50">
                      <td className="table-cell">
                        <div className="text-sm font-medium text-gray-900">
                          {command.command_type.replace('_', ' ')}
                        </div>
                      </td>
                      <td className="table-cell">
                        {command.package_name && command.package_type ? (
                          <Link
                            to={`/updates/package/${command.package_type}/${command.package_name}`}
                            className="text-sm text-gray-900 hover:text-primary-600 hover:underline"
                          >
                            {command.package_name}
                          </Link>
                        ) : (
                          <div className="text-sm text-gray-900">
                            {command.package_name || '—'}
                          </div>
                        )}
                      </td>
                      <td className="table-cell">
                        {command.agent_id ? (
                          <Link
                            to={`/agents/${command.agent_id}`}
                            className="text-sm text-gray-900 hover:text-primary-600 hover:underline"
                          >
                            {command.agent_hostname}
                          </Link>
                        ) : (
                          <div className="text-sm text-gray-900">
                            {command.agent_hostname}
                          </div>
                        )}
                      </td>
                      <td className="table-cell">
                        <CommandStatusBadge status={command.status} />
                      </td>
                      <td className="table-cell">
                        <div className="text-sm text-gray-900">
                          {formatRelativeTime(command.created_at)}
                        </div>
                      </td>
                      <td className="table-cell">
                        <div className="flex items-center space-x-2">
                          {(command.status === 'failed' || command.status === 'cancelled' || command.status === 'timed_out') && (
                            <button
                              onClick={() => handleRetryCommand(command.id)}
                              disabled={retryMutation.isPending}
                              className="text-amber-600 hover:text-amber-800"
                              title="Retry command"
                            >
                              <RotateCcw className="h-4 w-4" />
                            </button>
                          )}

                          {(command.status === 'pending' || command.status === 'sent') && (
                            <button
                              onClick={() => handleCancelCommand(command.id)}
                              disabled={cancelMutation.isPending}
                              className="text-red-600 hover:text-red-800"
                              title="Cancel command"
                            >
                              <X className="h-4 w-4" />
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    );
  }

  // Updates list view
  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold text-gray-900">Updates</h1>
            <p className="mt-1 text-sm text-gray-600">
              Review and approve available updates for your agents
            </p>
          </div>
          <div className="text-right">
            <div className="text-sm text-gray-600">
              Showing {displayedPackages.length} of {packageTotal} packages
            </div>
            {packageTotal > 100 && (
              <select
                value={pageSize}
                onChange={(e) => handlePageSizeChange(Number(e.target.value))}
                className="mt-1 text-sm border border-gray-300 rounded px-3 py-1"
              >
                <option value={50}>50 per page</option>
                <option value={100}>100 per page</option>
                <option value={200}>200 per page</option>
                <option value={500}>500 per page</option>
              </select>
            )}
          </div>
        </div>

        {/* Statistics Cards — StatCard primitives */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          <StatCard title="Total Updates" value={totalStats.total} icon={Package} />
          <StatCardGroup
            left={{ title: 'Approved', value: totalStats.approved, icon: CheckCircle, color: 'text-green-600' }}
            right={{ title: 'Pending', value: totalStats.pending, icon: Clock, color: 'text-orange-600' }}
          />
          <StatCardGroup
            left={{ title: 'Critical', value: totalStats.critical, icon: AlertTriangle, color: 'text-red-600' }}
            right={{ title: 'High Priority', value: totalStats.high, icon: AlertTriangle, color: 'text-yellow-600' }}
          />
        </div>

        {/* Quick Filters */}
        <div className="flex flex-wrap gap-2 mb-4">
          <button
            onClick={() => handleQuickFilter('all')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              !filter.values.status && !filter.values.severity && !filter.values.type && !filter.values.agent && !filter.values.vuln
                ? "bg-primary-100 border-primary-300 text-primary-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            All Updates
          </button>
          <button
            onClick={() => handleQuickFilter('critical')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'pending' && filter.values.severity === 'critical'
                ? "bg-red-100 border-red-300 text-red-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <AlertTriangle className="h-4 w-4 mr-1 inline" />
            Critical
          </button>
          <button
            onClick={() => handleQuickFilter('pending')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'pending' && !filter.values.severity
                ? "bg-orange-100 border-orange-300 text-orange-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <Clock className="h-4 w-4 mr-1 inline" />
            Pending Approval
          </button>
          <button
            onClick={() => handleQuickFilter('approved')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'approved' && !filter.values.severity
                ? "bg-green-100 border-green-300 text-green-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <CheckCircle className="h-4 w-4 mr-1 inline" />
            Approved
          </button>
          <button
            onClick={() => handleQuickFilter('installing')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'installing'
                ? "bg-blue-100 border-blue-300 text-blue-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <Loader2 className="h-4 w-4 mr-1 inline" />
            Installing
          </button>
          <button
            onClick={() => handleQuickFilter('installed')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'installed'
                ? "bg-emerald-100 border-emerald-300 text-emerald-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <CheckCircle className="h-4 w-4 mr-1 inline" />
            Installed
          </button>
          <button
            onClick={() => handleQuickFilter('failed')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'failed'
                ? "bg-red-100 border-red-300 text-red-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <XCircle className="h-4 w-4 mr-1 inline" />
            Failed
          </button>
          <button
            onClick={() => handleQuickFilter('dependencies')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.status === 'pending_dependencies'
                ? "bg-amber-100 border-amber-300 text-amber-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <AlertTriangle className="h-4 w-4 mr-1 inline" />
            Dependencies
          </button>
          <button
            onClick={() => handleQuickFilter('vuln')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              filter.values.vuln
                ? "bg-orange-100 border-orange-300 text-orange-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            <ShieldAlert className="h-4 w-4 mr-1 inline" />
            Vulnerable
          </button>
        </div>
      </div>

      <FilterBar
        search={{ value: searchQuery, onChange: setSearchQuery, placeholder: 'Search updates by package name...' }}
        filters={[
          { label: 'Status', value: filter.values.status, onChange: (v) => filter.setFilter('status', v), options: statuses.map((s: string) => ({ value: s, label: s.replace(/_/g, ' ') })), placeholder: 'All Status' },
          { label: 'Severity', value: filter.values.severity, onChange: (v) => filter.setFilter('severity', v), options: severities.map((s: string) => ({ value: s, label: s })), placeholder: 'All Severities' },
          { label: 'Type', value: filter.values.type, onChange: (v) => filter.setFilter('type', v), options: packageTypes.map((t: string) => ({ value: t, label: t.toUpperCase() })), placeholder: 'All Types' },
        ]}
        pills={buildFilterPills(filter, filterConfig)}
        onClearAll={() => filter.clearAll()}
        activeCount={filter.activeCount}
        actions={
          <>
            {selectedUpdates.length > 0 && (
              <button
                onClick={handleBulkApprove}
                disabled={bulkApproveMutation.isPending}
                className="btn btn-success"
              >
                <CheckCircle className="h-4 w-4 mr-2" />
                Approve Selected ({selectedUpdates.length})
              </button>
            )}
            <button
              onClick={() => selectActiveTab('commands')}
              className="btn btn-ghost"
            >
              <RotateCcw className="h-4 w-4 mr-2" />
              Command History
            </button>
          </>
        }
        className="mb-6"
      />

      {/* Package list — PageState primitive for loading/error/empty */}
      <PageState
        loading={packagesPending}
        error={packagesError ? 'Failed to load updates' : null}
        empty={displayedPackages.length === 0}
        emptyTitle="No updates found"
        emptyMessage={
          debouncedSearchQuery || filter.values.status || filter.values.severity || filter.values.type || filter.values.agent || filter.values.vuln
            ? 'Try adjusting your search or filters.'
            : 'All agents are up to date!'
        }
        errorAction={() => queryClient.invalidateQueries({ queryKey: ['packages'] })}
      >
        <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('package_name')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Package
                      {renderSortIcon('package_name')}
                    </button>
                  </th>
                  <th className="table-header">Type</th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('agent_count')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Agents
                      {renderSortIcon('agent_count')}
                    </button>
                  </th>
                  <th className="table-header">Versions</th>
                  <th className="table-header">
                    <button
                      onClick={() => handleSort('severity')}
                      className="flex items-center hover:text-primary-600 font-medium"
                    >
                      Severity
                      {renderSortIcon('severity')}
                    </button>
                  </th>
                  <th className="table-header">Supply chain</th>
                  <th className="table-header">Status</th>
                  <th className="table-header">Actions</th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {displayedPackages.map((pkg) => {
                  const versionLabel = pkg.version_count > 1
                    ? `${pkg.version_count} versions`
                    : (pkg.sample_current_version && pkg.sample_current_version !== pkg.sample_available_version
                        ? `${pkg.sample_current_version} → ${pkg.sample_available_version}`
                        : pkg.sample_available_version || '—');
                  // Dominant status for the rollup chip, in order of operator urgency.
                  const rollup =
                    pkg.failed_count > 0 ? { label: `${pkg.failed_count} failed`, cls: 'bg-red-50 text-red-700 border-red-200' } :
                    pkg.pending_dependencies_count > 0 ? { label: `${pkg.pending_dependencies_count} deps`, cls: 'bg-amber-50 text-amber-700 border-amber-200' } :
                    pkg.pending_count > 0 ? { label: `${pkg.pending_count} pending`, cls: 'bg-orange-50 text-orange-700 border-orange-200' } :
                    pkg.installing_count > 0 ? { label: `${pkg.installing_count} installing`, cls: 'bg-blue-50 text-blue-700 border-blue-200' } :
                    pkg.approved_count > 0 ? { label: `${pkg.approved_count} approved`, cls: 'bg-green-50 text-green-700 border-green-200' } :
                    pkg.installed_count > 0 ? { label: 'up to date', cls: 'bg-emerald-50 text-emerald-700 border-emerald-200' } :
                    { label: '—', cls: 'bg-gray-50 text-gray-500 border-gray-200' };
                  const RowTypeIcon = getPackageTypeIcon(pkg.package_type);
                  return (
                    <tr key={`${pkg.package_type}/${pkg.package_name}`} className="hover:bg-gray-50">
                      <td className="table-cell">
                        <div className="flex items-center gap-2 min-w-0">
                          <RowTypeIcon className="h-4 w-4 text-gray-400 flex-shrink-0" />
                          <button
                            onClick={() => navigate(`/updates/package/${pkg.package_type}/${pkg.package_name}`)}
                            className="text-sm font-medium text-gray-900 hover:text-primary-600 truncate block max-w-[16rem]"
                            title={pkg.package_name}
                          >
                            {pkg.package_name}
                          </button>
                        </div>
                      </td>
                      <td className="table-cell">
                        <span className="text-xs font-medium text-gray-900 bg-gray-100 px-2 py-1 rounded">
                          {pkg.package_type.toUpperCase()}
                        </span>
                      </td>
                      <td className="table-cell">
                        <span className="inline-flex items-center gap-1 text-sm text-gray-900">
                          <Computer className="h-3.5 w-3.5 text-gray-400" />
                          {pkg.agent_count}
                        </span>
                      </td>
                      <td className="table-cell">
                        <span className="text-sm font-mono text-gray-900">{versionLabel}</span>
                      </td>
                      <td className="table-cell">
                        <SeverityBadge severity={pkg.max_severity} />
                        {pkg.has_vulns && (
                          <span
                            title="Known vulnerabilities detected"
                            className={cn(
                              'badge ml-1 inline-flex items-center gap-1 border text-[10px] font-bold',
                              pkg.max_severity === 'critical' || pkg.max_severity === 'high'
                                ? 'bg-red-100 text-red-800 border-red-300'
                                : 'bg-amber-100 text-amber-800 border-amber-300'
                            )}
                          >
                            <AlertTriangle className="w-3 h-3" />
                            {pkg.vuln_count > 0
                              ? `${pkg.vuln_count} CVE${pkg.vuln_count === 1 ? '' : 's'}`
                              : 'CVE'}
                          </span>
                        )}
                      </td>
                      <td className="table-cell">
                        {pkg.hash_pinned_count > 0 ? (
                          <span
                            title={`${pkg.hash_pinned_count} of ${pkg.agent_count} agents have a pinned artifact hash`}
                            className="inline-flex items-center gap-1 text-xs font-medium text-green-700"
                          >
                            <Shield className="h-3.5 w-3.5" />
                            {pkg.hash_pinned_count}/{pkg.agent_count}
                          </span>
                        ) : (
                          <span className="text-xs text-gray-400">—</span>
                        )}
                      </td>
                      <td className="table-cell">
                        <span className={cn('text-xs font-medium border rounded px-2 py-0.5', rollup.cls)}>
                          {rollup.label}
                        </span>
                      </td>
                      <td className="table-cell">
                        <button
                          onClick={() => navigate(`/updates/package/${pkg.package_type}/${pkg.package_name}`)}
                          className="text-gray-400 hover:text-primary-600"
                          title="View package detail"
                        >
                          <ExternalLink className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {packageTotalPages > 1 && (
            <Pagination
              page={currentPage}
              total={packageTotal}
              pageSize={pageSize}
              onChange={handlePageChange}
            />
          )}
        </div>
      </PageState>
    </div>
  );
};

export default Updates;

