import React, { useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Database, Settings, Shield, User, Eye, EyeOff, CheckCircle, Key } from 'lucide-react';
import { toast } from 'react-hot-toast';
import { setupApi } from '@/lib/api';
import { useAuthStore } from '@/lib/store';
import { cn } from '@/lib/utils';

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
  publicURL: string;
  maxSeats: string;
}

interface SigningKeys {
  public_key: string;
  private_key: string;
  fingerprint: string;
  algorithm: string;
}

// ---- local primitives (reused within Setup) ----

/** Section header with icon — repeated 4× in the form. */
const FormSection: React.FC<{ icon: React.ElementType; title: string; children: React.ReactNode }> = ({
  icon: Icon, title, children,
}) => (
  <div>
    <div className="flex items-center mb-4">
      <Icon className="h-5 w-5 text-indigo-600 mr-2" />
      <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
    </div>
    {children}
  </div>
);

type AlertKind = 'error' | 'info' | 'success' | 'warning';

const ALERT_STYLES: Record<AlertKind, string> = {
  error:   'bg-red-50 border-red-200 text-red-800',
  info:    'bg-blue-50 border-blue-200 text-blue-800',
  success: 'bg-green-50 border-green-200 text-green-800',
  warning: 'bg-yellow-50 border-yellow-200 text-yellow-800',
};

const Alert: React.FC<{ kind: AlertKind; children: React.ReactNode; className?: string }> = ({
  kind, children, className,
}) => (
  <div className={cn('border rounded-md p-3 text-sm', ALERT_STYLES[kind], className)}>
    {children}
  </div>
);

/** Standard text input with label — 15+ instances in the form. */
const TextField: React.FC<{
  id: string; label: string; name: string; value: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  type?: string; placeholder?: string; required?: boolean; readOnly?: boolean;
  rightButton?: React.ReactNode;
}> = ({ id, label, name, value, onChange, type = 'text', placeholder, required, readOnly, rightButton }) => (
  <div>
    <label htmlFor={id} className="block text-sm font-medium text-gray-700 mb-1">{label}</label>
    <div className="relative">
      <input
        type={type} id={id} name={name} value={value} onChange={onChange}
        placeholder={placeholder} required={required} readOnly={readOnly}
        className={cn(
          'block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm sm:text-sm',
          'placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500',
          readOnly && 'bg-gray-100',
          rightButton && 'pr-10',
        )}
      />
      {rightButton}
    </div>
  </div>
);

