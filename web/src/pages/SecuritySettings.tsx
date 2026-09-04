import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Shield,
  Key,
  FileText,
  AlertTriangle,
  CheckCircle,
  XCircle,
  Download,
  Upload,
  Save,
  RotateCcw,
  Eye,
  EyeOff,
  Activity,
  History,
  Terminal,
  Server
} from 'lucide-react';

import { useSecuritySettings, useSecurityValidation } from '@/hooks/useSecuritySettings';
import { SecuritySettings as SecuritySettingsType, SecuritySetting, ConfirmationDialogState } from '@/types/security';
import SecurityStatusCard from '@/components/security/SecurityStatusCard';
import SecurityCategorySection from '@/components/security/SecurityCategorySection';
import SecurityEvents from '@/components/security/SecurityEvents';
import SigningKeyRoster from '@/components/security/SigningKeyRoster';
import Modal from '@/components/primitives/Modal';

const SecuritySettings: React.FC = () => {
  const navigate = useNavigate();
  const { tab = 'overview' } = useParams();

  const {
    settings,
    securityOverview,
    loading,
    saving,
    error,
    updateSetting,
    updateSettings,
    exportSettings,
    importSettings,
    resetToDefaults,
    refetch,
  } = useSecuritySettings();

  const { validateAll } = useSecurityValidation();

  // State management
  const [activeTab, setActiveTab] = useState(tab);
  const [localSettings, setLocalSettings] = useState<SecuritySettingsType | null>(null);
  const [hasChanges, setHasChanges] = useState(false);
  const [validationErrors, setValidationErrors] = useState<Record<string, string>>({});
  const [confirmationDialog, setConfirmationDialog] = useState<ConfirmationDialogState>({
    isOpen: false,
    title: '',
    message: '',
    severity: 'warning',
    requiresConfirmation: false,
    onConfirm: () => {},
    onCancel: () => {},
  });
  const [confirmationTyped, setConfirmationTyped] = useState('');
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Fresh typed-confirmation field every time the dialog opens or closes.
  useEffect(() => {
    setConfirmationTyped('');
  }, [confirmationDialog.isOpen]);

  // Sync URL tab with state
  useEffect(() => {
    if (tab !== activeTab) {
      navigate(`/settings/security/${activeTab}`, { replace: true });
    }
  }, [activeTab, tab, navigate]);

  // Initialize local settings
  useEffect(() => {
    if (settings && !localSettings) {
      setLocalSettings(JSON.parse(JSON.stringify(settings)));
    }
  }, [settings, localSettings]);

  // Validate settings when they change
  useEffect(() => {
    if (localSettings) {
      const errors = validateAll(localSettings);
      setValidationErrors(errors);
    }
  }, [localSettings, validateAll]);

  // Tab configuration
  const tabs = [
    { id: 'overview', label: 'Overview', icon: Shield },
    { id: 'command-signing', label: 'Command Signing', icon: Terminal },
    { id: 'update-security', label: 'Update Security', icon: Download },
    { id: 'machine-binding', label: 'Machine Binding', icon: Server },
    { id: 'logging', label: 'Logging', icon: FileText },
    { id: 'key-management', label: 'Key Management', icon: Key },
    { id: 'events', label: 'Security Events', icon: Activity },
    { id: 'audit', label: 'Audit Trail', icon: History },
  ];

  // Command Signing Settings
  // Signing is fixed to the provisioned Ed25519 service; enforcement is
  // agent-local and default-closed. Only stale-key tolerance is server-tunable.
  const commandSigningSettings: SecuritySetting[] = [
    {
      key: 'stale_key_max_age_hours',
      label: 'Stale Key Tolerance (hours)',
      type: 'number',
      value: localSettings?.command_signing?.stale_key_max_age_hours ?? 168,
      min: 1,
      max: 720,
      description: 'How long an offline agent keeps trusting its cached server key before failing closed. Forward-only doctrine bounds this 1-720h; the agent enforces the ceiling.',
    },
  ];

  // Update Security Settings
  const updateSecuritySettings: SecuritySetting[] = [
    {
      key: 'enabled',
      label: 'Enable Update Security',
      type: 'toggle',
      value: localSettings?.update_security?.enabled ?? false,
      description: 'Require signed updates and nonce validation',
    },
    {
      key: 'enforcement_mode',
      label: 'Enforcement Mode',
      type: 'select',
      value: localSettings?.update_security?.enforcement_mode ?? 'strict',
      options: ['strict', 'warning', 'disabled'],
      description: 'How to handle unsigned or invalid updates',
      disabled: !localSettings?.update_security?.enabled,
    },
    {
      key: 'nonce_timeout_seconds',
      label: 'Nonce Timeout',
      type: 'slider',
      value: localSettings?.update_security?.nonce_timeout_seconds ?? 300,
      min: 60,
      max: 3600,
      step: 60,
      description: 'How long a nonce is valid (in seconds)',
      disabled: !localSettings?.update_security?.enabled,
    },
    {
      key: 'require_signature_verification',
      label: 'Require Signature Verification',
      type: 'toggle',
      value: localSettings?.update_security?.require_signature_verification ?? true,
      description: 'Verify digital signatures on all updates',
      disabled: !localSettings?.update_security?.enabled,
    },
  ];

  // Machine Binding Settings
  const machineBindingSettings: SecuritySetting[] = [
    {
      key: 'enabled',
      label: 'Enable Machine Binding',
      type: 'toggle',
      value: localSettings?.machine_binding?.enabled ?? false,
      description: 'Bind agents to specific machine fingerprint',
    },
    {
      key: 'enforcement_mode',
      label: 'Enforcement Mode',
      type: 'select',
      value: localSettings?.machine_binding?.enforcement_mode ?? 'strict',
      options: ['strict', 'warning', 'disabled'],
      description: 'How to handle machine binding violations',
      disabled: !localSettings?.machine_binding?.enabled,
    },
    {
      key: 'binding_grace_period_minutes',
      label: 'Grace Period',
      type: 'slider',
      value: localSettings?.machine_binding?.binding_grace_period_minutes ?? 5,
      min: 1,
      max: 60,
      step: 1,
      description: 'Minutes to allow before enforcing binding',
      disabled: !localSettings?.machine_binding?.enabled,
    },
    {
      key: 'binding_components',
      label: 'Binding Components',
      type: 'checkbox-group',
      value: localSettings?.machine_binding?.binding_components ?? {},
      options: [
        { label: 'Hardware ID', value: 'hardware_id' },
        { label: 'BIOS UUID', value: 'bios_uuid' },
        { label: 'MAC Addresses', value: 'mac_addresses' },
        { label: 'CPU ID', value: 'cpu_id' },
        { label: 'Disk Serial', value: 'disk_serial' },
      ],
      description: 'Machine components to bind against',
      disabled: !localSettings?.machine_binding?.enabled,
    },
    {
      key: 'violation_action',
      label: 'Violation Action',
      type: 'select',
      value: localSettings?.machine_binding?.violation_action ?? 'block',
      options: ['block', 'warn', 'log_only'],
      description: 'Action to take on binding violations',
      disabled: !localSettings?.machine_binding?.enabled,
    },
  ];

  // Logging Settings
  const loggingSettings: SecuritySetting[] = [
    {
      key: 'log_level',
      label: 'Log Level',
      type: 'select',
      value: localSettings?.logging?.log_level ?? 'info',
      options: ['debug', 'info', 'warn', 'error'],
      description: 'Minimum severity level to log',
    },
    {
      key: 'retention_days',
      label: 'Retention Period',
      type: 'number',
      value: localSettings?.logging?.retention_days ?? 30,
      min: 1,
      max: 365,
      description: 'Days to retain security logs (1-365)',
    },
    {
      key: 'log_failures',
      label: 'Log Security Failures',
      type: 'toggle',
      value: localSettings?.logging?.log_failures ?? true,
      description: 'Record all security failures and violations',
    },
    {
      key: 'log_successes',
      label: 'Log Security Successes',
      type: 'toggle',
      value: localSettings?.logging?.log_successes ?? false,
      description: 'Record successful security operations',
    },
    {
      key: 'export_format',
      label: 'Export Format',
      type: 'select',
      value: localSettings?.logging?.export_format ?? 'json',
      options: ['json', 'csv', 'syslog'],
      description: 'Default format for log exports',
    },
  ];

  // Handle settings change
  const handleSettingChange = async (category: string, key: string, value: any) => {
    if (!localSettings) return;

    const newSettings = {
      ...localSettings,
      [category]: {
        ...localSettings[category as keyof SecuritySettingsType],
        [key]: value,
      },
    };

    setLocalSettings(newSettings);
    setHasChanges(true);

    // Auto-save for simple toggles
    if (typeof value === 'boolean') {
      try {
        await updateSetting(category, key, value);
        setHasChanges(false);
      } catch (error) {
        // Revert on error
        setLocalSettings(settings);
        setHasChanges(false);
      }
    }
  };

  // Save all changes
  const handleSaveChanges = async () => {
    if (!localSettings || !hasChanges) return;

    try {
      await updateSettings(localSettings);
      setHasChanges(false);
    } catch (error) {
      console.error('Failed to save settings:', error);
    }
  };

  // Show confirmation dialog
  const showConfirmation = (
    title: string,
    message: string,
    onConfirm: () => void,
    requiresConfirmation: boolean = false
  ) => {
    setConfirmationDialog({
      isOpen: true,
      title,
      message,
      severity: 'danger',
      requiresConfirmation,
      onConfirm: () => {
        onConfirm();
        setConfirmationDialog(prev => ({ ...prev, isOpen: false }));
      },
      onCancel: () => setConfirmationDialog(prev => ({ ...prev, isOpen: false })),
    });
  };

  // Handle reset to defaults
  const handleResetDefaults = () => {
    showConfirmation(
      'Reset to Defaults',
      'This will reset all security settings to their default values. This may affect your system security. Type "RESET" to confirm.',
      async () => {
        await resetToDefaults();
        setLocalSettings(null);
        setHasChanges(false);
      },
      true
    );
  };

  // Render tab content
  const renderTabContent = () => {
    if (!localSettings) {
      return (
        <div className="flex items-center justify-center h-64">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
        </div>
      );
    }

    switch (activeTab) {
      case 'overview':
        return (
          <div className="space-y-6">
            {securityOverview && (
              <SecurityStatusCard
                status={{
                  overall: securityOverview.overall_status === 'healthy' ? 'healthy' : securityOverview.overall_status === 'degraded' ? 'warning' : 'critical',
                  features: securityOverview.subsystems ? Object.entries(securityOverview.subsystems).map(([name, data]: [string, any]) => ({
                    name: name.replace('_', ' ').replace(/\b\w/g, l => l.toUpperCase()),
                    enabled: data.enabled,
                    status: data.status === 'healthy' ? 'healthy' : data.status === 'warning' ? 'warning' : 'error',
                    last_check: new Date().toISOString(),
                    details: data.status,
                  })) : [],
                  recent_events: securityOverview.alerts?.length || 0,
                  last_updated: new Date().toISOString(),
                }}
                onRefresh={refetch}
                onViewLogs={() => setActiveTab('audit')}
                onMonitorEvents={() => setActiveTab('events')}
                loading={loading}
              />
            )}

            {/* Quick Actions */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <button
                onClick={handleSaveChanges}
                disabled={!hasChanges || saving || Object.keys(validationErrors).length > 0}
                className="flex items-center justify-center gap-2 px-4 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Save className="w-4 h-4" />
                {saving ? 'Saving...' : 'Save Changes'}
              </button>

              <button
                onClick={exportSettings}
                className="flex items-center justify-center gap-2 px-4 py-3 bg-gray-600 text-white rounded-lg hover:bg-gray-700"
              >
                <Download className="w-4 h-4" />
                Export Settings
              </button>

              <button
                onClick={() => document.getElementById('import-file')?.click()}
                className="flex items-center justify-center gap-2 px-4 py-3 bg-gray-600 text-white rounded-lg hover:bg-gray-700"
              >
                <Upload className="w-4 h-4" />
                Import Settings
              </button>
              <input
                id="import-file"
                type="file"
                accept=".json"
                className="hidden"
                onChange={async (e) => {
                  const file = e.target.files?.[0];
                  if (file) {
                    try {
                      await importSettings(file);
                      refetch();
                    } catch (error) {
                      console.error('Import failed:', error);
                    }
                  }
                  e.target.value = '';
                }}
              />

              <button
                onClick={handleResetDefaults}
                className="flex items-center justify-center gap-2 px-4 py-3 bg-red-600 text-white rounded-lg hover:bg-red-700"
              >
                <RotateCcw className="w-4 h-4" />
                Reset to Defaults
              </button>
            </div>

            {/* Status Summary */}
            <div className="bg-white border border-gray-200 rounded-lg p-6">
              <h2 className="text-lg font-semibold text-gray-900 mb-4">Security Status</h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
                {[
                  { name: 'Command Signing', status: true },
                  { name: 'Update Security', status: localSettings.update_security.enabled },
                  { name: 'Machine Binding', status: localSettings.machine_binding.enabled },
                  { name: 'Security Logging', status: true },
                ].map((feature) => (
                  <div key={feature.name} className="flex items-center space-x-2">
                    {feature.status ? (
                      <CheckCircle className="w-5 h-5 text-green-500" />
                    ) : (
                      <XCircle className="w-5 h-5 text-gray-400" />
                    )}
                    <span className="text-sm font-medium text-gray-900">{feature.name}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        );

      case 'command-signing':
        return (
          <SecurityCategorySection
            title="Command Signing"
            description="Commands use the provisioned Ed25519 signing service. Agents enforce verification locally; this console can only tune the bounded stale-key window."
            settings={commandSigningSettings}
            onSettingChange={(key, value) => handleSettingChange('command_signing', key, value)}
            disabled={loading}
            error={error?.message ?? null}
          />
        );

      case 'update-security':
        return (
          <SecurityCategorySection
            title="Update Security"
            description="Configure security measures for agent updates including signature verification and nonce validation"
            settings={updateSecuritySettings}
            onSettingChange={(key, value) => handleSettingChange('update_security', key, value)}
            disabled={loading}
            error={error?.message ?? null}
          />
        );

      case 'machine-binding':
        return (
          <SecurityCategorySection
            title="Machine Binding"
            description="Bind agents to specific machine fingerprints to prevent unauthorized access"
            settings={machineBindingSettings}
            onSettingChange={(key, value) => handleSettingChange('machine_binding', key, value)}
            disabled={loading}
            error={error?.message ?? null}
          />
        );

      case 'logging':
        return (
          <SecurityCategorySection
            title="Security Logging"
            description="Configure logging of security events, failures, and audit trails"
            settings={loggingSettings}
            onSettingChange={(key, value) => handleSettingChange('logging', key, value)}
            disabled={loading}
            error={error?.message ?? null}
          />
        );

      case 'key-management':
        return <SigningKeyRoster />;

      case 'events':
        return <SecurityEvents />;

      case 'audit':
        return (
          <div className="bg-white border border-gray-200 rounded-lg p-6">
            <h2 className="text-xl font-semibold text-gray-900 mb-4">Audit Trail</h2>
            <p className="text-gray-600">Audit trail implementation coming soon...</p>
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-gray-900 flex items-center gap-3">
              <Shield className="w-8 h-8 text-blue-600" />
              Security Settings
            </h1>
            <p className="mt-2 text-gray-600">
              Configure security features to protect your RedFlag deployment
            </p>
          </div>
          <button
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex items-center gap-2 px-3 py-2 text-sm text-gray-600 hover:text-gray-900"
          >
            {showAdvanced ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            {showAdvanced ? 'Hide Advanced' : 'Show Advanced'}
          </button>
        </div>
      </div>

      {/* Error Display */}
      {error && (
        <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-lg">
          <div className="flex items-center gap-2">
            <AlertTriangle className="w-5 h-5 text-red-600" />
            <span className="text-red-800">{error.message}</span>
          </div>
        </div>
      )}

      {/* Validation Errors */}
      {Object.keys(validationErrors).length > 0 && (
        <div className="mb-6 p-4 bg-yellow-50 border border-yellow-200 rounded-lg">
          <div className="flex items-center gap-2 mb-2">
            <AlertTriangle className="w-5 h-5 text-yellow-600" />
            <span className="font-medium text-yellow-800">Validation Errors</span>
          </div>
          <ul className="text-sm text-yellow-700 space-y-1">
            {Object.entries(validationErrors).map(([key, error]) => (
              <li key={key}>• {error}</li>
            ))}
          </ul>
        </div>
      )}

      {/* Tabs */}
      <div className="border-b border-gray-200 mb-6">
        <nav className="-mb-px flex space-x-8 overflow-x-auto">
          {tabs.map((tabItem) => (
            <button
              key={tabItem.id}
              onClick={() => setActiveTab(tabItem.id)}
              className={`flex items-center gap-2 py-3 px-1 border-b-2 font-medium text-sm ${
                activeTab === tabItem.id
                  ? 'border-blue-500 text-blue-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
              }`}
            >
              <tabItem.icon className="w-4 h-4" />
              {tabItem.label}
              {tabItem.id === 'events' && (securityOverview?.alerts?.length ?? 0) > 0 && (
                <span className="ml-1 px-2 py-0.5 text-xs bg-red-100 text-red-800 rounded-full">
                  {securityOverview?.alerts?.length}
                </span>
              )}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab Content */}
      <div className="min-h-[600px]">
        {renderTabContent()}
      </div>

      {/* Confirmation Dialog */}
      <Modal
        open={confirmationDialog.isOpen}
        onClose={confirmationDialog.onCancel}
        title={confirmationDialog.title}
        maxWidth="sm"
      >
        <Modal.Body>
          <p className="text-gray-600 mb-4">
            {confirmationDialog.message}
          </p>
          {confirmationDialog.requiresConfirmation && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-2">
                Type "{confirmationDialog.title === 'Rotate Security Key' ? 'CONFIRM' : 'RESET'}" to proceed
              </label>
              <input
                type="text"
                value={confirmationTyped}
                onChange={(e) => setConfirmationTyped(e.target.value)}
                className={`w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-red-500 ${
                  confirmationTyped === ''
                    ? 'border-gray-300'
                    : confirmationTyped === (confirmationDialog.title === 'Rotate Security Key' ? 'CONFIRM' : 'RESET')
                      ? 'border-green-300'
                      : 'border-red-300'
                }`}
              />
            </div>
          )}
        </Modal.Body>
        <Modal.Footer>
          <button
            onClick={confirmationDialog.onConfirm}
            disabled={
              confirmationDialog.requiresConfirmation &&
              confirmationTyped !== (confirmationDialog.title === 'Rotate Security Key' ? 'CONFIRM' : 'RESET')
            }
            className={`inline-flex items-center px-4 py-2 rounded border text-white disabled:opacity-50 disabled:cursor-not-allowed ${
              confirmationDialog.severity === 'danger'
                ? 'bg-red-600 border-red-700 hover:bg-red-700 disabled:hover:bg-red-600'
                : 'bg-blue-600 border-blue-700 hover:bg-blue-700 disabled:hover:bg-blue-600'
            }`}
          >
            Confirm
          </button>
          <button
            onClick={confirmationDialog.onCancel}
            className="inline-flex items-center px-4 py-2 text-gray-700 bg-white border border-gray-300 rounded hover:bg-gray-50"
          >
            Cancel
          </button>
        </Modal.Footer>
      </Modal>
    </div>
  );
};

export default SecuritySettings;
