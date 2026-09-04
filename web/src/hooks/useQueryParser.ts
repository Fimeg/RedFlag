import { useCallback, useMemo, useState } from 'react';
import { parseQuery, buildQueryString } from '@/lib/queryParser';
import { useDebounce } from './useDebounce';

/**
 * Manages a raw search string and parses it into structured filters + free text.
 *
 * This hook owns ONLY the raw query ↔ parsed state mapping.
 * It does NOT sync to URL — compose with useFilterUrl at the page level
 * if you need that.
 *
 * Usage:
 *   const { rawQuery, setRawQuery, parsed, pills } = useQueryParser(
 *     ['status', 'severity', 'agent']   // known key:value keys
 *   );
 */

export type ParsedPill = {
  key: string;
  value: string;
};

export type QueryParserState = {
  rawQuery: string;
  setRawQuery: (q: string) => void;
  debouncedText: string;
  parsed: { text: string; filters: Record<string, string> };
  pills: ParsedPill[];
  removePill: (key: string) => void;
};

export function useQueryParser(
  knownKeys: string[],
  opts?: { debounce?: number }
): QueryParserState {
  const keySet = useMemo(() => new Set(knownKeys), [knownKeys]);
  const [rawQuery, setRawQuery] = useState('');

  const debouncedRaw = useDebounce(rawQuery, opts?.debounce ?? 300);

  const parsed = useMemo(() => parseQuery(debouncedRaw, keySet), [debouncedRaw, keySet]);

  const pills: ParsedPill[] = useMemo(
    () =>
      Object.entries(parsed.filters).map(([key, value]) => ({ key, value })),
    [parsed.filters]
  );

  const removePill = useCallback(
    (key: string) => {
      // Rebuild the query string without the removed key
      const current = parseQuery(rawQuery, keySet);
      const remaining = { ...current.filters };
      delete remaining[key];
      setRawQuery(buildQueryString(current.text, remaining));
    },
    [rawQuery, keySet]
  );

  return { rawQuery, setRawQuery, debouncedText: parsed.text, parsed, pills, removePill };
}
