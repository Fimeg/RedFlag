import React, { useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { setupApi } from '@/lib/api';
import { clientLogger } from '@/lib/client-logger';

interface SetupCompletionCheckerProps {
  children: React.ReactNode;
}

export const SetupCompletionChecker: React.FC<SetupCompletionCheckerProps> = ({ children }) => {
  const navigate = useNavigate();
  const location = useLocation();

  // BUG-017: only poll the setup health endpoint while the user is
  // actually on /setup. Polling from inside the dashboard turned every
  // session into a 3-second spam against the server and contributed to
  // the auth-flicker timing window when /setup transitioned out from
  // under us. The checker is wrapped around <Setup> in App.tsx, so the
  // location guard is the safe boundary.
  useEffect(() => {
    if (location.pathname !== '/setup') {
      return;
    }

    let wasInSetup = false;

    const checkSetupStatus = async () => {
      try {
        const data = await setupApi.checkHealth();

        const currentSetupMode = data.status === 'waiting for configuration';

        if (currentSetupMode) {
          wasInSetup = true;
        }

        if (wasInSetup && !currentSetupMode) {
          clientLogger.debug('Setup completed — redirecting to login');
          navigate('/login', { replace: true });
          return;
        }
      } catch (error) {
        if (wasInSetup) {
          clientLogger.debug('Setup completed (endpoint unreachable) — redirecting to login');
          navigate('/login', { replace: true });
          return;
        }
      }
    };

    checkSetupStatus();

    const interval = setInterval(checkSetupStatus, 3000);

    return () => clearInterval(interval);
  }, [location.pathname, navigate]);

  // Always render children - this component only handles redirects
  return <>{children}</>;
};