import { useEffect, useState } from 'react';

/**
 * Debounces a value by `delay` ms. Returns the debounced value.
 * The input value updates immediately; the output lags behind.
 */
export function useDebounce<T>(value: T, delay: number = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(id);
  }, [value, delay]);

  return debounced;
}
