/**
 * Clipboard utilities for MrRSS.
 *
 * Works in every build/context:
 * - Desktop webview and HTTPS pages: the async Clipboard API.
 * - The headless server/web build accessed over plain http://LAN-IP (an
 *   "insecure context"): browsers disable navigator.clipboard there, so we
 *   fall back to a hidden <textarea> + document.execCommand('copy'), which
 *   still works over http.
 */

/**
 * Legacy clipboard copy via a hidden textarea. Works in insecure contexts
 * (plain http on a LAN IP) where navigator.clipboard is unavailable.
 */
function execCommandCopy(text: string): boolean {
  try {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    // Keep it off-screen and non-disruptive
    textarea.style.position = 'fixed';
    textarea.style.top = '0';
    textarea.style.left = '0';
    textarea.style.width = '1px';
    textarea.style.height = '1px';
    textarea.style.opacity = '0';
    textarea.setAttribute('readonly', '');
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(textarea);
    return ok;
  } catch (error) {
    console.error('execCommand copy failed:', error);
    return false;
  }
}

/**
 * Copy text to clipboard with graceful fallback.
 * @param text Text to copy
 * @returns Promise that resolves to true if successful, false otherwise
 */
async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) {
    console.warn('copyToClipboard: text is empty');
    return false;
  }

  // Preferred: async Clipboard API (secure contexts: https / localhost / desktop)
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (error) {
    console.warn('navigator.clipboard failed, falling back:', error);
  }

  // Fallback: works over plain http (server/web build on a LAN IP)
  return execCommandCopy(text);
}

/**
 * Copy arbitrary text to clipboard (with insecure-context fallback).
 * @param text Text to copy
 * @returns Promise that resolves to true if successful
 */
export async function copyText(text: string): Promise<boolean> {
  return copyToClipboard(text);
}

/**
 * Copy article URL to clipboard
 * @param url Article URL
 * @returns Promise that resolves to true if successful
 */
export async function copyArticleLink(url: string): Promise<boolean> {
  return copyToClipboard(url);
}

/**
 * Copy article title to clipboard
 * @param title Article title
 * @returns Promise that resolves to true if successful
 */
export async function copyArticleTitle(title: string): Promise<boolean> {
  return copyToClipboard(title);
}

/**
 * Copy feed URL to clipboard
 * @param url Feed URL
 * @returns Promise that resolves to true if successful
 */
export async function copyFeedURL(url: string): Promise<boolean> {
  return copyToClipboard(url);
}
