import { isCustomNavIconName } from './customNav';

export const CANVAS_APP_ORIGIN = 'https://canvas.jo2api.com';
export const DEFAULT_CANVAS_ICON = 'Brush';

function parseSidebarModulesRecord(raw) {
  if (!raw || String(raw).trim() === '') return null;
  if (raw && typeof raw === 'object') return raw;

  try {
    return JSON.parse(String(raw));
  } catch {
    return null;
  }
}

export function normalizeCanvasOrigin(value, fallback = CANVAS_APP_ORIGIN) {
  const raw = typeof value === 'string' ? value.trim() : '';
  if (!raw) return fallback;

  const candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(raw)
    ? raw
    : `https://${raw}`;

  try {
    const url = new URL(candidate);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return fallback;
    return url.origin;
  } catch {
    return fallback;
  }
}

export function getCanvasSettingsFromSidebarModules(raw) {
  const parsed = parseSidebarModulesRecord(raw);
  const chat =
    parsed?.chat &&
    typeof parsed.chat === 'object' &&
    !Array.isArray(parsed.chat)
      ? parsed.chat
      : {};

  return {
    canvasOrigin: normalizeCanvasOrigin(chat.canvasOrigin),
    canvasIcon: isCustomNavIconName(chat.canvasIcon)
      ? chat.canvasIcon
      : DEFAULT_CANVAS_ICON,
  };
}

export function buildCanvasLaunchUrl({
  canvasOrigin = CANVAS_APP_ORIGIN,
  newApiOrigin,
  group,
  textGroup,
  imageGroup,
  audioGroup,
  videoGroup,
}) {
  const canvasUrl = new URL('/', canvasOrigin.trim());
  const normalizedOrigin = newApiOrigin.trim().replace(/\/+$/, '');

  canvasUrl.searchParams.set('mode', 'newapi');
  canvasUrl.searchParams.set('baseUrl', `${normalizedOrigin}/canvas`);
  canvasUrl.searchParams.set('group', group);
  if (textGroup) canvasUrl.searchParams.set('textGroup', textGroup);
  if (imageGroup) canvasUrl.searchParams.set('imageGroup', imageGroup);
  if (audioGroup) canvasUrl.searchParams.set('audioGroup', audioGroup);
  if (videoGroup) canvasUrl.searchParams.set('videoGroup', videoGroup);

  return canvasUrl.toString();
}
