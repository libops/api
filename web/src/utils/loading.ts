// Loading state management

let activeRequests = 0;
let loadingTimeout: number | undefined;

/**
 * Show loading indicator
 */
function showLoading() {
  const loader = document.getElementById('global-loader');
  if (loader) {
    loader.classList.remove('hidden');
  }
}

/**
 * Hide loading indicator
 */
function hideLoading() {
  const loader = document.getElementById('global-loader');
  if (loader) {
    loader.classList.add('hidden');
  }
}

/**
 * Start a loading operation
 */
export function startLoading() {
  activeRequests++;

  // Only show loader if request takes longer than 200ms
  if (activeRequests === 1) {
    loadingTimeout = window.setTimeout(() => {
      showLoading();
    }, 200);
  }
}

/**
 * Stop a loading operation
 */
export function stopLoading() {
  activeRequests = Math.max(0, activeRequests - 1);

  if (activeRequests === 0) {
    if (loadingTimeout) {
      clearTimeout(loadingTimeout);
      loadingTimeout = undefined;
    }
    hideLoading();
  }
}

/**
 * Wrap an async function with loading state
 */
export async function withLoading<T>(fn: () => Promise<T>): Promise<T> {
  startLoading();
  try {
    return await fn();
  } finally {
    stopLoading();
  }
}

/**
 * Disable a button and show loading state
 */
export function setButtonLoading(button: HTMLButtonElement, loading: boolean) {
  if (loading) {
    button.disabled = true;
    button.dataset.originalText = button.textContent || '';
    button.innerHTML = `
      <svg class="animate-spin inline-block w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
      </svg>
      Loading...
    `;
  } else {
    button.disabled = false;
    button.textContent = button.dataset.originalText || '';
  }
}
