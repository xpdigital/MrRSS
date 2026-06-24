import { ref, onBeforeUnmount } from 'vue';

// localStorage keys for persisting the user's manually-dragged panel widths so
// they survive reloads, layout/settings changes and app restarts.
const LS_SIDEBAR_WIDTH = 'mrrss.sidebarWidth';
const LS_ARTICLE_WIDTH = 'mrrss.articleListWidth';

function readSavedWidth(key: string): number | null {
  const raw = localStorage.getItem(key);
  if (raw === null) return null;
  const n = parseInt(raw, 10);
  return Number.isFinite(n) ? n : null;
}

export function useResizablePanels() {
  const savedSidebar = readSavedWidth(LS_SIDEBAR_WIDTH);
  const savedArticle = readSavedWidth(LS_ARTICLE_WIDTH);

  const sidebarWidth = ref<number>(savedSidebar ?? 256);
  const articleListWidth = ref<number>(savedArticle ?? 400);
  const isResizingSidebar = ref<boolean>(false);
  const isResizingArticleList = ref<boolean>(false);
  const compactMode = ref<boolean>(false);

  // Track if the user has manually resized the article list. If a saved width
  // was restored, treat it as user-set so programmatic defaults don't override.
  const userManuallyResized = ref<boolean>(savedArticle !== null);

  // Track initial mouse position when starting resize
  const initialMouseX = ref<number>(0);
  const initialArticleListWidth = ref<number>(400);

  // Set compact mode state (doesn't change width by itself)
  function setCompactMode(enabled: boolean): void {
    compactMode.value = enabled;
  }

  // Set article list width (called when settings are loaded or layout/compact
  // mode changes). Once the user has manually dragged the width, we keep their
  // choice and ignore these programmatic defaults.
  function setArticleListWidth(width: number): void {
    if (userManuallyResized.value) return;
    articleListWidth.value = width;
  }

  // Sidebar resize handlers
  function startResizeSidebar(): void {
    isResizingSidebar.value = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    window.addEventListener('mousemove', handleResizeSidebar);
    window.addEventListener('mouseup', stopResizeSidebar);
  }

  function handleResizeSidebar(): void {
    if (!isResizingSidebar.value) return;
    const newWidth = (window.event as MouseEvent).clientX;
    if (newWidth >= 180 && newWidth <= 450) {
      sidebarWidth.value = newWidth;
    }
  }

  function stopResizeSidebar(): void {
    isResizingSidebar.value = false;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    window.removeEventListener('mousemove', handleResizeSidebar);
    window.removeEventListener('mouseup', stopResizeSidebar);
    // Persist the chosen sidebar width
    try {
      localStorage.setItem(LS_SIDEBAR_WIDTH, String(Math.round(sidebarWidth.value)));
    } catch {
      // ignore storage errors
    }
  }

  // Article list resize handlers
  function startResizeArticleList(event: MouseEvent): void {
    isResizingArticleList.value = true;
    // Store initial mouse position and article list width
    initialMouseX.value = event.clientX;
    initialArticleListWidth.value = articleListWidth.value;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    window.addEventListener('mousemove', handleResizeArticleList);
    window.addEventListener('mouseup', stopResizeArticleList);
  }

  function handleResizeArticleList(): void {
    if (!isResizingArticleList.value) return;
    const currentMouseX = (window.event as MouseEvent).clientX;
    // Calculate the delta from the initial position and apply to initial width
    const deltaX = currentMouseX - initialMouseX.value;
    const newWidth = initialArticleListWidth.value + deltaX;
    // In compact mode, allow wider range (300-800), in normal mode (250-600)
    const minWidth = compactMode.value ? 300 : 280;
    const maxWidth = compactMode.value ? 800 : 600;
    if (newWidth >= minWidth && newWidth <= maxWidth) {
      articleListWidth.value = newWidth;
      // Mark that user has manually resized
      userManuallyResized.value = true;
    }
  }

  function stopResizeArticleList(): void {
    isResizingArticleList.value = false;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    window.removeEventListener('mousemove', handleResizeArticleList);
    window.removeEventListener('mouseup', stopResizeArticleList);
    // Persist the chosen article list width so it survives reloads/restarts
    if (userManuallyResized.value) {
      try {
        localStorage.setItem(LS_ARTICLE_WIDTH, String(Math.round(articleListWidth.value)));
      } catch {
        // ignore storage errors
      }
    }
  }

  // Cleanup
  onBeforeUnmount(() => {
    window.removeEventListener('mousemove', handleResizeSidebar);
    window.removeEventListener('mouseup', stopResizeSidebar);
    window.removeEventListener('mousemove', handleResizeArticleList);
    window.removeEventListener('mouseup', stopResizeArticleList);
  });

  return {
    sidebarWidth,
    articleListWidth,
    startResizeSidebar,
    startResizeArticleList,
    setCompactMode,
    setArticleListWidth,
  };
}
