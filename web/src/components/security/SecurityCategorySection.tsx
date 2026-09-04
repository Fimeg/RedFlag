import React from 'react';
import { AlertTriangle, Info, Lock, Shield, CheckCircle } from 'lucide-react';
import { SecurityCategorySectionProps, SecuritySetting as SecuritySettingType } from '@/types/security';
import SecuritySetting from './SecuritySetting';

const SecurityCategorySection: React.FC<SecurityCategorySectionProps> = ({
  title,
  description,
  settings,
  onSettingChange,
  disabled = false,
  loading = false,
  error = null,
}) => {
  // Group settings by type for better organization
  const groupedSettings = settings.reduce((acc, setting) => {
    const group = setting.type === 'toggle' ? 'main' : 'advanced';
    if (!acc[group]) acc[group] = [];
    acc[group].push(setting);
    return acc;
  }, {} as Record<string, SecuritySettingType[]>);

  const isSectionEnabled = settings.find(s => s.key === 'enabled')?.value ?? true;

  return (
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      {/* Header */}
      <div className="flex items-start justify-between mb-6">
        <div className="flex-1">
          <div className="flex items-center gap-3 mb-2">
            <h2 className="text-xl font-semibold text-gray-900">{title}</h2>
            {isSectionEnabled ? (
              <CheckCircle className="w-5 h-5 text-green-500" />
            ) : (
              <Lock className="w-5 h-5 text-gray-400" />
            )}
          </div>
          <p className="text-gray-600">{description}</p>
        </div>
        {error && (
          <div className="ml-4 p-2 alert alert-danger rounded-lg">
            <AlertTriangle className="w-5 h-5 text-red-600" />
          </div>
        )}
      </div>

      {/* Loading State */}
      {loading && (
        <div className="flex items-center justify-center py-12">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
        </div>
      )}

      {/* Settings Grid */}
      {!loading && (
        <div className="space-y-6">
          {/* Main Settings (Toggles) */}
          {groupedSettings.main && groupedSettings.main.length > 0 && (
            <div className="space-y-4">
              {groupedSettings.main.map((setting) => (
                <div key={setting.key} className="flex items-start gap-4 p-4 bg-gray-50 rounded-lg">
                  <SecuritySetting
                    setting={setting}
                    onChange={(value) => onSettingChange(setting.key, value)}
                    disabled={disabled || setting.disabled}
                    error={null}
                  />
                  {setting.description && (
                    <div className="flex-1">
                      <p className="text-sm text-gray-600">{setting.description}</p>
                      {setting.key === 'enabled' && !setting.value && (
                        <div className="mt-2 p-2 alert alert-warning rounded">
                          <div className="flex items-center gap-2">
                            <AlertTriangle className="w-4 h-4 text-yellow-600 flex-shrink-0" />
                            <p className="text-sm text-yellow-800">
                              Disabling this feature may reduce system security
                            </p>
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Advanced Settings */}
          {groupedSettings.advanced && groupedSettings.advanced.length > 0 && (
            <div className="border-t border-gray-200 pt-6">
              <div className="flex items-center gap-2 mb-4">
                <Shield className="w-4 h-4 text-gray-500" />
                <h3 className="text-lg font-medium text-gray-900">Advanced Configuration</h3>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                {groupedSettings.advanced.map((setting) => (
                  <div
                    key={setting.key}
                    className={`
                      ${setting.type === 'checkbox-group' ? 'md:col-span-2' : ''}
                      ${setting.type === 'json' ? 'md:col-span-2' : ''}
                    `}
                  >
                    <SecuritySetting
                      setting={setting}
                      onChange={(value) => onSettingChange(setting.key, value)}
                      disabled={disabled || setting.disabled || !isSectionEnabled}
                      error={null}
                    />
                    {setting.description && (
                      <div className="mt-2 flex items-start gap-2">
                        <Info className="w-4 h-4 text-gray-400 flex-shrink-0 mt-0.5" />
                        <p className="text-sm text-gray-600">{setting.description}</p>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Section Footer Info */}
      {!loading && !error && (
        <div className="mt-6 pt-4 border-t border-gray-200">
          <div className="flex items-center justify-between text-sm text-gray-500">
            <div className="flex items-center gap-2">
              <Shield className="w-4 h-4" />
              <span>
                {isSectionEnabled ? 'Feature is active' : 'Feature is disabled'}
              </span>
            </div>
            <div className="flex items-center gap-4">
              <span>{settings.length} settings</span>
              {settings.filter(s => s.disabled).length > 0 && (
                <span className="text-amber-600">
                  {settings.filter(s => s.disabled).length} disabled
                </span>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default SecurityCategorySection;