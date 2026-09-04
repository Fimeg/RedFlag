import { useQuery } from '@tanstack/react-query';
import api from '@/lib/api';

export interface ServerKeySecurityStatus {
  has_private_key: boolean;
  public_key_fingerprint?: string;
  algorithm?: string;
}

export const useServerKeySecurity = () => {
  return useQuery<ServerKeySecurityStatus, Error>({
    queryKey: ['serverKeySecurity'],
    queryFn: async () => {
      const response = await api.get('/security/overview');
      const overview = response.data;
      const signingStatus = overview.subsystems.ed25519_signing;

      return {
        has_private_key: signingStatus.status === 'healthy',
        public_key_fingerprint: signingStatus.checks?.public_key_fingerprint,
        algorithm: signingStatus.checks?.algorithm,
      };
    },
  });
};
