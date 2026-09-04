import React, { useMemo } from 'react';
import { Package, AlertTriangle, Container, Layers } from 'lucide-react';
import { useDockerContainers, useDockerStats, useFleetRuntimeContainers, useFleetRuntimeStacks } from '@/hooks/useDocker';
import type { DockerContainer, DockerImage, RuntimeDockerContainer, RuntimeDockerStack } from '@/types';
import { formatRelativeTime, cn } from '@/lib/utils';
import { useMultimodalFilter } from '@/hooks/useMultimodalFilter';
import { useColumnSort } from '@/hooks/useColumnSort';
import { SearchInput, FilterPill, FilterCountButton, PageState, SortableTable, StatusBadge, SeverityBadge } from '@/components/primitives';
import type { Column } from '@/components/primitives';

// ─── Filter config ────────────────────────────────────────────────────────────

const FILTER_CONFIG = {
  status:   { urlParam: 'status' },
  severity: { urlParam: 'severity' },
  agent:    { urlParam: 'agent' },
  stack:    { urlParam: 'stack' },
};

// ─── Colour helpers ───────────────────────────────────────────────────────────

const getContainerStateColor = (state: string) => {
  switch (state.toLowerCase()) {
    case 'running': return 'bg-green-50 text-green-700 border-green-200';
    case 'paused':  return 'bg-yellow-50 text-yellow-700 border-yellow-200';
    case 'exited':
    case 'stopped': return 'bg-red-50 text-red-700 border-red-200';
    default:        return 'bg-gray-50 text-gray-600 border-gray-200';
  }
};

const getHealthColor = (health: string) => {
  switch (health.toLowerCase()) {
    case 'healthy':   return 'bg-green-50 text-green-700 border-green-200';
    case 'unhealthy': return 'bg-red-50 text-red-700 border-red-200';
    case 'starting':  return 'bg-blue-50 text-blue-700 border-blue-200';
    default:          return 'bg-gray-50 text-gray-500 border-gray-200';
  }
};

const formatPorts = (ports: any[]): string => {
  if (!ports || ports.length === 0) return '—';
  return ports.map(p => {
    const host = p.host_ip === '0.0.0.0' ? '*' : (p.host_ip || '');
    const port = host ? `${host}:${p.container_port}` : `${p.container_port}`;
    return `${port}/${p.protocol}`;
  }).join(', ');
};

// ─── Unified row type ─────────────────────────────────────────────────────────

interface UnifiedRow {
  key: string;
  name: string;
  image: string;
  state: string;
  health: string | null;
  currentVersion?: string;
  availableVersion?: string | null;
  severity?: string;
  status?: string;
  stackName?: string;
  ports: string;
  agentName: string;
  seenAt?: string;
}

// ─── Component ────────────────────────────────────────────────────────────────

