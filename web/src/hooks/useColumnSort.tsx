import { useState, useCallback } from 'react';
import { ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';

export interface SortConfig {
  sortBy: string;
  sortOrder: 'asc' | 'desc';
}

interface UseColumnSortOptions {
  /** Sync sort state to URL search params (default false). */
  syncUrl?: boolean;
  /** Default column to sort by. */
  defaultSortBy?: string;
  /** Default sort direction (default 'desc'). */
  defaultOrder?: 'asc' | 'desc';
}

/**
 * Reusable column-sort state hook. Extracted from Updates.tsx; use anywhere a
 * table needs click-to-sort headers with URL persistence.
 *
 * Returns the sort config, a header click handler, a sort-icon renderer, and a
 * generic sort-applier for arrays.
 */
export function useColumnSort(opts: UseColumnSortOptions = {}) {
  const { syncUrl, defaultSortBy = '', defaultOrder = 'desc' } = opts;
  const [searchParams, setSearchParams] = syncUrl ? useSearchParams() : [null, null] as any;

  const [sortBy, setSortBy] = useState<string>(
    syncUrl ? (searchParams.get('sort_by') || defaultSortBy) : defaultSortBy,
  );
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(
    syncUrl
      ? ((searchParams.get('sort_order') as 'asc' | 'desc') || defaultOrder)
      : defaultOrder,
  );

  const handleSort = useCallback(
    (column: string) => {
      setSortBy((prev) => {
        if (prev === column) {
          const next = sortOrder === 'asc' ? 'desc' : 'asc';
          setSortOrder(next);
          if (syncUrl) {
            const p = new URLSearchParams(searchParams);
            p.set('sort_by', column);
            p.set('sort_order', next);
            setSearchParams(p, { replace: true });
          }
          return column;
        }
        setSortOrder('desc');
        if (syncUrl) {
          const p = new URLSearchParams(searchParams);
          p.set('sort_by', column);
          p.set('sort_order', 'desc');
          setSearchParams(p, { replace: true });
        }
        return column;
      });
    },
    [sortOrder, syncUrl, searchParams, setSearchParams],
  );

  const renderSortIcon = useCallback(
    (column: string) => {
      if (sortBy !== column) {
        return <ArrowUpDown className="h-4 w-4 ml-1 text-gray-400" />;
      }
      return sortOrder === 'asc' ? (
        <ArrowUp className="h-4 w-4 ml-1 text-primary-600" />
      ) : (
        <ArrowDown className="h-4 w-4 ml-1 text-primary-600" />
      );
    },
    [sortBy, sortOrder],
  );

  /**
   * Sort an array of items by a column, given a value extractor. Handles
   * strings (localeCompare), numbers, and dates; nulls sort last regardless
   * of direction.
   */
  const applySort = useCallback(
    <T,>(items: T[], extract: (item: T) => string | number | Date | null | undefined): T[] => {
      if (!sortBy) return items;
      const dir = sortOrder === 'asc' ? 1 : -1;
      return [...items].sort((a, b) => {
        const va = extract(a);
        const vb = extract(b);
        // Nulls last
        if (va == null && vb == null) return 0;
        if (va == null) return 1;
        if (vb == null) return -1;
        if (va instanceof Date && vb instanceof Date) return (va.getTime() - vb.getTime()) * dir;
        if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * dir;
        return String(va).localeCompare(String(vb), undefined, { numeric: true }) * dir;
      });
    },
    [sortBy, sortOrder],
  );

  return { sortBy, sortOrder, handleSort, renderSortIcon, applySort } as const;
}
