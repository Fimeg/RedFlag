import React from 'react';
import { Activity, Save } from 'lucide-react';
import {
  useProcessExplorerSettings,
  useUpdateProcessExplorerSetting,
  PROCESS_EXPLORER_DEFAULTS,
  ProcessExplorerSettings,
} from '../../hooks/useProcessExplorer';

interface Field {
  key: keyof ProcessExplorerSettings;
  label: string;
  unit: string;
  min: number;
  max: number;
  description: string;
}

const FIELDS: Field[] = [
  {
    key: 'max_open_files',
    label: 'Open Files Cap',
    unit: 'entries',
    min: 0,
    max: 50000,
    description:
      'Maximum open file descriptors returned per process. Set to 0 for no cap. Processes like databases may have thousands of open files.',
  },
  {
    key: 'max_sockets',
    label: 'Sockets Cap',
    unit: 'entries',
    min: 0,
    max: 50000,
    description:
      'Maximum open sockets (TCP/UDP/UNIX) returned per process. Set to 0 for no cap.',
  },
  {
    key: 'max_pipes',
    label: 'Pipes Cap',
    unit: 'entries',
    min: 0,
    max: 50000,
    description:
      'Maximum open pipes returned per process. Set to 0 for no cap.',
  },
  {
    key: 'max_memory_map',
    label: 'Memory Map Cap',
    unit: 'entries',
    min: 0,
    max: 100000,
    description:
      'Maximum memory-mapped regions returned per process. Chrome can have 20,000+ entries. Set to 0 for no cap.',
  },
  {
    key: 'max_namespaces',
    label: 'Namespaces Cap',
    unit: 'entries',
    min: 0,
    max: 1000,
    description:
      'Maximum Linux namespaces returned per process. Typically under 20. Set to 0 for no cap.',
  },
  {
    key: 'max_env_keys',
    label: 'Environment Keys Cap',
    unit: 'keys',
    min: 0,
    max: 10000,
    description:
      'Maximum environment variable key names returned per process. Values are never transmitted (security). Set to 0 for no cap.',
  },
  {
    key: 'max_listening_ports',
    label: 'Listening Ports Cap',
    unit: 'ports',
    min: 0,
    max: 10000,
    description:
      'Maximum TCP listening ports returned per process. Set to 0 for no cap.',
  },
];

const ProcessExplorer: React.FC = () => {
  const { data, isLoading } = useProcessExplorerSettings();
  const updateSetting = useUpdateProcessExplorerSetting();

  const [draft, setDraft] = React.useState<ProcessExplorerSettings>(PROCESS_EXPLORER_DEFAULTS);
  const [saved, setSaved] = React.useState(false);
  const [errors, setErrors] = React.useState<Partial<Record<keyof ProcessExplorerSettings, string>>>({});

  React.useEffect(() => {
    if (data) setDraft(data);
  }, [data]);

  const validate = (key: keyof ProcessExplorerSettings, value: number): string | null => {
    const field = FIELDS.find((f) => f.key === key);
    if (!field) return null;
    if (!Number.isFinite(value) || value < field.min) return `Must be at least ${field.min}`;
    if (value > field.max) return `Must be at most ${field.max}`;
    return null;
  };

  const handleChange = (key: keyof ProcessExplorerSettings, raw: string) => {
    const value = raw === '' ? 0 : parseInt(raw, 10);
    setDraft((prev) => ({ ...prev, [key]: Number.isFinite(value) ? value : 0 }));
    setErrors((prev) => ({ ...prev, [key]: validate(key, value) }));
    setSaved(false);
  };

  const handleSave = async (key: keyof ProcessExplorerSettings) => {
    const value = draft[key];
    const error = validate(key, value);
    if (error) return;
    try {
      await updateSetting.mutateAsync({ key, value });
      setSaved(true);
    } catch {
      setErrors((prev) => ({ ...prev, [key]: 'Failed to save' }));
    }
  };

  const handleSaveAll = async () => {
    const allErrors: typeof errors = {};
    for (const field of FIELDS) {
      const error = validate(field.key, draft[field.key]);
      if (error) allErrors[field.key] = error;
    }
    if (Object.keys(allErrors).length > 0) {
      setErrors(allErrors);
      return;
    }
    try {
      await Promise.all(
        FIELDS.map((field) =>
          updateSetting.mutateAsync({ key: field.key, value: draft[field.key] })
        )
      );
      setSaved(true);
    } catch {
      // individual errors shown inline
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8 text-muted-foreground">
        Loading process explorer settings...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <Activity className="h-5 w-5" />
            Process Explorer
          </h3>
          <p className="text-sm text-muted-foreground mt-1">
            Control how much data the agent collects per process during drill-down scans.
            Set to 0 to disable a cap. Changes propagate to all agents on their next check-in.
          </p>
        </div>
        <button
          onClick={handleSaveAll}
          className="inline-flex items-center gap-2 px-4 py-2 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 text-sm"
        >
          <Save className="h-4 w-4" />
          Save All
        </button>
      </div>

      {saved && (
        <div className="text-sm text-green-600 bg-green-50 border border-green-200 rounded-md p-3">
          Settings saved. Agents will pick up changes on their next check-in.
        </div>
      )}

      <div className="grid gap-4">
        {FIELDS.map((field) => (
          <div
            key={field.key}
            className="border rounded-lg p-4 space-y-2"
          >
            <div className="flex items-center justify-between">
              <div>
                <label className="text-sm font-medium">{field.label}</label>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {field.description}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="number"
                  min={field.min}
                  max={field.max}
                  value={draft[field.key]}
                  onChange={(e) => handleChange(field.key, e.target.value)}
                  className="w-24 px-3 py-1.5 border rounded-md text-sm text-right"
                />
                <span className="text-xs text-muted-foreground w-12">{field.unit}</span>
                <button
                  onClick={() => handleSave(field.key)}
                  className="px-3 py-1.5 text-xs border rounded-md hover:bg-accent"
                >
                  Save
                </button>
              </div>
            </div>
            {errors[field.key] && (
              <p className="text-xs text-destructive">{errors[field.key]}</p>
            )}
          </div>
        ))}
      </div>

      <div className="text-xs text-muted-foreground border-t pt-4">
        <p>
          <strong>How it works:</strong> These caps are stored on the server and delivered to agents
          via the config endpoint. When an agent performs a process drill-down scan, it respects
          these limits to prevent multi-megabyte responses for processes with thousands of open
          files or memory map entries.
        </p>
        <p className="mt-1">
          <strong>Default values</strong> are tuned for typical server workloads. Set a cap to 0
          to disable it (no limit, but the underlying /proc data is still bounded by the kernel).
        </p>
      </div>
    </div>
  );
};

export default ProcessExplorer;
