import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

// Utility function for combining class names
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Date formatting utilities
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

export const isOnline = (lastCheckin: string): boolean => {
  const lastCheck = new Date(lastCheckin);
  const now = new Date();
  const diffMs = now.getTime() - lastCheck.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  return diffMins < 15; // Consider online if checked in within 15 minutes (allows for 5min check-in + buffer)
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

// Status and severity utilities
export const getStatusColor = (status: string): string => {
  switch (status) {
    case 'online':
      return 'text-success-600 bg-success-100';
    case 'offline':
      return 'text-danger-600 bg-danger-100';
    case 'pending':
      return 'text-warning-600 bg-warning-100';
    case 'checking_dependencies':
      return 'text-blue-500 bg-blue-100';
    case 'pending_dependencies':
      return 'text-orange-600 bg-orange-100';
    case 'approved':
    case 'scheduled':
      return 'text-blue-600 bg-blue-100';
    case 'installing':
      return 'text-indigo-600 bg-indigo-100';
    case 'installed':
      return 'text-success-600 bg-success-100';
    case 'failed':
      return 'text-danger-600 bg-danger-100';
    default:
      return 'text-gray-600 bg-gray-100';
  }
};

export const getSeverityColor = (severity: string): string => {
  switch (severity) {
    case 'critical':
      return 'text-danger-600 bg-danger-100';
    case 'important':
    case 'high':
      return 'text-warning-600 bg-warning-100';
    case 'moderate':
    case 'medium':
      return 'text-blue-600 bg-blue-100';
    case 'low':
    case 'none':
      return 'text-gray-600 bg-gray-100';
    default:
      return 'text-gray-600 bg-gray-100';
  }
};

export const getPackageTypeIcon = (type: string): string => {
  switch (type) {
    case 'apt':
      return '📦';
    case 'docker':
      return '🐳';
    case 'yum':
    case 'dnf':
      return '🐧';
    case 'windows':
      return '🪟';
    case 'winget':
      return '📱';
    default:
      return '📋';
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