const Setup: React.FC = () => {
  const navigate = useNavigate();
  const { logout } = useAuthStore();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [envContent, setEnvContent] = useState<string | null>(null);
  const [showSuccess, setShowSuccess] = useState(false);
  const [visiblePasswords, setVisiblePasswords] = useState<Set<string>>(new Set());
  const [signingKeys, setSigningKeys] = useState<SigningKeys | null>(null);
  const [generatingKeys, setGeneratingKeys] = useState(false);

  const togglePassword = useCallback((field: string) => {
    setVisiblePasswords(prev => {
      const next = new Set(prev);
      next.has(field) ? next.delete(field) : next.add(field);
      return next;
    });
  }, []);

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
    publicURL: typeof window !== 'undefined' ? window.location.origin : '',
    maxSeats: '50',
  });

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleGenerateKeys = async () => {
    setGeneratingKeys(true);
    try {
      const response = await fetch('/api/setup/generate-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });
      if (!response.ok) {
        throw new Error('Failed to generate keys');
      }
      const keys: SigningKeys = await response.json();
      setSigningKeys(keys);
      toast.success('Signing keys generated successfully!');
    } catch (error: any) {
      toast.error(error.message || 'Failed to generate keys');
      setError('Failed to generate signing keys');
    } finally {
      setGeneratingKeys(false);
    }
  };

  const validateForm = (): boolean => {
    // BUG-004: signing keys must exist before configure runs. The previous
    // flow accepted an empty signingKeys state and produced a server config
    // with no REDFLAG_SIGNING_PRIVATE_KEY, which then failed silently at
    // first agent registration. Surface the dependency up front.
    if (!signingKeys) {
      setError('Generate signing keys before configuring the server (see "Generate Signing Keys" above).');
      return false;
    }
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
    if (!formData.publicURL.trim()) {
      setError('Agent-facing server URL is required');
      return false;
    }
    try {
      const publicURL = new URL(formData.publicURL);
      if (!['http:', 'https:'].includes(publicURL.protocol)) {
        setError('Agent-facing server URL must start with http:// or https://');
        return false;
      }
    } catch {
      setError('Agent-facing server URL must be a valid URL');
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
      const result = await setupApi.configure({
        ...formData,
        signingPrivateKey: signingKeys?.private_key || '',
        signingPublicKey: signingKeys?.public_key || '',
      });

      const configContent = generateEnvContent(result);

      setEnvContent(configContent || null);
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

  const generateEnvContent = (result: any): string => {
    // Server-side createSharedEnvContentForDisplay already embeds the signing
    // keys in envContent — appending here would duplicate the key (BUG-006).
    return result.envContent || '';
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
              <div className="mt-3 alert alert-success rounded-md p-3">
                <p className="text-sm text-green-800">
                  <strong>Important:</strong> Save these credentials securely. You'll use them to login to the RedFlag dashboard.
                </p>
              </div>
            </div>

            {/* Configuration Content Section */}
            {envContent && (
              <div className="mb-6">
                <h3 className="text-lg font-semibold text-gray-900 mb-3">
                  Environment Configuration (.env)
                </h3>

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
                    toast.success('.env content copied to clipboard!');
                  }}
                  className="mt-3 w-full flex justify-center py-2 px-4 border border-transparent rounded-md text-sm font-medium text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500"
                >
                  Copy .env Content
                </button>
                <div className="mt-3 alert alert-info rounded-md p-3">
                  <p className="text-sm text-blue-800">
                    <strong>Next Steps:</strong> Save this content to <code className="code code-info">config/.env</code> and run <code className="code code-info">docker compose down && docker compose up -d</code> to apply the configuration.
                  </p>
                </div>
                <div className="mt-3 alert alert-warning rounded-md p-3">
                  <p className="text-sm text-yellow-800">
                    <strong>Security Note:</strong> The <code className="code code-warning">config/.env</code> file contains sensitive credentials. Ensure it has restricted permissions (<code className="code code-warning">chmod 600</code>) and is excluded from version control.
                  </p>
                </div>
              </div>
            )}


            {/* Next Steps */}
            <div className="border-t border-gray-200 pt-6">
              <h3 className="text-lg font-semibold text-gray-900 mb-3">Next Steps</h3>
              <ol className="list-decimal list-inside space-y-2 text-sm text-gray-600">
                <li>Copy the .env content using the green button above</li>
                <li>Save it to <code className="code code-neutral">config/.env</code></li>
                <li>Run <code className="code code-neutral">docker compose down && docker compose up -d</code></li>
                <li>Login to the dashboard with your admin username and password</li>
              </ol>
            </div>

            <div className="mt-6 pt-6 border-t border-gray-200 space-y-3">
              <div className="alert alert-warning rounded-md p-3">
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
            {error && <Alert kind="error">{error}</Alert>}

            <FormSection icon={User} title="Administrator Account">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <TextField id="adminUser" label="Admin Username" name="adminUser"
                  value={formData.adminUser} onChange={handleInputChange} placeholder="admin" required />
                <TextField id="adminPassword" label="Admin Password" name="adminPassword"
                  value={formData.adminPassword} onChange={handleInputChange}
                  type={visiblePasswords.has('admin') ? 'text' : 'password'}
                  placeholder="Enter secure password" required
                  rightButton={
                    <button type="button" className="absolute inset-y-0 right-0 pr-3 flex items-center"
                      onClick={() => togglePassword('admin')}>
                      {visiblePasswords.has('admin')
                        ? <EyeOff className="h-4 w-4 text-gray-400" />
                        : <Eye className="h-4 w-4 text-gray-400" />}
                    </button>
                  } />
              </div>
            </FormSection>

            <FormSection icon={Key} title="Security Keys">
              <Alert kind="info" className="mb-4">
                Generate Ed25519 signing keys for secure agent updates.
                <strong> Save the private key securely</strong> — it will be included in your configuration.
              </Alert>
              {!signingKeys ? (
                <button type="button" onClick={handleGenerateKeys} disabled={generatingKeys}
                  className="w-full py-2 px-4 border border-indigo-600 text-indigo-600 rounded-md hover:bg-indigo-50 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center">
                  {generatingKeys ? (
                    <><div className="animate-spin rounded-full h-4 w-4 border-b-2 border-indigo-600 mr-2" />Generating Keys...</>
                  ) : (
                    <><Key className="h-4 w-4 mr-2" />Generate Signing Keys</>
                  )}
                </button>
              ) : (
                <div className="space-y-3">
                  <TextField id="keyFingerprint" label="Public Key Fingerprint" name="keyFingerprint"
                    value={signingKeys.fingerprint} onChange={() => {}} readOnly />
                  <TextField id="keyAlgorithm" label="Algorithm" name="keyAlgorithm"
                    value={signingKeys.algorithm.toUpperCase()} onChange={() => {}} readOnly />
                  <Alert kind="success">
                    Keys generated! Private key will be securely included in your configuration file.
                  </Alert>
                </div>
              )}
            </FormSection>

            <FormSection icon={Database} title="Database Configuration">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <TextField id="dbHost" label="Database Host" name="dbHost"
                  value={formData.dbHost} onChange={handleInputChange} placeholder="postgres" required />
                <TextField id="dbPort" label="Database Port" name="dbPort"
                  value={formData.dbPort} onChange={handleInputChange} type="number" required />
                <TextField id="dbName" label="Database Name" name="dbName"
                  value={formData.dbName} onChange={handleInputChange} placeholder="redflag" required />
                <TextField id="dbUser" label="Database User" name="dbUser"
                  value={formData.dbUser} onChange={handleInputChange} placeholder="redflag" required />
                <TextField id="dbPassword" label="Database Password" name="dbPassword"
                  value={formData.dbPassword} onChange={handleInputChange}
                  type={visiblePasswords.has('db') ? 'text' : 'password'}
                  placeholder="Enter database password" required
                  rightButton={
                    <button type="button" className="absolute inset-y-0 right-0 pr-3 flex items-center"
                      onClick={() => togglePassword('db')}>
                      {visiblePasswords.has('db')
                        ? <EyeOff className="h-4 w-4 text-gray-400" />
                        : <Eye className="h-4 w-4 text-gray-400" />}
                    </button>
                  } />
              </div>
            </FormSection>

            <FormSection icon={Settings} title="Server Configuration">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <TextField id="serverHost" label="Server Host" name="serverHost"
                  value={formData.serverHost} onChange={handleInputChange} placeholder="0.0.0.0" required />
                <TextField id="serverPort" label="Server Port" name="serverPort"
                  value={formData.serverPort} onChange={handleInputChange} type="number" placeholder="8080" required />
                <TextField id="maxSeats" label="Maximum Agent Seats" name="maxSeats"
                  value={formData.maxSeats} onChange={handleInputChange} type="number" required
                  placeholder="50" />
                <p className="mt-1 text-xs text-gray-500">Security limit for agent registration</p>
                <div className="sm:col-span-2">
                  <TextField id="publicURL" label="Agent-facing Server URL" name="publicURL"
                    value={formData.publicURL} onChange={handleInputChange} type="url"
                    placeholder="http://redflag.example.com:8080" required />
                  <p className="mt-1 text-xs text-gray-500">Used in generated install commands and agent callbacks</p>
                </div>
              </div>
            </FormSection>

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
