import React, { useEffect, useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { setupApi } from '@/lib/api';

interface SetupCompletionCheckerProps {
  children: React.ReactNode;
}

export const SetupCompletionChecker: React.FC<SetupCompletionCheckerProps> = ({ children }) => {
  const [wasInSetupMode, setWasInSetupMode] = useState(false);
  const [isSetupMode, setIsSetupMode] = useState<boolean | null>(null);
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    const checkSetupStatus = async () => {
      try {
        const data = await setupApi.checkHealth();

        const currentSetupMode = data.status === 'waiting for configuration';

        // Track if we were previously in setup mode
        if (currentSetupMode) {
          setWasInSetupMode(true);
        }

        // If we were in setup mode and now we're not, redirect to login
        if (wasInSetupMode && !currentSetupMode && location.pathname === '/setup') {
          console.log('Setup completed - redirecting to login');
          navigate('/login', { replace: true });
          return; // Prevent further state updates
        }

        setIsSetupMode(currentSetupMode);
      } catch (error) {
        // If we can't reach the health endpoint, assume normal mode
        if (wasInSetupMode && location.pathname === '/setup') {
          console.log('Setup completed (endpoint reachable) - redirecting to login');
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
  }, [wasInSetupMode, location.pathname, navigate]);

  // Always render children - this component only handles redirects
  return <>{children}</>;
};