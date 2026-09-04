import { api } from './api';

export interface ClientErrorLog {
  subsystem: string;
  error_type: 'javascript_error' | 'api_error' | 'ui_error' | 'validation_error';
  message: string;
  stack_trace?: string;
  metadata?: Record<string, any>;
  url: string;
  timestamp: string;
}

/**
 * ClientErrorLogger provides reliable frontend error logging with retry logic
 * Implements ETHOS #3: Assume Failure; Build for Resilience
 */
export class ClientErrorLogger {
  private maxRetries = 3;
  private baseDelayMs = 1000;
  private localStorageKey = 'redflag-error-queue';
  private offlineBuffer: ClientErrorLog[] = [];
  private isOnline = typeof navigator !== 'undefined' ? navigator.onLine : true;
  // Throttle identical errors so a render loop / error storm can't flood the
  // backend or the localStorage queue. Key = error_type|subsystem|message.
  private dedupWindowMs = 10000;
  private recentlyLogged = new Map<string, number>();
  private maxQueueLength = 100;

  constructor() {
    // Guard against non-DOM import contexts (vitest without jsdom, any future
    // SSR/prerender pass) — the singleton is constructed at module import.
    if (typeof window !== 'undefined') {
      window.addEventListener('online', () => {
        this.isOnline = true;
        this.flushOfflineBuffer().catch(() => {});
      });
      window.addEventListener('offline', () => { this.isOnline = false; });
    }
  }

  /**
   * Log an error with automatic retry and offline queuing
   */
  async logError(errorData: Omit<ClientErrorLog, 'url' | 'timestamp'>): Promise<void> {
    // Suppress an identical error seen within the dedup window. Matters now that
    // global error/unhandledrejection handlers can fire logError automatically.
    const key = `${errorData.error_type}|${errorData.subsystem}|${errorData.message}`;
    const now = Date.now();
    const last = this.recentlyLogged.get(key);
    if (last !== undefined && now - last < this.dedupWindowMs) {
      return;
    }
    this.recentlyLogged.set(key, now);
    // Keep the dedup map bounded under high message variety.
    if (this.recentlyLogged.size > 200) {
      for (const [k, t] of this.recentlyLogged) {
        if (now - t >= this.dedupWindowMs) this.recentlyLogged.delete(k);
      }
    }

    const fullError: ClientErrorLog = {
      ...errorData,
      url: window.location.href,
      timestamp: new Date().toISOString(),
    };

    // Try to send immediately
    try {
      await this.sendWithRetry(fullError);
      return;
    } catch (error) {
      // If failed after retries, queue for later
      this.queueForRetry(fullError);
    }
  }

  /**
   * Send error to backend with exponential backoff retry
   */
  private async sendWithRetry(error: ClientErrorLog): Promise<void> {
    for (let attempt = 1; attempt <= this.maxRetries; attempt++) {
      try {
        await api.post('/logs/client-error', error, {
          headers: { 'X-Error-Logger-Request': 'true' },
        });

        // Success, remove from queue if it was there
        this.removeFromQueue(error);
        return;
      } catch (error) {
        if (attempt === this.maxRetries) {
          throw error; // Rethrow after final attempt
        }

        // Exponential backoff
        await this.sleep(this.baseDelayMs * Math.pow(2, attempt - 1));
      }
    }
  }

  /**
   * Queue error for retry when network is available
   */
  private queueForRetry(error: ClientErrorLog): void {
    try {
      const queue = this.getQueue();
      queue.push({
        ...error,
        retryCount: (error as any).retryCount || 0,
        queuedAt: new Date().toISOString(),
      });

      // Cap the queue so an error storm can't grow localStorage without bound.
      if (queue.length > this.maxQueueLength) {
        queue.splice(0, queue.length - this.maxQueueLength);
      }

      // Save to localStorage for persistence
      localStorage.setItem(this.localStorageKey, JSON.stringify(queue));

      // Also keep in memory buffer
      this.offlineBuffer.push(error);
    } catch (storageError) {
      // localStorage might be full or unavailable
      console.warn('Failed to queue error for retry:', storageError);
    }
  }

  /**
   * Flush offline buffer when coming back online
   */
  async flushOfflineBuffer(): Promise<void> {
    if (!this.isOnline) return;

    const queue = this.getQueue();
    if (queue.length === 0) return;

    const failed: typeof queue = [];

    for (const queuedError of queue) {
      try {
        await this.sendWithRetry(queuedError);
      } catch (error) {
        failed.push(queuedError);
      }
    }

    // Update queue with remaining failed items
    if (failed.length < queue.length) {
      localStorage.setItem(this.localStorageKey, JSON.stringify(failed));
    }
  }

  /**
   * Get current error queue from localStorage
   */
  private getQueue(): any[] {
    try {
      const stored = localStorage.getItem(this.localStorageKey);
      return stored ? JSON.parse(stored) : [];
    } catch {
      return [];
    }
  }

  /**
   * Remove successfully sent error from queue
   */
  private removeFromQueue(sentError: ClientErrorLog): void {
    try {
      const queue = this.getQueue();
      const filtered = queue.filter(queued =>
        queued.timestamp !== sentError.timestamp ||
        queued.message !== sentError.message
      );

      if (filtered.length < queue.length) {
        localStorage.setItem(this.localStorageKey, JSON.stringify(filtered));
      }
    } catch {
      // Best effort cleanup
    }
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}

// Singleton instance
export const clientErrorLogger = new ClientErrorLogger();

// Auto-retry failed logs on app load
if (typeof window !== 'undefined') {
  window.addEventListener('load', () => {
    clientErrorLogger.flushOfflineBuffer().catch(() => {});
  });
}
