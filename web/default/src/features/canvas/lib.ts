export const CANVAS_APP_ORIGIN = 'https://canvas.maolaoapi.com'

type CanvasLaunchUrlOptions = {
  canvasOrigin: string
  newApiOrigin: string
  group: string
}

export function buildCanvasLaunchUrl(options: CanvasLaunchUrlOptions): string {
  const canvasUrl = new URL('/', options.canvasOrigin.trim())
  const newApiOrigin = options.newApiOrigin.trim().replace(/\/+$/, '')

  canvasUrl.searchParams.set('mode', 'newapi')
  canvasUrl.searchParams.set('baseUrl', `${newApiOrigin}/canvas`)
  canvasUrl.searchParams.set('group', options.group)

  return canvasUrl.toString()
}
