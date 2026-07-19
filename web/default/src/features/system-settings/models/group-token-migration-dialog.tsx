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
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRightLeft, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { migrateTokenGroup, previewTokenGroupMigration } from '../api'

type MigrationGroup = {
  id?: number
  code: string
  name: string
  status: number
}

type GroupTokenMigrationDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  groups: MigrationGroup[]
}

function groupLabel(group: MigrationGroup) {
  return `ID ${group.id} · ${group.name || group.code}`
}

export function GroupTokenMigrationDialog({
  open,
  onOpenChange,
  groups,
}: GroupTokenMigrationDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [sourceGroupID, setSourceGroupID] = useState('')
  const [targetGroupID, setTargetGroupID] = useState('')
  const persistedGroups = useMemo(
    () => groups.filter((group) => Number(group.id) > 0),
    [groups]
  )
  const targetGroups = useMemo(
    () =>
      persistedGroups.filter(
        (group) => group.status === 1 && String(group.id) !== sourceGroupID
      ),
    [persistedGroups, sourceGroupID]
  )
  const request = useMemo(() => {
    const source = Number(sourceGroupID)
    const target = Number(targetGroupID)
    if (source <= 0 || target <= 0 || source === target) return null
    return { source_group_id: source, target_group_id: target }
  }, [sourceGroupID, targetGroupID])

  useEffect(() => {
    if (!open) {
      setSourceGroupID('')
      setTargetGroupID('')
    }
  }, [open])

  useEffect(() => {
    if (
      targetGroupID &&
      !targetGroups.some((group) => String(group.id) === targetGroupID)
    ) {
      setTargetGroupID('')
    }
  }, [targetGroupID, targetGroups])

  const previewQuery = useQuery({
    queryKey: [
      'system-settings',
      'group-token-migration-preview',
      request?.source_group_id,
      request?.target_group_id,
    ],
    queryFn: () => previewTokenGroupMigration(request!),
    enabled: open && request !== null,
    retry: false,
  })

  const migration = useMutation({
    mutationFn: migrateTokenGroup,
    onSuccess: async (summary) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['keys'] }),
        queryClient.invalidateQueries({
          queryKey: ['system-settings', 'group-token-migration-preview'],
        }),
      ])
      if (summary.warning) {
        toast.warning(summary.warning)
      } else {
        toast.success(
          t('{{count}} tokens migrated successfully.', {
            count: summary.migrated_tokens,
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
  const isBusy = previewQuery.isFetching || migration.isPending

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Migrate tokens between groups')}
      desc={t(
        'All tokens explicitly bound to the source group will be moved to the target group. Automatic and inherited groups are not affected.'
      )}
      confirmText={
        migration.isPending ? t('Migrating...') : t('Migrate tokens')
      }
      disabled={!request || previewQuery.isFetching || previewQuery.isError}
      isLoading={migration.isPending}
      handleConfirm={() => {
        if (request) migration.mutate(request)
      }}
      className='sm:max-w-lg'
    >
      <div className='grid gap-4 sm:grid-cols-2'>
        <div className='space-y-2'>
          <Label>{t('Source group')}</Label>
          <Select
            value={sourceGroupID || null}
            onValueChange={(value) => setSourceGroupID(value ?? '')}
            disabled={isBusy}
          >
            <SelectTrigger className='w-full'>
              <SelectValue placeholder={t('Select source group')} />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {persistedGroups.map((group) => (
                  <SelectItem key={group.id} value={String(group.id)}>
                    {groupLabel(group)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>

        <div className='space-y-2'>
          <Label>{t('Target group')}</Label>
          <Select
            value={targetGroupID || null}
            onValueChange={(value) => setTargetGroupID(value ?? '')}
            disabled={!sourceGroupID || isBusy}
          >
            <SelectTrigger className='w-full'>
              <SelectValue placeholder={t('Select target group')} />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {targetGroups.map((group) => (
                  <SelectItem key={group.id} value={String(group.id)}>
                    {groupLabel(group)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </div>

      {previewQuery.isFetching && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Loader2 className='h-4 w-4 animate-spin' />
          {t('Checking affected tokens...')}
        </div>
      )}

      {previewQuery.isError && (
        <p className='text-destructive text-sm'>
          {previewQuery.error.message || t('Failed to preview token migration')}
        </p>
      )}

      {preview && !previewQuery.isFetching && (
        <div className='bg-muted/40 space-y-1 rounded-md border px-3 py-2 text-sm'>
          <div className='flex items-center gap-2 font-medium'>
            <ArrowRightLeft className='h-4 w-4' />
            {t('{{count}} tokens will be migrated.', {
              count: preview.migrated_tokens,
            })}
          </div>
          <p className='text-muted-foreground'>
            {t(
              '{{users}} users affected; {{duplicates}} duplicate bindings will be removed.',
              {
                users: preview.affected_users,
                duplicates: preview.deduplicated_tokens,
              }
            )}
          </p>
          {preview.cleaned_deleted_tokens > 0 && (
            <p className='text-muted-foreground'>
              {t(
                '{{count}} deleted token records will have stale group references cleared.',
                {
                  count: preview.cleaned_deleted_tokens,
                }
              )}
            </p>
          )}
        </div>
      )}
    </ConfirmDialog>
  )
}
