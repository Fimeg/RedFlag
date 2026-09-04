import React, { lazy, Suspense, useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { Toaster } from 'react-hot-toast';
import { useAuthStore, useUIStore } from '@/lib/store';
import { authApi } from '@/lib/api';
import ErrorBoundary from '@/components/ErrorBoundary';
import Layout from '@/components/Layout';
import { WelcomeChecker } from '@/components/WelcomeChecker';
import { SetupCompletionChecker } from '@/components/SetupCompletionChecker';
import { useServerStatus } from '@/hooks/useServerStatus';
import ServerStatusOverlay from '@/components/ServerStatusOverlay';

const Dashboard = lazy(() => import('@/pages/Dashboard'));
const Agents = lazy(() => import('@/pages/Agents'));
const Updates = lazy(() => import('@/pages/Updates'));
const PackageDetail = lazy(() => import('@/pages/PackageDetail'));
const Docker = lazy(() => import('@/pages/Docker'));
const LiveOperations = lazy(() => import('@/pages/LiveOperations'));
const History = lazy(() => import('@/pages/History'));
const Settings = lazy(() => import('@/pages/Settings'));
const RateLimiting = lazy(() => import('@/pages/RateLimiting'));
const AgentsEnrollment = lazy(() => import('@/pages/settings/AgentsEnrollment'));
const MaintenanceWindows = lazy(() => import('@/pages/settings/MaintenanceWindows'));
const UpstreamTracking = lazy(() => import('@/pages/settings/UpstreamTracking'));
const General = lazy(() => import('@/pages/settings/General'));
const AgentPolling = lazy(() => import('@/pages/settings/AgentPolling'));
const ProcessExplorer = lazy(() => import('@/pages/settings/ProcessExplorer'));
const SecuritySettings = lazy(() => import('@/pages/SecuritySettings'));
const Login = lazy(() => import('@/pages/Login'));
const Setup = lazy(() => import('@/pages/Setup'));

// Protected route component
const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isAuthenticated } = useAuthStore();

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
};

const RouteFallback: React.FC = () => (
  <div className="min-h-screen bg-gray-50 flex items-center justify-center">
    <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-indigo-600" />
  </div>
);

const App: React.FC = () => {
  const { isAuthenticated, token } = useAuthStore();
  const { theme } = useUIStore();
  const { connected, checkNow } = useServerStatus();

  // Apply theme to document
  useEffect(() => {
    if (theme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [theme]);

  // Check for existing token on app start.
  // BUG-017: a stale token in localStorage (e.g. after a server reinstall
  // rotates JWT_SECRET) used to be silently accepted, then every API call
  // 401'd and the interceptor full-page-redirected to /login — the flicker.
  // We now validate against /auth/verify before marking the session
  // authenticated. On failure, drop the bad credentials silently.
  useEffect(() => {
    const storedToken = localStorage.getItem('auth_token');
    if (storedToken && !token) {
      let cancelled = false;
      authApi.verifyToken()
        .then((res) => {
          if (cancelled) return;
          if (res?.valid) {
            useAuthStore.getState().setToken(storedToken);
          } else {
            useAuthStore.getState().logout();
          }
        })
        .catch(() => {
          if (cancelled) return;
          useAuthStore.getState().logout();
        });
      return () => { cancelled = true; };
    }
  }, [token]);

  return (
    <ErrorBoundary>
    <div className={`min-h-screen bg-gray-50 ${theme === 'dark' ? 'dark' : ''}`}>
      {/* Toast notifications */}
      <Toaster
        position="top-right"
        toastOptions={{
          duration: 4000,
          style: {
            background: theme === 'dark' ? '#374151' : '#ffffff',
            color: theme === 'dark' ? '#ffffff' : '#000000',
            border: '1px solid',
            borderColor: theme === 'dark' ? '#4b5563' : '#e5e7eb',
          },
          success: {
            iconTheme: {
              primary: '#22c55e',
              secondary: '#ffffff',
            },
          },
          error: {
            iconTheme: {
              primary: '#ef4444',
              secondary: '#ffffff',
            },
          },
        }}
      />

  
          {/* App routes */}
      <Suspense fallback={<RouteFallback />}>
        <Routes>
          {/* Setup route - shown when server needs configuration */}
          <Route
            path="/setup"
            element={
              <SetupCompletionChecker>
                <Setup />
              </SetupCompletionChecker>
            }
          />

          {/* Login route */}
          <Route
            path="/login"
            element={isAuthenticated ? <Navigate to="/" replace /> : <Login />}
          />

          {/* Protected routes */}
          <Route
            path="/*"
            element={
              <WelcomeChecker>
                <ProtectedRoute>
                  <Layout>
                    <Routes>
                      <Route path="/" element={<Dashboard />} />
                      <Route path="/dashboard" element={<Dashboard />} />
                      <Route path="/agents" element={<Agents />} />
                      <Route path="/agents/:id" element={<Agents />} />
                      <Route path="/updates" element={<Updates />} />
                      <Route path="/updates/:id" element={<Updates />} />
                      <Route path="/updates/package/:type/:name" element={<PackageDetail />} />
                      <Route path="/docker" element={<Docker />} />
                      <Route path="/live" element={<Navigate to="/staging" replace />} />
                      <Route path="/staging" element={<LiveOperations />} />
                      <Route path="/history" element={<History />} />
                      <Route path="/settings" element={<Settings />} />
                      <Route path="/settings/general" element={<General />} />
                      <Route path="/settings/tokens" element={<Navigate to="/settings/agents" replace />} />
                      <Route path="/settings/rate-limiting" element={<RateLimiting />} />
                      <Route path="/settings/agents" element={<AgentsEnrollment />} />
                      <Route path="/settings/polling" element={<AgentPolling />} />
                      <Route path="/settings/security" element={<SecuritySettings />} />
                      <Route path="/settings/security/:tab" element={<SecuritySettings />} />
                      <Route path="/settings/maintenance-windows" element={<MaintenanceWindows />} />
                      <Route path="/settings/upstream" element={<UpstreamTracking />} />
                      <Route path="/settings/process-explorer" element={<ProcessExplorer />} />
                      <Route path="*" element={<Navigate to="/" replace />} />
                    </Routes>
                  </Layout>
                </ProtectedRoute>
              </WelcomeChecker>
            }
          />
        </Routes>
      </Suspense>

      {/* Floating overlay when backend is unreachable */}
      {!connected && (
        <ServerStatusOverlay onRetryNow={checkNow} />
      )}
    </div>
    </ErrorBoundary>
  );
};

export default App;
