import React from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Key, ShieldCheck, Archive, AlertTriangle, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { adminApi } from '@/lib/api';
import type { SigningKey } from '@/types';
import { formatDateTime } from '@/lib/utils';
import { useConfirm } from '@/components/primitives';

// SigningKeyRoster renders every Ed25519 signing key the server has ever held.
// The primary key is highlighted; accepted-but-not-primary keys can be deprecated
// inline. The primary cannot be deprecated — the server enforces this and returns
// 409 Conflict, which we surface to the operator. Generating a new key still
// happens at startup via REDFLAG_SIGNING_PRIVATE_KEY; this surface is the
// roster + retirement half of rotation.
const SigningKeyRoster: React.FC = () => {
  const queryClient = useQueryClient();
  const confirm = useConfirm();

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['signing-keys'],
    queryFn: () => adminApi.signingKeys.list(),
  });

  const deprecate = useMutation({
    mutationFn: (keyId: string) => adminApi.signingKeys.deprecate(keyId),
    onSuccess: (resp) => {
      toast.success(`Key ${resp.key_id.slice(0, 12)}… deprecated`);
      queryClient.invalidateQueries({ queryKey: ['signing-keys'] });
    },
    onError: (err: any) => {
      const msg = err?.response?.data?.error || err?.message || 'Failed to deprecate key';
      toast.error(msg);
    },
  });

  const handleDeprecate = async (key: SigningKey) => {
    if (key.is_primary) {
      toast.error('The primary key cannot be deprecated. Promote a successor first.');
      return;
    }
    if (!(await confirm({
      title: 'Deprecate Signing Key',
      body: `Deprecate key ${key.key_id.slice(0, 16)}? Agents that cached this key will need to refresh from /api/v1/public-keys before they can verify signed commands. This cannot be undone from the dashboard.`,
      confirmLabel: 'Deprecate',
      danger: true,
    }))) return;
    deprecate.mutate(key.key_id);
  };

  if (isLoading) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg p-8 text-center">
        <Loader2 className="w-6 h-6 text-blue-600 animate-spin mx-auto" />
        <p className="text-sm text-gray-500 mt-2">Loading signing keys…</p>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="bg-white border border-amber-200 rounded-lg p-6">
        <div className="flex items-center gap-2 text-amber-700">
          <AlertTriangle className="w-5 h-5" />
          <span className="font-medium">Failed to load signing keys</span>
        </div>
        <button
          onClick={() => refetch()}
          className="mt-3 px-3 py-1.5 text-sm bg-amber-50 border border-amber-200 rounded hover:bg-amber-100"
        >
          Retry
        </button>
      </div>
    );
  }

  const keys = data?.keys ?? [];
  const primary = keys.find(k => k.is_primary);
  const accepted = keys.filter(k => !k.is_primary && k.is_active);
  const deprecated = keys.filter(k => !k.is_active);

  return (
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <div className="flex items-start justify-between mb-4">
        <div>
          <h3 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Key className="w-5 h-5 text-blue-600" />
            Signing Key Roster
          </h3>
          <p className="text-sm text-gray-600 mt-1">
            Every Ed25519 key the server has ever held. The primary signs new commands;
            accepted keys still verify previously-signed commands during a rotation
            window; deprecated keys are retired and no longer trusted.
          </p>
        </div>
        <span className="text-xs text-gray-500">
          {keys.length} {keys.length === 1 ? 'key' : 'keys'}
        </span>
      </div>

      {/* Primary key */}
      {primary && (
        <div className="mb-4">
          <div className="flex items-center gap-2 mb-2">
            <ShieldCheck className="w-4 h-4 text-green-600" />
            <span className="text-xs font-semibold text-gray-700 uppercase tracking-wider">Primary (signing)</span>
          </div>
          <SigningKeyRow keyData={primary} />
        </div>
      )}

      {/* Accepted (non-primary, active) */}
      {accepted.length > 0 && (
        <div className="mb-4">
          <div className="flex items-center gap-2 mb-2">
            <Key className="w-4 h-4 text-blue-600" />
            <span className="text-xs font-semibold text-gray-700 uppercase tracking-wider">
              Accepted ({accepted.length}) — verify only
            </span>
          </div>
          <div className="space-y-2">
            {accepted.map(k => (
              <SigningKeyRow
                key={k.key_id}
                keyData={k}
                onDeprecate={() => handleDeprecate(k)}
                deprecating={deprecate.isPending && deprecate.variables === k.key_id}
              />
            ))}
          </div>
        </div>
      )}

      {/* Deprecated */}
      {deprecated.length > 0 && (
        <div>
          <div className="flex items-center gap-2 mb-2">
            <Archive className="w-4 h-4 text-gray-500" />
            <span className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
              Deprecated ({deprecated.length}) — no longer trusted
            </span>
          </div>
          <div className="space-y-2">
            {deprecated.map(k => (
              <SigningKeyRow key={k.key_id} keyData={k} />
            ))}
          </div>
        </div>
      )}

      {keys.length === 0 && (
        <div className="text-center py-6 text-sm text-gray-500">
          No signing keys registered. The signing service may be disabled.
        </div>
      )}
    </div>
  );
};

interface SigningKeyRowProps {
  keyData: SigningKey;
  onDeprecate?: () => void;
  deprecating?: boolean;
}

const SigningKeyRow: React.FC<SigningKeyRowProps> = ({ keyData, onDeprecate, deprecating }) => {
  const isPrimary = keyData.is_primary;
  const isDeprecated = !keyData.is_active;

  return (
    <div
      className={`flex items-center justify-between gap-4 p-3 rounded border ${
        isPrimary
          ? 'bg-green-50 border-green-200'
          : isDeprecated
          ? 'bg-gray-50 border-gray-200 opacity-75'
          : 'bg-blue-50 border-blue-200'
      }`}
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <code className="font-mono text-xs text-gray-800 truncate">{keyData.key_id}</code>
          <span className="text-xs text-gray-500">v{keyData.version}</span>
          <span className="text-xs text-gray-500 uppercase">{keyData.algorithm}</span>
        </div>
        <div className="text-xs text-gray-600">
          Created {formatDateTime(keyData.created_at)}
          {keyData.deprecated_at && (
            <> · Deprecated {formatDateTime(keyData.deprecated_at)}</>
          )}
        </div>
      </div>
      {onDeprecate && (
        <button
          onClick={onDeprecate}
          disabled={deprecating}
          className="flex-shrink-0 px-3 py-1.5 text-sm text-amber-700 bg-white border border-amber-200 rounded hover:bg-amber-50 disabled:opacity-50"
        >
          {deprecating ? (
            <span className="flex items-center gap-1">
              <Loader2 className="w-3 h-3 animate-spin" /> Deprecating…
            </span>
          ) : (
            'Deprecate'
          )}
        </button>
      )}
    </div>
  );
};

export default SigningKeyRoster;
