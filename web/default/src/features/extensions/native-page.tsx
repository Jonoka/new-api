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
import {
  Component,
  type ComponentType,
  type ErrorInfo,
  type ReactNode,
  useEffect,
  useState,
} from 'react'
import type { TFunction } from 'i18next'
import { Loader2, Puzzle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  NATIVE_EXTENSION_PLATFORM,
  NATIVE_EXTENSION_SDK_VERSION,
  registerDefaultNativeExtensionSdk,
} from './native-sdk'
import type { ExtensionModule, ExtensionPage } from './types'

export type NativeExtensionComponentProps = {
  module: ExtensionModule
  page: ExtensionPage
  platform: typeof NATIVE_EXTENSION_PLATFORM
}

type NativeExtensionEntryModule = {
  default?: unknown
}

type NativeExtensionPageProps = {
  module: ExtensionModule
  page: ExtensionPage
}

type NativeExtensionBoundaryProps = {
  children: ReactNode
  resetKey: string
  onRetry: () => void
}

type NativeExtensionBoundaryState = {
  error: Error | null
}

class NativeExtensionBoundary extends Component<
  NativeExtensionBoundaryProps,
  NativeExtensionBoundaryState
> {
  state: NativeExtensionBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(_error: Error, _info: ErrorInfo) {
    // 渲染错误由宿主错误态展示，避免模块异常破坏整个后台页面。
  }

  componentDidUpdate(previousProps: NativeExtensionBoundaryProps) {
    if (previousProps.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null })
    }
  }

  render() {
    if (this.state.error) {
      return (
        <NativeExtensionError
          message={this.state.error.message}
          onRetry={this.props.onRetry}
        />
      )
    }
    return this.props.children
  }
}

function getNativeAssetUrl(
  moduleId: string,
  pageKey: string,
  moduleVersion: string,
  assetRevision: string | undefined,
  asset: 'entry' | `style-${number}`,
  loadAttempt = 0
) {
  const baseUrl = `/api/extensions/${encodeURIComponent(moduleId)}/native/${encodeURIComponent(pageKey)}/default/${asset}`
  const params = new URLSearchParams({ module_version: moduleVersion })
  if (assetRevision) params.set('asset_revision', assetRevision)
  if (loadAttempt > 0) params.set('load_attempt', String(loadAttempt))
  return `${baseUrl}?${params.toString()}`
}

function loadStyles(urls: string[], t: TFunction) {
  const links = urls.map((url) => {
    const link = document.createElement('link')
    link.rel = 'stylesheet'
    link.href = url
    link.dataset.newApiExtensionNativeStyle = 'true'
    return link
  })

  const ready = Promise.all(
    links.map(
      (link) =>
        new Promise<void>((resolve, reject) => {
          link.addEventListener('load', () => resolve(), { once: true })
          link.addEventListener(
            'error',
            () =>
              reject(
                new Error(
                  t('Failed to load extension style: {{url}}', {
                    url: link.href,
                  })
                )
              ),
            { once: true }
          )
          document.head.appendChild(link)
        })
    )
  )

  return {
    ready,
    remove: () => links.forEach((link) => link.remove()),
  }
}

async function importNativeEntry(
  url: string
): Promise<NativeExtensionEntryModule> {
  return import(
    /* webpackIgnore: true */ url
  ) as Promise<NativeExtensionEntryModule>
}

function isComponentType(
  value: unknown
): value is ComponentType<NativeExtensionComponentProps> {
  if (typeof value === 'function') return true
  if (typeof value !== 'object' || value === null) return false
  return '$$typeof' in value
}

function getConfigurationError(page: ExtensionPage, t: TFunction) {
  if (page.render?.type !== 'native') {
    return t('The extension page is not configured for native rendering.')
  }
  if (page.render.sdk !== NATIVE_EXTENSION_SDK_VERSION) {
    return t('Unsupported native extension SDK: {{sdk}}', {
      sdk: page.render.sdk || t('(missing)'),
    })
  }
  const target = page.render.targets?.default
  if (!target?.entry?.trim()) {
    return t('The extension does not provide a Default native entry.')
  }
  if (target.styles && !Array.isArray(target.styles)) {
    return t('The extension native styles configuration is invalid.')
  }
  return null
}

export function NativeExtensionPage({
  module,
  page,
}: NativeExtensionPageProps) {
  const { t } = useTranslation()
  const [loadAttempt, setLoadAttempt] = useState(0)
  const [Entry, setEntry] =
    useState<ComponentType<NativeExtensionComponentProps> | null>(null)
  const [error, setError] = useState<string | null>(null)
  const boundaryResetKey = [
    module.id,
    module.version,
    module.asset_revision ?? '',
    page.key,
    loadAttempt,
  ].join(':')

  useEffect(() => {
    let active = true
    const configurationError = getConfigurationError(page, t)
    if (configurationError) {
      setEntry(null)
      setError(configurationError)
      return
    }

    const target = page.render?.targets?.default
    if (!target) return

    registerDefaultNativeExtensionSdk()
    setEntry(null)
    setError(null)

    const styles = loadStyles(
      (target.styles ?? []).map((_, index) =>
        getNativeAssetUrl(
          module.id,
          page.key,
          module.version,
          module.asset_revision,
          `style-${index}`,
          loadAttempt
        )
      ),
      t
    )
    const entryUrl = getNativeAssetUrl(
      module.id,
      page.key,
      module.version,
      module.asset_revision,
      'entry',
      loadAttempt
    )

    void styles.ready
      .then(() => importNativeEntry(entryUrl))
      .then((entryModule) => {
        const EntryComponent = entryModule.default
        if (!isComponentType(EntryComponent)) {
          throw new Error(
            t(
              'The native extension entry must default-export a React component.'
            )
          )
        }
        if (active) setEntry(() => EntryComponent)
      })
      .catch((loadError: unknown) => {
        if (!active) return
        styles.remove()
        setError(
          loadError instanceof Error ? loadError.message : String(loadError)
        )
      })

    return () => {
      active = false
      styles.remove()
    }
  }, [loadAttempt, module.asset_revision, module.id, module.version, page, t])

  const retry = () => setLoadAttempt((current) => current + 1)

  if (error) return <NativeExtensionError message={error} onRetry={retry} />

  if (!Entry) {
    return (
      <div className='text-muted-foreground flex min-h-[60vh] items-center justify-center gap-2 p-6 text-sm'>
        <Loader2 className='size-4 animate-spin' />
        {t('Loading extension...')}
      </div>
    )
  }

  return (
    <NativeExtensionBoundary resetKey={boundaryResetKey} onRetry={retry}>
      <Entry module={module} page={page} platform={NATIVE_EXTENSION_PLATFORM} />
    </NativeExtensionBoundary>
  )
}

function NativeExtensionError({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex min-h-[60vh] items-center justify-center p-6'>
      <Empty className='max-w-xl border'>
        <EmptyMedia variant='icon'>
          <Puzzle className='size-4' />
        </EmptyMedia>
        <EmptyTitle>{t('Failed to load native extension')}</EmptyTitle>
        <EmptyDescription>{message}</EmptyDescription>
        <EmptyContent>
          <Button variant='outline' onClick={onRetry}>
            <RefreshCw className='size-4' />
            {t('Retry')}
          </Button>
        </EmptyContent>
      </Empty>
    </div>
  )
}
