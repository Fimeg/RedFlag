import React, { useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { Toaster } from 'react-hot-toast';
import { useAuthStore, useUIStore } from '@/lib/store';
import Layout from '@/components/Layout';
import Dashboard from '@/pages/Dashboard';
import Agents from '@/pages/Agents';
import Updates from '@/pages/Updates';
import Docker from '@/pages/Docker';
import Logs from '@/pages/Logs';
import Settings from '@/pages/Settings';
import Login from '@/pages/Login';

// Protected route component
const ProtectedRoute: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { isAuthenticated } = useAuthStore();

  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
};

const App: React.FC = () => {
  const { isAuthenticated, token } = useAuthStore();
  const { theme } = useUIStore();

  // Apply theme to document
  useEffect(() => {
    if (theme === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }, [theme]);

  // Check for existing token on app start
  useEffect(() => {
    const storedToken = localStorage.getItem('auth_token');
    if (storedToken && !token) {
      useAuthStore.getState().setToken(storedToken);
    }
  }, [token]);

  return (
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
      <Routes>
        {/* Login route */}
        <Route
          path="/login"
          element={isAuthenticated ? <Navigate to="/" replace /> : <Login />}
        />

        {/* Protected routes */}
        <Route
          path="/*"
          element={
            <ProtectedRoute>
              <Layout>
                <Routes>
                  <Route path="/" element={<Dashboard />} />
                  <Route path="/dashboard" element={<Dashboard />} />
                  <Route path="/agents" element={<Agents />} />
                  <Route path="/agents/:id" element={<Agents />} />
                  <Route path="/updates" element={<Updates />} />
                  <Route path="/updates/:id" element={<Updates />} />
                  <Route path="/docker" element={<Docker />} />
                  <Route path="/logs" element={<Logs />} />
                  <Route path="/settings" element={<Settings />} />
                  <Route path="*" element={<Navigate to="/" replace />} />
                </Routes>
              </Layout>
            </ProtectedRoute>
          }
        />
      </Routes>
    </div>
  );
};

export default App;