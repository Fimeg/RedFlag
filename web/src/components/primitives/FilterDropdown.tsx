import React from 'react';
import { cn } from '@/lib/utils';

/**
 * Pure filter dropdown — renders a styled <select> with a label.
 * No state management, no URL sync. Just UI.
 *
 * Props:
 *   label      — label text (e.g. "Status", "Severity")
 *   value      — current value
 *   onChange    — called with new value
 *   options    — list of { value, label } options
 *   placeholder — placeholder text for the empty option
 *   className  — additional classes
 */
interface FilterDropdownProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
  placeholder?: string;
  className?: string;
}

const FilterDropdown: React.FC<FilterDropdownProps> = ({
  label,
  value,
  onChange,
  options,
  placeholder,
  className,
}) => {
  const isActive = value !== '';

  return (
    <div className={cn('flex items-center gap-1.5', className)}>
      <label className="text-xs text-gray-500 shrink-0">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cn(
          'px-2.5 py-1.5 text-sm rounded-lg border transition-colors',
          'focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent',
          isActive
            ? 'border-primary-300 bg-primary-50 text-primary-800'
            : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400'
        )}
      >
        <option value="">{placeholder ?? `All ${label}s`}</option>
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  );
};

export default FilterDropdown;
