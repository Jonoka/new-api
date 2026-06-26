export const CANVAS_APP_ORIGIN = 'https://canvas.jo2api.com';

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
