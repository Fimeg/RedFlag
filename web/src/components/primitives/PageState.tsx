import React from 'react';
import { AlertTriangle, Loader2, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * PageState — loading/error/empty/content state wrapper.
 *
 * Renders exactly one of its states. Most pages follow the same ternary:
 *   `{loading ? <Loader/> : error ? <Error/> : empty ? <Empty/> : <Content/>}`
 *
 * Props:
 *   loading        — true while data is fetching
 *   error          — error message string, or null/undefined when no error
 *   empty          — true when there's nothing to show
 *   emptyTitle     — heading for the empty state (default "No items found")
 *   emptyMessage   — subtext for the empty state
 *   loadingTitle   — subtext shown while loading (default "")
 *   errorTitle     — heading for the error state (default "Something went wrong")
 *   errorAction    — called when the "Retry" button is clicked
 *   skeleton       — optional custom skeleton; defaults to `<PageSkeleton rows={3} />`
 *   icon           — icon component for empty state (default Package from lucide)
 *   children       — rendered when all other states are false
 */
interface PageStateProps {
  loading: boolean;
  error?: string | null;
  empty: boolean;
  emptyTitle?: string;
  emptyMessage?: string;
  loadingTitle?: string;
  errorTitle?: string;
  errorAction?: () => void;
  skeleton?: React.ReactNode;
  icon?: React.ComponentType<{ className?: string }>;
  children?: React.ReactNode;
}

/** Generic pulsing-box skeleton. */
export const PageSkeleton: React.FC<{ rows?: number; className?: string }> = ({
  rows = 3,
  className,
}) => (
  <div className={cn('animate-pulse', className)}>
    <div className="bg-white rounded-lg border border-gray-200">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="p-4 border-b border-gray-200 last:border-b-0">
          <div className="h-4 bg-gray-200 rounded w-1/4 mb-2" />
          <div className="h-3 bg-gray-200 rounded w-1/2" />
        </div>
      ))}
    </div>
  </div>
);

/** Centered block with icon + heading + subtext. */
const CenterBlock: React.FC<{
  Icon: React.ComponentType<{ className?: string }>;
  title: string;
  message?: string;
  children?: React.ReactNode;
}> = ({ Icon, title, message, children }) => (
  <div className="text-center py-12">
    <Icon className="mx-auto h-12 w-12 text-gray-400" />
    <h3 className="mt-2 text-sm font-medium text-gray-900">{title}</h3>
    {message && <p className="mt-1 text-sm text-gray-500">{message}</p>}
    {children}
  </div>
);

const PageState: React.FC<PageStateProps> = ({
  loading,
  error,
  empty,
  emptyTitle = 'No items found',
  emptyMessage,
  loadingTitle,
  errorTitle = 'Something went wrong',
  errorAction,
  skeleton,
  icon,
  children,
}) => {
  const IconComponent = icon || Loader2;

  if (loading) {
    return (
      <div>
        {loadingTitle && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-5 w-5 animate-spin text-gray-400 mr-2" />
            <span className="text-sm text-gray-500">{loadingTitle}</span>
          </div>
        )}
        {skeleton ?? <PageSkeleton />}
      </div>
    );
  }

  if (error) {
    return (
      <CenterBlock Icon={AlertTriangle} title={errorTitle} message={error}>
        {errorAction && (
          <button
            onClick={errorAction}
            className="mt-3 inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-blue-700 bg-blue-50 border border-blue-200 rounded-md hover:bg-blue-100 transition-colors"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Retry
          </button>
        )}
      </CenterBlock>
    );
  }

  if (empty) {
    return <CenterBlock Icon={IconComponent as React.ComponentType<{ className?: string }>} title={emptyTitle} message={emptyMessage} />;
  }

  return <>{children}</>;
};

export default PageState;
