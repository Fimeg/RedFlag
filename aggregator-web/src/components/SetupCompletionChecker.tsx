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
    const checkSetupStatus = async () => {
      try {
        const data = await setupApi.checkHealth();

        if (data.status === 'waiting for configuration') {
          setIsSetupMode(true);
        } else {
          setIsSetupMode(false);
        }
      } catch (error) {
        // If we can't reach the health endpoint, assume normal mode
        setIsSetupMode(false);
      }
    };

    checkSetupStatus();

    // Check periodically for configuration changes
    const interval = setInterval(checkSetupStatus, 3000);

    return () => clearInterval(interval);
  }, []);

  // If we're on the setup page and server is now healthy, redirect to login
  useEffect(() => {
    if (isSetupMode === false && location.pathname === '/setup') {
      console.log('Setup completed - redirecting to login');
      navigate('/login', { replace: true });
    }
  }, [isSetupMode, location.pathname, navigate]);

  // Always render children - this component only handles redirects
  return <>{children}</>;
};