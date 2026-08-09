import { normalizeCanvasOrigin } from '@/lib/canvas-settings'

type CanvasLaunchUrlOptions = {
  canvasOrigin: string
  newApiOrigin: string
  group: string
}

export function buildCanvasLaunchUrl(options: CanvasLaunchUrlOptions): string {
  const canvasUrl = new URL('/', normalizeCanvasOrigin(options.canvasOrigin))
  const newApiOrigin = options.newApiOrigin.trim().replace(/\/+$/, '')

  canvasUrl.searchParams.set('mode', 'newapi')
  canvasUrl.searchParams.set('baseUrl', `${newApiOrigin}/canvas`)
  canvasUrl.searchParams.set('group', options.group)

  return canvasUrl.toString()
}
