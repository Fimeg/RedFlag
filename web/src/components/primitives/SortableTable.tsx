import React from 'react';
import { ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import Pagination from './Pagination';

/**
 * Column definition for SortableTable.
 *
 * `key` doubles as the sort key unless `sortKey` overrides it.
 * Set `sortable: false` to render a static header (e.g. Actions column).
 */
export interface Column<T> {
  key: string;
  label: string;
  sortable?: boolean;   // default true (set false for Actions columns)
  sortKey?: string;      // override sort key passed to onSort (defaults to key)
  className?: string;
  render: (item: T) => React.ReactNode;
}

function SortIcon({ columnKey, sortBy, sortOrder }: { columnKey: string; sortBy: string; sortOrder: 'asc' | 'desc' }) {
  if (sortBy !== columnKey) return <ArrowUpDown className="h-4 w-4 ml-1 text-gray-400" />;
  return sortOrder === 'asc'
    ? <ArrowUp className="h-4 w-4 ml-1 text-primary-600" />
    : <ArrowDown className="h-4 w-4 ml-1 text-primary-600" />;
}

interface SortableTableProps<T> {
  columns: Column<T>[];
  data: T[];
  getKey: (item: T) => string;
  /** Current sort column (controlled). */
  sortBy: string;
  /** Current sort direction (controlled). */
  sortOrder: 'asc' | 'desc';
  /** Called when a sortable column header is clicked. */
  onSort: (column: string) => void;

  // Selection
  selectable?: boolean;
  selected?: string[];
  onSelectAll?: (all: boolean) => void;
  onSelectOne?: (key: string, checked: boolean) => void;

  // Row interaction
  onRowClick?: (item: T) => void;

  // Empty / loading
  emptyMessage?: string;
  loading?: boolean;

  // Pagination (optional — if omitted the table fills its container)
  page?: number;
  pageSize?: number;
  total?: number;
  onPageChange?: (page: number) => void;

  className?: string;
}

/**
 * SortableTable — reusable data table with sortable headers, optional row
 * selection, pagination, and standard RedFlag styling. Column sorting is
 * controlled externally — wire `useColumnSort` or manage state directly.
 *
 * Usage:
 * ```tsx
 * const { sortBy, sortOrder, handleSort, applySort } = useColumnSort({ defaultSortBy: 'hostname' });
 * const sorted = applySort(data, (item) => colSortValue(item, sortBy));
 * <SortableTable columns={cols} data={sorted} sortBy={sortBy} sortOrder={sortOrder} onSort={handleSort} />
 * ```
 */
function SortableTable<T>({
  columns,
  data,
  getKey,
  sortBy,
  sortOrder,
  onSort,

  selectable,
  selected = [],
  onSelectAll,
  onSelectOne,

  onRowClick,

  emptyMessage = 'No data.',
  loading = false,

  page,
  pageSize,
  total,
  onPageChange,

  className,
}: SortableTableProps<T>) {
  const allSelected = data.length > 0 && selected.length === data.length;

  return (
    <div className={cn('bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden', className)}>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              {selectable && (
                <th className="table-header w-10">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={(e) => onSelectAll?.(e.target.checked)}
                    className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                  />
                </th>
              )}
              {columns.map((col) => {
                const sortable = col.sortable !== false;
                const sortKey = col.sortKey || col.key;
                return (
                  <th key={col.key} className={cn('table-header', col.className)}>
                    {sortable ? (
                      <button
                        onClick={() => onSort(sortKey)}
                        className="flex items-center hover:text-primary-600 font-medium"
                      >
                        {col.label}
                        <SortIcon columnKey={sortKey} sortBy={sortBy} sortOrder={sortOrder} />
                      </button>
                    ) : (
                      col.label
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {loading ? (
              <tr>
                <td
                  colSpan={columns.length + (selectable ? 1 : 0)}
                  className="px-6 py-12 text-center text-sm text-gray-400"
                >
                  Loading&hellip;
                </td>
              </tr>
            ) : data.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length + (selectable ? 1 : 0)}
                  className="px-6 py-12 text-center text-sm text-gray-400"
                >
                  {emptyMessage}
                </td>
              </tr>
            ) : (
              data.map((item) => {
                const key = getKey(item);
                return (
                  <tr
                    key={key}
                    className={cn('hover:bg-gray-50 group', onRowClick && 'cursor-pointer')}
                    onClick={() => onRowClick?.(item)}
                  >
                    {selectable && (
                      <td className="table-cell w-10" onClick={(e) => e.stopPropagation()}>
                        <input
                          type="checkbox"
                          checked={selected.includes(key)}
                          onChange={(e) => onSelectOne?.(key, e.target.checked)}
                          className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        />
                      </td>
                    )}
                    {columns.map((col) => (
                      <td key={col.key} className={cn('table-cell', col.className)}>
                        {col.render(item)}
                      </td>
                    ))}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
      {page && pageSize && total && onPageChange ? (
        <div className="border-t border-gray-200 px-4 py-3">
          <Pagination page={page} total={total} pageSize={pageSize} onChange={onPageChange} />
        </div>
      ) : null}
    </div>
  );
}

export default SortableTable;
