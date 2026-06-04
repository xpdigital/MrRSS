/**
 * Image drag-out support for MrRSS.
 *
 * Allows dragging images from the article reading view directly out of the
 * application window into Finder / File Explorer (or other apps) so they are
 * saved as local files.
 *
 * Platform behavior:
 * - macOS (WKWebView): WebKit natively supports dragging images out — it puts
 *   a "file promise" on the drag pasteboard and writes the image file when it
 *   is dropped on Finder/Desktop. We must NOT call e.dataTransfer.setData()
 *   on WebKit, because author-supplied drag data replaces the default
 *   (file-promise) drag payload and would break the native behavior.
 * - Windows (WebView2, Chromium-based): Chromium supports the non-standard
 *   "DownloadURL" drag data type. Dropping on Explorer/Desktop makes the OS
 *   download the image to the drop target.
 * - Linux (WebKitGTK): same WebKit code path as macOS; drag-out provides
 *   text/uri-list. Saving depends on the file manager (best effort).
 */

/** True when running inside a Chromium-based webview (e.g. WebView2 on Windows). */
const isChromium = /Chrome\//.test(navigator.userAgent);

/**
 * Resolve the original (remote) image URL from a possibly-proxied src.
 * MrRSS proxies article images through /api/media/proxy?url_b64=<base64>
 * when media caching is enabled.
 *
 * @param src Image src attribute (may be a proxy URL, relative URL or data URL)
 * @returns The original absolute image URL, or the input if not proxied
 */
export function getOriginalImageUrl(src: string): string {
  if (!src) return src;
  try {
    const u = new URL(src, window.location.href);
    if (u.pathname === '/api/media/proxy') {
      const b64 = u.searchParams.get('url_b64');
      if (b64) {
        try {
          return atob(b64);
        } catch {
          // Fall through to other params if base64 is malformed
        }
      }
      const direct = u.searchParams.get('url');
      if (direct) return direct;
    }
    return u.href;
  } catch {
    return src;
  }
}

const IMAGE_EXT_RE = /\.(jpg|jpeg|png|gif|webp|svg|bmp|avif|ico)$/i;

const MIME_BY_EXT: Record<string, string> = {
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  png: 'image/png',
  gif: 'image/gif',
  webp: 'image/webp',
  svg: 'image/svg+xml',
  bmp: 'image/bmp',
  avif: 'image/avif',
  ico: 'image/x-icon',
};

/**
 * Derive a safe local filename from the original image URL.
 * Falls back to "image.png" when nothing meaningful can be extracted.
 */
export function filenameForImage(originalUrl: string): string {
  let filename = 'image';
  try {
    const u = new URL(originalUrl, window.location.href);
    const segments = u.pathname.split('/').filter((s) => s.length > 0);
    if (segments.length > 0) {
      const last = segments[segments.length - 1].split('?')[0];
      const sanitized = last.replace(/[^a-zA-Z0-9._-]/g, '_').replace(/^[._]+|[._]+$/g, '');
      if (sanitized) filename = sanitized;
    }
  } catch {
    // Keep fallback name
  }
  if (!IMAGE_EXT_RE.test(filename)) {
    filename += '.png';
  }
  return filename;
}

/** MIME type for a filename based on its extension (defaults to image/png). */
function mimeForFilename(filename: string): string {
  const ext = filename.split('.').pop()?.toLowerCase() || '';
  return MIME_BY_EXT[ext] || 'image/png';
}

/**
 * Tracks elements that already have the drag-out listener attached.
 * A WeakSet (instead of a data-* flag) is used on purpose: article images are
 * re-created via cloneNode() when content changes (e.g. after translation),
 * which copies attributes but NOT event listeners. A WeakSet guard correctly
 * treats clones as new elements while still preventing duplicate listeners
 * when called repeatedly on the same element.
 */
const dragOutAttached = new WeakSet<HTMLImageElement>();

/**
 * Enable drag-out-to-save on an article image.
 * Safe to call multiple times on the same element.
 *
 * @param img The image element inside the rendered article content
 */
export function enableImageDragOut(img: HTMLImageElement): void {
  if (dragOutAttached.has(img)) {
    return;
  }
  dragOutAttached.add(img);

  // Images are draggable by default, but be explicit in case content
  // or custom CSS disabled it.
  img.draggable = true;

  img.addEventListener('dragstart', (e: DragEvent) => {
    const dt = e.dataTransfer;
    if (!dt) return;

    dt.effectAllowed = 'copy';

    // WebKit (macOS/Linux): leave the default drag payload untouched so the
    // native file promise keeps working. Only Chromium needs custom data.
    if (!isChromium) {
      return;
    }

    const src = img.currentSrc || img.src;
    if (!src) return;

    let absoluteSrc = src;
    try {
      absoluteSrc = new URL(src, window.location.href).href;
    } catch {
      // Keep src as-is
    }

    const originalUrl = getOriginalImageUrl(src);
    const filename = filenameForImage(originalUrl);
    const mimeType = mimeForFilename(filename);

    // Chromium-only: dropping on Explorer/Desktop downloads the file.
    // Use the (possibly proxied) absolute src so cached images save instantly
    // and anti-hotlinking is handled by the backend proxy.
    dt.setData('DownloadURL', `${mimeType}:${filename}:${absoluteSrc}`);

    // Generic fallbacks so dropping into other apps (editors, chat, browser)
    // inserts the original remote URL.
    dt.setData('text/uri-list', originalUrl);
    dt.setData('text/plain', originalUrl);
  });
}
