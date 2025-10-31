import React, { useEffect, useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { setupApi } from '@/lib/api';

interface SetupCompletionCheckerProps {
  children: React.ReactNode;
}

export const SetupCompletionChecker: React.FC<SetupCompletionCheckerProps> = ({ children }) => {
  const [isSetupMode, setIsSetupMode] = useState<boolean | null>(null);
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    let wasInSetup = false; // Local variable instead of state

    const checkSetupStatus = async () => {
      try {
        const data = await setupApi.checkHealth();

        const currentSetupMode = data.status === 'waiting for configuration';

        // Track if we were previously in setup mode
        if (currentSetupMode) {
          wasInSetup = true;
        }

        // If we were in setup mode and now we're not, redirect to login
        if (wasInSetup && !currentSetupMode && location.pathname === '/setup') {
          console.log('Setup completed - redirecting to login');
          navigate('/login', { replace: true });
          return; // Prevent further state updates
        }

        setIsSetupMode(currentSetupMode);
      } catch (error) {
        // If we can't reach the health endpoint, assume normal mode
        if (wasInSetup && location.pathname === '/setup') {
          console.log('Setup completed (endpoint unreachable) - redirecting to login');
          navigate('/login', { replace: true });
          return;
        }
        setIsSetupMode(false);
      }
    };

    checkSetupStatus();

    // Check periodically for configuration changes
    const interval = setInterval(checkSetupStatus, 3000);

    return () => clearInterval(interval);
  }, [location.pathname, navigate]); // Removed wasInSetupMode from dependencies

  // Always render children - this component only handles redirects
  return <>{children}</>;
};