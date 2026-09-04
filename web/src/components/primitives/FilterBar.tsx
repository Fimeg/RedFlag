import React from 'react';
import { SearchInput, FilterDropdown, FilterPill } from '@/components/primitives';

/**
 * Composable filter bar — search input + dropdown filters + active pill row.
 *
 * No state management, no URL sync.  Just layout for the existing primitives.
 * Compose with useFilterUrl at the page level.
 *
 * Usage:
 *   const filter = useFilterUrl({ status: { urlParam: 'status' } });
 *   const [searchQuery, setSearchQuery] = useState('');
 *   const debounced = useDebounce(searchQuery, 300);
 *
 *   <FilterBar
 *     search={{ value: searchQuery, onChange: setSearchQuery, placeholder: '...' }}
 *     filters={[
 *       { label: 'Status', value: filter.values.status, onChange: v => filter.setFilter('status', v),
 *         options: statuses, placeholder: 'All Status' },
 *     ]}
 *     pills={filterPills(filter.values, filter.clearFilter)}
 *     onClearAll={filter.clearAll}
 *     activeCount={filter.activeCount}
 *     actions={<button ...>Bulk Action</button>}
 *   />
 */

interface FilterBarFilter {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
  placeholder?: string;
}

interface FilterBarPill {
  label: string;
  value: string;
  onClear: () => void;
}

export interface FilterBarProps {
  /** Optional search input — shown as the first element in the top row */
  search?: {
    value: string;
    onChange: (value: string) => void;
    placeholder?: string;
    className?: string;
  };
  /** Dropdown filters — shown inline after the search input */
  filters?: FilterBarFilter[];
  /** Active filter pills — shown below the top row */
  pills?: FilterBarPill[];
  /** Called when "clear all" is clicked. Only shown when activeCount > 0. */
  onClearAll?: () => void;
  /** Number of active filters (controls "clear all" visibility) */
  activeCount?: number;
  /** Extra elements shown at the right end of the top row (bulk actions, etc.) */
  actions?: React.ReactNode;
  className?: string;
}

const FilterBar: React.FC<FilterBarProps> = ({
  search,
  filters = [],
  pills = [],
  onClearAll,
  activeCount = 0,
  actions,
  className,
}) => {
  return (
    <div className={className}>
      {/* Top row: search + filters + actions */}
      <div className="flex flex-col sm:flex-row gap-4">
        {search && (
          <SearchInput
            value={search.value}
            onChange={search.onChange}
            placeholder={search.placeholder ?? 'Search...'}
            className={search.className ?? 'flex-1'}
          />
        )}
        {filters.map((f, i) => (
          <FilterDropdown
            key={i}
            label={f.label}
            value={f.value}
            onChange={f.onChange}
            options={f.options}
            placeholder={f.placeholder}
          />
        ))}
        {actions && (
          <div className="flex items-center gap-2">
            {actions}
          </div>
        )}
      </div>

      {/* Pill row */}
      {(pills.length > 0 || activeCount > 0) && (
        <div className="flex flex-wrap items-center gap-1.5 mt-3">
          {pills.map((p, i) => (
            <FilterPill
              key={i}
              label={p.label}
              value={p.value}
              onClear={p.onClear}
            />
          ))}
          {onClearAll && activeCount > 0 && (
            <button
              onClick={onClearAll}
              className="text-xs text-gray-400 hover:text-gray-700 transition-colors ml-1 underline"
            >
              clear all
            </button>
          )}
        </div>
      )}
    </div>
  );
};

export default FilterBar;
