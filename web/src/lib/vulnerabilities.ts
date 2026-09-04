export interface VulnerabilityDisplay {
  id: string;
  summary?: string;
  aliases: string[];
  severity?: string;
  cvss_vector?: string;
  cvss_score?: number;
  fixed_version?: string;
  published?: string;
  advisory_type?: string;
  affected_ranges: string[];
  known_exploited: boolean;
  source?: string;
}

interface OSVSeverityRecord {
  score?: string;
}

interface OSVEventRecord {
  introduced?: string;
  fixed?: string;
  last_affected?: string;
  limit?: string;
}

const stringValue = (value: unknown): string | undefined => {
  return typeof value === 'string' && value.trim() !== '' ? value.trim() : undefined;
};

const numberValue = (value: unknown): number | undefined => {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
};

const objectValue = (value: unknown): Record<string, unknown> | undefined => {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
};

const parseRawList = (raw: unknown): unknown[] => {
  if (!raw) return [];
  if (Array.isArray(raw)) return raw;
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }
  return [];
};

export const osvVulnerabilityUrl = (id: string): string => {
  return `https://osv.dev/vulnerability/${encodeURIComponent(id)}`;
};

export const parseVulnerabilityList = (raw: unknown): VulnerabilityDisplay[] => {
  const seen = new Set<string>();
  const result: VulnerabilityDisplay[] = [];

  for (const item of parseRawList(raw)) {
    const normalized = normalizeVulnerability(item);
    if (!normalized || seen.has(normalized.id)) continue;
    seen.add(normalized.id);
    result.push(normalized);
  }

  return result;
};

const normalizeVulnerability = (raw: unknown): VulnerabilityDisplay | null => {
  const record = objectValue(raw);
  if (!record) return null;

  const id = stringValue(record.id);
  if (!id) return null;

  const databaseSpecific = objectValue(record.database_specific);
  const cvssVector = extractCVSSVector(record);
  const cvssScore =
    numberValue(record.cvss_score) ??
    numberValue(record.cvssScore) ??
    numberValue(databaseSpecific?.cvss_score) ??
    numberValue(databaseSpecific?.cvssScore) ??
    parseCVSS3BaseScore(cvssVector);

  return {
    id,
    summary: stringValue(record.summary) ?? stringValue(record.description),
    aliases: normalizeAliases(record.aliases),
    severity: extractSeverity(record, databaseSpecific, cvssScore),
    cvss_vector: cvssVector,
    cvss_score: cvssScore,
    fixed_version: stringValue(record.fixed_version) ?? firstFixedVersion(record.affected),
    published: stringValue(record.published),
    advisory_type: stringValue(record.advisory_type) ?? advisoryType(id),
    affected_ranges: normalizeAffectedRanges(record.affected_ranges, record.affected),
    known_exploited: isKnownExploited(record, databaseSpecific),
    source: stringValue(record.source),
  };
};

const normalizeAliases = (raw: unknown): string[] => {
  if (!Array.isArray(raw)) return [];
  return raw.filter((entry): entry is string => typeof entry === 'string' && entry.trim() !== '');
};

const extractCVSSVector = (record: Record<string, unknown>): string | undefined => {
  const explicit = stringValue(record.cvss_vector) ?? stringValue(record.cvssVector);
  if (explicit?.startsWith('CVSS:')) return explicit;

  const severity = record.severity;
  if (typeof severity === 'string' && severity.startsWith('CVSS:')) return severity;
  if (!Array.isArray(severity)) return undefined;

  const vectors = severity
    .map((entry) => objectValue(entry) as OSVSeverityRecord | undefined)
    .map((entry) => stringValue(entry?.score))
    .filter((score): score is string => !!score && score.startsWith('CVSS:'));

  return vectors.find((score) => score.startsWith('CVSS:4')) ?? vectors[0];
};

const extractSeverity = (
  record: Record<string, unknown>,
  databaseSpecific: Record<string, unknown> | undefined,
  cvssScore: number | undefined
): string | undefined => {
  const direct = stringValue(record.severity);
  if (direct && !direct.startsWith('CVSS:')) return direct.toUpperCase();

  const databaseSeverity =
    stringValue(databaseSpecific?.severity) ??
    stringValue(databaseSpecific?.severity_rating) ??
    stringValue(databaseSpecific?.severityRating);
  if (databaseSeverity) return databaseSeverity.toUpperCase();

  return severityFromCVSSScore(cvssScore);
};

const severityFromCVSSScore = (score: number | undefined): string | undefined => {
  if (score === undefined) return undefined;
  if (score >= 9) return 'CRITICAL';
  if (score >= 7) return 'HIGH';
  if (score >= 4) return 'MEDIUM';
  if (score > 0) return 'LOW';
  return undefined;
};

