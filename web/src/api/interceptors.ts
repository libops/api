import { Interceptor } from "@connectrpc/connect";
import { showNotification } from "@/utils/helpers";
import { startLoading, stopLoading } from "@/utils/loading";

export { retryInterceptor } from "./retry";

/**
 * Loading interceptor that shows global loading state
 */
export const loadingInterceptor: Interceptor = (next) => async (req) => {
  startLoading();
  try {
    return await next(req);
  } finally {
    stopLoading();
  }
};

/**
 * Error handling interceptor that shows user-friendly error messages
 */
export const errorInterceptor: Interceptor = (next) => async (req) => {
  try {
    return await next(req);
  } catch (error: unknown) {
    // Extract error message from ConnectError
    let errorMessage = "An unexpected error occurred";

    if (error instanceof Error) {
      // ConnectRPC errors have a message property
      if (error.message) {
        errorMessage = error.message;
      }

      // Check for specific error codes
      if ('code' in error) {
        const code = (error as any).code;
        switch (code) {
          case 'permission_denied':
          case 'unauthenticated':
            errorMessage = "You don't have permission to perform this action";
            break;
          case 'not_found':
            errorMessage = "The requested resource was not found";
            break;
          case 'already_exists':
            errorMessage = "This resource already exists";
            break;
          case 'invalid_argument':
            errorMessage = "Invalid input provided";
            break;
          case 'unavailable':
            errorMessage = "Service temporarily unavailable. Please try again.";
            break;
        }
      }
    }

    console.error("API Error:", error);
    showNotification("error", errorMessage);
    throw error;
  }
};

/**
 * Logging interceptor for development
 */
export const loggingInterceptor: Interceptor = (next) => async (req) => {
  const start = Date.now();
  console.log(`→ ${req.method.name}`, req.message);

  try {
    const response = await next(req);
    const duration = Date.now() - start;
    console.log(`← ${req.method.name} (${duration}ms)`, response.message);
    return response;
  } catch (error) {
    const duration = Date.now() - start;
    console.error(`✗ ${req.method.name} (${duration}ms)`, error);
    throw error;
  }
};
