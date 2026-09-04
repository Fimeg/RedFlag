import React, { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { setupApi } from '@/lib/api';
import { useAuthStore } from '@/lib/store';

interface WelcomeCheckerProps {
  children: React.ReactNode;
}

export const WelcomeChecker: React.FC<WelcomeCheckerProps> = ({ children }) => {
  const [isWelcomeMode, setIsWelcomeMode] = useState<boolean | null>(null);
  const { isAuthenticated } = useAuthStore();

  // BUG-017: an authenticated user can't be in "welcome / awaiting config"
  // mode by definition (you can't log in to a server still in setup), so
  // skip the periodic check entirely. The previous behavior 5-second-spammed
  // the setup health endpoint forever and was part of the flicker timing.
  useEffect(() => {
    if (isAuthenticated) {
      setIsWelcomeMode(false);
      return;
    }

    const checkWelcomeMode = async () => {
      try {
        const data = await setupApi.checkHealth();

        if (data.status === 'waiting for configuration') {
          setIsWelcomeMode(true);
        } else {
          setIsWelcomeMode(false);
        }
      } catch (error) {
        setIsWelcomeMode(false);
      }
    };

    checkWelcomeMode();

    const interval = setInterval(checkWelcomeMode, 5000);

    return () => clearInterval(interval);
  }, [isAuthenticated]);

  if (isWelcomeMode === null) {
    // Loading state
    return (
      <div className="min-h-screen bg-gray-100 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Checking server status...</p>
        </div>
      </div>
    );
  }

  if (isWelcomeMode) {
    // Redirect to setup page
    return <Navigate to="/setup" replace />;
  }

  // Normal mode - render children
  return <>{children}</>;
};