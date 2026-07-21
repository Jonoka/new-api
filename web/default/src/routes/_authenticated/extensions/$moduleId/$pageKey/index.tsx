import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { ExternalLink, Puzzle, RefreshCw, ServerOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { getExtensions, getExtensionPageUrl } from '@/features/extensions/api'

export const Route = createFileRoute(
  '/_authenticated/extensions/$moduleId/$pageKey/'
)({
  beforeLoad: ({ params }) => {
    if (!params.moduleId || !params.pageKey) {
      throw redirect({ to: '/extensions' })
    }
  },
  component: ExtensionModulePage,
})

function ExtensionModulePage() {
  const { t } = useTranslation()
  const { moduleId, pageKey } = Route.useParams()
  const { data, isError, isLoading, refetch } = useQuery({
    queryKey: ['extensions'],
    queryFn: () => getExtensions(),
  })

  const module = data?.modules.find((item) => item.id === moduleId)
  const page = module?.ui?.pages?.find((item) => item.key === pageKey)
  const src = page
    ? getExtensionPageUrl(
        moduleId,
        page.path,
        module?.runtime?.type === 'static' ? module.version : ''
      )
    : ''
  const healthSrc = useMemo(() => {
    const healthPath = module?.runtime?.health_path || page?.path || ''
    if (!healthPath) return ''
    return getExtensionPageUrl(moduleId, healthPath)
  }, [module?.runtime?.health_path, moduleId, page?.path])
  const [probe, setProbe] = useState<{
    status: 'idle' | 'checking' | 'ready' | 'offline'
    message?: string
  }>({ status: 'idle' })

  useEffect(() => {
    if (
      !module ||
      !page ||
      page.embed === false ||
      module.runtime?.type === 'static' ||
      !healthSrc
    ) {
      setProbe({ status: 'idle' })
      return
    }

    const controller = new AbortController()
    const timer = window.setTimeout(() => controller.abort(), 7000)

    setProbe({ status: 'checking' })
    fetch(healthSrc, {
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (response.ok) {
          setProbe({ status: 'ready' })
          return
        }
        const body = await response.text().catch(() => '')
        setProbe({
          status: 'offline',
          message:
            body.trim() ||
            t('Extension service returned HTTP {{status}}.', {
              status: response.status,
            }),
        })
      })
      .catch((error) => {
        setProbe({
          status: 'offline',
          message:
            error instanceof DOMException && error.name === 'AbortError'
              ? t('Extension service health check timed out.')
              : String(error?.message || error),
        })
      })
      .finally(() => window.clearTimeout(timer))

    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [healthSrc, module, page, t])

  if (isLoading) {
    return (
      <div className='text-muted-foreground p-6 text-sm'>
        {t('Loading extension...')}
      </div>
    )
  }

  if (isError) {
    return (
      <div className='flex min-h-[60vh] items-center justify-center p-6'>
        <Empty className='max-w-md border'>
          <EmptyMedia variant='icon'>
            <Puzzle className='size-4' />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load extensions')}</EmptyTitle>
          <EmptyDescription>
            {t('Refresh extensions or check the current login session.')}
          </EmptyDescription>
          <EmptyContent>
            <Button variant='outline' onClick={() => refetch()}>
              <RefreshCw className='size-4' />
              {t('Retry')}
            </Button>
          </EmptyContent>
        </Empty>
      </div>
    )
  }

  if (!module || !page) {
    return (
      <div className='flex min-h-[60vh] items-center justify-center p-6'>
        <Empty className='max-w-md border'>
          <EmptyMedia variant='icon'>
            <Puzzle className='size-4' />
          </EmptyMedia>
          <EmptyTitle>{t('Extension page not found')}</EmptyTitle>
          <EmptyDescription>
            {t('Refresh extensions or check the module manifest.')}
          </EmptyDescription>
          <Button variant='outline' render={<a href='/extensions' />}>
            {t('Back to extensions')}
          </Button>
        </Empty>
      </div>
    )
  }

  if (page.embed === false) {
    return (
      <div className='flex min-h-[60vh] items-center justify-center p-6'>
        <Empty className='max-w-md border'>
          <EmptyMedia variant='icon'>
            <ExternalLink className='size-4' />
          </EmptyMedia>
          <EmptyTitle>{page.title || module.name}</EmptyTitle>
          <EmptyDescription>
            {t('This extension page opens outside the dashboard frame.')}
          </EmptyDescription>
          <Button render={<a href={src} target='_blank' rel='noreferrer' />}>
            <ExternalLink className='size-4' />
            {t('Open extension')}
          </Button>
        </Empty>
      </div>
    )
  }

  return (
    <div className='flex h-[calc(100vh-4rem)] min-h-[640px] flex-col'>
      <div className='border-border flex shrink-0 items-center justify-between gap-3 border-b px-4 py-3'>
        <div className='min-w-0'>
          <h1 className='truncate text-base font-medium'>
            {page.title || module.name}
          </h1>
          <p className='text-muted-foreground truncate text-xs'>
            {module.name} · {module.version}
          </p>
        </div>
        <Button
          size='sm'
          variant='outline'
          render={<a href={src} target='_blank' rel='noreferrer' />}
        >
          <ExternalLink className='size-3.5' />
          {t('Open')}
        </Button>
      </div>
      {probe.status === 'offline' ? (
        <div className='flex min-h-0 flex-1 items-center justify-center p-6'>
          <Empty className='max-w-xl border'>
            <EmptyMedia variant='icon'>
              <ServerOff className='size-4' />
            </EmptyMedia>
            <EmptyTitle>{t('Extension service is not reachable')}</EmptyTitle>
            <EmptyDescription>
              {t('The module is installed and enabled, but its HTTP service is not responding through the host proxy. Start the module service or update runtime.base_url in manifest.json.')}
            </EmptyDescription>
            <EmptyContent className='max-w-none'>
              <div className='bg-muted text-muted-foreground w-full rounded-lg px-3 py-2 text-left font-mono text-xs break-all'>
                {healthSrc}
              </div>
              {probe.message ? (
                <div className='text-muted-foreground w-full text-left text-xs break-all'>
                  {probe.message}
                </div>
              ) : null}
              <div className='flex flex-wrap justify-center gap-2'>
                <Button
                  variant='outline'
                  onClick={() => {
                    setProbe({ status: 'idle' })
                    void refetch()
                  }}
                >
                  <RefreshCw className='size-4' />
                  {t('Retry')}
                </Button>
                <Button
                  variant='outline'
                  render={<a href={src} target='_blank' rel='noreferrer' />}
                >
                  <ExternalLink className='size-4' />
                  {t('Open proxy URL')}
                </Button>
              </div>
            </EmptyContent>
          </Empty>
        </div>
      ) : (
        <iframe
          src={src}
          title={page.title || module.name}
          className='bg-background h-full min-h-0 flex-1 border-0'
        />
      )}
    </div>
  )
}
