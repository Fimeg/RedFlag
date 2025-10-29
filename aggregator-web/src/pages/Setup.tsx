import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { XCircle } from 'lucide-react';
import { toast } from 'react-hot-toast';
import { setupApi } from '@/lib/api';

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
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

    setIsLoading(true);

    try {
      const result = await setupApi.configure(formData);

      toast.success(result.message || 'Configuration saved successfully!');

      if (result.restart) {
        // Server is restarting, wait for it to come back online
        setTimeout(() => {
          navigate('/login');
        }, 5000); // Give server time to restart
      } else {
        // No restart, redirect immediately
        setTimeout(() => {
          navigate('/login');
        }, 2000);
      }

    } catch (error: any) {
      console.error('Setup error:', error);
      const errorMessage = error.response?.data?.error || error.message || 'Setup failed';
      setError(errorMessage);
      toast.error(errorMessage);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="px-4 sm:px-6 lg:px-8">
      <div className="max-w-4xl mx-auto">
        <div className="py-8">
          <h2 className="text-2xl font-bold text-gray-900">Server Setup</h2>
          <p className="mt-1 text-sm text-gray-600">
            Configure your update management server
          </p>
        </div>

        <div className="bg-white shadow rounded-lg">
          <form onSubmit={handleSubmit} className="divide-y divide-gray-200">
            {/* Error Display */}
            {error && (
              <div className="px-6 py-4 bg-red-50">
                <div className="flex">
                  <div className="flex-shrink-0">
                    <XCircle className="h-5 w-5 text-red-400" />
                  </div>
                  <div className="ml-3">
                    <h3 className="text-sm font-medium text-red-800">{error}</h3>
                  </div>
                </div>
              </div>
            )}

            {/* Admin Account */}
            <div className="px-6 py-6">
              <h3 className="text-lg font-medium text-gray-900 mb-4">Admin Account</h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="adminUser" className="block text-sm font-medium text-gray-700">
                    Admin Username
                  </label>
                  <input
                    type="text"
                    id="adminUser"
                    name="adminUser"
                    value={formData.adminUser}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="adminPassword" className="block text-sm font-medium text-gray-700">
                    Admin Password
                  </label>
                  <input
                    type="password"
                    id="adminPassword"
                    name="adminPassword"
                    value={formData.adminPassword}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
              </div>
            </div>

            {/* Database Configuration */}
            <div className="px-6 py-6">
              <h3 className="text-lg font-medium text-gray-900 mb-4">Database Configuration</h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <div>
                  <label htmlFor="dbHost" className="block text-sm font-medium text-gray-700">
                    Database Host
                  </label>
                  <input
                    type="text"
                    id="dbHost"
                    name="dbHost"
                    value={formData.dbHost}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbPort" className="block text-sm font-medium text-gray-700">
                    Database Port
                  </label>
                  <input
                    type="number"
                    id="dbPort"
                    name="dbPort"
                    value={formData.dbPort}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbName" className="block text-sm font-medium text-gray-700">
                    Database Name
                  </label>
                  <input
                    type="text"
                    id="dbName"
                    name="dbName"
                    value={formData.dbName}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbUser" className="block text-sm font-medium text-gray-700">
                    Database User
                  </label>
                  <input
                    type="text"
                    id="dbUser"
                    name="dbUser"
                    value={formData.dbUser}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="dbPassword" className="block text-sm font-medium text-gray-700">
                    Database Password
                  </label>
                  <input
                    type="password"
                    id="dbPassword"
                    name="dbPassword"
                    value={formData.dbPassword}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
              </div>
            </div>

            {/* Server Configuration */}
            <div className="px-6 py-6">
              <h3 className="text-lg font-medium text-gray-900 mb-4">Server Configuration</h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="serverHost" className="block text-sm font-medium text-gray-700">
                    Server Host
                  </label>
                  <input
                    type="text"
                    id="serverHost"
                    name="serverHost"
                    value={formData.serverHost}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="serverPort" className="block text-sm font-medium text-gray-700">
                    Server Port
                  </label>
                  <input
                    type="number"
                    id="serverPort"
                    name="serverPort"
                    value={formData.serverPort}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    required
                  />
                </div>
                <div>
                  <label htmlFor="maxSeats" className="block text-sm font-medium text-gray-700">
                    Maximum Agent Seats
                  </label>
                  <input
                    type="number"
                    id="maxSeats"
                    name="maxSeats"
                    value={formData.maxSeats}
                    onChange={handleInputChange}
                    className="mt-1 block w-full border-gray-300 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
                    min="1"
                    max="1000"
                    required
                  />
                  <p className="mt-1 text-xs text-gray-500">Security limit for agent registration</p>
                </div>
              </div>
            </div>

            {/* Submit Button */}
            <div className="px-6 py-4 bg-gray-50">
              <button
                type="submit"
                disabled={isLoading}
                className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isLoading ? (
                  <div className="flex items-center">
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
                    Configuring...
                  </div>
                ) : (
                  'Configure Server'
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