const parseCVSS3BaseScore = (vector?: string): number | undefined => {
  if (!vector || !vector.startsWith('CVSS:3.')) return undefined;

  const metrics: Record<string, string> = {};
  for (const part of vector.split('/')) {
    const [key, value] = part.split(':');
    if (key && value) metrics[key] = value;
  }

  const av = { N: 0.85, A: 0.62, L: 0.55, P: 0.2 }[metrics.AV];
  const ac = { L: 0.77, H: 0.44 }[metrics.AC];
  const scope = metrics.S;
  const pr = (scope === 'C'
    ? { N: 0.85, L: 0.68, H: 0.5 }
    : { N: 0.85, L: 0.62, H: 0.27 })[metrics.PR];
  const ui = { N: 0.85, R: 0.62 }[metrics.UI];
  const c = { H: 0.56, L: 0.22, N: 0 }[metrics.C];
  const i = { H: 0.56, L: 0.22, N: 0 }[metrics.I];
  const a = { H: 0.56, L: 0.22, N: 0 }[metrics.A];

  if (
    av === undefined ||
    ac === undefined ||
    pr === undefined ||
    ui === undefined ||
    c === undefined ||
    i === undefined ||
    a === undefined ||
    (scope !== 'U' && scope !== 'C')
  ) {
    return undefined;
  }

  const exploitability = 8.22 * av * ac * pr * ui;
  const impactSubScore = 1 - (1 - c) * (1 - i) * (1 - a);
  const impact =
    scope === 'U'
      ? 6.42 * impactSubScore
      : 7.52 * (impactSubScore - 0.029) - 3.25 * Math.pow(impactSubScore - 0.02, 15);

  if (impact <= 0) return 0;
  const rawScore =
    scope === 'U'
      ? Math.min(impact + exploitability, 10)
      : Math.min(1.08 * (impact + exploitability), 10);
  return Math.ceil((rawScore - 1e-10) * 10) / 10;
};

const firstFixedVersion = (affected: unknown): string | undefined => {
  if (!Array.isArray(affected)) return undefined;

  for (const affectedEntry of affected) {
    const ranges = objectValue(affectedEntry)?.ranges;
    if (!Array.isArray(ranges)) continue;
    for (const range of ranges) {
      const events = objectValue(range)?.events;
      if (!Array.isArray(events)) continue;
      for (const event of events) {
        const fixed = stringValue(objectValue(event)?.fixed);
        if (fixed) return fixed;
      }
    }
  }

  return undefined;
};

const normalizeAffectedRanges = (displayRanges: unknown, affected: unknown): string[] => {
  if (Array.isArray(displayRanges)) {
    return displayRanges.filter(
      (entry): entry is string => typeof entry === 'string' && entry.trim() !== ''
    );
  }
  return formatAffectedRanges(affected);
};

const formatAffectedRanges = (affected: unknown): string[] => {
  if (!Array.isArray(affected)) return [];

  const ranges: string[] = [];
  for (const affectedEntry of affected) {
    const affectedRecord = objectValue(affectedEntry);
    if (!affectedRecord || !Array.isArray(affectedRecord.ranges)) continue;

    for (const rangeEntry of affectedRecord.ranges) {
      const rangeRecord = objectValue(rangeEntry);
      if (!rangeRecord || !Array.isArray(rangeRecord.events)) continue;

      const type = stringValue(rangeRecord.type);
      let introduced: string | undefined;
      let emitted = false;

      for (const event of rangeRecord.events) {
        const eventRecord = objectValue(event) as OSVEventRecord | undefined;
        if (!eventRecord) continue;

        if (eventRecord.introduced !== undefined) {
          introduced = eventRecord.introduced;
        }
        if (eventRecord.fixed) {
          ranges.push(formatRange(type, introduced, '<', eventRecord.fixed));
          emitted = true;
          introduced = undefined;
        } else if (eventRecord.last_affected) {
          ranges.push(formatRange(type, introduced, '<=', eventRecord.last_affected));
          emitted = true;
        } else if (eventRecord.limit) {
          ranges.push(formatRange(type, introduced, '<', eventRecord.limit));
          emitted = true;
        }
      }

      if (!emitted && introduced) {
        ranges.push(formatRange(type, introduced, undefined, undefined));
      }
    }
  }

  return ranges;
};

const formatRange = (
  type: string | undefined,
  introduced: string | undefined,
  upperOp: '<' | '<=' | undefined,
  upperVersion: string | undefined
): string => {
  const lower = introduced && introduced !== '0' ? `>= ${introduced}` : 'all prior versions';
  const body = upperOp && upperVersion ? `${lower}, ${upperOp} ${upperVersion}` : `${lower} and later`;
  return type ? `${type}: ${body}` : body;
};

const isKnownExploited = (
  record: Record<string, unknown>,
  databaseSpecific: Record<string, unknown> | undefined
): boolean => {
  const keys = [
    'known_exploited',
    'knownExploited',
    'known_exploited_vulnerability',
    'knownExploitedVulnerability',
    'cisa_kev',
    'cisaKev',
    'cisa_known_exploited',
    'cisaKnownExploited',
    'cisaExploitAdd',
    'cisaActionDue',
    'cisaRequiredAction',
    'cisaVulnerabilityName',
    'kev',
  ];

  return keys.some((key) => truthy(record[key]) || truthy(databaseSpecific?.[key]));
};

const truthy = (value: unknown): boolean => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value > 0;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    return normalized !== '' && !['false', 'no', 'none', 'unknown', '0'].includes(normalized);
  }
  if (Array.isArray(value)) return value.length > 0;
  if (value && typeof value === 'object') return Object.keys(value).length > 0;
  return false;
};

const advisoryType = (id: string): string => {
  const up = id.toUpperCase();
  if (up.startsWith('ALSA-')) return 'AlmaLinux advisory';
  if (up.startsWith('RHSA-')) return 'Red Hat advisory';
  if (up.startsWith('USN-')) return 'Ubuntu advisory';
  if (up.startsWith('GHSA-')) return 'GitHub advisory';
  if (up.startsWith('CVE-')) return 'CVE';
  return 'security advisory';
};
