import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ExternalLink, Puzzle, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { getCustomNavIcon } from '@/lib/custom-nav'
import { ROLE } from '@/lib/roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Empty, EmptyDescription, EmptyTitle } from '@/components/ui/empty'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getExtensionAdminList,
  refreshExtensions,
  setExtensionEnabled,
} from './api'
import type { ExtensionModule } from './types'

const EXTENSIONS_QUERY_KEY = ['extensions']
const EXTENSIONS_ADMIN_QUERY_KEY = ['extensions', 'admin']

export function Extensions() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = Boolean(user && user.role >= ROLE.SUPER_ADMIN)

  const { data, isLoading } = useQuery({
    queryKey: EXTENSIONS_ADMIN_QUERY_KEY,
    queryFn: getExtensionAdminList,
    enabled: isRoot,
  })

  const refreshMutation = useMutation({
    mutationFn: refreshExtensions,
    onSuccess: async (res) => {
      if (!res.success) {
        toast.error(res.message || t('Failed to refresh extensions'))
        return
      }
      await queryClient.invalidateQueries({ queryKey: EXTENSIONS_QUERY_KEY })
      toast.success(t('Extensions refreshed'))
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      setExtensionEnabled(id, enabled),
    onSuccess: async (res) => {
      if (!res.success) {
        toast.error(res.message || t('Failed to update extension'))
        return
      }
      await queryClient.invalidateQueries({ queryKey: EXTENSIONS_QUERY_KEY })
      toast.success(t('Extension updated'))
    },
  })

  const modules = data?.modules ?? []
  const root = data?.root ?? 'data/modules'

  if (!isRoot) {
    return (
      <div className='mx-auto flex w-full max-w-5xl flex-col gap-4 p-4 md:p-6'>
        <Alert variant='destructive'>
          <AlertTitle>{t('Permission denied')}</AlertTitle>
          <AlertDescription>
            {t('Only root users can manage extensions.')}
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <div className='mx-auto flex w-full max-w-6xl flex-col gap-4 p-4 md:p-6'>
      <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
        <div className='min-w-0'>
          <h1 className='text-2xl font-semibold tracking-normal'>
            {t('Extensions')}
          </h1>
          <p className='text-muted-foreground mt-1 text-sm'>
            {t('Drop modules into the modules directory, then refresh here.')}
          </p>
        </div>
        <Button
          variant='outline'
          onClick={() => refreshMutation.mutate()}
          disabled={refreshMutation.isPending}
        >
          <RefreshCw className='size-4' />
          {t('Refresh')}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('Module directory')}</CardTitle>
          <CardDescription className='break-all'>{root}</CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className='text-muted-foreground py-8 text-sm'>
              {t('Loading extensions...')}
            </div>
          ) : modules.length === 0 ? (
            <Empty className='border'>
              <Puzzle className='text-muted-foreground size-8' />
              <EmptyTitle>{t('No extensions found')}</EmptyTitle>
              <EmptyDescription>
                {t('Create a module folder with a manifest.json file.')}
              </EmptyDescription>
            </Empty>
          ) : (
            <ExtensionsTable
              modules={modules}
              pendingId={
                toggleMutation.variables
                  ? `${toggleMutation.variables.id}:${toggleMutation.variables.enabled}`
                  : ''
              }
              onToggle={(id, enabled) => toggleMutation.mutate({ id, enabled })}
            />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function ExtensionsTable({
  modules,
  pendingId,
  onToggle,
}: {
  modules: ExtensionModule[]
  pendingId: string
  onToggle: (id: string, enabled: boolean) => void
}) {
  const { t } = useTranslation()

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('Module')}</TableHead>
          <TableHead>{t('Permissions')}</TableHead>
          <TableHead>{t('Pages')}</TableHead>
          <TableHead className='w-24'>{t('Enabled')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {modules.map((module) => {
          const roles = module.permissions?.roles ?? []
          const firstPage = module.ui?.pages?.[0]
          const disabled = Boolean(module.error)
          return (
            <TableRow key={module.id}>
              <TableCell className='min-w-[280px] whitespace-normal'>
                <div className='flex items-start gap-3'>
                  <ModuleIcon module={module} />
                  <div className='min-w-0 space-y-1'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <span className='font-medium'>{module.name}</span>
                      <Badge variant='outline'>{module.version}</Badge>
                      {module.error ? (
                        <Badge variant='destructive'>{t('Invalid')}</Badge>
                      ) : null}
                    </div>
                    <div className='text-muted-foreground text-xs break-all'>
                      {module.id}
                    </div>
                    {module.description ? (
                      <div className='text-muted-foreground text-sm'>
                        {module.description}
                      </div>
                    ) : null}
                    {module.error ? (
                      <div className='text-destructive text-sm break-all'>
                        {module.error}
                      </div>
                    ) : null}
                  </div>
                </div>
              </TableCell>
              <TableCell>
                <div className='flex flex-wrap gap-1'>
                  {(roles.length ? roles : ['user']).map((role) => (
                    <Badge key={role} variant='secondary'>
                      {role}
                    </Badge>
                  ))}
                </div>
              </TableCell>
              <TableCell>
                {firstPage ? (
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={!module.enabled}
                    render={
                      module.enabled ? (
                        <Link
                          to='/extensions/$moduleId/$pageKey'
                          params={{
                            moduleId: module.id,
                            pageKey: firstPage.key,
                          }}
                        />
                      ) : (
                        <button />
                      )
                    }
                  >
                    <ExternalLink className='size-3.5' />
                    {firstPage.title || t('Open')}
                  </Button>
                ) : (
                  <span className='text-muted-foreground text-sm'>-</span>
                )}
              </TableCell>
              <TableCell>
                <Switch
                  checked={module.enabled}
                  disabled={disabled || pendingId.startsWith(`${module.id}:`)}
                  onCheckedChange={(checked) => onToggle(module.id, checked)}
                  aria-label={t('Toggle extension')}
                />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}

function ModuleIcon({ module }: { module: ExtensionModule }) {
  const iconName = useMemo(() => module.ui?.nav?.[0]?.icon, [module])
  const Icon = getCustomNavIcon(iconName) ?? Puzzle
  return (
    <div className='bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg'>
      <Icon className='size-4' />
    </div>
  )
}

export function getExtensionQueryKey() {
  return EXTENSIONS_QUERY_KEY
}
