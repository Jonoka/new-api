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
import { ChevronDown, Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { safeJsonParseWithValidation } from '../utils/json-parser'
import { isObjectRecord } from '../utils/json-validators'

type RateLimitTuple = [number, number]

type UserGroupRateLimitConfig = Record<
  string,
  {
    global?: RateLimitTuple
    groups?: Record<string, RateLimitTuple>
  }
>

type RequestGroupLimit = {
  groupName: string
  maxRequests: number
  maxSuccess: number
}

type UserGroupLimit = {
  userGroup: string
  global?: {
    maxRequests: number
    maxSuccess: number
  }
  groups: RequestGroupLimit[]
}

type UserGroupRateLimitVisualEditorProps = {
  value: string
  onChange: (value: string) => void
}

function isRateLimitTuple(value: unknown): value is RateLimitTuple {
  return (
    Array.isArray(value) &&
    value.length === 2 &&
    Number.isInteger(value[0]) &&
    Number.isInteger(value[1]) &&
    value[0] >= 0 &&
    value[1] >= 1 &&
    value[0] <= 2147483647 &&
    value[1] <= 2147483647
  )
}

function isUserGroupRateLimitConfig(
  value: unknown
): value is UserGroupRateLimitConfig {
  if (!isObjectRecord(value)) return false

  for (const [userGroup, limits] of Object.entries(value)) {
    if (!userGroup || !isObjectRecord(limits)) return false
    if ('global' in limits && !isRateLimitTuple(limits.global)) return false
    if ('groups' in limits) {
      if (!isObjectRecord(limits.groups)) return false
      for (const [groupName, groupLimits] of Object.entries(limits.groups)) {
        if (!groupName || !isRateLimitTuple(groupLimits)) return false
      }
    }
  }

  return true
}

function parseUserGroupRateLimits(value: string): UserGroupLimit[] {
  if (!value || value.trim() === '') return []

  const parsed = safeJsonParseWithValidation<UserGroupRateLimitConfig>(value, {
    fallback: {},
    validator: isUserGroupRateLimitConfig,
    validatorMessage: 'User group rate limits must be a valid JSON object',
    context: 'user group rate limits',
  })

  return Object.entries(parsed).map(([userGroup, limits]) => ({
    userGroup,
    global: limits.global
      ? {
          maxRequests: limits.global[0],
          maxSuccess: limits.global[1],
        }
      : undefined,
    groups: Object.entries(limits.groups ?? {}).map(([groupName, limit]) => ({
      groupName,
      maxRequests: limit[0],
      maxSuccess: limit[1],
    })),
  }))
}

function serializeUserGroupRateLimits(limits: UserGroupLimit[]): string {
  const config: UserGroupRateLimitConfig = {}

  for (const userLimit of limits) {
    const userGroup = userLimit.userGroup.trim()
    if (!userGroup) continue

    const item: UserGroupRateLimitConfig[string] = {}
    if (userLimit.global) {
      item.global = [
        userLimit.global.maxRequests,
        userLimit.global.maxSuccess,
      ]
    }

    const groups: Record<string, RateLimitTuple> = {}
    for (const groupLimit of userLimit.groups) {
      const groupName = groupLimit.groupName.trim()
      if (!groupName) continue
      groups[groupName] = [groupLimit.maxRequests, groupLimit.maxSuccess]
    }
    if (Object.keys(groups).length > 0) {
      item.groups = groups
    }

    config[userGroup] = item
  }

  return JSON.stringify(config, null, 2)
}

function formatLimit(
  maxRequests: number,
  maxSuccess: number,
  t: (key: string) => string
) {
  const totalText =
    maxRequests === 0 ? t('Unlimited') : maxRequests.toLocaleString()
  return `${totalText} / ${maxSuccess.toLocaleString()}`
}

export function UserGroupRateLimitVisualEditor({
  value,
  onChange,
}: UserGroupRateLimitVisualEditorProps) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [userGroupDialogOpen, setUserGroupDialogOpen] = useState(false)
  const [userGroupInput, setUserGroupInput] = useState('')
  const [limitDialogOpen, setLimitDialogOpen] = useState(false)
  const [limitDialogUserGroup, setLimitDialogUserGroup] = useState('')
  const [limitDialogType, setLimitDialogType] = useState<'global' | 'group'>(
    'global'
  )
  const [limitEditData, setLimitEditData] = useState<
    RequestGroupLimit | UserGroupLimit['global'] | null
  >(null)

  const userGroupLimits = useMemo(
    () => parseUserGroupRateLimits(value),
    [value]
  )

  const filteredUserGroupLimits = useMemo(() => {
    if (!searchText) return userGroupLimits
    const lowerSearch = searchText.toLowerCase()
    return userGroupLimits.filter((item) =>
      item.userGroup.toLowerCase().includes(lowerSearch)
    )
  }, [searchText, userGroupLimits])

  const emit = (limits: UserGroupLimit[]) => {
    onChange(serializeUserGroupRateLimits(limits))
  }

  const handleAddUserGroup = () => {
    setUserGroupInput('')
    setUserGroupDialogOpen(true)
  }

  const handleSaveUserGroup = () => {
    const nextUserGroup = userGroupInput.trim()
    if (!nextUserGroup) return

    if (!userGroupLimits.some((item) => item.userGroup === nextUserGroup)) {
      emit([
        ...userGroupLimits,
        {
          userGroup: nextUserGroup,
          groups: [],
        },
      ])
    }
    setUserGroupDialogOpen(false)
  }

  const handleDeleteUserGroup = (userGroup: string) => {
    emit(userGroupLimits.filter((item) => item.userGroup !== userGroup))
  }

  const handleOpenGlobalLimit = (userGroup: string) => {
    const current = userGroupLimits.find((item) => item.userGroup === userGroup)
    setLimitDialogUserGroup(userGroup)
    setLimitDialogType('global')
    setLimitEditData(current?.global ?? null)
    setLimitDialogOpen(true)
  }

  const handleOpenRequestGroupLimit = (
    userGroup: string,
    groupLimit?: RequestGroupLimit
  ) => {
    setLimitDialogUserGroup(userGroup)
    setLimitDialogType('group')
    setLimitEditData(groupLimit ?? null)
    setLimitDialogOpen(true)
  }

  const handleDeleteGlobalLimit = (userGroup: string) => {
    emit(
      userGroupLimits.map((item) =>
        item.userGroup === userGroup ? { ...item, global: undefined } : item
      )
    )
  }

  const handleDeleteRequestGroupLimit = (
    userGroup: string,
    groupName: string
  ) => {
    emit(
      userGroupLimits.map((item) =>
        item.userGroup === userGroup
          ? {
              ...item,
              groups: item.groups.filter((group) => group.groupName !== groupName),
            }
          : item
      )
    )
  }

  const handleSaveLimit = (data: RequestGroupLimit) => {
    emit(
      userGroupLimits.map((item) => {
        if (item.userGroup !== limitDialogUserGroup) return item

        if (limitDialogType === 'global') {
          return {
            ...item,
            global: {
              maxRequests: data.maxRequests,
              maxSuccess: data.maxSuccess,
            },
          }
        }

        const editGroupName =
          limitEditData && 'groupName' in limitEditData
            ? limitEditData.groupName
            : data.groupName
        const groups = item.groups.filter(
          (group) => group.groupName !== editGroupName
        )
        groups.push(data)
        return {
          ...item,
          groups,
        }
      })
    )
    setLimitDialogOpen(false)
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search user groups...')}
            value={searchText}
            onChange={(event) => setSearchText(event.target.value)}
            className='pl-9'
          />
        </div>
        <Button type='button' onClick={handleAddUserGroup}>
          <Plus className='mr-2 h-4 w-4' />
          {t('Add user group')}
        </Button>
      </div>

      {filteredUserGroupLimits.length === 0 ? (
        <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center'>
          {searchText
            ? t('No user groups match your search')
            : t('No user group rate limits configured.')}
        </div>
      ) : (
        <div className='space-y-3'>
          {filteredUserGroupLimits.map((item) => (
            <Collapsible key={item.userGroup} defaultOpen>
              <div className='overflow-hidden rounded-lg border'>
                <div className='flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between'>
                  <div className='flex min-w-0 items-center gap-2'>
                    <CollapsibleTrigger
                      render={<Button type='button' variant='ghost' size='sm' />}
                    >
                      <ChevronDown className='h-4 w-4' />
                    </CollapsibleTrigger>
                    <div className='min-w-0'>
                      <div className='truncate font-semibold'>
                        {item.userGroup}
                      </div>
                      <div className='text-muted-foreground text-sm'>
                        {item.global
                          ? t('Global limit: {{limit}}', {
                              limit: formatLimit(
                                item.global.maxRequests,
                                item.global.maxSuccess,
                                t
                              ),
                            })
                          : t('No user group global limit')}
                      </div>
                    </div>
                  </div>
                  <div className='flex flex-wrap gap-2'>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => handleOpenGlobalLimit(item.userGroup)}
                    >
                      <Pencil className='mr-2 h-4 w-4' />
                      {item.global ? t('Edit global') : t('Set global')}
                    </Button>
                    {item.global && (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => handleDeleteGlobalLimit(item.userGroup)}
                      >
                        <Trash2 className='mr-2 h-4 w-4' />
                        {t('Clear global')}
                      </Button>
                    )}
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        handleOpenRequestGroupLimit(item.userGroup)
                      }
                    >
                      <Plus className='mr-2 h-4 w-4' />
                      {t('Add request group')}
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => handleDeleteUserGroup(item.userGroup)}
                    >
                      <Trash2 className='mr-2 h-4 w-4' />
                      {t('Delete')}
                    </Button>
                  </div>
                </div>

                <CollapsibleContent>
                  <div className='border-t'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Request group')}</TableHead>
                          <TableHead className='text-right'>
                            {t('Max Requests (incl. failures)')}
                          </TableHead>
                          <TableHead className='text-right'>
                            {t('Max Success')}
                          </TableHead>
                          <TableHead className='text-right'>
                            {t('Actions')}
                          </TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {item.groups.length === 0 ? (
                          <TableRow>
                            <TableCell
                              colSpan={4}
                              className='text-muted-foreground h-16 text-center text-sm'
                            >
                              {t('No request group limits configured.')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          item.groups.map((groupLimit) => (
                            <TableRow key={groupLimit.groupName}>
                              <TableCell className='font-medium'>
                                {groupLimit.groupName}
                              </TableCell>
                              <TableCell className='text-right font-mono'>
                                {groupLimit.maxRequests === 0
                                  ? t('Unlimited')
                                  : groupLimit.maxRequests.toLocaleString()}
                              </TableCell>
                              <TableCell className='text-right font-mono'>
                                {groupLimit.maxSuccess.toLocaleString()}
                              </TableCell>
                              <TableCell className='text-right'>
                                <div className='flex justify-end gap-2'>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    onClick={() =>
                                      handleOpenRequestGroupLimit(
                                        item.userGroup,
                                        groupLimit
                                      )
                                    }
                                    aria-label={t('Edit')}
                                  >
                                    <Pencil className='h-4 w-4' />
                                  </Button>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    onClick={() =>
                                      handleDeleteRequestGroupLimit(
                                        item.userGroup,
                                        groupLimit.groupName
                                      )
                                    }
                                    aria-label={t('Delete')}
                                  >
                                    <Trash2 className='h-4 w-4' />
                                  </Button>
                                </div>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </CollapsibleContent>
              </div>
            </Collapsible>
          ))}
        </div>
      )}

      <Dialog open={userGroupDialogOpen} onOpenChange={setUserGroupDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add user group')}</DialogTitle>
            <DialogDescription>
              {t('Create a user group entry for independent rate limits.')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-2 py-4'>
            <Label>{t('User group name')}</Label>
            <Input
              value={userGroupInput}
              onChange={(event) => setUserGroupInput(event.target.value)}
              placeholder={t('vip')}
            />
          </div>
          <DialogFooter>
            <Button
              type='button'
              variant='outline'
              onClick={() => setUserGroupDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='button' onClick={handleSaveUserGroup}>
              {t('Add')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <RateLimitRuleDialog
        open={limitDialogOpen}
        onOpenChange={setLimitDialogOpen}
        type={limitDialogType}
        editData={limitEditData}
        onSave={handleSaveLimit}
      />
    </div>
  )
}

type RateLimitRuleDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  type: 'global' | 'group'
  editData: RequestGroupLimit | UserGroupLimit['global'] | null
  onSave: (data: RequestGroupLimit) => void
}

function RateLimitRuleDialog({
  open,
  onOpenChange,
  type,
  editData,
  onSave,
}: RateLimitRuleDialogProps) {
  const { t } = useTranslation()
  const isGroupLimit = type === 'group'
  const [groupName, setGroupName] = useState('')
  const [maxRequests, setMaxRequests] = useState('0')
  const [maxSuccess, setMaxSuccess] = useState('1')

  useEffect(() => {
    if (!open) return

    setGroupName(editData && 'groupName' in editData ? editData.groupName : '')
    setMaxRequests(String(editData?.maxRequests ?? 0))
    setMaxSuccess(String(editData?.maxSuccess ?? 1))
  }, [editData, open])

  const handleSave = () => {
    const parsedMaxRequests = parseInt(maxRequests, 10)
    const parsedMaxSuccess = parseInt(maxSuccess, 10)
    const nextGroupName = groupName.trim()

    if (isGroupLimit && !nextGroupName) return
    if (
      !Number.isInteger(parsedMaxRequests) ||
      !Number.isInteger(parsedMaxSuccess) ||
      parsedMaxRequests < 0 ||
      parsedMaxSuccess < 1 ||
      parsedMaxRequests > 2147483647 ||
      parsedMaxSuccess > 2147483647
    ) {
      return
    }

    onSave({
      groupName: isGroupLimit ? nextGroupName : '__global__',
      maxRequests: parsedMaxRequests,
      maxSuccess: parsedMaxSuccess,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isGroupLimit
              ? editData
                ? t('Edit request group limit')
                : t('Add request group limit')
              : t('Configure user group global limit')}
          </DialogTitle>
          <DialogDescription>
            {isGroupLimit
              ? t('Limit this user group only when it uses a request group.')
              : t('Limit all model requests from this user group.')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4 py-4'>
          {isGroupLimit && (
            <div className='space-y-2'>
              <Label>{t('Request group')}</Label>
              <Input
                value={groupName}
                onChange={(event) => setGroupName(event.target.value)}
                placeholder={t('codex')}
                disabled={!!editData}
              />
            </div>
          )}
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label>{t('Max Requests (including failures)')}</Label>
              <Input
                type='number'
                min={0}
                max={2147483647}
                step={1}
                value={maxRequests}
                onChange={(event) => setMaxRequests(event.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label>{t('Max Successful Requests')}</Label>
              <Input
                type='number'
                min={1}
                max={2147483647}
                step={1}
                value={maxSuccess}
                onChange={(event) => setMaxSuccess(event.target.value)}
              />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleSave}>
            {editData ? t('Update') : t('Add')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
