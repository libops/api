// Retry logic with exponential backoff

import { Interceptor } from "@connectrpc/connect";

interface RetryOptions {
  maxRetries?: number;
  initialDelay?: number;
  maxDelay?: number;
  backoffMultiplier?: number;
  retryableStatuses?: string[];
}

const DEFAULT_OPTIONS: Required<RetryOptions> = {
  maxRetries: 3,
  initialDelay: 1000, // 1 second
  maxDelay: 10000, // 10 seconds
  backoffMultiplier: 2,
  retryableStatuses: [
    'unavailable',
    'deadline_exceeded',
    'internal',
    'unknown',
  ],
};

/**
 * Sleep for a given number of milliseconds
 */
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Calculate delay with exponential backoff and jitter
 */
function calculateDelay(
  attempt: number,
  initialDelay: number,
  maxDelay: number,
  multiplier: number
): number {
  const exponentialDelay = initialDelay * Math.pow(multiplier, attempt);
  const cappedDelay = Math.min(exponentialDelay, maxDelay);

  // Add jitter (±25%)
  const jitter = cappedDelay * 0.25 * (Math.random() * 2 - 1);
  return Math.floor(cappedDelay + jitter);
}

/**
 * Check if an error is retryable
 */
function isRetryableError(error: unknown, retryableStatuses: string[]): boolean {
  if (error instanceof Error && 'code' in error) {
    const code = (error as any).code as string;
    return retryableStatuses.includes(code);
  }
  return false;
}

/**
 * Create a retry interceptor with exponential backoff
 */
export function createRetryInterceptor(options: RetryOptions = {}): Interceptor {
  const opts = { ...DEFAULT_OPTIONS, ...options };

  return (next) => async (req) => {
    let lastError: unknown;

    for (let attempt = 0; attempt <= opts.maxRetries; attempt++) {
      try {
        return await next(req);
      } catch (error) {
        lastError = error;

        // Don't retry if this is the last attempt
        if (attempt === opts.maxRetries) {
          break;
        }

        // Don't retry if error is not retryable
        if (!isRetryableError(error, opts.retryableStatuses)) {
          break;
        }

        // Calculate delay and wait
        const delay = calculateDelay(
          attempt,
          opts.initialDelay,
          opts.maxDelay,
          opts.backoffMultiplier
        );

        console.log(
          `Request failed, retrying in ${delay}ms (attempt ${attempt + 1}/${opts.maxRetries})`,
          error
        );

        await sleep(delay);
      }
    }

    // All retries failed, throw the last error
    throw lastError;
  };
}

/**
 * Default retry interceptor with sensible defaults
 */
export const retryInterceptor = createRetryInterceptor();
