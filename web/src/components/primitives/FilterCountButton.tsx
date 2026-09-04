import React from 'react';
import { cn } from '@/lib/utils';

/**
 * Clickable stat/filter button — shows a count with a label and color dot.
 * Modeled after Portainer's FilterBarButton.
 *
 * Props:
 *   count      — number to display
 *   label      — label text (e.g. "Critical", "Updates Available")
 *   isSelected — whether this filter is currently active
 *   onClick    — called when clicked
 *   color      — color variant
 *   icon       — optional icon element
 *   className  — additional classes
 */
interface FilterCountButtonProps {
  count: number;
  label: string;
  isSelected: boolean;
  onClick: () => void;
  color?: 'blue' | 'orange' | 'red' | 'green' | 'gray';
  icon?: React.ReactNode;
  className?: string;
}

const COLOR_MAP = {
  blue:   { dot: 'bg-blue-500',   border: 'border-blue-200',   hover: 'hover:bg-blue-50',   text: 'text-blue-600' },
  orange: { dot: 'bg-orange-500', border: 'border-orange-200', hover: 'hover:bg-orange-50', text: 'text-orange-600' },
  red:    { dot: 'bg-red-500',    border: 'border-red-200',    hover: 'hover:bg-red-50',    text: 'text-red-600' },
  green:  { dot: 'bg-green-500',  border: 'border-green-200',  hover: 'hover:bg-green-50',  text: 'text-green-600' },
  gray:   { dot: 'bg-gray-400',   border: 'border-gray-200',   hover: 'hover:bg-gray-50',   text: 'text-gray-600' },
};

const FilterCountButton: React.FC<FilterCountButtonProps> = ({
  count,
  label,
  isSelected,
  onClick,
  color = 'gray',
  icon,
  className,
}) => {
  const c = COLOR_MAP[color];

  return (
    <button
      onClick={onClick}
      className={cn(
        'relative p-4 rounded-lg border text-left transition-colors',
        isSelected
          ? `${c.border} bg-opacity-50`
          : 'border-gray-200 bg-white',
        c.hover,
        className
      )}
    >
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-gray-600">{label}</p>
          <p className={cn('text-2xl font-bold', c.text)}>{count}</p>
        </div>
        {icon ?? (
          <span className={cn('h-2.5 w-2.5 rounded-full', c.dot)} aria-hidden="true" />
        )}
      </div>
      {isSelected && (
        <span
          className={cn('absolute bottom-0 left-0 right-0 h-1 rounded-b-lg', c.dot)}
          aria-hidden="true"
        />
      )}
    </button>
  );
};

export default FilterCountButton;
