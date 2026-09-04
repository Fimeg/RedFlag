import React, {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
} from 'react';
import Modal from './Modal';

/**
 * ConfirmDialog — imperative async confirm() primitive.
 *
 * Usage (wrap the app once with ConfirmProvider, then call useConfirm in any component):
 * ```tsx
 * // main.tsx
 * <ConfirmProvider>
 *   <App />
 * </ConfirmProvider>
 *
 * // any component
 * const confirm = useConfirm();
 * if (!(await confirm({ title: 'Delete?', body: 'Cannot be undone.', danger: true }))) return;
 * doTheDestructiveThing();
 * ```
 *
 * Keyboard behavior:
 *   Escape  — always cancels (handled by Modal).
 *   Enter   — confirms on non-danger dialogs only.
 *   Danger dialogs require an explicit click on the confirm button; Enter does nothing.
 *   Rationale: an accidental Enter on a destructive action (delete, revoke, reset) is a
 *   worse outcome than the minor friction of a deliberate click.
 */

export interface ConfirmOptions {
  title: string;
  body: React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** Render the confirm button in red and block Enter-to-confirm. */
  danger?: boolean;
}

type Resolver = (value: boolean) => void;

interface ConfirmContextValue {
  confirm: (opts: ConfirmOptions) => Promise<boolean>;
}

const ConfirmContext = createContext<ConfirmContextValue | null>(null);

interface DialogState {
  opts: ConfirmOptions;
  resolve: Resolver;
}

export const ConfirmProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const resolverRef = useRef<Resolver | null>(null);

  const confirm = useCallback((opts: ConfirmOptions): Promise<boolean> => {
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
      setDialog({ opts, resolve });
    });
  }, []);

  const settle = useCallback((value: boolean) => {
    resolverRef.current?.(value);
    resolverRef.current = null;
    setDialog(null);
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Enter' && dialog && !dialog.opts.danger) {
        e.preventDefault();
        settle(true);
      }
    },
    [dialog, settle],
  );

  return (
    <ConfirmContext.Provider value={{ confirm }}>
      {children}
      {dialog && (
        <Modal
          open
          onClose={() => settle(false)}
          title={dialog.opts.title}
          maxWidth="sm"
        >
          <div onKeyDown={handleKeyDown}>
            <Modal.Body>
              <div className="text-sm text-gray-700">{dialog.opts.body}</div>
            </Modal.Body>
            <Modal.Footer>
              {/* Danger dialogs focus Cancel so a stray Enter lands on the safe
                  action; the wrapper's Enter handler is also gated on !danger.
                  Non-danger dialogs focus the confirm button. */}
              <button
                type="button"
                autoFocus={!dialog.opts.danger}
                onClick={() => settle(true)}
                className={
                  dialog.opts.danger
                    ? 'inline-flex items-center px-4 py-2 text-sm font-medium text-white bg-red-600 border border-red-700 rounded hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500'
                    : 'inline-flex items-center px-4 py-2 text-sm font-medium text-white bg-blue-600 border border-blue-700 rounded hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500'
                }
              >
                {dialog.opts.confirmLabel ?? (dialog.opts.danger ? 'Confirm' : 'OK')}
              </button>
              <button
                type="button"
                autoFocus={dialog.opts.danger}
                onClick={() => settle(false)}
                className="inline-flex items-center px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {dialog.opts.cancelLabel ?? 'Cancel'}
              </button>
            </Modal.Footer>
          </div>
        </Modal>
      )}
    </ConfirmContext.Provider>
  );
};

/**
 * Returns the async confirm() function from the nearest ConfirmProvider.
 * Throws if called outside a ConfirmProvider.
 */
export function useConfirm(): (opts: ConfirmOptions) => Promise<boolean> {
  const ctx = useContext(ConfirmContext);
  if (!ctx) {
    throw new Error('useConfirm must be used within a ConfirmProvider');
  }
  return ctx.confirm;
}
