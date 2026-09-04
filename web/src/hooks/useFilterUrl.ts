import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

/**
 * Syncs a set of named filters to URL search params.
 *
 * This hook owns ONLY the URL ↔ state mapping.  It knows nothing about
 * search boxes, query parsing, debounce, or UI.  Compose it with
 * useQueryParser / useDebounce / UI primitives at the page level.
 *
 * Usage:
 *   const { values, setFilter, clearFilter, clearAll } = useFilterUrl({
 *     status:   { urlParam: 'status', label: 'Status' },
 *     severity: { urlParam: 'severity', label: 'Severity', default: 'low' },
 *     agent:    { urlParam: 'agent', label: 'Agent', default: '' },
 *   });
 */

export type FilterDef = {
  urlParam: string;
  /** Human-readable label for pills and UI. Falls back to the config key. */
  label?: string;
  default?: string;
};

export type FilterConfig = Record<string, FilterDef>;

export type FilterValues<C extends FilterConfig> = {
  [K in keyof C]: string;
};

export type FilterUrlState<C extends FilterConfig> = {
  values: FilterValues<C>;
  setFilter: (key: keyof C, value: string) => void;
  clearFilter: (key: keyof C) => void;
  clearAll: () => void;
  activeCount: number;
};

export function useFilterUrl<C extends FilterConfig>(config: C): FilterUrlState<C> {
  const [searchParams, setSearchParams] = useSearchParams();
  const paramsRef = useRef(searchParams);
  const isFirstSyncRef = useRef(true);

  // Keep a ref to the latest searchParams so the write effect never reads
  // a stale closure.  The ref is updated before effects run (during render).
  paramsRef.current = searchParams;

  function readFromUrl(sp: URLSearchParams): FilterValues<C> {
    const result = {} as FilterValues<C>;
    for (const [key, def] of Object.entries(config)) {
      const raw = sp.get(def.urlParam);
      (result as any)[key] = raw ?? def.default ?? '';
    }
    return result;
  }

  const [values, setValues] = useState<FilterValues<C>>(() => readFromUrl(searchParams));

  // Write state → URL when values change.
  // Reads from paramsRef (not the searchParams closure) to avoid stale reads
  // when other URL params (tab, page, sort) change in the same render.
  //
  // searchParams is intentionally NOT in the dependency array: this effect
  // writes React state TO the URL.  Adding searchParams here would fire the
  // write effect on external URL changes (browser back/forward) with stale
  // React state, overwriting the URL before the sync effect below can react.
  // The sync effect (URL → state) is the correct handler for browser nav.
  useEffect(() => {
    const params = new URLSearchParams(paramsRef.current);
    let changed = false;
    for (const [key, def] of Object.entries(config)) {
      const val = (values as any)[key] as string;
      const isDefault = val === (def.default ?? '');
      if (isDefault) {
        if (params.has(def.urlParam)) {
          params.delete(def.urlParam);
          changed = true;
        }
      } else if (params.get(def.urlParam) !== val) {
        params.set(def.urlParam, val);
        changed = true;
      }
    }
    if (changed) setSearchParams(params, { replace: true });
  }, [values, config, setSearchParams]);

  // Back-sync URL → state on external URL changes (browser back/forward).
  // Skipped on mount (useState already reads the initial URL).
  useEffect(() => {
    if (isFirstSyncRef.current) {
      isFirstSyncRef.current = false;
      return;
    }
    const urlValues = readFromUrl(searchParams);
    const differs = Object.keys(config).some(
      (key) => (urlValues as any)[key] !== (values as any)[key]
    );
    if (differs) setValues(urlValues);
  }, [searchParams]); // eslint-disable-line react-hooks/exhaustive-deps

  const setFilter = useCallback(
    (key: keyof C, value: string) => {
      setValues((prev) => ({ ...prev, [key]: value }));
    },
    []
  );

  const clearFilter = useCallback(
    (key: keyof C) => {
      const def = config[key as string];
      setValues((prev) => ({ ...prev, [key]: def.default ?? '' }));
    },
    [config]
  );

  const clearAll = useCallback(() => {
    const reset = {} as FilterValues<C>;
    for (const [key, def] of Object.entries(config)) {
      (reset as any)[key] = def.default ?? '';
    }
    setValues(reset);
  }, [config]);

  const activeCount = Object.entries(config).filter(
    ([key, def]) => (values as any)[key] !== (def.default ?? '')
  ).length;

  return { values, setFilter, clearFilter, clearAll, activeCount };
}

/**
 * Build a FilterBar pills array from useFilterUrl state.
 * Each active (non-default) filter becomes a dismissible pill.
 * Uses the FilterDef label (or the config key as fallback) for display.
 */
export function buildFilterPills<C extends FilterConfig>(
  filter: FilterUrlState<C>,
  config: C
): { label: string; value: string; onClear: () => void }[] {
  const pills: { label: string; value: string; onClear: () => void }[] = [];
  for (const [key, def] of Object.entries(config)) {
    const val = (filter.values as Record<string, string>)[key];
    if (val && val !== (def.default ?? '')) {
      pills.push({
        label: def.label ?? key,
        value: val,
        onClear: () => filter.clearFilter(key as keyof C),
      });
    }
  }
  return pills;
}
