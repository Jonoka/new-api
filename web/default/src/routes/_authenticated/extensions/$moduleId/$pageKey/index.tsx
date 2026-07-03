import { useQuery } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { ExternalLink, Puzzle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyTitle,
  EmptyMedia,
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
  const { data, isLoading } = useQuery({
    queryKey: ['extensions'],
    queryFn: () => getExtensions(),
  })

  const module = data?.modules.find((item) => item.id === moduleId)
  const page = module?.ui?.pages?.find((item) => item.key === pageKey)

  if (isLoading) {
    return (
      <div className='text-muted-foreground p-6 text-sm'>
        {t('Loading extension...')}
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

  const src = getExtensionPageUrl(module.id, page.path)

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
      <iframe
        src={src}
        title={page.title || module.name}
        className='bg-background min-h-0 flex-1 border-0'
      />
    </div>
  )
}
