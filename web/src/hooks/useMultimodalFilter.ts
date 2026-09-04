import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useFilterUrl, type FilterConfig } from './useFilterUrl';
import { useQueryParser, type ParsedPill } from './useQueryParser';
import { buildQueryString } from '@/lib/queryParser';

/**
 * Composes useFilterUrl + useQueryParser into a unified multimodal filter.
 *
 * This is the "page-level" hook — it's NOT a primitive.  It wires together
 * the two primitive hooks so that:
 *   - Typing in the search box parses key:value pairs → updates URL filters
 *   - Clicking a FilterCountButton → updates URL → updates search box
 *   - Clearing a pill → removes from query string → updates URL
 *
 * Pages that don't need the multimodal search box should use useFilterUrl
 * directly with FilterDropdown primitives.
 *
 * Usage:
 *   const filter = useMultimodalFilter(
 *     { status: { urlParam: 'status' }, severity: { urlParam: 'severity' } },
 *     { placeholder: 'Search images, or filter with key:value...' }
 *   );
 *
 *   // filter.rawQuery        — controlled value for <SearchInput>
 *   // filter.setRawQuery     — onChange handler for <SearchInput>
 *   // filter.pills           — array of { key, value, onClear } for <FilterPill>
 *   // filter.values          — resolved filter values for API calls
 *   // filter.setFilter       — set a specific filter (e.g. from a button click)
 *   // filter.clearAll        — reset everything
 *   // filter.activeCount     — number of active filters
 */

export type MultimodalFilter<C extends FilterConfig> = {
  // Raw query for the search input
  rawQuery: string;
  setRawQuery: (q: string) => void;
  // Debounced free-text portion (for API search param)
  searchText: string;
  // Parsed pills with onClear attached
  pills: (ParsedPill & { onClear: () => void })[];
  // Resolved filter values for API calls
  values: { [K in keyof C]: string };
  // Set a specific filter programmatically (e.g. from a button click)
  setFilter: (key: keyof C, value: string) => void;
  clearFilter: (key: keyof C) => void;
  clearAll: () => void;
  activeCount: number;
};

export function useMultimodalFilter<C extends FilterConfig>(
  config: C,
  opts?: { debounce?: number }
): MultimodalFilter<C> {
  const knownKeys = useMemo(() => Object.keys(config), [config]);
  const url = useFilterUrl(config);
  const parser = useQueryParser(knownKeys, opts);

  // When parsed filters change, sync to URL (only for keys that are in the config).
  // This writes to the URL, so it's a side effect — must live in useEffect, not
  // useMemo (which React may run twice in StrictMode / skip arbitrarily).
  const lastSynced = useRef<Record<string, string>>({});

  useEffect(() => {
    for (const key of knownKeys) {
      const parsedVal = parser.parsed.filters[key] ?? '';
      const currentUrlVal = (url.values as any)[key] ?? '';
      if (parsedVal !== lastSynced.current[key]) {
        lastSynced.current[key] = parsedVal;
        if (parsedVal !== currentUrlVal) {
          url.setFilter(key, parsedVal);
        }
      }
    }
    // Clear URL filters that are no longer in the query
    for (const key of knownKeys) {
      if (!parser.parsed.filters[key] && (url.values as any)[key]) {
        url.setFilter(key, '');
      }
    }
  }, [parser.parsed.filters]);

  // Build pills from parsed filters, attaching onClear
  const pills = useMemo(
    () =>
      parser.pills.map((p) => ({
        ...p,
        onClear: () => parser.removePill(p.key),
      })),
    [parser.pills, parser.removePill]
  );

  // When setFilter is called programmatically (e.g. button click),
  // update the raw query string to reflect the change
  const setFilter = useCallback(
    (key: keyof C, value: string) => {
      const current = parser.parsed.filters;
      const updated = { ...current, [key as string]: value };
      // Remove empty entries
      for (const k of Object.keys(updated)) {
        if (!updated[k]) delete updated[k];
      }
      parser.setRawQuery(buildQueryString(parser.parsed.text, updated));
    },
    [parser]
  );

  const clearFilter = useCallback(
    (key: keyof C) => {
      parser.removePill(key as string);
    },
    [parser]
  );

  const clearAll = useCallback(() => {
    parser.setRawQuery('');
    url.clearAll();
  }, [parser, url]);

  return {
    rawQuery: parser.rawQuery,
    setRawQuery: parser.setRawQuery,
    searchText: parser.debouncedText,
    pills,
    values: url.values,
    setFilter,
    clearFilter,
    clearAll,
    activeCount: url.activeCount,
  };
}
