import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Settings, Database, User, Shield, Eye, EyeOff, CheckCircle } from 'lucide-react';
import { toast } from 'react-hot-toast';
import { setupApi } from '@/lib/api';
import { useAuthStore } from '@/lib/store';

interface SetupFormData {
  adminUser: string;
  adminPassword: string;
  dbHost: string;
  dbPort: string;
  dbName: string;
  dbUser: string;
  dbPassword: string;
  serverHost: string;
  serverPort: string;
  maxSeats: string;
}

const Setup: React.FC = () => {
  const navigate = useNavigate();
  const { logout } = useAuthStore();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
    const [envContent, setEnvContent] = useState<string | null>(null);
  const [showSuccess, setShowSuccess] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showDbPassword, setShowDbPassword] = useState(false);

  const [formData, setFormData] = useState<SetupFormData>({
    adminUser: 'admin',
    adminPassword: '',
    dbHost: 'postgres',
    dbPort: '5432',
    dbName: 'redflag',
    dbUser: 'redflag',
    dbPassword: 'redflag',
    serverHost: '0.0.0.0',
    serverPort: '8080',
    maxSeats: '50',
  });

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const validateForm = (): boolean => {
    if (!formData.adminUser.trim()) {
      setError('Admin username is required');
      return false;
    }
    if (!formData.adminPassword.trim()) {
      setError('Admin password is required');
      return false;
    }
    if (!formData.dbHost.trim()) {
      setError('Database host is required');
      return false;
    }
    if (!formData.dbPort.trim()) {
      setError('Database port is required');
      return false;
    }
    const dbPort = parseInt(formData.dbPort);
    if (isNaN(dbPort) || dbPort <= 0 || dbPort > 65535) {
      setError('Database port must be between 1 and 65535');
      return false;
    }
    if (!formData.dbName.trim()) {
      setError('Database name is required');
      return false;
    }
    if (!formData.dbUser.trim()) {
      setError('Database user is required');
      return false;
    }
    if (!formData.dbPassword.trim()) {
      setError('Database password is required');
      return false;
    }
    if (!formData.serverPort.trim()) {
      setError('Server port is required');
      return false;
    }
    const serverPort = parseInt(formData.serverPort);
    if (isNaN(serverPort) || serverPort <= 0 || serverPort > 65535) {
      setError('Server port must be between 1 and 65535');
      return false;
    }
    const maxSeats = parseInt(formData.maxSeats);
    if (isNaN(maxSeats) || maxSeats <= 0) {
      setError('Maximum agent seats must be greater than 0');
      return false;
    }
    return true;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!validateForm()) {
      return;
    }

    logout();
    setIsLoading(true);

    try {
      const result = await setupApi.configure(formData);

      setEnvContent(result.envContent || null);
      setShowSuccess(true);
      toast.success(result.message || 'Configuration saved successfully!');

    } catch (error: any) {
      console.error('Setup error:', error);
      const errorMessage = error.response?.data?.error || error.message || 'Setup failed';
      setError(errorMessage);
      toast.error(errorMessage);
    } finally {
      setIsLoading(false);
    }
  };

  // Success screen with configuration display
  if (showSuccess && envContent) {
    return (
      <div className="px-4 sm:px-6 lg:px-8">
        <div className="max-w-3xl mx-auto">
          {/* Header */}
          <div className="mb-8">
            <div className="flex items-center justify-center mb-4">
              <div className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center">
                <CheckCircle className="w-8 h-8 text-green-600" />
              </div>
            </div>
            <h1 className="text-3xl font-bold text-gray-900 text-center mb-2">
              Configuration Complete!
            </h1>
            <p className="text-gray-600 text-center">
              Your RedFlag server is ready to use
            </p>
          </div>

          {/* Success Card */}
          <div className="bg-white rounded-lg border border-gray-200 shadow-sm p-6">
            {/* Admin Credentials Section */}
            <div className="mb-6">
              <h3 className="text-lg font-semibold text-gray-900 mb-3">Administrator Credentials</h3>
              <div className="bg-gray-50 border border-gray-200 rounded-md p-4">
                <div className="grid grid-cols-1 gap-3">
                  <div>
                    <label className="text-xs font-medium text-gray-500 uppercase tracking-wide">Username</label>
                    <div className="mt-1 p-2 bg-white border border-gray-300 rounded text-sm font-mono">{formData.adminUser}</div>
                  </div>
                  <div>
                    <label className="text-xs font-medium text-gray-500 uppercase tracking-wide">Password</label>
                    <div className="mt-1 p-2 bg-white border border-gray-300 rounded text-sm font-mono">••••••••</div>
                  </div>
                </div>
              </div>
              <div className="mt-3 p-3 bg-green-50 border border-green-200 rounded-md">
                <p className="text-sm text-green-800">
                  <strong>Important:</strong> Save these credentials securely. You'll use them to login to the RedFlag dashboard.
                </p>
              </div>
            </div>

            {/* Configuration Content Section */}
            {envContent && (
              <div className="mb-6">
                <h3 className="text-lg font-semibold text-gray-900 mb-3">Configuration File Content</h3>
                <div className="bg-gray-50 border border-gray-200 rounded-md p-4">
                  <textarea
                    readOnly
                    value={envContent}
                    className="w-full h-64 p-3 text-xs font-mono text-gray-800 bg-white border border-gray-300 rounded-md resize-none focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                </div>
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(envContent);
                    toast.success('Configuration content copied to clipboard!');
                  }}
                  className="mt-3 w-full flex justify-center py-2 px-4 border border-transparent rounded-md text-sm font-medium text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500"
                >
                  Copy Configuration Content
                </button>
                <div className="mt-3 p-3 bg-blue-50 border border-blue-200 rounded-md">
                  <p className="text-sm text-blue-800">
                    <strong>Important:</strong> Copy this configuration content and save it to <code className="bg-blue-100 px-1 rounded">./config/.env</code>, then run <code className="bg-blue-100 px-1 rounded">docker-compose down && docker-compose up -d</code> to apply the configuration.
                  </p>
                </div>
              </div>
            )}

            
            {/* Next Steps */}
            <div className="border-t border-gray-200 pt-6">
              <h3 className="text-lg font-semibold text-gray-900 mb-3">Next Steps</h3>
              <ol className="list-decimal list-inside space-y-2 text-sm text-gray-600">
                <li>Copy the configuration content using the green button above</li>
                <li>Save it to <code className="bg-gray-100 px-1 rounded">./config/.env</code></li>
                <li>Run <code className="bg-gray-100 px-1 rounded">docker-compose down && docker-compose up -d</code></li>
                <li>Login to the dashboard with your admin username and password</li>
              </ol>
            </div>

            <div className="mt-6 pt-6 border-t border-gray-200 space-y-3">
              <div className="p-3 bg-yellow-50 border border-yellow-200 rounded-md">
                <p className="text-sm text-yellow-800">
                  <strong>Important:</strong> You must restart the containers to apply the configuration before logging in.
                </p>
              </div>
              <button
                onClick={() => {
                  toast.success('Please run: docker-compose down && docker-compose up -d');
                }}
                className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md text-sm font-medium text-white bg-yellow-600 hover:bg-yellow-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-yellow-500"
              >
                Show Restart Command
              </button>
              <button
                onClick={() => {
                  setTimeout(() => navigate('/login'), 500);
                }}
                className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
              >
                Continue to Login (After Restart)
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      <div className="max-w-4xl mx-auto">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center justify-center mb-4">
            <div className="w-16 h-16 bg-indigo-100 rounded-full flex items-center justify-center">
              <span className="text-2xl">🚩</span>
            </div>
          </div>
          <h1 className="text-3xl font-bold text-gray-900 text-center mb-2">
            Configure RedFlag Server
          </h1>
          <p className="text-gray-600 text-center">
            Set up your update management server configuration
          </p>
        </div>

        {/* Setup Form */}
        <div className="bg-white rounded-lg border border-gray-200 shadow-sm p-6">
          <form onSubmit={handleSubmit} className="space-y-8">
            {/* Error Display */}
            {error && (
              <div className="bg-red-50 border border-red-200 rounded-md p-4">
                <div className="text-sm text-red-800">{error}</div>
              </div>
            )}

            {/* Administrator Account */}
            <div>
              <div className="flex items-center mb-4">
                <User className="h-5 w-5 text-indigo-600 mr-2" />
                <h3 className="text-lg font-semibold text-gray-900">Administrator Account</h3>
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="adminUser" className="block text-sm font-medium text-gray-700 mb-1">
                    Admin Username
                  </label>
                  <input
                    type="text"
                    id="adminUser"
                    name="adminUser"
                    value={formData.adminUser}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    placeholder="admin"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="adminPassword" className="block text-sm font-medium text-gray-700 mb-1">
                    Admin Password
                  </label>
                  <div className="relative">
                    <input
                      type={showPassword ? 'text' : 'password'}
                      id="adminPassword"
                      name="adminPassword"
                      value={formData.adminPassword}
                      onChange={handleInputChange}
                      className="block w-full px-3 py-2 pr-10 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                      placeholder="Enter secure password"
                      required
                    />
                    <button
                      type="button"
                      className="absolute inset-y-0 right-0 pr-3 flex items-center"
                      onClick={() => setShowPassword(!showPassword)}
                    >
                      {showPassword ? (
                        <EyeOff className="h-4 w-4 text-gray-400" />
                      ) : (
                        <Eye className="h-4 w-4 text-gray-400" />
                      )}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            {/* Database Configuration */}
            <div>
              <div className="flex items-center mb-4">
                <Database className="h-5 w-5 text-indigo-600 mr-2" />
                <h3 className="text-lg font-semibold text-gray-900">Database Configuration</h3>
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <div>
                  <label htmlFor="dbHost" className="block text-sm font-medium text-gray-700 mb-1">
                    Database Host
                  </label>
                  <input
                    type="text"
                    id="dbHost"
                    name="dbHost"
                    value={formData.dbHost}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    placeholder="postgres"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbPort" className="block text-sm font-medium text-gray-700 mb-1">
                    Database Port
                  </label>
                  <input
                    type="number"
                    id="dbPort"
                    name="dbPort"
                    value={formData.dbPort}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbName" className="block text-sm font-medium text-gray-700 mb-1">
                    Database Name
                  </label>
                  <input
                    type="text"
                    id="dbName"
                    name="dbName"
                    value={formData.dbName}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    placeholder="redflag"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbUser" className="block text-sm font-medium text-gray-700 mb-1">
                    Database User
                  </label>
                  <input
                    type="text"
                    id="dbUser"
                    name="dbUser"
                    value={formData.dbUser}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    placeholder="redflag"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbPassword" className="block text-sm font-medium text-gray-700 mb-1">
                    Database Password
                  </label>
                  <div className="relative">
                    <input
                      type={showDbPassword ? 'text' : 'password'}
                      id="dbPassword"
                      name="dbPassword"
                      value={formData.dbPassword}
                      onChange={handleInputChange}
                      className="block w-full px-3 py-2 pr-10 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                      placeholder="Enter database password"
                      required
                    />
                    <button
                      type="button"
                      className="absolute inset-y-0 right-0 pr-3 flex items-center"
                      onClick={() => setShowDbPassword(!showDbPassword)}
                    >
                      {showDbPassword ? (
                        <EyeOff className="h-4 w-4 text-gray-400" />
                      ) : (
                        <Eye className="h-4 w-4 text-gray-400" />
                      )}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            {/* Server Configuration */}
            <div>
              <div className="flex items-center mb-4">
                <Settings className="h-5 w-5 text-indigo-600 mr-2" />
                <h3 className="text-lg font-semibold text-gray-900">Server Configuration</h3>
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="serverHost" className="block text-sm font-medium text-gray-700 mb-1">
                    Server Host
                  </label>
                  <input
                    type="text"
                    id="serverHost"
                    name="serverHost"
                    value={formData.serverHost}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    placeholder="0.0.0.0"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="serverPort" className="block text-sm font-medium text-gray-700 mb-1">
                    Server Port
                  </label>
                  <input
                    type="number"
                    id="serverPort"
                    name="serverPort"
                    value={formData.serverPort}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    placeholder="8080"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="maxSeats" className="block text-sm font-medium text-gray-700 mb-1">
                    Maximum Agent Seats
                  </label>
                  <input
                    type="number"
                    id="maxSeats"
                    name="maxSeats"
                    value={formData.maxSeats}
                    onChange={handleInputChange}
                    className="block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                    min="1"
                    max="1000"
                    placeholder="50"
                    required
                  />
                  <p className="mt-1 text-xs text-gray-500">Security limit for agent registration</p>
                </div>
              </div>
            </div>

            {/* Submit Button */}
            <div className="pt-6 border-t border-gray-200">
              <button
                type="submit"
                disabled={isLoading}
                className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isLoading ? (
                  <div className="flex items-center">
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                    Configuring RedFlag Server...
                  </div>
                ) : (
                  <div className="flex items-center">
                    <Shield className="w-4 h-4 mr-2" />
                    Configure RedFlag Server
                  </div>
                )}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
};

export default Setup;