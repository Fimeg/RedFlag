import React from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * Pure filter pill — renders a labeled chip with a dismiss button.
 * No state management, no URL sync. Just UI.
 *
 * Props:
 *   label    — key label (e.g. "severity", "agent")
 *   value    — current value (e.g. "critical", "waystation")
 *   onClear  — called when the X is clicked
 *   color    — optional color override (Tailwind classes)
 */

// Color map for well-known values so pills match the badge colors used in tables.
const VALUE_COLORS: Record<string, string> = {
  critical:      'bg-red-100 text-red-800 border-red-200',
  high:          'bg-orange-100 text-orange-800 border-orange-200',
  medium:        'bg-blue-100 text-blue-800 border-blue-200',
  low:           'bg-gray-100 text-gray-700 border-gray-200',
  'up-to-date':  'bg-green-100 text-green-800 border-green-200',
  'update-available': 'bg-blue-100 text-blue-800 border-blue-200',
  'update-approved':  'bg-orange-100 text-orange-800 border-orange-200',
  'update-failed':    'bg-red-100 text-red-800 border-red-200',
  running:       'bg-green-100 text-green-800 border-green-200',
  stopped:       'bg-red-100 text-red-800 border-red-200',
  healthy:       'bg-green-100 text-green-800 border-green-200',
  unhealthy:     'bg-red-100 text-red-800 border-red-200',
};

interface FilterPillProps {
  label: string;
  value: string;
  onClear: () => void;
  color?: string;
}

const FilterPill: React.FC<FilterPillProps> = ({ label, value, onClear, color }) => {
  const colorClasses = color ?? VALUE_COLORS[value] ?? 'bg-gray-100 text-gray-700 border-gray-200';

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 pl-2.5 pr-1 py-0.5 rounded-full text-xs font-medium border',
        colorClasses
      )}
    >
      <span className="text-gray-500 font-normal">{label}:</span>
      <span>{value}</span>
      <button
        onClick={onClear}
        className="ml-0.5 p-0.5 rounded-full hover:bg-black/10 transition-colors"
        aria-label={`Remove ${label} filter`}
      >
        <X className="w-3 h-3" />
      </button>
    </span>
  );
};

export default FilterPill;
