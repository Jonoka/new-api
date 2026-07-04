export type ExtensionHostCompat = {
  min?: string
  max?: string
}

export type ExtensionRuntime = {
  type?: string
  health_path?: string
  static_dir?: string
}

export type ExtensionNavItem = {
  title: string
  page: string
  icon?: string
  section?: string
  order?: number
}

export type ExtensionPage = {
  key: string
  title?: string
  path: string
  embed?: boolean
}

export type ExtensionUI = {
  nav?: ExtensionNavItem[]
  pages?: ExtensionPage[]
}

export type ExtensionPermissions = {
  roles?: string[]
}

export type ExtensionModule = {
  id: string
  name: string
  version: string
  description?: string
  author?: string
  host?: ExtensionHostCompat
  runtime?: ExtensionRuntime
  ui?: ExtensionUI
  permissions?: ExtensionPermissions
  enabled: boolean
  error?: string
}

export type ExtensionListResponse = {
  success: boolean
  message?: string
  data?: {
    root: string
    modules: ExtensionModule[]
  }
}
