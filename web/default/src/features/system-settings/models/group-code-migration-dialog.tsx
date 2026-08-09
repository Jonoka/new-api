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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { migrateGroupCodes, previewGroupCodeMigration } from '../api'

type GroupCodeMigrationDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const affectedQueryKeys = [
  ['system-settings', 'group-details'],
  ['system-options'],
  ['pricing'],
  ['channels'],
  ['groups'],
  ['keys'],
  ['user-groups'],
  ['user-self-groups'],
  ['playground-groups'],
  ['canvas-groups'],
] as const

export function GroupCodeMigrationDialog({
  open,
  onOpenChange,
}: GroupCodeMigrationDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const previewQuery = useQuery({
    queryKey: ['system-settings', 'group-code-migration-preview'],
    queryFn: previewGroupCodeMigration,
    enabled: open,
    retry: false,
  })
  const migration = useMutation({
    mutationFn: migrateGroupCodes,
    onSuccess: async (summary) => {
      await Promise.all(
        affectedQueryKeys.map((queryKey) =>
          queryClient.invalidateQueries({ queryKey })
        )
      )
      await queryClient.invalidateQueries({
        queryKey: ['system-settings', 'group-code-migration-preview'],
      })
      if (summary.warning) {
        toast.warning(summary.warning)
      } else {
        toast.success(
          t('{{count}} legacy group codes migrated successfully.', {
            count: summary.groups.length,
          })
        )
      }
      onOpenChange(false)
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : t('Migration failed')
      )
    },
  })

  const preview = previewQuery.data
  const groups = preview?.groups ?? []
  const blockers = preview?.blockers ?? []
  const canExecute =
    preview?.can_execute === true && groups.length > 0 && blockers.length === 0

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Migrate legacy group codes')}
      desc={t(
        'Preview and replace legacy group codes with stable numeric IDs.'
      )}
      confirmText={
        migration.isPending
          ? t('Migrating group codes...')
          : t('Migrate group codes')
      }
      disabled={!canExecute || previewQuery.isFetching || previewQuery.isError}
      isLoading={migration.isPending}
      handleConfirm={() => migration.mutate()}
      className='sm:max-w-2xl'
    >
      <div className='border-warning/40 bg-warning/10 flex gap-2 rounded-md border px-3 py-2 text-sm'>
        <AlertTriangle className='text-warning mt-0.5 h-4 w-4 shrink-0' />
        <p>
          {t(
            'All application instances must be upgraded to the same version and configuration writes should be paused before migration.'
          )}
        </p>
      </div>

      {previewQuery.isFetching && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Checking legacy group codes...')}
        </div>
      )}

      {previewQuery.isError && (
        <p className='text-destructive text-sm'>
          {previewQuery.error.message ||
            t('Failed to preview group code migration')}
        </p>
      )}

      {preview && !previewQuery.isFetching && groups.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('No legacy group codes need migration.')}
        </p>
      )}

      {groups.length > 0 && (
        <div className='space-y-3'>
          <div className='flex items-center gap-2 text-sm font-medium'>
            <RefreshCw className='h-4 w-4' />
            {t('{{count}} group codes will be migrated.', {
              count: groups.length,
            })}
          </div>
          <div className='max-h-48 overflow-y-auto rounded-md border'>
            {groups.map((group) => (
              <div
                key={group.group_id}
                className='flex items-center justify-between gap-3 border-b px-3 py-2 text-sm last:border-b-0'
              >
                <span className='min-w-0 truncate'>
                  ID {group.group_id} · {group.name}
                </span>
                <code className='shrink-0'>
                  {group.old_code} → {group.target_code}
                </code>
              </div>
            ))}
          </div>
          <p className='text-muted-foreground text-sm'>
            {t(
              '{{channels}} channels, {{tokens}} tokens, {{users}} users, {{abilities}} ability records, {{plans}} plans, and {{subscriptions}} subscriptions are affected.',
              {
                channels: preview?.affected_channels ?? 0,
                tokens: preview?.affected_tokens ?? 0,
                users: preview?.affected_users ?? 0,
                abilities: preview?.affected_abilities ?? 0,
                plans: preview?.affected_subscription_plans ?? 0,
                subscriptions: preview?.affected_subscriptions ?? 0,
              }
            )}
          </p>
        </div>
      )}

      {blockers.length > 0 && (
        <div className='border-destructive/40 bg-destructive/10 rounded-md border px-3 py-2 text-sm'>
          <p className='text-destructive font-medium'>
            {t('Migration is blocked by these conflicts:')}
          </p>
          <ul className='text-destructive mt-2 list-disc space-y-1 pl-5'>
            {blockers.map((blocker) => (
              <li key={blocker}>{blocker}</li>
            ))}
          </ul>
        </div>
      )}
    </ConfirmDialog>
  )
}
