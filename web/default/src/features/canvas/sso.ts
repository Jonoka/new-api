const CANVAS_RETURN_KEY = 'canvas2-login-return';
const RETURN_TTL_MS = 5 * 60 * 1000;

export function safeCanvasDestination(value: unknown): string {
  if (typeof value !== 'string' || value.length > 1024 || !value.startsWith('/') || value.startsWith('//')) return '/';
  try {
    const decoded = decodeURIComponent(value);
    if (decoded.startsWith('//') || /[\\\\\u0000-\u0020\u007f]/.test(decoded)) return '/';
    const url = new URL(value, 'https://canvas.invalid');
    if (url.origin !== 'https://canvas.invalid') return '/';
    return url.pathname + url.search + url.hash;
  } catch {
    return '/';
  }
}

export function buildCanvasSSOLaunchUrl(origin: unknown, group: string, next: unknown = '/', returned = false): string {
  if (typeof origin !== 'string' || !group) return '';
  try {
    const base = new URL(origin);
    if (base.protocol !== 'https:' || base.origin !== origin) return '';
    const url = new URL(safeCanvasDestination(next), origin);
    url.searchParams.delete('apiKey');
    url.searchParams.delete('baseUrl');
    url.searchParams.set('newapi_launch', '1');
    url.searchParams.set('group', group);
    url.searchParams.delete('newapi_returned');
    if (returned) url.searchParams.set('newapi_returned', '1');
    return url.toString();
  } catch {
    return '';
  }
}

function canvasReturnPath(value: unknown): string {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return '';
  try {
    const url = new URL(value, window.location.origin);
    if (url.origin !== window.location.origin || !['/canvas', '/canvas/', '/console/canvas'].includes(url.pathname)) return '';
    if (url.searchParams.get('canvas_resume') !== '1') return '';
    const query = new URLSearchParams({ canvas_resume: '1' });
    query.set('canvas_next', safeCanvasDestination(url.searchParams.get('canvas_next')));
    const group = url.searchParams.get('group');
    if (group && group.length <= 64 && !/[\u0000-\u001f\u007f]/.test(group)) query.set('group', group);
    return url.pathname + '?' + query.toString();
  } catch {
    return '';
  }
}

export function rememberCanvasReturn(value: string): void {
  const path = canvasReturnPath(value);
  if (!path) return;
  try {
    sessionStorage.setItem(CANVAS_RETURN_KEY, JSON.stringify({ path, expires: Date.now() + RETURN_TTL_MS }));
  } catch { /* The normal login redirect still works when storage is unavailable. */ }
}

export function consumeCanvasReturn(): string {
  try {
    const raw = sessionStorage.getItem(CANVAS_RETURN_KEY);
    sessionStorage.removeItem(CANVAS_RETURN_KEY);
    if (!raw) return '';
    const value = JSON.parse(raw);
    if (typeof value.expires !== 'number' || value.expires <= Date.now()) return '';
    return canvasReturnPath(value.path);
  } catch {
    return '';
  }
}
