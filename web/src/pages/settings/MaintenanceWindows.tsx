import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useConfirm } from '@/components/primitives';
import {
  ArrowLeft,
  Plus,
  Trash2,
  Clock,
  Edit3,
  Check,
  X,
} from 'lucide-react';
import {
  useMaintenanceWindows,
  useCreateMaintenanceWindow,
  useUpdateMaintenanceWindow,
  useDeleteMaintenanceWindow,
  useMaintenanceWindowCheck,
} from '@/hooks/useMaintenanceWindows';
import { MaintenanceWindow, CreateMaintenanceWindowRequest } from '@/types';

const DAY_LABELS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

const emptyForm = (): CreateMaintenanceWindowRequest => ({
  day_of_week: 1,
  start_time: '22:00',
  end_time: '06:00',
  description: '',
  enabled: true,
});

const MaintenanceWindowsPage: React.FC = () => {
  const navigate = useNavigate();
  const confirm = useConfirm();
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<CreateMaintenanceWindowRequest>(emptyForm());

  const { data, isLoading } = useMaintenanceWindows();
  const { data: check } = useMaintenanceWindowCheck();
  const createMutation = useCreateMaintenanceWindow();
  const updateMutation = useUpdateMaintenanceWindow();
  const deleteMutation = useDeleteMaintenanceWindow();

  const windows = data?.windows || [];

  const resetForm = () => {
    setForm(emptyForm());
    setShowForm(false);
    setEditingId(null);
  };

  const handleEdit = (w: MaintenanceWindow) => {
    setForm({
      day_of_week: w.day_of_week,
      start_time: w.start_time.slice(0, 5),
      end_time: w.end_time.slice(0, 5),
      description: w.description,
      enabled: w.enabled,
    });
    setEditingId(w.id);
    setShowForm(true);
  };

  const handleSubmit = async () => {
    if (editingId) {
      await updateMutation.mutateAsync({ id: editingId, data: form });
    } else {
      await createMutation.mutateAsync(form);
    }
    resetForm();
  };

  const handleDelete = async (id: string) => {
    if (!(await confirm({
      title: 'Delete Maintenance Window',
      body: 'Delete this maintenance window?',
      confirmLabel: 'Delete',
      danger: true,
    }))) return;
    await deleteMutation.mutateAsync(id);
  };

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-4">
            <button onClick={() => navigate('/settings')} className="p-2 hover:bg-gray-200 rounded-lg transition-colors">
              <ArrowLeft className="w-5 h-5 text-gray-600" />
            </button>
            <div>
              <h1 className="text-2xl font-bold text-gray-900">Maintenance Windows</h1>
              <p className="text-sm text-gray-600 mt-1">Schedule when updates can be installed</p>
            </div>
          </div>
        </div>

        {/* Status Banner */}
        <div className={`mb-6 p-4 rounded-lg border ${check?.inside_window ? 'bg-green-50 border-green-200' : 'bg-gray-50 border-gray-200'}`}>
          <div className="flex items-center gap-3">
            <Clock className={`w-5 h-5 ${check?.inside_window ? 'text-green-600' : 'text-gray-400'}`} />
            <span className={`text-sm font-medium ${check?.inside_window ? 'text-green-800' : 'text-gray-600'}`}>
              {check?.inside_window
                ? 'Currently inside a maintenance window — installations will proceed'
                : 'Outside maintenance windows — approved installs are held until the next scheduled window'}
            </span>
          </div>
        </div>

        {/* Add Button */}
        {!showForm && (
          <button
            onClick={() => { setShowForm(true); setEditingId(null); setForm(emptyForm()); }}
            className="mb-6 flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors text-sm font-medium"
          >
            <Plus className="w-4 h-4" />
            Add Window
          </button>
        )}

        {/* Form */}
        {showForm && (
          <div className="mb-6 p-5 bg-white border border-gray-200 rounded-lg shadow-sm">
            <h3 className="font-semibold text-gray-900 mb-4">{editingId ? 'Edit Window' : 'New Window'}</h3>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Day of Week</label>
                <select
                  value={form.day_of_week}
                  onChange={e => setForm({ ...form, day_of_week: parseInt(e.target.value) })}
                  className="w-full border border-gray-300 rounded-md px-3 py-2 text-sm focus:ring-indigo-500 focus:border-indigo-500"
                >
                  {DAY_LABELS.map((label, i) => (
                    <option key={i} value={i}>{label}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Enabled</label>
                <div className="flex items-center h-[38px]">
                  <input
                    type="checkbox"
                    checked={form.enabled ?? true}
                    onChange={e => setForm({ ...form, enabled: e.target.checked })}
                    className="h-4 w-4 text-indigo-600 rounded border-gray-300"
                  />
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Start Time</label>
                <input
                  type="time"
                  value={form.start_time}
                  onChange={e => setForm({ ...form, start_time: e.target.value })}
                  className="w-full border border-gray-300 rounded-md px-3 py-2 text-sm focus:ring-indigo-500 focus:border-indigo-500"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">End Time</label>
                <input
                  type="time"
                  value={form.end_time}
                  onChange={e => setForm({ ...form, end_time: e.target.value })}
                  className="w-full border border-gray-300 rounded-md px-3 py-2 text-sm focus:ring-indigo-500 focus:border-indigo-500"
                />
              </div>
              <div className="col-span-2">
                <label className="block text-sm font-medium text-gray-700 mb-1">Description (optional)</label>
                <input
                  type="text"
                  value={form.description || ''}
                  onChange={e => setForm({ ...form, description: e.target.value })}
                  placeholder="e.g., Weekend window for production"
                  className="w-full border border-gray-300 rounded-md px-3 py-2 text-sm focus:ring-indigo-500 focus:border-indigo-500"
                />
              </div>
            </div>
            <div className="flex gap-2 mt-4">
              <button
                onClick={handleSubmit}
                disabled={createMutation.isPending || updateMutation.isPending}
                className="flex items-center gap-1 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors text-sm font-medium disabled:opacity-50"
              >
                <Check className="w-4 h-4" />
                {editingId ? 'Update' : 'Create'}
              </button>
              <button
                onClick={resetForm}
                className="flex items-center gap-1 px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 transition-colors text-sm"
              >
                <X className="w-4 h-4" />
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Windows List */}
        {isLoading ? (
          <div className="text-center py-12 text-gray-500">Loading...</div>
        ) : windows.length === 0 ? (
          <div className="text-center py-12 bg-white border border-gray-200 rounded-lg">
            <Clock className="w-12 h-12 text-gray-300 mx-auto mb-3" />
            <p className="text-gray-500 font-medium">No maintenance windows configured</p>
            <p className="text-gray-400 text-sm mt-1">Installations will proceed unrestricted</p>
          </div>
        ) : (
          <div className="bg-white border border-gray-200 rounded-lg overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-200 bg-gray-50">
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">Day</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">Time</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">Description</th>
                  <th className="text-center px-4 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                  <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {windows.map(w => (
                  <tr key={w.id} className="hover:bg-gray-50 transition-colors">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">{DAY_LABELS[w.day_of_week]}</td>
                    <td className="px-4 py-3 text-sm text-gray-600">
                      {w.start_time.slice(0, 5)} – {w.end_time.slice(0, 5)}
                      {w.start_time > w.end_time && (
                        <span className="ml-2 text-xs text-amber-600">(crosses midnight)</span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500">{w.description || '—'}</td>
                    <td className="px-4 py-3 text-center">
                      <span className={`badge ${
                        w.enabled ? 'badge-success' : 'bg-gray-100 text-gray-500'
                      }`}>
                        {w.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex justify-end gap-2">
                        <button
                          onClick={() => handleEdit(w)}
                          className="p-1.5 text-gray-400 hover:text-indigo-600 hover:bg-indigo-50 rounded transition-colors"
                          title="Edit"
                        >
                          <Edit3 className="w-4 h-4" />
                        </button>
                        <button
                          onClick={() => handleDelete(w.id)}
                          className="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded transition-colors"
                          title="Delete"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export default MaintenanceWindowsPage;
