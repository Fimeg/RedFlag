// Parses a search string like "nginx severity:critical agent:waystation" into
// structured filters plus remaining free text.
//
// Rules:
//   key:value      → exact structured filter (value may be quoted: key:"foo bar")
//   bare words     → appended to the free-text portion
//   unknown keys   → treated as bare words, preserved in text

export type ParsedQuery = {
  text: string;
  filters: Record<string, string>;
};

// Known filter keys per page — callers pass the set they support so unknown
// keys fall through to text search rather than silently disappearing.
export function parseQuery(raw: string, knownKeys: Set<string>): ParsedQuery {
  if (!raw.trim()) return { text: '', filters: {} };

  const filters: Record<string, string> = {};
  const textParts: string[] = [];

  // Split on whitespace but respect quoted values (key:"foo bar")
  const tokens = raw.match(/\S+:"[^"]*"|\S+/g) ?? [];

  for (const token of tokens) {
    const colon = token.indexOf(':');
    if (colon > 0) {
      const key = token.slice(0, colon).toLowerCase();
      const val = token.slice(colon + 1).replace(/^"(.*)"$/, '$1');
      if (knownKeys.has(key)) {
        filters[key] = val;
        continue;
      }
    }
    textParts.push(token);
  }

  return { text: textParts.join(' '), filters };
}

// Serialises structured filters back to a query string (for display in the input).
export function buildQueryString(text: string, filters: Record<string, string>): string {
  const parts: string[] = [];
  for (const [key, val] of Object.entries(filters)) {
    parts.push(val.includes(' ') ? `${key}:"${val}"` : `${key}:${val}`);
  }
  if (text.trim()) parts.push(text.trim());
  return parts.join(' ');
}
