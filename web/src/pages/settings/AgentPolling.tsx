import React from 'react';
import { RadioTower, Save } from 'lucide-react';
import {
  usePollingSettings,
  useUpdatePollingSetting,
  POLLING_DEFAULTS,
  PollingSettings,
} from '../../hooks/useAgentPolling';

interface Field {
  key: keyof PollingSettings;
  label: string;
  unit: string;
  min: number;
  max: number;
  description: string;
}

const FIELDS: Field[] = [
  {
    key: 'jitter_max_seconds',
    label: 'Jitter Cap',
    unit: 'seconds',
    min: 0,
    max: 300,
    description:
      'Maximum random delay added before each check-in. Spreads fleet check-ins so agents do not all hit the server at once.',
  },
  {
    key: 'backoff_base_seconds',
    label: 'Backoff Floor',
    unit: 'seconds',
    min: 1,
    max: 300,
    description:
      'Shortest reconnect delay after a failed check-in. The exponential backoff curve starts here.',
  },
  {
    key: 'backoff_max_seconds',
    label: 'Backoff Ceiling',
    unit: 'seconds',
    min: 10,
    max: 3600,
    description:
      'Longest reconnect delay. Caps how far the exponential backoff can grow during an outage.',
  },
];

const AgentPolling: React.FC = () => {
  const { data, isLoading } = usePollingSettings();
  const updateSetting = useUpdatePollingSetting();

  const [draft, setDraft] = React.useState<PollingSettings>(POLLING_DEFAULTS);
  const [saved, setSaved] = React.useState(false);
  const [errors, setErrors] = React.useState<Partial<Record<keyof PollingSettings, string>>>({});

  React.useEffect(() => {
    if (data) setDraft(data);
  }, [data]);

  const validate = (field: Field, value: number): string | null => {
    if (Number.isNaN(value)) return 'Must be a number';
    if (value < field.min || value > field.max) {
      return `Must be between ${field.min} and ${field.max}`;
    }
    return null;
  };

  const handleChange = (field: Field, raw: string) => {
    setSaved(false);
    const value = parseInt(raw, 10);
    setDraft((d) => ({ ...d, [field.key]: value }));
    setErrors((e) => ({ ...e, [field.key]: validate(field, value) || undefined }));
  };

  const dirty = data
    ? FIELDS.some((f) => draft[f.key] !== data[f.key])
    : false;
  const hasErrors = Object.values(errors).some(Boolean);

  const handleSave = async () => {
    if (!data || !dirty || hasErrors) return;
    const changed = FIELDS.filter((f) => draft[f.key] !== data[f.key]);
    for (const f of changed) {
      await updateSetting.mutateAsync({ key: f.key, value: draft[f.key] });
    }
    setSaved(true);
  };

  return (
    <div className="max-w-6xl mx-auto px-6 py-8">
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900 flex items-center gap-3">
          <RadioTower className="w-8 h-8 text-amber-600" />
          Agent Polling
        </h1>
        <p className="mt-2 text-gray-600">
          Fleet-wide check-in jitter and reconnect backoff. Delivered to agents over their
          authenticated config channel and merged into local agent config — a change here reaches
          each agent on its next config refresh (within ~15 minutes), no host edits required.
        </p>
      </div>

      <div className="bg-white border border-gray-200 rounded-lg p-6 mb-8">
        <h2 className="text-xl font-semibold text-gray-900 mb-6 pb-2 border-b border-gray-200">
          Resilience Tuning
        </h2>

        {isLoading ? (
          <div className="text-center py-8">
            <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-amber-600 mx-auto"></div>
            <p className="text-sm text-gray-500 mt-2">Loading polling settings...</p>
          </div>
        ) : (
          <div className="space-y-8">
            {FIELDS.map((field) => (
              <div key={field.key}>
                <label className="block text-lg font-medium text-gray-900 mb-1">{field.label}</label>
                <p className="text-sm text-gray-600 mb-3">{field.description}</p>
                <div className="flex items-center gap-3">
                  <input
                    type="number"
                    min={field.min}
                    max={field.max}
                    value={Number.isNaN(draft[field.key]) ? '' : draft[field.key]}
                    onChange={(e) => handleChange(field, e.target.value)}
                    className="w-32 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-amber-500"
                  />
                  <span className="text-sm text-gray-500">{field.unit}</span>
                  <span className="text-xs text-gray-400">
                    (default {POLLING_DEFAULTS[field.key]}, range {field.min}–{field.max})
                  </span>
                </div>
                {errors[field.key] && (
                  <p className="mt-2 text-sm text-red-600">{errors[field.key]}</p>
                )}
              </div>
            ))}

            <div className="pt-2 border-t border-gray-200 flex items-center gap-4">
              <button
                onClick={handleSave}
                disabled={!dirty || hasErrors || updateSetting.isPending}
                className="flex items-center gap-2 px-4 py-2 bg-amber-600 text-white rounded-lg hover:bg-amber-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <Save className="w-4 h-4" />
                {updateSetting.isPending ? 'Saving...' : 'Save Changes'}
              </button>
              {updateSetting.isError && (
                <span className="text-sm text-red-600">Failed to save. Check the values and try again.</span>
              )}
              {saved && !dirty && !updateSetting.isError && (
                <span className="text-sm text-green-600">Saved. Agents will pick this up on next config refresh.</span>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

export default AgentPolling;
