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
import { useState, useMemo, useEffect, useCallback, memo } from 'react'
import {
  ArrowRightLeft,
  Pencil,
  Plus,
  Trash2,
  GripVertical,
  ChevronUp,
  ChevronDown,
} from 'lucide-react'
import { Reorder, useDragControls } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ConfirmDialog } from '@/components/confirm-dialog'
import type { GroupDetailInput } from '../types'
import { safeJsonParse } from '../utils/json-parser'
import { applyAutoGroupOrder } from './auto-group-order'
import {
  createUniqueGroupCode,
  getGroupIdDisplayValue,
  getGroupNameByCode,
} from './group-identity'
import { GroupTokenMigrationDialog } from './group-token-migration-dialog'

type GroupRatioVisualEditorProps = {
  groups: EditableGroupDetail[]
  reservedGroupCodes: ReadonlySet<string>
  isLoadingGroups: boolean
  groupLoadError: string | null
  topupGroupRatio: string
  groupGroupRatio: string
  onChange: (field: string, value: string) => void
  onGroupsChange: (groups: EditableGroupDetail[]) => void
  onRetryGroups: () => void
}

type SimpleGroup = {
  name: string
  value: string
}

export type EditableGroupDetail = Omit<GroupDetailInput, 'status'> & {
  _key: string
  status: number
}

type GroupOverride = {
  targetGroup: string
  ratio: number
}

const sectionCardClassName =
  'relative shadow-sm ring-0 before:pointer-events-none before:absolute before:inset-0 before:rounded-xl before:border before:border-border/90'
const sectionHeaderClassName = 'border-b bg-muted/20'

let groupPricingIdCounter = 0
function createGroupPricingId() {
  groupPricingIdCounter += 1
  return `gpr_${groupPricingIdCounter}`
}

function normalizeRatio(value: unknown): number {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 1
}

function AutoGroupReorderItem({
  group,
  index,
  itemCount,
  onMove,
  onDelete,
}: {
  group: EditableGroupDetail
  index: number
  itemCount: number
  onMove: (index: number, direction: 'up' | 'down') => void
  onDelete: (index: number) => void
}) {
  const { t } = useTranslation()
  const dragControls = useDragControls()

  return (
    <Reorder.Item
      as='div'
      value={group._key}
      dragListener={false}
      dragControls={dragControls}
      className='bg-background relative flex items-center gap-2 rounded-md border p-3'
      whileDrag={{
        boxShadow: '0 4px 16px rgba(0, 0, 0, 0.12)',
        zIndex: 50,
      }}
    >
      <span
        aria-hidden='true'
        className='text-muted-foreground hover:text-foreground shrink-0 cursor-grab touch-none active:cursor-grabbing'
        title={t('Sort')}
        onPointerDown={(event) => dragControls.start(event)}
      >
        <GripVertical className='h-4 w-4' />
      </span>
      <span className='min-w-0 flex-1 truncate font-medium'>{group.name}</span>
      <div className='flex shrink-0 gap-1'>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          disabled={index === 0}
          aria-label={`${t('Sort')} ↑`}
          title={`${t('Sort')} ↑`}
          onClick={() => onMove(index, 'up')}
        >
          <ChevronUp className='h-4 w-4' />
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          disabled={index === itemCount - 1}
          aria-label={`${t('Sort')} ↓`}
          title={`${t('Sort')} ↓`}
          onClick={() => onMove(index, 'down')}
        >
          <ChevronDown className='h-4 w-4' />
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          aria-label={t('Delete')}
          title={t('Delete')}
          onClick={() => onDelete(index)}
        >
          <Trash2 className='h-4 w-4' />
        </Button>
      </div>
    </Reorder.Item>
  )
}

