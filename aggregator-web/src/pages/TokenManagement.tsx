import React, { useState } from 'react';
import {
  Shield,
  Plus,
  Search,
  Filter,
  RefreshCw,
  Download,
  Trash2,
  Copy,
  Eye,
  EyeOff,
  AlertTriangle,
  CheckCircle,
  Clock,
  Users
} from 'lucide-react';
import {
  useRegistrationTokens,
  useCreateRegistrationToken,
  useRevokeRegistrationToken,
  useRegistrationTokenStats,
  useCleanupRegistrationTokens
} from '../hooks/useRegistrationTokens';
import { RegistrationToken, CreateRegistrationTokenRequest } from '@/types';
import { formatDateTime } from '@/lib/utils';

const TokenManagement: React.FC = () => {
  // Filters and search
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'used' | 'expired' | 'revoked'>('all');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [showToken, setShowToken] = useState<Record<string, boolean>>({});

  // Pagination
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 50;

  // Token management
  const { data: tokensData, isLoading, refetch } = useRegistrationTokens({
    page: currentPage,
    page_size: pageSize,
    is_active: statusFilter === 'all' ? undefined : statusFilter === 'active',
    label: searchTerm || undefined,
  });

  const { data: stats, isLoading: isLoadingStats } = useRegistrationTokenStats();
  const createToken = useCreateRegistrationToken();
  const revokeToken = useRevokeRegistrationToken();
  const cleanupTokens = useCleanupRegistrationTokens();

  // Reset page when filters change
  React.useEffect(() => {
    setCurrentPage(1);
  }, [searchTerm, statusFilter]);

  // Form state
  const [formData, setFormData] = useState<CreateRegistrationTokenRequest>({
    label: '',
    max_seats: 10,
    expires_at: '',
  });

  const handleCreateToken = (e: React.FormEvent) => {
    e.preventDefault();
    createToken.mutate(formData, {
      onSuccess: () => {
        setFormData({ label: '', max_seats: 10, expires_at: '' });
        setShowCreateForm(false);
        refetch();
      },
    });
  };

  const handleRevokeToken = (tokenId: string, tokenLabel: string) => {
    if (confirm(`Revoke token "${tokenLabel}"? Agents using it will need to re-register.`)) {
      revokeToken.mutate(tokenId, { onSuccess: () => refetch() });
    }
  };

  const handleCleanup = () => {
    if (confirm('Clean up all expired tokens? This cannot be undone.')) {
      cleanupTokens.mutate(undefined, { onSuccess: () => refetch() });
    }
  };

  const copyToClipboard = async (text: string) => {
    await navigator.clipboard.writeText(text);
    // Show success feedback
  };

  const copyInstallCommand = async (token: string) => {
    const command = `curl -sSL https://get.redflag.dev | bash -s -- ${token}`;
    await navigator.clipboard.writeText(command);
  };

  const generateInstallCommand = (token: string) => {
    return `curl -sSL https://get.redflag.dev | bash -s -- ${token}`;
  };

  const getStatusColor = (token: RegistrationToken) => {
    if (!token.is_active) return 'text-gray-500';
    if (token.expires_at && new Date(token.expires_at) < new Date()) return 'text-red-600';
    if (token.max_seats && token.current_seats >= token.max_seats) return 'text-yellow-600';
    return 'text-green-600';
  };

  const getStatusText = (token: RegistrationToken) => {
    if (!token.is_active) return 'Revoked';
    if (token.expires_at && new Date(token.expires_at) < new Date()) return 'Expired';
    if (token.max_seats && token.current_seats >= token.max_seats) return 'Full';
    return 'Active';
  };

  const filteredTokens = tokensData?.tokens || [];

  return (
    <div className="max-w-7xl mx-auto px-6 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900">Registration Tokens</h1>
            <p className="mt-2 text-gray-600">Manage agent registration tokens and monitor their usage</p>
          </div>
          <div className="flex gap-3">
            <button
              onClick={handleCleanup}
              disabled={cleanupTokens.isPending}
              className="inline-flex items-center gap-2 px-4 py-2 bg-orange-600 text-white rounded-lg hover:bg-orange-700 disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 ${cleanupTokens.isPending ? 'animate-spin' : ''}`} />
              Cleanup Expired
            </button>
            <button
              onClick={() => refetch()}
              className="inline-flex items-center gap-2 px-4 py-2 border border-gray-300 text-gray-700 rounded-lg hover:bg-gray-50"
            >
              <RefreshCw className="w-4 h-4" />
              Refresh
            </button>
            <button
              onClick={() => setShowCreateForm(!showCreateForm)}
              className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
            >
              <Plus className="w-4 h-4" />
              Create Token
            </button>
          </div>
        </div>
      </div>

      {/* Statistics Cards */}
      {stats && (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4 mb-8">
          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Total Tokens</p>
                <p className="text-2xl font-bold text-gray-900">{stats.total_tokens}</p>
              </div>
              <Shield className="w-8 h-8 text-blue-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Active</p>
                <p className="text-2xl font-bold text-green-600">{stats.active_tokens}</p>
              </div>
              <CheckCircle className="w-8 h-8 text-green-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Used</p>
                <p className="text-2xl font-bold text-blue-600">{stats.used_tokens}</p>
              </div>
              <Users className="w-8 h-8 text-blue-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Expired</p>
                <p className="text-2xl font-bold text-gray-600">{stats.expired_tokens}</p>
              </div>
              <Clock className="w-8 h-8 text-gray-600" />
            </div>
          </div>

          <div className="bg-white rounded-lg border border-gray-200 p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-600">Seats Used</p>
                <p className="text-2xl font-bold text-purple-600">
                  {stats.total_seats_used}/{stats.total_seats_available || '∞'}
                </p>
              </div>
              <Users className="w-8 h-8 text-purple-600" />
            </div>
          </div>
        </div>
      )}

      {/* Create Token Form */}
      {showCreateForm && (
        <div className="bg-white rounded-lg border border-gray-200 p-6 mb-8">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">Create New Registration Token</h3>
          <form onSubmit={handleCreateToken} className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Label *</label>
                <input
                  type="text"
                  required
                  value={formData.label}
                  onChange={(e) => setFormData({ ...formData, label: e.target.value })}
                  placeholder="e.g., Production Team"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Max Seats</label>
                <input
                  type="number"
                  min="1"
                  value={formData.max_seats}
                  onChange={(e) => setFormData({ ...formData, max_seats: e.target.value ? parseInt(e.target.value) : undefined })}
                  placeholder="Leave empty for unlimited"
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Expiration Date</label>
                <input
                  type="datetime-local"
                  value={formData.expires_at}
                  onChange={(e) => setFormData({ ...formData, expires_at: e.target.value })}
                  min={new Date().toISOString().slice(0, 16)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </div>

            <div className="flex gap-3">
              <button
                type="submit"
                disabled={createToken.isPending}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50"
              >
                {createToken.isPending ? 'Creating...' : 'Create Token'}
              </button>
              <button
                type="button"
                onClick={() => setShowCreateForm(false)}
                className="px-4 py-2 bg-gray-200 text-gray-800 rounded-lg hover:bg-gray-300"
              >
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Filters and Search */}
      <div className="bg-white rounded-lg border border-gray-200 p-6 mb-8">
        <div className="flex flex-col lg:flex-row gap-4">
          <div className="flex-1">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 w-4 h-4" />
              <input
                type="text"
                placeholder="Search by label..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>
          </div>

          <div className="flex gap-2">
            <button
              onClick={() => setStatusFilter('all')}
              className={`px-4 py-2 rounded-lg transition-colors ${
                statusFilter === 'all'
                  ? 'bg-gray-100 text-gray-800 border border-gray-300'
                  : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
              }`}
            >
              All
            </button>
            <button
              onClick={() => setStatusFilter('active')}
              className={`px-4 py-2 rounded-lg transition-colors ${
                statusFilter === 'active'
                  ? 'bg-green-100 text-green-800 border border-green-300'
                  : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
              }`}
            >
              Active
            </button>
            <button
              onClick={() => setStatusFilter('used')}
              className={`px-4 py-2 rounded-lg transition-colors ${
                statusFilter === 'used'
                  ? 'bg-blue-100 text-blue-800 border border-blue-300'
                  : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
              }`}
            >
              Used
            </button>
            <button
              onClick={() => setStatusFilter('expired')}
              className={`px-4 py-2 rounded-lg transition-colors ${
                statusFilter === 'expired'
                  ? 'bg-red-100 text-red-800 border border-red-300'
                  : 'bg-white text-gray-600 border border-gray-300 hover:bg-gray-50'
              }`}
            >
              Expired
            </button>
          </div>
        </div>
      </div>

      {/* Tokens List */}
      <div className="bg-white rounded-lg border border-gray-200">
        {isLoading ? (
          <div className="p-12 text-center">
            <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
            <p className="mt-2 text-gray-600">Loading tokens...</p>
          </div>
        ) : filteredTokens.length === 0 ? (
          <div className="p-12 text-center">
            <Shield className="w-16 h-16 text-gray-400 mx-auto mb-4" />
            <h3 className="text-lg font-medium text-gray-900 mb-2">No tokens found</h3>
            <p className="text-gray-600">
              {searchTerm || statusFilter !== 'all'
                ? 'Try adjusting your search or filter criteria'
                : 'Create your first token to begin registering agents'}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Token
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Label
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Status
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Seats
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Created
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Expires
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Last Used
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {filteredTokens.map((token) => (
                  <tr key={token.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="flex items-center gap-3">
                        <div className="font-mono text-sm bg-gray-100 px-3 py-2 rounded">
                          {showToken[token.id] ? token.token : '•••••••••••••••••'}
                        </div>
                        <button
                          onClick={() => setShowToken({ ...showToken, [token.id]: !showToken[token.id] })}
                          className="text-gray-400 hover:text-gray-600"
                        >
                          {showToken[token.id] ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                        </button>
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900">{token.label}</div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className={`flex items-center gap-1 px-3 py-1 rounded-full text-xs font-medium ${getStatusColor(token)}`}>
                        {getStatusText(token)}
                      </div>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm text-gray-900">
                        {token.current_seats}
                        {token.max_seats && ` / ${token.max_seats}`}
                      </div>
                      {token.max_seats && (
                        <div className="w-full bg-gray-200 rounded-full h-2 mt-1">
                          <div
                            className="bg-blue-600 h-2 rounded-full"
                            style={{ width: `${Math.min((token.current_seats / token.max_seats) * 100, 100)}%` }}
                          ></div>
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDateTime(token.created_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDateTime(token.expires_at) || 'Never'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatDateTime(token.last_used_at) || 'Never'}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => copyToClipboard(token.token)}
                          className="text-blue-600 hover:text-blue-800"
                          title="Copy token"
                        >
                          <Copy className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => copyInstallCommand(token.token)}
                          className="text-blue-600 hover:text-blue-800"
                          title="Copy install command"
                        >
                          <Download className="w-4 h-4" />
                        </button>
                        {token.is_active && (
                          <button
                            onClick={() => handleRevokeToken(token.id, token.label)}
                            disabled={revokeToken.isPending}
                            className="text-red-600 hover:text-red-800 disabled:opacity-50"
                            title="Revoke token"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Pagination */}
      {tokensData && tokensData.total > pageSize && (
        <div className="mt-6 flex items-center justify-between">
          <div className="text-sm text-gray-700">
            Showing {((currentPage - 1) * pageSize) + 1}-{Math.min(currentPage * pageSize, tokensData.total)} of {tokensData.total} tokens
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
              disabled={currentPage === 1}
              className="px-3 py-1 border border-gray-300 rounded text-sm disabled:opacity-50 disabled:cursor-not-allowed hover:bg-gray-50"
            >
              Previous
            </button>

            <div className="flex items-center gap-1">
              {Array.from({ length: Math.min(5, Math.ceil(tokensData.total / pageSize)) }, (_, i) => {
                const totalPages = Math.ceil(tokensData.total / pageSize);
                let pageNum;

                if (totalPages <= 5) {
                  pageNum = i + 1;
                } else if (currentPage <= 3) {
                  pageNum = i + 1;
                } else if (currentPage >= totalPages - 2) {
                  pageNum = totalPages - 4 + i;
                } else {
                  pageNum = currentPage - 2 + i;
                }

                return (
                  <button
                    key={pageNum}
                    onClick={() => setCurrentPage(pageNum)}
                    className={`px-3 py-1 border rounded text-sm ${
                      currentPage === pageNum
                        ? 'bg-blue-600 text-white border-blue-600'
                        : 'border-gray-300 hover:bg-gray-50'
                    }`}
                  >
                    {pageNum}
                  </button>
                );
              })}
            </div>

            <button
              onClick={() => setCurrentPage(Math.min(Math.ceil(tokensData.total / pageSize), currentPage + 1))}
              disabled={currentPage >= Math.ceil(tokensData.total / pageSize)}
              className="px-3 py-1 border border-gray-300 rounded text-sm disabled:opacity-50 disabled:cursor-not-allowed hover:bg-gray-50"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export default TokenManagement;