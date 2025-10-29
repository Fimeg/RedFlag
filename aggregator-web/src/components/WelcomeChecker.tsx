import React, { useEffect, useState } from 'react';
import { Navigate } from 'react-router-dom';
import { setupApi } from '@/lib/api';

interface WelcomeCheckerProps {
  children: React.ReactNode;
}

export const WelcomeChecker: React.FC<WelcomeCheckerProps> = ({ children }) => {
  const [isWelcomeMode, setIsWelcomeMode] = useState<boolean | null>(null);

  useEffect(() => {
    const checkWelcomeMode = async () => {
      try {
        const data = await setupApi.checkHealth();

        if (data.status === 'waiting for configuration') {
          setIsWelcomeMode(true);
        } else {
          setIsWelcomeMode(false);
        }
      } catch (error) {
        // If we can't reach the health endpoint, assume normal mode
        setIsWelcomeMode(false);
      }
    };

    checkWelcomeMode();

    // Check periodically for configuration changes
    const interval = setInterval(checkWelcomeMode, 5000);

    return () => clearInterval(interval);
  }, []);

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