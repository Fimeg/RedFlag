import React, { useState } from 'react';
import { Search, Filter, RefreshCw, Package, AlertTriangle, Container, CheckCircle, XCircle, Play } from 'lucide-react';
import { useDockerContainers, useDockerStats, useApproveDockerUpdate, useRejectDockerUpdate, useInstallDockerUpdate, useBulkDockerActions } from '@/hooks/useDocker';
import type { DockerContainer, DockerImage } from '@/types';
import { formatRelativeTime, cn } from '@/lib/utils';
import toast from 'react-hot-toast';

const Docker: React.FC = () => {
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [severityFilter, setSeverityFilter] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize] = useState(50);
  const [selectedImages, setSelectedImages] = useState<string[]>([]);

  // Fetch Docker containers and images
  const { data: dockerData, isPending, error } = useDockerContainers({
    search: searchQuery || undefined,
    status: statusFilter || undefined,
    page: currentPage,
    page_size: pageSize,
  });

  const { data: stats } = useDockerStats();

  const containers = dockerData?.containers || [];
  const images = dockerData?.images || [];
  const totalCount = dockerData?.total_images || 0;

  // Mutations
  const approveUpdate = useApproveDockerUpdate();
  const rejectUpdate = useRejectDockerUpdate();
  const installUpdate = useInstallDockerUpdate();
  const { approveMultiple, rejectMultiple } = useBulkDockerActions();

  // Group containers by agent for better organization
  const containersByAgent = containers.reduce((acc, container) => {
    const agentKey = container.agent_id;
    if (!acc[agentKey]) {
      acc[agentKey] = {
        agentId: container.agent_id,
        agentName: container.agent_name || container.agent_hostname || `Agent ${container.agent_id.substring(0, 8)}`,
        containers: []
      };
    }
    acc[agentKey].containers.push(container);
    return acc;
  }, {} as Record<string, { agentId: string; agentName: string; containers: DockerContainer[] }>);

  const agentGroups = Object.values(containersByAgent);

  // Get unique values for filters
  const statuses = [...new Set(images.map((i: DockerImage) => i.status))];
  const severities = [...new Set(images.map((i: DockerImage) => i.severity).filter(Boolean))];
  const agents = [...new Set(images.map((i: DockerImage) => i.agent_id))];

  // Quick filter functions
  const handleQuickFilter = (filter: string) => {
    switch (filter) {
      case 'critical':
        // Filter to show only images with critical severity updates
        setSearchQuery('critical');
        setSeverityFilter('');
        break;
      case 'pending':
        setStatusFilter('update-available');
        setSeverityFilter('');
        break;
      case 'all':
        setStatusFilter('');
        setSeverityFilter('');
        setSearchQuery('');
        break;
      default:
        break;
    }
    setCurrentPage(1);
  };

  // Handle image selection
  const handleSelectImage = (imageId: string, checked: boolean) => {
    if (checked) {
      setSelectedImages([...selectedImages, imageId]);
    } else {
      setSelectedImages(selectedImages.filter(id => id !== imageId));
    }
  };

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedImages(images.map((image: DockerImage) => image.id));
    } else {
      setSelectedImages([]);
    }
  };

  // Handle bulk actions
  const handleBulkApprove = async () => {
    if (selectedImages.length === 0) {
      toast.error('Please select at least one image');
      return;
    }

    const updates = selectedImages.map(imageId => {
      const image = images.find(img => img.id === imageId);
      return {
        containerId: image?.agent_id || '', // Use agent_id as containerId for now
        imageId: imageId,
      };
    });

    try {
      await approveMultiple.mutateAsync({ updates });
      setSelectedImages([]);
    } catch (error) {
      // Error handling is done in the hook
    }
  };

  // Format bytes utility
  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  // Format port information for display
  const formatPorts = (ports: any[]): string => {
    if (!ports || ports.length === 0) return '-';

    return ports.map(port => {
      const hostPort = port.host_port ? `:${port.host_port}` : '';
      return `${port.container_port}${hostPort}/${port.protocol}`;
    }).join(', ');
  };

  // Helper functions for status and severity colors
  const getStatusColor = (status: string): string => {
    switch (status) {
      case 'up-to-date':
        return 'bg-green-100 text-green-800';
      case 'update-available':
        return 'bg-blue-100 text-blue-800';
      case 'update-approved':
        return 'bg-orange-100 text-orange-800';
      case 'update-scheduled':
        return 'bg-purple-100 text-purple-800';
      case 'update-installing':
        return 'bg-yellow-100 text-yellow-800';
      case 'update-failed':
        return 'bg-red-100 text-red-800';
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  const getSeverityColor = (severity: string): string => {
    switch (severity) {
      case 'low':
        return 'bg-gray-100 text-gray-800';
      case 'medium':
        return 'bg-blue-100 text-blue-800';
      case 'high':
        return 'bg-orange-100 text-orange-800';
      case 'critical':
        return 'bg-red-100 text-red-800';
      default:
        return 'bg-gray-100 text-gray-800';
    }
  };

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      {/* Header */}
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
          <div className="text-right">
            <div className="text-sm text-gray-600">
              {totalCount} container images found
            </div>
          </div>
        </div>

        {/* Statistics Cards */}
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

          <div className="bg-white p-4 rounded-lg border border-blue-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Updates Available</p>
                <p className="text-2xl font-bold text-blue-600">{images.filter((i: DockerImage) => i.update_available).length}</p>
              </div>
              <Container className="h-8 w-8 text-blue-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-orange-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Pending Approval</p>
                <p className="text-2xl font-bold text-orange-600">{images.filter((i: DockerImage) => i.status === 'update-available').length}</p>
              </div>
              <AlertTriangle className="h-8 w-8 text-orange-400" />
            </div>
          </div>

          <div className="bg-white p-4 rounded-lg border border-red-200 shadow-sm">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-600">Critical Updates</p>
                <p className="text-2xl font-bold text-red-600">{images.filter((i: DockerImage) => i.severity === 'critical').length}</p>
              </div>
              <AlertTriangle className="h-8 w-8 text-red-400" />
            </div>
          </div>
        </div>

        {/* Quick Filters */}
        <div className="flex flex-wrap gap-2 mb-4">
          <button
            onClick={() => handleQuickFilter('all')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              !statusFilter && !severityFilter && !searchQuery
                ? "bg-blue-100 border-blue-300 text-blue-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            All Images
          </button>
          <button
            onClick={() => handleQuickFilter('pending')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              statusFilter === 'update-available' && !severityFilter && !searchQuery
                ? "bg-orange-100 border-orange-300 text-orange-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            Pending Approval
          </button>
          <button
            onClick={() => handleQuickFilter('critical')}
            className={cn(
              "px-4 py-2 text-sm font-medium rounded-lg border transition-colors",
              searchQuery === 'critical' && !statusFilter && !severityFilter
                ? "bg-red-100 border-red-300 text-red-700"
                : "bg-white border-gray-300 text-gray-700 hover:bg-gray-50"
            )}
          >
            Critical Only
          </button>
        </div>
      </div>

      {/* Search and filters */}
      <div className="mb-6 space-y-4">
        <div className="flex flex-col sm:flex-row gap-4">
          {/* Search */}
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search container images..."
                className="pl-10 pr-4 py-2 w-full border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
              />
            </div>
          </div>

          {/* Filters */}
          <div className="flex gap-2">
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
            >
              <option value="">All Status</option>
              {statuses.map((status: string) => (
                <option key={status} value={status}>{status}</option>
              ))}
            </select>

            <select
              value={severityFilter}
              onChange={(e) => setSeverityFilter(e.target.value)}
              className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
            >
              <option value="">All Severities</option>
              {severities.map((severity: string) => (
                <option key={severity} value={severity}>{severity}</option>
              ))}
            </select>
          </div>
        </div>
      </div>

      {/* Container updates table */}
      {isPending ? (
        <div className="animate-pulse">
          <div className="bg-white rounded-lg border border-gray-200">
            {[...Array(5)].map((_, i) => (
              <div key={i} className="p-4 border-b border-gray-200">
                <div className="h-4 bg-gray-200 rounded w-1/4 mb-2"></div>
                <div className="h-3 bg-gray-200 rounded w-1/2"></div>
              </div>
            ))}
          </div>
        </div>
      ) : error ? (
        <div className="text-center py-12">
          <div className="text-red-500 mb-2">Failed to load container updates</div>
          <p className="text-sm text-gray-600">Please check your connection and try again.</p>
        </div>
      ) : images.length === 0 ? (
        <div className="text-center py-12">
          <Container className="w-16 h-16 mx-auto mb-4 text-gray-400" />
          <h3 className="mt-2 text-sm font-medium text-gray-900">No container images found</h3>
          <p className="mt-1 text-sm text-gray-500">
            {searchQuery || statusFilter || severityFilter
              ? 'Try adjusting your search or filters.'
              : 'No Docker containers or images found on any agents.'}
          </p>
        </div>
      ) : (
        <div className="space-y-6">
          {agentGroups.map((agentGroup) => (
            <div key={agentGroup.agentId} className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
              {/* Agent Header */}
              <div className="bg-gray-50 px-6 py-4 border-b border-gray-200">
                <div className="flex items-center justify-between">
                  <div className="flex items-center">
                    <Container className="w-6 h-6 mr-3 text-blue-600" />
                    <div>
                      <h3 className="text-lg font-medium text-gray-900">{agentGroup.agentName}</h3>
                      <p className="text-sm text-gray-500">
                        {agentGroup.containers.length} container image{agentGroup.containers.length !== 1 ? 's' : ''}
                        {agentGroup.containers.filter(c => c.update_available).length > 0 &&
                          ` • ${agentGroup.containers.filter(c => c.update_available).length} update${agentGroup.containers.filter(c => c.update_available).length !== 1 ? 's' : ''} available`
                        }
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center space-x-2">
                    {agentGroup.containers.filter(c => c.update_available).length > 0 && (
                      <span className="inline-flex px-2 py-1 text-xs font-semibold rounded-full bg-blue-100 text-blue-800">
                        Updates Available
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {/* Containers for this agent */}
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-200">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Container Image
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Versions
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Ports
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Severity
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Status
                      </th>
                      <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                        Discovered
                      </th>
                    </tr>
                  </thead>
                  <tbody className="bg-white divide-y divide-gray-200">
                    {agentGroup.containers.map((container) => (
                      <tr key={container.id} className="hover:bg-gray-50">
                        <td className="px-6 py-4 whitespace-nowrap">
                          <div className="flex items-center">
                            <Container className="w-6 h-6 mr-3 text-blue-600" />
                            <div>
                              <div className="text-sm font-medium text-gray-900">
                                {container.image}:{container.tag}
                              </div>
                              <div className="text-xs text-gray-500">
                                {container.container_id !== container.image && container.container_id}
                              </div>
                            </div>
                          </div>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <div className="text-sm">
                            {container.update_available ? (
                              <>
                                <div className="text-gray-900">{container.current_version}</div>
                                <div className="text-green-600 font-medium">→ {container.available_version}</div>
                              </>
                            ) : (
                              <div className="text-gray-900">{container.current_version || container.tag}</div>
                            )}
                          </div>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <div className="text-sm text-gray-500 font-mono">
                            {formatPorts(container.ports)}
                          </div>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <span className={cn('inline-flex px-2 py-1 text-xs font-semibold rounded-full', getSeverityColor('medium'))}>
                            {/* Docker updates are typically medium or low severity by default */}
                            {'medium'}
                          </span>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap">
                          <span className={cn('inline-flex px-2 py-1 text-xs font-semibold rounded-full', getStatusColor(container.status))}>
                            {container.status}
                          </span>
                        </td>
                        <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                          {formatRelativeTime(container.created_at)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

export default Docker;