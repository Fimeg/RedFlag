import React, { useEffect, useRef, useCallback } from 'react';
import { X } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * Modal — compound dialog with header/body/footer sections.
 *
 * Usage:
 * ```tsx
 * <Modal open onClose={() => setOpen(false)} title="Installation Logs" maxWidth="4xl">
 *   <Modal.Body>
 *     ... content ...
 *   </Modal.Body>
 *   <Modal.Footer>
 *     <button onClick={...}>Close</button>
 *   </Modal.Footer>
 * </Modal>
 * ```
 *
 * Handles: Escape key, overlay click, focus trap, scroll lock.
 * Only renders when `open` is true.
 *
 * maxHeight: constrains the panel height and switches the panel to flex-col layout so
 * Modal.Body with scrollable=true can absorb remaining space. Use when inner content
 * may overflow (logs, long lists, tabbed detail views).
 */
interface ModalProps {
  open: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '4xl';
  /** Constrain panel height and switch to flex-col. Pair with Modal.Body scrollable. */
  maxHeight?: '80vh' | '85vh';
  children: React.ReactNode;
}

const WIDTH_MAP: Record<string, string> = {
  sm: 'sm:max-w-sm',
  md: 'sm:max-w-md',
  lg: 'sm:max-w-lg',
  xl: 'sm:max-w-xl',
  '2xl': 'sm:max-w-2xl',
  '4xl': 'sm:max-w-4xl',
};

const MAX_HEIGHT_MAP: Record<string, string> = {
  '80vh': 'max-h-[80vh]',
  '85vh': 'max-h-[85vh]',
};

const Modal: React.FC<ModalProps> & { Body: typeof ModalBody; Footer: typeof ModalFooter } = ({
  open,
  onClose,
  title,
  maxWidth = '2xl',
  maxHeight,
  children,
}) => {
  const overlayRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);

  // Close on Escape
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    },
    [onClose]
  );

  // Focus trap: focus the content on open, restore focus on close
  useEffect(() => {
    if (open) {
      document.addEventListener('keydown', handleKeyDown);
      document.body.style.overflow = 'hidden';
      // Focus the content after a tick so the transition doesn't fight it.
      // Yield if a child already took focus (e.g. an autoFocus button) —
      // React commits autoFocus before this frame runs.
      requestAnimationFrame(() => {
        const el = contentRef.current;
        if (!el) return;
        if (document.activeElement && el.contains(document.activeElement) && document.activeElement !== el) return;
        el.focus();
      });
    }
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = '';
    };
  }, [open, handleKeyDown]);

  if (!open) return null;

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-50 overflow-y-auto"
      onClick={(e) => {
        if (e.target === overlayRef.current) onClose();
      }}
    >
      <div className="flex min-h-full items-end justify-center p-4 text-center sm:items-center sm:p-0">
        <div
          ref={contentRef}
          tabIndex={-1}
          className={cn(
            'relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:w-full border border-gray-200',
            WIDTH_MAP[maxWidth],
            maxHeight && MAX_HEIGHT_MAP[maxHeight],
            maxHeight && 'flex flex-col',
          )}
        >
          {/* Header */}
          {title && (
            <div className="bg-white border-b border-gray-200 px-6 py-4 flex items-center justify-between rounded-t-lg">
              <h3 className="text-lg font-semibold text-gray-900">{title}</h3>
              <button
                type="button"
                onClick={onClose}
                className="text-gray-400 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-primary-500 rounded-md p-1"
                aria-label="Close"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
          )}

          {children}
        </div>
      </div>
    </div>
  );
};

// ─── Sub-components ────────────────────────────────────────────────────────────

interface ModalBodyProps {
  children: React.ReactNode;
  className?: string;
  /** Flex-grow + overflow-y-auto. Use when the parent Modal has maxHeight set. */
  scrollable?: boolean;
}

const ModalBody: React.FC<ModalBodyProps> = ({ children, className, scrollable }) => (
  <div className={cn('bg-white px-6 py-4', scrollable && 'flex-1 overflow-y-auto', className)}>{children}</div>
);

interface ModalFooterProps {
  children: React.ReactNode;
  className?: string;
}

const ModalFooter: React.FC<ModalFooterProps> = ({ children, className }) => (
  <div
    className={cn(
      'bg-gray-50 px-6 py-4 sm:flex sm:flex-row-reverse rounded-b-lg border-t border-gray-200 gap-2',
      className
    )}
  >
    {children}
  </div>
);

Modal.Body = ModalBody;
Modal.Footer = ModalFooter;

export default Modal;
