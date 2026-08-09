/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
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

export type ExtensionNativeRenderTarget = {
  entry: string
  styles?: string[]
}

export type ExtensionNativePageRender = {
  type: 'native'
  sdk: string
  targets?: {
    default?: ExtensionNativeRenderTarget
    classic?: ExtensionNativeRenderTarget
  }
}

export type ExtensionPage = {
  key: string
  title?: string
  path?: string
  embed?: boolean
  render?: ExtensionNativePageRender
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
  asset_revision?: string
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
