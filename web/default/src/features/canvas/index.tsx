import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowUpRight, Brush } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getCanvasSettingsFromSidebarModules } from '@/lib/canvas-settings'
import { getCustomNavIcon } from '@/lib/custom-nav'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { GroupSelector } from '@/components/model-group-selector'
import { getUserGroups } from '@/features/playground/api'
import { buildCanvasLaunchUrl } from './lib'

export function CanvasLauncher() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [selectedGroup, setSelectedGroup] = useState('')
  const canvasSettings = useMemo(
    () =>
      getCanvasSettingsFromSidebarModules(
        (status as Record<string, unknown> | null)?.SidebarModulesAdmin
      ),
    [status]
  )
  const CanvasIcon = getCustomNavIcon(canvasSettings.canvasIcon) ?? Brush

  const { data: groups = [], isLoading } = useQuery({
    queryKey: ['canvas-groups'],
    queryFn: async () => {
      try {
        return await getUserGroups()
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to load playground groups')
        )
        return []
      }
    },
  })

  useEffect(() => {
    if (selectedGroup || groups.length === 0) return
    const fallback =
      groups.find((group) => group.value === 'default')?.value ??
      groups[0].value
    setSelectedGroup(fallback)
  }, [groups, selectedGroup])

  const launchUrl = useMemo(() => {
    if (!selectedGroup || typeof window === 'undefined') return ''
    return buildCanvasLaunchUrl({
      canvasOrigin: canvasSettings.canvasOrigin,
      newApiOrigin: window.location.origin,
      group: selectedGroup,
    })
  }, [canvasSettings.canvasOrigin, selectedGroup])

  const handleOpenCanvas = () => {
    if (!launchUrl) return
    window.open(launchUrl, '_blank', 'noopener')
  }

  return (
    <div className='flex min-h-full items-center justify-center p-4 sm:p-8'>
      <Card className='w-full max-w-xl'>
        <CardHeader>
          <div className='bg-muted mb-2 flex h-10 w-10 items-center justify-center rounded-lg'>
            <CanvasIcon className='h-5 w-5' aria-hidden='true' />
          </div>
          <CardTitle>{t('Infinite Canvas')}</CardTitle>
          <CardDescription>
            {t(
              'Choose a group and open Infinite Canvas. The canvas uses your current New API login session to call models.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='space-y-2'>
            <p className='text-sm font-medium'>{t('Model Group')}</p>
            <GroupSelector
              selectedGroup={selectedGroup}
              groups={groups}
              onGroupChange={setSelectedGroup}
              disabled={isLoading || groups.length === 0}
              className='h-9 w-full justify-between px-3 sm:w-full'
            />
          </div>

          <Button
            className='w-full'
            onClick={handleOpenCanvas}
            disabled={!launchUrl || isLoading}
          >
            <ArrowUpRight className='h-4 w-4' aria-hidden='true' />
            {t('Open in new tab')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
