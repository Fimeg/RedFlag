import React from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * Pagination — page navigation with prev/next, number buttons, and result summary.
 *
 * Usage:
 * ```tsx
 * <Pagination
 *   page={currentPage}
 *   total={totalCount}
 *   pageSize={pageSize}
 *   onChange={setCurrentPage}
 * />
 * ```
 *
 * Props:
 *   page        — current page (1-indexed)
 *   total       — total number of items across all pages
 *   pageSize    — items per page
 *   onChange    — called with the new page number
 *   windowSize  — number of page buttons to show (default 5)
 *   className   — additional wrapper classes
 */
interface PaginationProps {
  page: number;
  total: number;
  pageSize: number;
  onChange: (page: number) => void;
  windowSize?: number;
  className?: string;
}

const Pagination: React.FC<PaginationProps> = ({
  page,
  total,
  pageSize,
  onChange,
  windowSize = 5,
  className,
}) => {
  const totalPages = Math.ceil(total / pageSize);

  if (totalPages <= 1) return null;

  const hasPrev = page > 1;
  const hasNext = page < totalPages;

  // Generate the page-number window
  const pageNumbers = (() => {
    if (totalPages <= windowSize) {
      return Array.from({ length: totalPages }, (_, i) => i + 1);
    }

    if (page <= 3) {
      return Array.from({ length: windowSize }, (_, i) => i + 1);
    }

    if (page >= totalPages - 2) {
      return Array.from({ length: windowSize }, (_, i) => totalPages - windowSize + i + 1);
    }

    return Array.from({ length: windowSize }, (_, i) => page - 2 + i);
  })();

  // Button classes
  const btnBase =
    'relative inline-flex items-center px-4 py-2 border text-sm font-medium';
  const btnActive = 'z-10 bg-primary-50 border-primary-500 text-primary-600';
  const btnInactive =
    'bg-white border-gray-300 text-gray-500 hover:bg-gray-50';
  const btnDisabled = 'opacity-50 cursor-not-allowed';
  const btnNav =
    'relative inline-flex items-center px-2 py-2 border border-gray-300 bg-white text-sm font-medium text-gray-500 hover:bg-gray-50';

  const from = (page - 1) * pageSize + 1;
  const to = Math.min(page * pageSize, total);

  return (
    <div
      className={cn(
        'bg-white px-4 py-3 border-t border-gray-200 sm:px-6',
        className
      )}
    >
      <div className="flex items-center justify-between">
        {/* Mobile prev/next */}
        <div className="flex-1 flex justify-between sm:hidden">
          <button
            onClick={() => onChange(page - 1)}
            disabled={!hasPrev}
            className={cn(
              btnNav,
              !hasPrev && btnDisabled
            )}
          >
            Previous
          </button>
          <button
            onClick={() => onChange(page + 1)}
            disabled={!hasNext}
            className={cn(
              btnNav,
              !hasNext && btnDisabled
            )}
          >
            Next
          </button>
        </div>

        {/* Desktop */}
        <div className="hidden sm:flex-1 sm:flex sm:items-center sm:justify-between">
          {/* Result summary */}
          <div>
            <p className="text-sm text-gray-700">
              Showing{' '}
              <span className="font-medium">{total > 0 ? from : 0}</span> to{' '}
              <span className="font-medium">{to}</span> of{' '}
              <span className="font-medium">{total}</span> results
            </p>
          </div>

          {/* Page buttons */}
          <nav
            className="relative z-0 inline-flex rounded-md shadow-sm -space-x-px"
            aria-label="Pagination"
          >
            {/* Previous */}
            <button
              onClick={() => onChange(page - 1)}
              disabled={!hasPrev}
              className={cn('rounded-l-md', btnNav, !hasPrev && btnDisabled)}
            >
              <span className="sr-only">Previous</span>
              <ChevronLeft className="h-5 w-5" />
            </button>

            {pageNumbers.map((num) => {
              return (
                <button
                  key={num}
                  onClick={() => onChange(num)}
                  className={cn(
                    btnBase,
                    page === num ? btnActive : btnInactive
                  )}
                >
                  {num}
                </button>
              );
            })}

            {/* Next */}
            <button
              onClick={() => onChange(page + 1)}
              disabled={!hasNext}
              className={cn('rounded-r-md', btnNav, !hasNext && btnDisabled)}
            >
              <span className="sr-only">Next</span>
              <ChevronRight className="h-5 w-5" />
            </button>
          </nav>
        </div>
      </div>
    </div>
  );
};

export default Pagination;