export const GroupRatioVisualEditor = memo(function GroupRatioVisualEditor({
  groups,
  reservedGroupCodes,
  isLoadingGroups,
  groupLoadError,
  topupGroupRatio,
  groupGroupRatio,
  onChange,
  onGroupsChange,
  onRetryGroups,
}: GroupRatioVisualEditorProps) {
  const { t } = useTranslation()
  const [simpleDialogOpen, setSimpleDialogOpen] = useState(false)
  const [simpleEditData, setSimpleEditData] = useState<SimpleGroup | null>(null)

  const [autoGroupDialogOpen, setAutoGroupDialogOpen] = useState(false)
  const [autoGroupInput, setAutoGroupInput] = useState('')

  const [groupOverrideDialogOpen, setGroupOverrideDialogOpen] = useState(false)
  const [groupOverrideUserGroup, setGroupOverrideUserGroup] = useState<
    string | null
  >(null)
  const [groupOverrideEditData, setGroupOverrideEditData] =
    useState<GroupOverride | null>(null)

  const [userGroupDialogOpen, setUserGroupDialogOpen] = useState(false)
  const [userGroupInput, setUserGroupInput] = useState('')

  // Parse topup group ratios
  const topupRatioList = useMemo(() => {
    const map = safeJsonParse<Record<string, number>>(topupGroupRatio, {
      fallback: {},
      context: 'topup group ratios',
    })
    return Object.entries(map).map(([name, value]) => ({
      name,
      value: String(value),
    }))
  }, [topupGroupRatio])

  // 自动分组顺序直接维护在结构化分组数据中。
  const autoGroupsList = useMemo(() => {
    return groups
      .filter((group) => group.auto_enabled)
      .sort((a, b) => a.auto_order - b.auto_order)
  }, [groups])

  const availableAutoGroups = useMemo(
    () =>
      groups.filter(
        (group) => !group.auto_enabled && group.code.trim() && group.name.trim()
      ),
    [groups]
  )

  // Parse group-group ratios
  const groupGroupRatioList = useMemo(() => {
    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        context: 'group-group ratios',
      }
    )
    return Object.entries(map).map(([userGroup, overrides]) => ({
      userGroup,
      overrides: Object.entries(overrides).map(([targetGroup, ratio]) => ({
        targetGroup,
        ratio,
      })),
    }))
  }, [groupGroupRatio])
  const availableUserGroupOptions = useMemo(() => {
    const configuredCodes = new Set(
      groupGroupRatioList.map((item) => item.userGroup)
    )
    return groups
      .filter(
        (group) =>
          group.code.trim() &&
          group.name.trim() &&
          !configuredCodes.has(group.code)
      )
      .map((group) => ({ value: group.code, label: group.name }))
  }, [groupGroupRatioList, groups])

  // 充值倍率仍使用旧 Option 配置。
  const handleSimpleAdd = () => {
    setSimpleEditData(null)
    setSimpleDialogOpen(true)
  }

  const handleSimpleEdit = (group: SimpleGroup) => {
    setSimpleEditData(group)
    setSimpleDialogOpen(true)
  }

  const handleSimpleSave = (name: string, value: string) => {
    const map = safeJsonParse<Record<string, number>>(topupGroupRatio, {
      fallback: {},
      silent: true,
    })

    if (simpleEditData && simpleEditData.name !== name) {
      delete map[simpleEditData.name]
    }

    map[name] = parseFloat(value)

    onChange('TopupGroupRatio', JSON.stringify(map, null, 2))
    setSimpleDialogOpen(false)
  }

  const handleSimpleDelete = (name: string) => {
    const map = safeJsonParse<Record<string, number>>(topupGroupRatio, {
      fallback: {},
      silent: true,
    })
    delete map[name]

    onChange('TopupGroupRatio', JSON.stringify(map, null, 2))
  }

  // Auto groups handlers
  const handleAutoGroupAdd = () => {
    setAutoGroupInput('')
    setAutoGroupDialogOpen(true)
  }

  const handleAutoGroupSave = () => {
    if (!autoGroupInput) return

    onGroupsChange(
      applyAutoGroupOrder(groups, [
        ...autoGroupsList.map((group) => group._key),
        autoGroupInput,
      ])
    )
    setAutoGroupDialogOpen(false)
  }

  const handleAutoGroupDelete = (index: number) => {
    const orderedKeys = autoGroupsList
      .filter((_, currentIndex) => currentIndex !== index)
      .map((group) => group._key)
    onGroupsChange(applyAutoGroupOrder(groups, orderedKeys))
  }

  const handleAutoGroupMove = (index: number, direction: 'up' | 'down') => {
    const orderedKeys = autoGroupsList.map((group) => group._key)
    const newIndex = direction === 'up' ? index - 1 : index + 1

    if (newIndex < 0 || newIndex >= orderedKeys.length) return
    ;[orderedKeys[index], orderedKeys[newIndex]] = [
      orderedKeys[newIndex],
      orderedKeys[index],
    ]
    onGroupsChange(applyAutoGroupOrder(groups, orderedKeys))
  }

  const handleAutoGroupReorder = (orderedKeys: string[]) => {
    onGroupsChange(applyAutoGroupOrder(groups, orderedKeys))
  }

  // Group-group ratio handlers
  const handleUserGroupAdd = () => {
    setUserGroupInput('')
    setUserGroupDialogOpen(true)
  }

  const handleUserGroupSave = () => {
    if (!userGroupInput.trim()) return

    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )

    if (!map[userGroupInput.trim()]) {
      map[userGroupInput.trim()] = {}
    }

    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
    setUserGroupDialogOpen(false)
  }

  const handleUserGroupDelete = (userGroup: string) => {
    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )
    delete map[userGroup]
    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
  }

  const handleOverrideAdd = (userGroup: string) => {
    setGroupOverrideUserGroup(userGroup)
    setGroupOverrideEditData(null)
    setGroupOverrideDialogOpen(true)
  }

  const handleOverrideEdit = (userGroup: string, override: GroupOverride) => {
    setGroupOverrideUserGroup(userGroup)
    setGroupOverrideEditData(override)
    setGroupOverrideDialogOpen(true)
  }

  const handleOverrideSave = (
    targetGroup: string,
    ratio: number,
    oldTargetGroup?: string
  ) => {
    if (!groupOverrideUserGroup) return

    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )

    if (!map[groupOverrideUserGroup]) {
      map[groupOverrideUserGroup] = {}
    }

    if (oldTargetGroup && oldTargetGroup !== targetGroup) {
      delete map[groupOverrideUserGroup][oldTargetGroup]
    }

    map[groupOverrideUserGroup][targetGroup] = ratio

    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
    setGroupOverrideDialogOpen(false)
  }

  const handleOverrideDelete = (userGroup: string, targetGroup: string) => {
    const map = safeJsonParse<Record<string, Record<string, number>>>(
      groupGroupRatio,
      {
        fallback: {},
        silent: true,
      }
    )

    if (map[userGroup]) {
      delete map[userGroup][targetGroup]
      if (Object.keys(map[userGroup]).length === 0) {
        delete map[userGroup]
      }
    }

    onChange('GroupGroupRatio', JSON.stringify(map, null, 2))
  }

  return (
    <div className='space-y-4'>
      <GroupPricingTable
        groups={groups}
        reservedGroupCodes={reservedGroupCodes}
        isLoading={isLoadingGroups}
        loadError={groupLoadError}
        onGroupsChange={onGroupsChange}
        onRetry={onRetryGroups}
      />

      {/* Topup Group Ratios */}
      <Card className={sectionCardClassName}>
        <CardHeader className={sectionHeaderClassName}>
          <CardTitle>{t('Top-up group ratios')}</CardTitle>
          <CardDescription>
            {t('Multipliers for recharge pricing based on user groups.')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='space-y-4'>
            <Button onClick={handleSimpleAdd} size='sm'>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add group')}
            </Button>
            {topupRatioList.length > 0 && (
              <div className='rounded-md border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Group name')}</TableHead>
                      <TableHead>{t('Multiplier')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {topupRatioList.map((group) => (
                      <TableRow key={group.name}>
                        <TableCell className='font-medium'>
                          {getGroupNameByCode(groups, group.name) ||
                            t('Unknown')}
                        </TableCell>
                        <TableCell>{group.value}</TableCell>
                        <TableCell className='text-right'>
                          <div className='flex justify-end gap-2'>
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() => handleSimpleEdit(group)}
                            >
                              <Pencil className='h-4 w-4' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() => handleSimpleDelete(group.name)}
                            >
                              <Trash2 className='h-4 w-4' />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Inter-group ratio overrides */}
      <Card className={sectionCardClassName}>
        <CardHeader className={sectionHeaderClassName}>
          <CardTitle>{t('Inter-group ratio overrides')}</CardTitle>
          <CardDescription>
            {t(
              'Custom multipliers when specific user groups use specific token groups. Example: VIP users get 0.9x rate when using "edit_this" group tokens.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='space-y-4'>
            <Button onClick={handleUserGroupAdd} size='sm'>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add user group')}
            </Button>
            {groupGroupRatioList.length > 0 && (
              <div className='space-y-3'>
                {groupGroupRatioList.map((userGroupData) => (
                  <Collapsible key={userGroupData.userGroup}>
                    <div className='rounded-lg border'>
                      <div className='flex items-center justify-between p-4'>
                        <div className='flex items-center gap-2'>
                          <CollapsibleTrigger
                            render={<Button variant='ghost' size='sm' />}
                          >
                            <ChevronDown className='h-4 w-4' />
                          </CollapsibleTrigger>
                          <span className='font-semibold'>
                            {getGroupNameByCode(
                              groups,
                              userGroupData.userGroup
                            ) || t('Unknown')}
                          </span>
                          <span className='text-muted-foreground text-sm'>
                            {t('{{count}} override', {
                              count: userGroupData.overrides.length,
                            })}
                          </span>
                        </div>
                        <div className='flex gap-2'>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() =>
                              handleOverrideAdd(userGroupData.userGroup)
                            }
                          >
                            <Plus className='h-4 w-4' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() =>
                              handleUserGroupDelete(userGroupData.userGroup)
                            }
                          >
                            <Trash2 className='h-4 w-4' />
                          </Button>
                        </div>
                      </div>
                      <CollapsibleContent>
                        {userGroupData.overrides.length > 0 && (
                          <div className='border-t'>
                            <Table>
                              <TableHeader>
                                <TableRow>
                                  <TableHead>{t('Target group')}</TableHead>
                                  <TableHead>{t('Ratio')}</TableHead>
                                  <TableHead className='text-right'>
                                    {t('Actions')}
                                  </TableHead>
                                </TableRow>
                              </TableHeader>
                              <TableBody>
                                {userGroupData.overrides.map((override) => (
                                  <TableRow key={override.targetGroup}>
                                    <TableCell className='font-medium'>
                                      {getGroupNameByCode(
                                        groups,
                                        override.targetGroup
                                      ) || t('Unknown')}
                                    </TableCell>
                                    <TableCell>{override.ratio}</TableCell>
                                    <TableCell className='text-right'>
                                      <div className='flex justify-end gap-2'>
                                        <Button
                                          variant='ghost'
                                          size='sm'
                                          onClick={() =>
                                            handleOverrideEdit(
                                              userGroupData.userGroup,
                                              override
                                            )
                                          }
                                        >
                                          <Pencil className='h-4 w-4' />
                                        </Button>
                                        <Button
                                          variant='ghost'
                                          size='sm'
                                          onClick={() =>
                                            handleOverrideDelete(
                                              userGroupData.userGroup,
                                              override.targetGroup
                                            )
                                          }
                                        >
                                          <Trash2 className='h-4 w-4' />
                                        </Button>
                                      </div>
                                    </TableCell>
                                  </TableRow>
                                ))}
                              </TableBody>
                            </Table>
                          </div>
                        )}
                      </CollapsibleContent>
                    </div>
                  </Collapsible>
                ))}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Auto Groups */}
      <Card className={sectionCardClassName}>
        <CardHeader className={sectionHeaderClassName}>
          <CardTitle>{t('Auto assignment order')}</CardTitle>
          <CardDescription>
            {t(
              'Priority order for automatic group assignment. New tokens rotate through this list.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='space-y-4'>
            <Button onClick={handleAutoGroupAdd} size='sm'>
              <Plus className='mr-2 h-4 w-4' />
              {t('Add group')}
            </Button>
            {autoGroupsList.length > 0 && (
              <Reorder.Group
                as='div'
                axis='y'
                values={autoGroupsList.map((group) => group._key)}
                onReorder={handleAutoGroupReorder}
                className='flex flex-col gap-2'
                layoutScroll
              >
                {autoGroupsList.map((group, index) => (
                  <AutoGroupReorderItem
                    key={group._key}
                    group={group}
                    index={index}
                    itemCount={autoGroupsList.length}
                    onMove={handleAutoGroupMove}
                    onDelete={handleAutoGroupDelete}
                  />
                ))}
              </Reorder.Group>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Simple Group Dialog */}
      <SimpleGroupDialog
        open={simpleDialogOpen}
        onOpenChange={setSimpleDialogOpen}
        onSave={handleSimpleSave}
        editData={simpleEditData}
        groups={groups}
      />

      {/* Auto Group Dialog */}
      <Dialog open={autoGroupDialogOpen} onOpenChange={setAutoGroupDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add auto group')}</DialogTitle>
            <DialogDescription>{t('Select a group')}</DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label>{t('Group')}</Label>
              <Select
                items={availableAutoGroups.map((group) => ({
                  value: group._key,
                  label: group.name,
                }))}
                value={autoGroupInput}
                onValueChange={(value) => setAutoGroupInput(value ?? '')}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Select a group')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {availableAutoGroups.map((group) => (
                      <SelectItem key={group._key} value={group._key}>
                        {group.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              {availableAutoGroups.length === 0 && (
                <p className='text-muted-foreground text-sm'>
                  {t('All groups are already in the auto assignment list.')}
                </p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setAutoGroupDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleAutoGroupSave} disabled={!autoGroupInput}>
              {t('Add')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* User Group Dialog */}
      <Dialog open={userGroupDialogOpen} onOpenChange={setUserGroupDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add user group')}</DialogTitle>
            <DialogDescription>
              {t('Create a new user group to configure ratio overrides for.')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label>{t('User group')}</Label>
              <Select
                items={availableUserGroupOptions}
                value={userGroupInput}
                onValueChange={(value) => setUserGroupInput(value ?? '')}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Select a group')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {availableUserGroupOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setUserGroupDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleUserGroupSave} disabled={!userGroupInput}>
              {t('Add')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Group Override Dialog */}
      <GroupOverrideDialog
        open={groupOverrideDialogOpen}
        onOpenChange={setGroupOverrideDialogOpen}
        onSave={handleOverrideSave}
        editData={groupOverrideEditData}
        userGroup={groupOverrideUserGroup}
        groups={groups}
      />
    </div>
  )
})

type GroupPricingTableProps = {
  groups: EditableGroupDetail[]
  reservedGroupCodes: ReadonlySet<string>
  isLoading: boolean
  loadError: string | null
  onGroupsChange: (groups: EditableGroupDetail[]) => void
  onRetry: () => void
}

function GroupPricingTable({
  groups,
  reservedGroupCodes,
  isLoading,
  loadError,
  onGroupsChange,
  onRetry,
}: GroupPricingTableProps) {
  const { t } = useTranslation()
  const [migrationDialogOpen, setMigrationDialogOpen] = useState(false)
  const [pendingDeleteKey, setPendingDeleteKey] = useState<string | null>(null)

  const updateRow = useCallback(
    (
      key: string,
      field: 'name' | 'description' | 'ratio' | 'user_selectable',
      value: string | number | boolean
    ) => {
      onGroupsChange(
        groups.map((group) =>
          group._key === key ? { ...group, [field]: value } : group
        )
      )
    },
    [groups, onGroupsChange]
  )

  const addRow = useCallback(() => {
    const code = createUniqueGroupCode([
      ...reservedGroupCodes,
      ...groups.map((group) => group.code),
    ])

    onGroupsChange([
      ...groups,
      {
        _key: createGroupPricingId(),
        code,
        name: '',
        description: '',
        ratio: 1,
        user_selectable: true,
        status: 1,
        auto_enabled: false,
        auto_order: 0,
      },
    ])
  }, [groups, onGroupsChange, reservedGroupCodes])

  const removeRow = useCallback(
    (key: string) => {
      onGroupsChange(groups.filter((group) => group._key !== key))
    },
    [groups, onGroupsChange]
  )

  const duplicateNames = useMemo(() => {
    const counts = new Map<string, number>()
    for (const group of groups) {
      const name = group.name.trim()
      if (!name) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
    }
    return Array.from(counts.entries())
      .filter(([, count]) => count > 1)
      .map(([name]) => name)
  }, [groups])

  const hasMissingName = groups.some((group) => !group.name.trim())
  const pendingDeleteGroup = groups.find(
    (group) => group._key === pendingDeleteKey
  )

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div>
            <CardTitle>{t('Pricing groups')}</CardTitle>
            <CardDescription>
              {t(
                'Edit billing ratios and user-selectable groups in one table.'
              )}
            </CardDescription>
          </div>
          <div className='flex flex-wrap gap-2 sm:self-start'>
            <Button
              variant='outline'
              onClick={() => setMigrationDialogOpen(true)}
              size='sm'
              disabled={
                isLoading ||
                Boolean(loadError) ||
                groups.filter((group) => Number(group.id) > 0).length < 1
              }
            >
              <ArrowRightLeft className='mr-2 h-4 w-4' />
              {t('Migrate tokens')}
            </Button>
            <Button
              onClick={addRow}
              size='sm'
              disabled={isLoading || Boolean(loadError)}
            >
              <Plus className='mr-2 h-4 w-4' />
              {t('Add group')}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className='space-y-3'>
          <div className='overflow-x-auto rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-20'>{t('ID')}</TableHead>
                  <TableHead className='min-w-40'>{t('Group name')}</TableHead>
                  <TableHead className='w-28'>{t('Ratio')}</TableHead>
                  <TableHead className='w-28 text-center'>
                    {t('User selectable')}
                  </TableHead>
                  <TableHead className='min-w-56'>{t('Description')}</TableHead>
                  <TableHead className='w-16 text-right'>
                    {t('Actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {isLoading ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className='text-muted-foreground h-20 text-center text-sm'
                    >
                      {t('Loading groups...')}
                    </TableCell>
                  </TableRow>
                ) : loadError ? (
                  <TableRow>
                    <TableCell colSpan={6} className='h-24 text-center'>
                      <div className='flex flex-col items-center gap-2'>
                        <span className='text-destructive text-sm'>
                          {loadError}
                        </span>
                        <Button variant='outline' size='sm' onClick={onRetry}>
                          {t('Retry')}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : groups.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className='text-muted-foreground h-20 text-center text-sm'
                    >
                      {t('No groups yet. Add a group to get started.')}
                    </TableCell>
                  </TableRow>
                ) : (
                  groups.map((group) => {
                    const groupId = getGroupIdDisplayValue(group.id)
                    return (
                      <TableRow key={group._key}>
                        <TableCell className='text-muted-foreground font-mono text-xs'>
                          {groupId === 'New' ? t(groupId) : groupId}
                        </TableCell>
                        <TableCell>
                          <Input
                            value={group.name}
                            onChange={(event) =>
                              updateRow(group._key, 'name', event.target.value)
                            }
                            aria-invalid={
                              !group.name.trim() ||
                              duplicateNames.includes(group.name.trim())
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            type='number'
                            min={0}
                            step={0.1}
                            value={String(group.ratio)}
                            onChange={(event) =>
                              updateRow(
                                group._key,
                                'ratio',
                                normalizeRatio(event.target.value)
                              )
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <div className='flex justify-center'>
                            <Checkbox
                              checked={group.user_selectable}
                              onCheckedChange={(checked) =>
                                updateRow(
                                  group._key,
                                  'user_selectable',
                                  checked === true
                                )
                              }
                              aria-label={t('User selectable')}
                            />
                          </div>
                        </TableCell>
                        <TableCell>
                          <Input
                            value={group.description}
                            placeholder={t('Group description')}
                            onChange={(event) =>
                              updateRow(
                                group._key,
                                'description',
                                event.target.value
                              )
                            }
                          />
                        </TableCell>
                        <TableCell className='text-right'>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() => {
                              if (Number(group.id) > 0) {
                                setPendingDeleteKey(group._key)
                              } else {
                                removeRow(group._key)
                              }
                            }}
                            aria-label={t('Delete')}
                          >
                            <Trash2 className='h-4 w-4' />
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>

          {duplicateNames.length > 0 && (
            <p className='text-destructive text-sm'>
              {t('Duplicate group names: {{names}}', {
                names: duplicateNames.join(', '),
              })}
            </p>
          )}
          {hasMissingName && (
            <p className='text-destructive text-sm'>
              {`${t('Group name')}: ${t('Required')}`}
            </p>
          )}
        </div>
      </CardContent>
      <GroupTokenMigrationDialog
        open={migrationDialogOpen}
        onOpenChange={setMigrationDialogOpen}
        groups={groups}
      />
      <ConfirmDialog
        open={Boolean(pendingDeleteGroup)}
        onOpenChange={(open) => {
          if (!open) setPendingDeleteKey(null)
        }}
        title={t('Mark this group for deletion?')}
        desc={t(
          'Tokens bound to this group will switch to automatic grouping when you save. Channels, users, or other references can still prevent deletion.'
        )}
        confirmText={t('Mark for deletion')}
        destructive
        handleConfirm={() => {
          if (pendingDeleteKey) removeRow(pendingDeleteKey)
          setPendingDeleteKey(null)
        }}
      />
    </Card>
  )
}

// Simple Group Dialog Component
type SimpleGroupDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (name: string, value: string) => void
  editData: SimpleGroup | null
  groups: EditableGroupDetail[]
}

function SimpleGroupDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  groups,
}: SimpleGroupDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [value, setValue] = useState('')

  const title = t('top-up ratio')
  const groupOptions = useMemo(() => {
    const options = groups
      .filter((group) => group.code.trim() && group.name.trim())
      .map((group) => ({ value: group.code, label: group.name }))

    if (
      editData?.name &&
      !options.some((option) => option.value === editData.name)
    ) {
      options.push({ value: editData.name, label: t('Unknown') })
    }
    return options
  }, [editData?.name, groups, t])

  useEffect(() => {
    if (!open) {
      setName('')
      setValue('')
      return
    }

    setName(editData?.name ?? '')
    setValue(editData?.value ?? '')
  }, [editData, open])

  const handleSave = () => {
    if (!name.trim() || !value.trim()) return
    onSave(name.trim(), value.trim())
    setName('')
    setValue('')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {editData
              ? t('Edit {{title}}', { title })
              : t('Add {{title}}', { title })}
          </DialogTitle>
          <DialogDescription>
            {t('Configure the ratio for this group.')}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-4'>
          <div className='space-y-2'>
            <Label>{t('Group')}</Label>
            <Select
              items={groupOptions}
              value={name}
              onValueChange={(nextName) => setName(nextName ?? '')}
              disabled={!!editData}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Select a group')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {groupOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-2'>
            <Label>{t('Ratio')}</Label>
            <Input
              value={value}
              onChange={(e) => {
                const val = e.target.value
                if (val === '' || !isNaN(parseFloat(val))) {
                  setValue(val)
                }
              }}
              placeholder='1.0'
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave}>
            {editData ? t('Update') : t('Add')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Group Override Dialog Component
type GroupOverrideDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (targetGroup: string, ratio: number, oldTargetGroup?: string) => void
  editData: GroupOverride | null
  userGroup: string | null
  groups: EditableGroupDetail[]
}

function GroupOverrideDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  userGroup,
  groups,
}: GroupOverrideDialogProps) {
  const { t } = useTranslation()
  const [targetGroup, setTargetGroup] = useState('')
  const [ratio, setRatio] = useState('')
  const groupOptions = useMemo(() => {
    const options = groups
      .filter((group) => group.code.trim() && group.name.trim())
      .map((group) => ({ value: group.code, label: group.name }))

    if (
      editData?.targetGroup &&
      !options.some((option) => option.value === editData.targetGroup)
    ) {
      options.push({ value: editData.targetGroup, label: t('Unknown') })
    }

    return options
  }, [editData?.targetGroup, groups, t])
  const targetGroupName =
    groupOptions.find((option) => option.value === targetGroup)?.label ||
    t('this token group')
  const userGroupName = userGroup
    ? getGroupNameByCode(groups, userGroup) || t('Unknown')
    : t('this user group')

  useEffect(() => {
    if (!open) {
      setTargetGroup('')
      setRatio('')
      return
    }

    setTargetGroup(editData?.targetGroup ?? '')
    setRatio(editData ? String(editData.ratio) : '')
  }, [editData, open])

  const handleSave = () => {
    if (!targetGroup.trim() || !ratio.trim()) return
    const parsedRatio = parseFloat(ratio)
    if (isNaN(parsedRatio)) return

    onSave(targetGroup.trim(), parsedRatio, editData?.targetGroup)
    setTargetGroup('')
    setRatio('')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {editData ? t('Edit ratio override') : t('Add ratio override')}
          </DialogTitle>
          <DialogDescription>
            {userGroup
              ? t(
                  'Configure a custom ratio for "{{userGroup}}" users when using a specific token group.',
                  { userGroup: userGroupName }
                )
              : t(
                  'Configure a custom ratio for when users use a specific token group.'
                )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4 py-4'>
          <div className='space-y-2'>
            <Label>{t('Target group')}</Label>
            <Select
              items={groupOptions}
              value={targetGroup}
              onValueChange={(value) => setTargetGroup(value ?? '')}
              disabled={!!editData}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Select a group')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {groupOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              {t('The token group that will have a custom ratio')}
            </p>
          </div>
          <div className='space-y-2'>
            <Label>{t('Ratio')}</Label>
            <Input
              value={ratio}
              onChange={(e) => {
                const val = e.target.value
                if (val === '' || !isNaN(parseFloat(val))) {
                  setRatio(val)
                }
              }}
              placeholder='0.9'
            />
            <p className='text-muted-foreground text-xs'>
              {t('Multiplier applied when {{userGroup}} uses {{targetGroup}}', {
                userGroup: userGroupName,
                targetGroup: targetGroupName,
              })}
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave}>
            {editData ? t('Update') : t('Add')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
