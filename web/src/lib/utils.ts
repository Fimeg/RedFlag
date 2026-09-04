import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { Package, Container, Monitor, AppWindow, ClipboardList, type LucideIcon } from 'lucide-react';

// Utility function for combining class names
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Date formatting utilities

// formatTimeOnly — extracts the time portion of an ISO/DB timestamp as HH:MM:SS AM/PM.
// Used where only the time-of-day is shown (e.g. history timeline rows).
export const formatTimeOnly = (timestamp: string): string => {
  const date = new Date(timestamp);
  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
};

// formatUnixTime — converts a Unix epoch in seconds to a locale date+time string.
export const formatUnixTime = (seconds: number): string => {
  if (!seconds) return '—';
  return new Date(seconds * 1000).toLocaleString();
};

export const formatDate = (dateString: string): string => {
  const date = new Date(dateString);
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

export const formatDateTime = (dateString: string | null): string => {
  if (!dateString) return 'Never';

  const date = new Date(dateString);
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

export const formatRelativeTime = (dateString: string): string => {
  if (!dateString) return 'Never';

  let date: Date;
  try {
    // Handle various timestamp formats
    if (dateString.includes('T') && dateString.includes('Z')) {
      // ISO 8601 format
      date = new Date(dateString);
    } else if (dateString.includes(' ')) {
      // Database format like "2025-01-15 10:30:00"
      date = new Date(dateString.replace(' ', 'T') + 'Z');
    } else {
      // Try direct parsing
      date = new Date(dateString);
    }

    // Check if date is invalid
    if (isNaN(date.getTime())) {
      console.warn('Invalid date string:', dateString);
      return 'Invalid Date';
    }
  } catch (error) {
    console.warn('Error parsing date:', dateString, error);
    return 'Invalid Date';
  }

  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) {
    return 'Just now';
  } else if (diffMins < 60) {
    return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
  } else if (diffHours < 24) {
    return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
  } else if (diffDays < 7) {
    return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
  } else {
    return formatDate(date.toISOString());
  }
};

// Threshold must match server (stats.go: 10min). Make configurable later via
// a server-side setting surfaced on the agent model; hardcode for now.
const ONLINE_THRESHOLD_MS = 10 * 60 * 1000; // 10 minutes

export const isOnline = (lastCheckin: string): boolean => {
  const lastCheck = new Date(lastCheckin);
  const now = new Date();
  return (now.getTime() - lastCheck.getTime()) < ONLINE_THRESHOLD_MS;
};

// Size formatting utilities
export const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';

  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// Version comparison utilities
export const versionCompare = (v1: string, v2: string): number => {
  const parts1 = v1.split('.').map(Number);
  const parts2 = v2.split('.').map(Number);

  const maxLength = Math.max(parts1.length, parts2.length);

  for (let i = 0; i < maxLength; i++) {
    const part1 = parts1[i] || 0;
    const part2 = parts2[i] || 0;

    if (part1 > part2) return 1;
    if (part1 < part2) return -1;
  }

  return 0;
};

// Status and severity utilities — thin re-exports delegating to the canonical
// truth table in components/primitives/statusColors.ts. New code should import
// from there directly; these shims keep existing callers working unchanged.
import { packageStatusColor, packageSeverityColor } from '@/components/primitives/statusColors';

export const getStatusColor = packageStatusColor;
export const getSeverityColor = packageSeverityColor;

// Package-type icons — lucide components, consistent with the iconography
// used everywhere else in the UI (UI-DASHBOARD-AUDIT #4: the old emoji
// glyphs clashed with the aesthetic).
export const getPackageTypeIcon = (type: string): LucideIcon => {
  switch (type) {
    case 'apt':
    case 'yum':
    case 'dnf':
      return Package;
    case 'docker':
      return Container;
    case 'windows':
    case 'windows_update':
      return Monitor;
    case 'winget':
      return AppWindow;
    default:
      return ClipboardList;
  }
};

// Filter and search utilities
export const filterUpdates = (
  updates: any[],
  filters: {
    status: string[];
    severity: string[];
    type: string[];
    search: string;
  }
): any[] => {
  return updates.filter(update => {
    // Status filter
    if (filters.status.length > 0 && !filters.status.includes(update.status)) {
      return false;
    }

    // Severity filter
    if (filters.severity.length > 0 && !filters.severity.includes(update.severity)) {
      return false;
    }

    // Type filter
    if (filters.type.length > 0 && !filters.type.includes(update.package_type)) {
      return false;
    }

    // Search filter
    if (filters.search) {
      const searchLower = filters.search.toLowerCase();
      return (
        update.package_name.toLowerCase().includes(searchLower) ||
        update.current_version.toLowerCase().includes(searchLower) ||
        update.available_version.toLowerCase().includes(searchLower)
      );
    }

    return true;
  });
};

// Error handling utilities
export const getErrorMessage = (error: any): string => {
  if (typeof error === 'string') {
    return error;
  }

  if (error?.message) {
    return error.message;
  }

  if (error?.response?.data?.message) {
    return error.response.data.message;
  }

  return 'An unexpected error occurred';
};

// Debounce utility
export const debounce = <T extends (...args: any[]) => any>(
  func: T,
  wait: number
): ((...args: Parameters<T>) => void) => {
  let timeout: ReturnType<typeof setTimeout>;

  return (...args: Parameters<T>) => {
    clearTimeout(timeout);
    timeout = setTimeout(() => func(...args), wait);
  };
};

// Local storage utilities
export const storage = {
  get: (key: string): string | null => {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  },

  set: (key: string, value: string): void => {
    try {
      localStorage.setItem(key, value);
    } catch {
      // Silent fail for storage issues
    }
  },

  remove: (key: string): void => {
    try {
      localStorage.removeItem(key);
    } catch {
      // Silent fail for storage issues
    }
  },

  getJSON: <T = any>(key: string): T | null => {
    try {
      const item = localStorage.getItem(key);
      return item ? JSON.parse(item) : null;
    } catch {
      return null;
    }
  },

  setJSON: (key: string, value: any): void => {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      // Silent fail for storage issues
    }
  },
};

