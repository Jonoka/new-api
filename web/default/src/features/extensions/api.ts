import { api } from '@/lib/api'
import type { ExtensionListResponse, ExtensionModule } from './types'

export async function getExtensions(options: { all?: boolean } = {}) {
  const res = await api.get<ExtensionListResponse>('/api/extensions/', {
    params: options.all ? { all: 'true' } : undefined,
    skipBusinessError: true,
  })
  return res.data.data ?? { root: '', modules: [] }
}

export async function getExtensionAdminList() {
  const res = await api.get<ExtensionListResponse>('/api/extension-admin/', {
    params: { all: 'true' },
    skipBusinessError: true,
  })
  return res.data.data ?? { root: '', modules: [] }
}

export async function refreshExtensions() {
  const res = await api.post<ExtensionListResponse>(
    '/api/extension-admin/refresh',
    undefined,
    { skipBusinessError: true }
  )
  return res.data
}

export async function uploadExtension(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post<ExtensionListResponse>(
    '/api/extension-admin/upload',
    formData,
    { skipBusinessError: true }
  )
  return res.data
}

export async function setExtensionEnabled(id: string, enabled: boolean) {
  const res = await api.put<{
    success: boolean
    message?: string
    data?: ExtensionModule
  }>(
    `/api/extension-admin/${encodeURIComponent(id)}/enabled`,
    { enabled },
    { skipBusinessError: true }
  )
  return res.data
}

export async function uninstallExtension(id: string) {
  const res = await api.delete<ExtensionListResponse>(
    `/api/extension-admin/${encodeURIComponent(id)}`,
    { skipBusinessError: true }
  )
  return res.data
}

export function getExtensionPageUrl(
  moduleId: string,
  pagePath: string,
  moduleVersion = ''
) {
  const normalizedPath = pagePath.startsWith('/') ? pagePath : `/${pagePath}`
  const baseUrl = `/api/extensions/${encodeURIComponent(moduleId)}/proxy${normalizedPath}`
  if (!moduleVersion || /[?&]module_version=/.test(baseUrl)) return baseUrl
  const separator = baseUrl.includes('?') ? '&' : '?'
  return `${baseUrl}${separator}module_version=${encodeURIComponent(moduleVersion)}`
}