const Docker: React.FC = () => {
  const filter = useMultimodalFilter(FILTER_CONFIG);
  const { values, rawQuery, setRawQuery, setFilter, clearAll, activeCount, searchText, pills } = filter;

  const { data: dockerData, isPending, error } = useDockerContainers({
    search:   searchText || undefined,
    // NOTE: `status` is filtered client-side below (see filteredUnified). The server
    // reused the `status` query param as a severity filter, so sending update-state
    // values here matched nothing.
    page:     1,
    page_size: 50,
  });

  useDockerStats();
  const { data: fleetContainersData } = useFleetRuntimeContainers();
  const { data: fleetStacksData }     = useFleetRuntimeStacks();

  const runtimeContainers: RuntimeDockerContainer[] = fleetContainersData?.containers || [];
  const runtimeStacks: RuntimeDockerStack[]         = fleetStacksData?.stacks || [];

  const allContainers: DockerContainer[] = dockerData?.containers || [];
  const images: DockerImage[]            = dockerData?.images     || [];
  const totalCount                       = dockerData?.total_images || 0;

  // Client-side filter scan containers by agent (server handles search + status)
  const containers = allContainers.filter(c => {
    if (values.agent && (c.agent_hostname || c.agent_name || '').toLowerCase() !== values.agent.toLowerCase()) return false;
    return true;
  });

  // Build scan-by-image lookup
  const scanByImage = useMemo(() => {
    const map = new Map<string, DockerContainer>();
    for (const c of containers) {
      map.set(c.image, c);
    }
    return map;
  }, [containers]);

  // Build unified rows from runtime + scan data
  const unifiedRows: UnifiedRow[] = useMemo(() => {
    const rows: UnifiedRow[] = [];
    const seenImages = new Set<string>();

    // Runtime containers first (they have live state data)
    for (const r of runtimeContainers) {
      const scan = scanByImage.get(r.image);
      seenImages.add(r.image);
      rows.push({
        key: r.id,
        name: r.name.replace(/^\//, ''),
        image: r.image,
        state: r.state,
        health: r.health || null,
        currentVersion: scan?.current_version,
        availableVersion: scan?.available_version ?? null,
        severity: scan?.severity,
        status: (scan as any)?.status,
        stackName: r.stack_name || undefined,
        ports: r.ports,
        agentName: scan?.agent_name || scan?.agent_hostname || `Agent ${r.agent_id.substring(0, 8)}`,
        seenAt: r.last_seen_at,
      });
    }

    // Scan-only containers (no runtime match) — state/health = '—'
    for (const c of containers) {
      if (!seenImages.has(c.image)) {
        rows.push({
          key: c.id,
          name: `${c.image}:${c.tag}`,
          image: c.image,
          state: '—',
          health: null,
          currentVersion: c.current_version,
          availableVersion: c.available_version ?? null,
          severity: c.severity,
          status: (c as any).status,
          ports: formatPorts(c.ports),
          agentName: c.agent_name || c.agent_hostname || `Agent ${c.agent_id.substring(0, 8)}`,
        });
      }
    }

    return rows;
  }, [runtimeContainers, containers, scanByImage]);

  // Client-side filtering of unified rows (text search + severity + stack + agent)
  const filteredUnified = useMemo(() => {
    let rows = unifiedRows;

    if (searchText) {
      const q = searchText.toLowerCase();
      rows = rows.filter(r =>
        r.name.toLowerCase().includes(q) ||
        r.image.toLowerCase().includes(q) ||
        r.agentName.toLowerCase().includes(q) ||
        (r.stackName && r.stackName.toLowerCase().includes(q))
      );
    }

    if (values.status === 'update-available') {
      rows = rows.filter(r => r.availableVersion != null);
    } else if (values.status === 'pending-approval') {
      rows = rows.filter(r => r.status === 'update-available');
    }

    if (values.severity) {
      rows = rows.filter(r => r.severity === values.severity);
    }

    if (values.stack) {
      rows = rows.filter(r => r.stackName === values.stack);
    }

    if (values.agent) {
      rows = rows.filter(r => r.agentName.toLowerCase() === values.agent.toLowerCase());
    }

    return rows;
  }, [unifiedRows, searchText, values]);

  // Sort state for unified table
  const { sortBy, sortOrder, handleSort: unifiedHandleSort, applySort } = useColumnSort({
    defaultSortBy: 'name',
    defaultOrder: 'asc',
  });

  // Sort unified rows client-side
  const sortedUnified = applySort(filteredUnified, (r) => {
    switch (sortBy) {
      case 'name':         return r.name;
      case 'image':        return r.image;
      case 'state':        return r.state;
      case 'health':       return r.health || '';
      case 'severity':     return r.severity || '';
      case 'stack_name':   return r.stackName || '';
      case 'ports':        return r.ports || '';
      case 'agent_name':   return r.agentName;
      case 'last_seen_at': return r.seenAt ? new Date(r.seenAt) : null;
      default:             return null;
    }
  });

  // Unified column definitions
  const unifiedColumns: Column<UnifiedRow>[] = useMemo(() => [
    {
      key: 'name', label: 'Name', sortKey: 'name',
      render: (r) => (
        <div className="flex items-center">
          <Container className="w-5 h-5 mr-3 text-blue-500 shrink-0" />
          <span className="font-mono text-sm font-medium text-gray-900">{r.name}</span>
        </div>
      ),
    },
    {
      key: 'image', label: 'Image', sortKey: 'image',
      render: (r) => <span className="text-gray-600 max-w-[14rem] truncate block text-xs" title={r.image}>{r.image}</span>,
    },
    {
      key: 'state', label: 'State', sortKey: 'state',
      render: (r) => (
        r.state !== '—'
          ? <span className={cn('text-[10px] border rounded px-1.5 py-0.5', getContainerStateColor(r.state))}>{r.state}</span>
          : <span className="text-gray-300 text-sm">—</span>
      ),
    },
    {
      key: 'health', label: 'Health', sortKey: 'health',
      render: (r) => (
        r.health
          ? <span className={cn('text-[10px] border rounded px-1.5 py-0.5', getHealthColor(r.health))}>{r.health}</span>
          : <span className="text-gray-300">—</span>
      ),
    },
    {
      key: 'version', label: 'Version', sortable: false,
      render: (r) => (
        r.currentVersion
          ? <div className="text-xs">
              {r.availableVersion ? (
                <>
                  <div className="text-gray-900 font-mono">{r.currentVersion}</div>
                  <div className="flex items-center gap-1.5 mt-0.5">
                    <span className="text-green-600 font-medium font-mono">→ {r.availableVersion}</span>
                    {r.status && <StatusBadge status={r.status} />}
                  </div>
                </>
              ) : (
                <div className="flex items-center gap-1.5">
                  <span className="text-gray-700 font-mono">{r.currentVersion}</span>
                  {r.status && <StatusBadge status={r.status} />}
                </div>
              )}
            </div>
          : <span className="text-gray-300 text-sm">—</span>
      ),
    },
    {
      key: 'severity', label: 'Risk', sortKey: 'severity',
      render: (r) => (
        r.availableVersion && r.severity
          ? <SeverityBadge severity={r.severity} />
          : r.availableVersion
            ? <span className="badge bg-gray-100 text-gray-600">unknown</span>
            : <span className="text-gray-300 text-sm">—</span>
      ),
    },
    {
      key: 'stack', label: 'Stack', sortKey: 'stack_name',
      render: (r) => (
        r.stackName
          ? <button onClick={() => setRawQuery(`stack:${r.stackName}`)} className="hover:underline text-primary-600 text-xs">{r.stackName}</button>
          : <span className="text-gray-400 text-xs">—</span>
      ),
    },
    {
      key: 'ports', label: 'Ports', sortKey: 'ports',
      render: (r) => <span className="font-mono text-gray-400 text-xs">{r.ports || '—'}</span>,
    },
    {
      key: 'agent', label: 'Agent', sortKey: 'agent_name',
      render: (r) => <span className="text-sm text-gray-600">{r.agentName}</span>,
    },
    {
      key: 'seen', label: 'Seen', sortKey: 'last_seen_at',
      render: (r) => (
        r.seenAt
          ? <span className="text-gray-400 text-xs">{formatRelativeTime(r.seenAt)}</span>
          : <span className="text-gray-300 text-xs">—</span>
      ),
    },
  ] as Column<UnifiedRow>[], [setRawQuery]);

  // ─── Stat counts ─────────────────────────────────────────────────────────

  const updatesAvailable = images.filter((i: DockerImage) => i.update_available).length;
  const pendingApproval  = images.filter((i: DockerImage) => i.status === 'update-available').length;
  const criticalUpdates  = images.filter((i: DockerImage) => i.severity === 'critical').length;

  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 flex items-center">
            <Container className="w-8 h-8 mr-3 text-blue-600" />
            Docker Containers
          </h1>
          <p className="mt-1 text-sm text-gray-600">
            Manage container image updates across all agents
          </p>
        </div>
        <div className="text-sm text-gray-500">{totalCount} container images found</div>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white p-4 rounded-lg border border-gray-200 shadow-sm">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-600">Total Images</p>
              <p className="text-2xl font-bold text-gray-900">{totalCount}</p>
            </div>
            <Package className="h-8 w-8 text-gray-400" />
          </div>
        </div>
        <FilterCountButton
          count={updatesAvailable}
          label="Updates Available"
          isSelected={values.status === 'update-available'}
          onClick={() => setFilter('status', values.status === 'update-available' ? '' : 'update-available')}
          color="blue"
          icon={<Container className="h-8 w-8 text-blue-400" />}
        />
        <FilterCountButton
          count={pendingApproval}
          label="Pending Approval"
          isSelected={values.status === 'pending-approval'}
          onClick={() => setFilter('status', values.status === 'pending-approval' ? '' : 'pending-approval')}
          color="orange"
          icon={<AlertTriangle className="h-8 w-8 text-orange-400" />}
        />
        <FilterCountButton
          count={criticalUpdates}
          label="Critical Updates"
          isSelected={values.severity === 'critical'}
          onClick={() => setFilter('severity', values.severity === 'critical' ? '' : 'critical')}
          color="red"
          icon={<AlertTriangle className="h-8 w-8 text-red-400" />}
        />
      </div>

      {/* Search + pills */}
      <div className="mb-6">
        <SearchInput
          value={rawQuery}
          onChange={setRawQuery}
          placeholder="Search images, or filter with agent:name severity:critical status:update-available..."
        />
        {pills.length > 0 && (
          <div className="flex flex-wrap items-center gap-1.5 mt-2">
            {pills.map((p) => (
              <FilterPill key={p.key} label={p.key} value={p.value} onClear={p.onClear} />
            ))}
            <button
              onClick={clearAll}
              className="text-xs text-gray-400 hover:text-gray-700 transition-colors ml-1 underline"
            >
              clear all
            </button>
          </div>
        )}
        <p className="mt-1.5 text-xs text-gray-400">
          Tip: type <code className="font-mono bg-gray-100 px-1 rounded">severity:critical</code>,{' '}
          <code className="font-mono bg-gray-100 px-1 rounded">status:update-available</code>,{' '}
          <code className="font-mono bg-gray-100 px-1 rounded">agent:hostname</code>, or{' '}
          <code className="font-mono bg-gray-100 px-1 rounded">stack:name</code> to filter
        </p>
      </div>

      {/* Unified table — PageState wrapper */}
      <PageState
        loading={isPending}
        error={error ? 'Failed to load container updates' : null}
        empty={sortedUnified.length === 0}
        emptyTitle="No container images found"
        emptyMessage={
          activeCount > 0 || rawQuery
            ? 'Try adjusting your search or clearing filters.'
            : 'No Docker containers or images found on any agents.'
        }
      >
        <SortableTable
          columns={unifiedColumns}
          data={sortedUnified}
          getKey={(r) => r.key}
          sortBy={sortBy}
          sortOrder={sortOrder}
          onSort={unifiedHandleSort}
          emptyMessage="No containers match the current filters."
        />
      </PageState>

      {/* Compose Stacks */}
      {runtimeStacks.length > 0 && (
        <div className="mt-6 mb-8">
          <h2 className="text-sm font-medium text-gray-700 inline-flex items-center gap-2 mb-3">
            <Layers className="h-4 w-4 text-gray-500" />
            Compose Stacks
            <span className="text-xs text-gray-400 font-normal">({runtimeStacks.length})</span>
          </h2>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
            {runtimeStacks.map((s) => (
              <button
                key={s.id}
                onClick={() => setRawQuery(`stack:${s.name}`)}
                className="card text-left hover:shadow-md transition-shadow"
              >
                <div className="flex items-center gap-2 mb-2">
                  <Layers className="h-3.5 w-3.5 text-gray-400 shrink-0" />
                  <span className="text-sm font-medium text-gray-900 truncate">{s.name}</span>
                </div>
                <div className="text-xs">
                  <span className={cn(
                    'font-medium',
                    s.running_count === s.container_count ? 'text-green-700' : 'text-amber-600'
                  )}>
                    {s.running_count}/{s.container_count} running
                  </span>
                </div>
                <div className="text-[10px] text-gray-400 mt-1">{formatRelativeTime(s.last_seen_at)}</div>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

export default Docker;