// advisoryUrl resolves a vulnerability ID to its canonical advisory page.
// Unknown prefixes fall back to OSV.dev.
export const advisoryUrl = (id: string): string => {
  const up = id.toUpperCase();
  if (up.startsWith('CVE-')) return `https://nvd.nist.gov/vuln/detail/${id}`;
  if (up.startsWith('GHSA-')) return `https://github.com/advisories/${id}`;
  if (up.startsWith('ALSA-')) return `https://errata.almalinux.org/${id.split('-')[1]}/${id}.html`;
  if (up.startsWith('RHSA-') || up.startsWith('RHBA-') || up.startsWith('RLSA-'))
    return `https://access.redhat.com/errata/${id}`;
  if (up.startsWith('DSA-')) return `https://security-tracker.debian.org/tracker/${id}`;
  if (up.startsWith('USN-')) return `https://ubuntu.com/security/notices/${id}`;
  return `https://osv.dev/vulnerability/${id}`;
};

// cveSeverityBadge maps an OSV qualitative severity string to display label + Tailwind classes.
export const cveSeverityBadge = (severity?: string): { label: string; cls: string } => {
  const s = (severity || '').toUpperCase();
  switch (s) {
    case 'CRITICAL':
      return { label: 'CRITICAL', cls: 'bg-red-100 text-red-800 border-red-300' };
    case 'HIGH':
      return { label: 'HIGH', cls: 'bg-red-50 text-red-700 border-red-200' };
    case 'MODERATE':
    case 'MEDIUM':
      return { label: 'MEDIUM', cls: 'bg-amber-100 text-amber-800 border-amber-300' };
    case 'LOW':
      return { label: 'LOW', cls: 'bg-yellow-50 text-yellow-700 border-yellow-200' };
    default:
      return { label: 'UNSCORED', cls: 'bg-gray-100 text-gray-600 border-gray-300' };
  }
};