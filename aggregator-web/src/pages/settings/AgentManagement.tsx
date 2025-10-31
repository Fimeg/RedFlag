import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Download,
  Terminal,
  Copy,
  Check,
  Shield,
  Server,
  Monitor,
  AlertTriangle,
  ExternalLink,
  RefreshCw,
  Code,
  FileText,
  Package
} from 'lucide-react';
import { useRegistrationTokens } from '@/hooks/useRegistrationTokens';
import { toast } from 'react-hot-toast';

const AgentManagement: React.FC = () => {
  const navigate = useNavigate();
  const [copiedCommand, setCopiedCommand] = useState<string | null>(null);
  const [selectedPlatform, setSelectedPlatform] = useState<string>('linux');
  const { data: tokens, isLoading: tokensLoading } = useRegistrationTokens({ is_active: true });

  const platforms = [
    {
      id: 'linux',
      name: 'Linux',
      icon: Server,
      description: 'For Ubuntu, Debian, RHEL, CentOS, AlmaLinux, Rocky Linux',
      downloadUrl: '/api/v1/downloads/linux-amd64',
      installScript: '/api/v1/install/linux',
      extensions: ['amd64'],
      color: 'orange'
    },
    {
      id: 'windows',
      name: 'Windows',
      icon: Monitor,
      description: 'For Windows 10/11, Windows Server 2019/2022',
      downloadUrl: '/api/v1/downloads/windows-amd64',
      installScript: '/api/v1/install/windows',
      extensions: ['amd64'],
      color: 'blue'
    }
  ];

  const getServerUrl = () => {
    return `${window.location.protocol}//${window.location.host}`;
  };

  const getActiveToken = () => {
    // Defensive null checking to prevent crashes
    if (!tokens || !tokens.tokens || !Array.isArray(tokens.tokens) || tokens.tokens.length === 0) {
      return 'YOUR_REGISTRATION_TOKEN';
    }
    return tokens.tokens[0]?.token || 'YOUR_REGISTRATION_TOKEN';
  };

  const generateInstallCommand = (platform: typeof platforms[0]) => {
    const serverUrl = getServerUrl();
    const token = getActiveToken();

    if (platform.id === 'linux') {
      if (token !== 'YOUR_REGISTRATION_TOKEN') {
        return `curl -sfL ${serverUrl}${platform.installScript} | sudo bash -s -- ${token}`;
      } else {
        return `curl -sfL ${serverUrl}${platform.installScript} | sudo bash`;
      }
    } else if (platform.id === 'windows') {
      if (token !== 'YOUR_REGISTRATION_TOKEN') {
        return `iwr ${serverUrl}${platform.installScript} -OutFile install.bat; .\\install.bat ${token}`;
      } else {
        return `iwr ${serverUrl}${platform.installScript} -OutFile install.bat; .\\install.bat`;
      }
    }
    return '';
  };

  const generateManualCommand = (platform: typeof platforms[0]) => {
    const serverUrl = getServerUrl();
    const token = getActiveToken();

    if (platform.id === 'windows') {
      if (token !== 'YOUR_REGISTRATION_TOKEN') {
        return `# Download and run as Administrator with token\niwr ${serverUrl}${platform.installScript} -OutFile install.bat\n.\\install.bat ${token}`;
      } else {
        return `# Download and run as Administrator\niwr ${serverUrl}${platform.installScript} -OutFile install.bat\n.\\install.bat`;
      }
    } else {
      if (token !== 'YOUR_REGISTRATION_TOKEN') {
        return `# Download and run as root with token\ncurl -sfL ${serverUrl}${platform.installScript} | sudo bash -s -- ${token}`;
      } else {
        return `# Download and run as root\ncurl -sfL ${serverUrl}${platform.installScript} | sudo bash`;
      }
    }
  };

  const copyToClipboard = async (text: string, commandId: string) => {
    try {
      if (!text || text.trim() === '') {
        toast.error('No command to copy');
        return;
      }
      await navigator.clipboard.writeText(text);
      setCopiedCommand(commandId);
      toast.success('Command copied to clipboard!');
      setTimeout(() => setCopiedCommand(null), 2000);
    } catch (error) {
      console.error('Copy failed:', error);
      toast.error('Failed to copy command. Please copy manually.');
    }
  };

  const handleDownload = (platform: typeof platforms[0]) => {
    const link = document.createElement('a');
    link.href = `${getServerUrl()}${platform.downloadUrl}`;
    link.download = `redflag-agent-${platform.id}-amd64${platform.id === 'windows' ? '.exe' : ''}`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    toast.success(`Download started for ${platform.name} agent`);
  };

  const selectedPlatformData = platforms.find(p => p.id === selectedPlatform);

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      <button
        onClick={() => navigate('/settings')}
        className="text-sm text-gray-500 hover:text-gray-700 mb-4"
      >
        ← Back to Settings
      </button>

      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-3xl font-bold text-gray-900">Agent Management</h1>
            <p className="mt-2 text-gray-600">Deploy and configure RedFlag agents across your infrastructure</p>
          </div>
          <Link
            to="/settings/tokens"
            className="flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
          >
            <Shield className="w-4 h-4" />
            Manage Tokens
          </Link>
        </div>
      </div>

      {/* Token Status */}
      <div className="bg-blue-50 border border-blue-200 rounded-lg p-6 mb-8">
        <div className="flex items-start gap-4">
          <Shield className="w-6 h-6 text-blue-600 mt-1" />
          <div className="flex-1">
            <h3 className="font-semibold text-blue-900 mb-2">Registration Token Required</h3>
            <p className="text-blue-700 mb-4">
              Agents need a registration token to enroll with the server. You have {tokens?.tokens?.length || 0} active token(s).
            </p>
            {!tokens?.tokens || tokens.tokens.length === 0 ? (
              <Link
                to="/settings/tokens"
                className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
              >
                <Shield className="w-4 h-4" />
                Generate Registration Token
              </Link>
            ) : (
              <div className="flex items-center gap-4">
                <div>
                  <p className="text-sm text-blue-600 font-medium">Active Token:</p>
                  <code className="text-xs bg-blue-100 px-2 py-1 rounded">{tokens?.tokens?.[0]?.token || 'N/A'}</code>
                </div>
                <Link
                  to="/settings/tokens"
                  className="text-sm text-blue-600 hover:text-blue-800 underline"
                >
                  View all tokens →
                </Link>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Platform Selection */}
      <div className="bg-white border border-gray-200 rounded-lg p-6 mb-8">
        <h2 className="text-xl font-semibold text-gray-900 mb-6">1. Select Target Platform</h2>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {platforms.map((platform) => {
            const Icon = platform.icon;
            return (
              <button
                key={platform.id}
                onClick={() => setSelectedPlatform(platform.id)}
                className={`p-6 border-2 rounded-lg transition-all ${
                  selectedPlatform === platform.id
                    ? 'border-blue-500 bg-blue-50'
                    : 'border-gray-200 hover:border-gray-300'
                }`}
              >
                <div className="flex items-center justify-between mb-4">
                  <Icon className={`w-8 h-8 ${
                    platform.id === 'linux' ? 'text-orange-600' :
                    platform.id === 'windows' ? 'text-blue-600' : 'text-gray-600'
                  }`} />
                  {selectedPlatform === platform.id && (
                    <Check className="w-5 h-5 text-blue-600" />
                  )}
                </div>
                <h3 className="font-semibold text-gray-900 mb-2">{platform.name}</h3>
                <p className="text-sm text-gray-600">{platform.description}</p>
              </button>
            );
          })}
        </div>
      </div>

      {/* Installation Methods */}
      {selectedPlatformData && (
        <div className="space-y-8">
          {/* One-Liner Installation */}
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-xl font-semibold text-gray-900">2. One-Liner Installation (Recommended)</h2>
                <p className="text-gray-600 mt-1">
                  Automatically downloads and configures the agent for {selectedPlatformData.name}
                </p>
              </div>
              <Terminal className="w-6 h-6 text-gray-400" />
            </div>

            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">
                  Installation Command {selectedPlatformData.id === 'windows' && <span className="text-blue-600">(Run in PowerShell as Administrator)</span>}
                </label>
                <div className="relative">
                  <pre className="bg-gray-900 text-gray-100 p-4 rounded-lg overflow-x-auto">
                    <code>{generateInstallCommand(selectedPlatformData)}</code>
                  </pre>
                  <button
                    onClick={() => copyToClipboard(generateInstallCommand(selectedPlatformData), 'one-liner')}
                    className="absolute top-2 right-2 p-2 bg-gray-700 text-white rounded hover:bg-gray-600 transition-colors"
                  >
                    {copiedCommand === 'one-liner' ? (
                      <Check className="w-4 h-4" />
                    ) : (
                      <Copy className="w-4 h-4" />
                    )}
                  </button>
                </div>
              </div>

              <div className="bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                <div className="flex items-start gap-3">
                  <AlertTriangle className="w-5 h-5 text-yellow-600 mt-0.5" />
                  <div>
                    <h4 className="font-medium text-yellow-900">Before Running</h4>
                    <ul className="text-sm text-yellow-700 mt-1 space-y-1">
                      {selectedPlatformData.id === 'windows' ? (
                        <>
                          <li>• Open <strong>PowerShell as Administrator</strong></li>
                          <li>• The script will download and install the agent to <code className="bg-yellow-100 px-1 rounded">%ProgramFiles%\RedFlag</code></li>
                          <li>• A Windows service will be created and started automatically</li>
                          <li>• Script is idempotent - safe to re-run for upgrades</li>
                        </>
                      ) : (
                        <>
                          <li>• Run this command as <strong>root</strong> (use sudo)</li>
                          <li>• The script will create a dedicated <code className="bg-yellow-100 px-1 rounded">redflag-agent</code> user</li>
                          <li>• Limited sudo access will be configured via <code className="bg-yellow-100 px-1 rounded">/etc/sudoers.d/redflag-agent</code></li>
                          <li>• Systemd service will be installed and enabled automatically</li>
                          <li>• Script is idempotent - safe to re-run for upgrades</li>
                        </>
                      )}
                    </ul>
                  </div>
                </div>
              </div>
            </div>
          </div>

          {/* Security Information */}
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-xl font-semibold text-gray-900">3. Security Information</h2>
                <p className="text-gray-600 mt-1">
                  Understanding the security model and installation details
                </p>
              </div>
              <Shield className="w-6 h-6 text-gray-400" />
            </div>

            <div className="space-y-6">
              <div>
                <h4 className="font-medium text-gray-900 mb-3">🛡️ Security Model</h4>
                <p className="text-sm text-gray-600 mb-4">
                  The installation script follows the principle of least privilege by creating a dedicated system user with minimal permissions:
                </p>
                <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 space-y-2">
                  <div className="flex items-center gap-2">
                    <div className="w-2 h-2 bg-blue-600 rounded-full"></div>
                    <span className="text-sm text-blue-800"><strong>System User:</strong> <code className="bg-blue-100 px-1 rounded">redflag-agent</code> with no login shell</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-2 h-2 bg-blue-600 rounded-full"></div>
                    <span className="text-sm text-blue-800"><strong>Sudo Access:</strong> Limited to package management commands only</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-2 h-2 bg-blue-600 rounded-full"></div>
                    <span className="text-sm text-blue-800"><strong>Systemd Service:</strong> Runs with security hardening (ProtectSystem, ProtectHome)</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="w-2 h-2 bg-blue-600 rounded-full"></div>
                    <span className="text-sm text-blue-800"><strong>Configuration:</strong> Secured in <code className="bg-blue-100 px-1 rounded">/etc/aggregator/config.json</code> with restricted permissions</span>
                  </div>
                </div>
              </div>

              <div>
                <h4 className="font-medium text-gray-900 mb-3">📁 Installation Files</h4>
                <div className="bg-gray-50 rounded-lg p-4">
                  <pre className="text-sm text-gray-700 space-y-1">
{`Binary:     /usr/local/bin/redflag-agent
Config:     /etc/aggregator/config.json
Service:    /etc/systemd/system/redflag-agent.service
Sudoers:    /etc/sudoers.d/redflag-agent
Home Dir:   /var/lib/redflag-agent
Logs:       journalctl -u redflag-agent`}
                  </pre>
                </div>
              </div>

              <div>
                <h4 className="font-medium text-gray-900 mb-3">⚙️ Sudoers Configuration</h4>
                <p className="text-sm text-gray-600 mb-2">
                  The agent gets sudo access only for these specific commands:
                </p>
                <div className="bg-gray-50 rounded-lg p-4">
                  <pre className="text-xs text-gray-700 overflow-x-auto">
{`# APT (Debian/Ubuntu)
/usr/bin/apt-get update
/usr/bin/apt-get install -y *
/usr/bin/apt-get upgrade -y *
/usr/bin/apt-get install --dry-run --yes *

# DNF (RHEL/Fedora/Rocky/Alma)
/usr/bin/dnf makecache
/usr/bin/dnf install -y *
/usr/bin/dnf upgrade -y *
/usr/bin/dnf install --assumeno --downloadonly *

# Docker
/usr/bin/docker pull *
/usr/bin/docker image inspect *
/usr/bin/docker manifest inspect *`}
                  </pre>
                </div>
              </div>

              <div>
                <h4 className="font-medium text-gray-900 mb-3">🔄 Updates and Upgrades</h4>
                <p className="text-sm text-gray-600">
                  The installation script is <strong>idempotent</strong> - it's safe to run multiple times.
                  RedFlag agents update themselves automatically when new versions are released.
                  If you need to manually reinstall or upgrade, simply run the same one-liner command.
                </p>
              </div>
            </div>
          </div>

          {/* Advanced Configuration */}
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <div className="flex items-center justify-between mb-6">
              <div>
                <h2 className="text-xl font-semibold text-gray-900">4. Advanced Configuration</h2>
                <p className="text-gray-600 mt-1">
                  Additional agent configuration options
                </p>
              </div>
              <Code className="w-6 h-6 text-gray-400" />
            </div>

            <div className="space-y-6">
              {/* Configuration Options */}
              <div>
                <h4 className="font-medium text-gray-900 mb-3">Command Line Options</h4>
                <div className="bg-gray-50 rounded-lg p-4">
                  <pre className="text-sm text-gray-700">
{`./redflag-agent [options]

Options:
  --server <url>        Server URL (default: http://localhost:8080)
  --token <token>       Registration token
  --proxy-http <url>    HTTP proxy URL
  --proxy-https <url>   HTTPS proxy URL
  --log-level <level>   Log level (debug, info, warn, error)
  --organization <name> Organization name
  --tags <tags>         Comma-separated tags
  --name <display>      Display name for the agent
  --insecure-tls       Skip TLS certificate verification`}
                  </pre>
                </div>
              </div>

              {/* Environment Variables */}
              <div>
                <h4 className="font-medium text-gray-900 mb-3">Environment Variables</h4>
                <div className="bg-gray-50 rounded-lg p-4">
                  <pre className="text-sm text-gray-700">
{`REDFLAG_SERVER_URL="https://your-server.com"
REDFLAG_REGISTRATION_TOKEN="your-token-here"
REDFLAG_HTTP_PROXY="http://proxy.company.com:8080"
REDFLAG_HTTPS_PROXY="https://proxy.company.com:8080"
REDFLAG_NO_PROXY="localhost,127.0.0.1"
REDFLAG_LOG_LEVEL="info"
REDFLAG_ORGANIZATION="IT Department"`}
                  </pre>
                </div>
              </div>

              {/* Configuration File */}
              <div>
                <h4 className="font-medium text-gray-900 mb-3">Configuration File</h4>
                <p className="text-sm text-gray-600 mb-3">
                  After installation, the agent configuration is stored at <code>/etc/aggregator/config.json</code> (Linux) or
                  <code>%ProgramData%\RedFlag\config.json</code> (Windows):
                </p>
                <div className="bg-gray-50 rounded-lg p-4">
                  <pre className="text-sm text-gray-700 overflow-x-auto">
{`{
  "server_url": "https://your-server.com",
  "registration_token": "your-token-here",
  "proxy": {
    "enabled": true,
    "http": "http://proxy.company.com:8080",
    "https": "https://proxy.company.com:8080",
    "no_proxy": "localhost,127.0.0.1"
  },
  "network": {
    "timeout": "30s",
    "retry_count": 3,
    "retry_delay": "5s"
  },
  "tls": {
    "insecure_skip_verify": false
  },
  "logging": {
    "level": "info",
    "max_size": 100,
    "max_backups": 3
  },
  "tags": ["production", "webserver"],
  "organization": "IT Department",
  "display_name": "Web Server 01"
}`}
                  </pre>
                </div>
              </div>
            </div>
          </div>

          {/* Next Steps */}
          <div className="bg-green-50 border border-green-200 rounded-lg p-6">
            <div className="flex items-start gap-4">
              <Check className="w-6 h-6 text-green-600 mt-1" />
              <div>
                <h3 className="font-semibold text-green-900 mb-2">Next Steps</h3>
                <ol className="text-sm text-green-800 space-y-2">
                  <li>1. Deploy agents to your target machines using the methods above</li>
                  <li>2. Monitor agent registration in the <Link to="/agents" className="underline">Agents dashboard</Link></li>
                  <li>3. Configure update policies and scanning schedules</li>
                  <li>4. Review agent status and system information</li>
                </ol>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default AgentManagement;