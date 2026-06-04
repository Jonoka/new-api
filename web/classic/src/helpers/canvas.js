export const CANVAS_APP_ORIGIN = 'https://canvas.maolaoapi.com';

export function buildCanvasLaunchUrl({
  canvasOrigin = CANVAS_APP_ORIGIN,
  newApiOrigin,
  group,
}) {
  const canvasUrl = new URL('/', canvasOrigin.trim());
  const normalizedOrigin = newApiOrigin.trim().replace(/\/+$/, '');

  canvasUrl.searchParams.set('mode', 'newapi');
  canvasUrl.searchParams.set('baseUrl', `${normalizedOrigin}/canvas`);
  canvasUrl.searchParams.set('group', group);

  return canvasUrl.toString();
}
