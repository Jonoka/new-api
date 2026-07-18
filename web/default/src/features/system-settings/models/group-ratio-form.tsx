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
import { memo, useCallback, useEffect, useState } from 'react'
import { type UseFormReturn } from 'react-hook-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Code2, Eye, HelpCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { getGroupDetails, updateGroupDetails } from '../api'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageActionsPortal } from '../components/settings-page-context'
import type { GroupDetail, GroupDetailInput } from '../types'
import {
  GroupRatioVisualEditor,
  type EditableGroupDetail,
} from './group-ratio-visual-editor'
import { GroupSpecialUsableRulesEditor } from './group-special-usable-editor'

type GroupFormValues = {
  GroupRatio: string
  TopupGroupRatio: string
  UserUsableGroups: string
  GroupGroupRatio: string
  AutoGroups: string
  DefaultUseAutoGroup: boolean
  GroupSpecialUsableGroup: string
}

type GroupRatioFormProps = {
  form: UseFormReturn<GroupFormValues>
  onSave: (values: GroupFormValues) => Promise<void>
  isSaving: boolean
}

const groupDetailsQueryKey = ['system-settings', 'group-details'] as const

let editableGroupKeyCounter = 0

function createEditableGroup(group: GroupDetail): EditableGroupDetail {
  editableGroupKeyCounter += 1
  const code = String(group.code ?? '').trim()
  const ratio = Number(group.ratio)
  const autoOrder = Number(group.auto_order)

  return {
    id: Number(group.id),
    code,
    name: String(group.name ?? '').trim() || code,
    description: String(group.description ?? ''),
    ratio: Number.isFinite(ratio) && ratio >= 0 ? ratio : 1,
    user_selectable: group.user_selectable === true,
    status: Number(group.status) === 0 ? 0 : 1,
    auto_enabled: group.auto_enabled === true,
    auto_order: Number.isInteger(autoOrder) && autoOrder >= 0 ? autoOrder : 0,
    _key:
      group.id > 0
        ? `group_${group.id}`
        : `loaded_group_${editableGroupKeyCounter}`,
  }
}

function createGroupDetailsPayload(
  groups: EditableGroupDetail[]
): GroupDetailInput[] {
  const autoOrderByKey = new Map(
    groups
      .filter((group) => group.auto_enabled)
      .sort((a, b) => a.auto_order - b.auto_order)
      .map((group, index) => [group._key, index])
  )

  return groups.map((group) => {
    const autoOrder = autoOrderByKey.get(group._key)
    const input: GroupDetailInput = {
      code: group.code.trim(),
      name: group.name.trim(),
      description: group.description,
      ratio: group.ratio,
      user_selectable: group.user_selectable,
      status: group.status,
      auto_enabled: autoOrder !== undefined,
      auto_order: autoOrder ?? 0,
    }

    if (group.id && group.id > 0) {
      input.id = group.id
    }
    return input
  })
}

export const GroupRatioForm = memo(function GroupRatioForm({
  form,
  onSave,
  isSaving,
}: GroupRatioFormProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')
  const [guideOpen, setGuideOpen] = useState(false)
  const [groups, setGroups] = useState<EditableGroupDetail[]>([])
  const [deletedGroupIds, setDeletedGroupIds] = useState<number[]>([])

  const groupDetailsQuery = useQuery({
    queryKey: groupDetailsQueryKey,
    queryFn: getGroupDetails,
    refetchOnWindowFocus: false,
  })

  const { mutateAsync: saveGroupDetails, isPending: isSavingGroupDetails } =
    useMutation({
      mutationFn: updateGroupDetails,
    })

  useEffect(() => {
    if (!groupDetailsQuery.data) return
    setGroups(groupDetailsQuery.data.map(createEditableGroup))
    setDeletedGroupIds([])
  }, [groupDetailsQuery.data])

  const handleFieldChange = useCallback(
    (field: keyof GroupFormValues, value: string) => {
      form.setValue(field, value, {
        shouldValidate: true,
        shouldDirty: true,
      })
    },
    [form]
  )

  const handleGroupsChange = useCallback(
    (nextGroups: EditableGroupDetail[]) => {
      const nextIds = new Set(
        nextGroups
          .map((group) => group.id)
          .filter((id): id is number => Boolean(id && id > 0))
      )
      const removedIds = groups
        .map((group) => group.id)
        .filter((id): id is number => Boolean(id && id > 0 && !nextIds.has(id)))

      setDeletedGroupIds((current) =>
        Array.from(new Set([...current, ...removedIds])).filter(
          (id) => !nextIds.has(id)
        )
      )
      setGroups(nextGroups)
    },
    [groups]
  )

  const handleSave = useCallback(
    async (values: GroupFormValues) => {
      if (!groupDetailsQuery.data || groupDetailsQuery.isError) {
        toast.error(t('Load group details before saving.'))
        return
      }

      const codes = groups.map((group) => group.code.trim())
      if (groups.some((group) => !group.code.trim() || !group.name.trim())) {
        toast.error(t('Group code and group name are required.'))
        return
      }
      if (new Set(codes).size !== codes.length) {
        toast.error(t('Group codes must be unique.'))
        return
      }
      const names = groups.map((group) => group.name.trim())
      if (new Set(names).size !== names.length) {
        toast.error(t('Group names must be unique.'))
        return
      }
      if (
        groups.some((group) => !Number.isFinite(group.ratio) || group.ratio < 0)
      ) {
        toast.error(t('Group ratios must be non-negative numbers.'))
        return
      }

      try {
        const response = await saveGroupDetails({
          groups: createGroupDetailsPayload(groups),
          deleted_ids: deletedGroupIds,
        })
        if (!response.success) return

        const savedGroups = response.data ?? (await getGroupDetails())
        queryClient.setQueryData(groupDetailsQueryKey, savedGroups)
        queryClient.setQueryData(['channels', 'group-details'], {
          success: true,
          data: savedGroups,
        })
        setGroups(savedGroups.map(createEditableGroup))
        setDeletedGroupIds([])

        // 组间特殊倍率等高级配置仍通过旧 Option 接口保存。
        await onSave(values)
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['system-options'] }),
          queryClient.invalidateQueries({ queryKey: ['channels'] }),
          queryClient.invalidateQueries({ queryKey: ['user-groups'] }),
          queryClient.invalidateQueries({ queryKey: ['user-self-groups'] }),
          queryClient.invalidateQueries({ queryKey: ['pricing'] }),
        ])
        toast.success(t('Group settings saved successfully'))
      } catch {
        // 请求层和旧 Option mutation 已负责展示具体错误。
      }
    },
    [
      deletedGroupIds,
      groupDetailsQuery.data,
      groupDetailsQuery.isError,
      groups,
      onSave,
      queryClient,
      saveGroupDetails,
      t,
    ]
  )

  const toggleEditMode = useCallback(() => {
    setEditMode((prev) => (prev === 'visual' ? 'json' : 'visual'))
  }, [])

  const isSavingAll = isSaving || isSavingGroupDetails
  const groupLoadError = groupDetailsQuery.isError
    ? groupDetailsQuery.error.message || t('Failed to load groups')
    : null

  return (
    <div className='space-y-6'>
      <div className='flex flex-wrap justify-end gap-2'>
        <Button variant='outline' size='sm' onClick={() => setGuideOpen(true)}>
          <HelpCircle className='mr-2 h-4 w-4' />
          {t('Usage guide')}
        </Button>
        <Button variant='outline' size='sm' onClick={toggleEditMode}>
          {editMode === 'visual' ? (
            <>
              <Code2 className='mr-2 h-4 w-4' />
              {t('Switch to JSON')}
            </>
          ) : (
            <>
              <Eye className='mr-2 h-4 w-4' />
              {t('Switch to Visual')}
            </>
          )}
        </Button>
      </div>

      <GroupPricingGuide open={guideOpen} onOpenChange={setGuideOpen} />

      <Form {...form}>
        <SettingsPageActionsPortal>
          <Button
            type='button'
            size='sm'
            onClick={form.handleSubmit(handleSave)}
            disabled={
              isSavingAll ||
              groupDetailsQuery.isPending ||
              groupDetailsQuery.isError
            }
          >
            {isSavingAll ? t('Saving...') : t('Save group ratios')}
          </Button>
        </SettingsPageActionsPortal>
        {editMode === 'visual' ? (
          <div className='space-y-6'>
            <GroupRatioVisualEditor
              groups={groups}
              isLoadingGroups={groupDetailsQuery.isPending}
              groupLoadError={groupLoadError}
              topupGroupRatio={form.watch('TopupGroupRatio')}
              groupGroupRatio={form.watch('GroupGroupRatio')}
              onGroupsChange={handleGroupsChange}
              onRetryGroups={() => {
                void groupDetailsQuery.refetch()
              }}
              onChange={(field, value) =>
                handleFieldChange(field as keyof GroupFormValues, value)
              }
            />

            <GroupSpecialUsableRulesEditor
              value={form.watch('GroupSpecialUsableGroup')}
              onChange={(value) =>
                handleFieldChange('GroupSpecialUsableGroup', value)
              }
            />

            <FormField
              control={form.control}
              name='DefaultUseAutoGroup'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Default to auto groups')}</FormLabel>
                    <FormDescription>
                      {t(
                        'When enabled, newly created tokens start in the first auto group.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>
        ) : (
          <SettingsForm onSubmit={form.handleSubmit(handleSave)}>
            <div className='bg-muted/30 text-muted-foreground rounded-lg border p-3 text-sm'>
              {t(
                'Pricing groups and auto assignment are managed in Visual mode through the structured group interface.'
              )}
            </div>
            <FormField
              control={form.control}
              name='TopupGroupRatio'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Top-up group ratios')}</FormLabel>
                  <FormControl>
                    <Textarea rows={6} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Optional multiplier per user group used when calculating recharge pricing. Provide a JSON object such as'
                    )}
                    {` { "default": 1, "vip": 1.2 }`}.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='GroupGroupRatio'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Inter-group overrides')}</FormLabel>
                  <FormControl>
                    <Textarea rows={8} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Nested JSON: source group →')}{' '}
                    {`{ targetGroup: ratio }`}{' '}
                    {t(
                      'to override billing when a user in one group uses a token of another group.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='GroupSpecialUsableGroup'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Special usable group rules')}</FormLabel>
                  <FormControl>
                    <Textarea rows={8} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Nested JSON defining per-group rules for adding (+:), removing (-:), or appending usable groups.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='DefaultUseAutoGroup'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Default to auto groups')}</FormLabel>
                    <FormDescription>
                      {t(
                        'When enabled, newly created tokens start in the first auto group.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </SettingsForm>
        )}
      </Form>
    </div>
  )
})

type GroupPricingGuideProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function GuideCodeBlock({ children }: { children: string }) {
  return (
    <pre className='bg-muted/60 overflow-x-auto rounded-lg border px-3 py-2 text-xs leading-6 whitespace-pre-wrap'>
      {children}
    </pre>
  )
}

function GroupPricingGuide({ open, onOpenChange }: GroupPricingGuideProps) {
  const { t } = useTranslation()

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className={sideDrawerContentClassName('sm:max-w-2xl')}
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Group pricing usage guide')}</SheetTitle>
          <SheetDescription>
            {t(
              'Understand how user groups, token groups, ratios, and special rules work together.'
            )}
          </SheetDescription>
        </SheetHeader>

        <div className={sideDrawerFormClassName('gap-5')}>
          <section className='space-y-2'>
            <h3 className='text-sm font-semibold'>{t('Core concepts')}</h3>
            <div className='text-muted-foreground space-y-2 text-sm leading-6'>
              <p>
                <span className='text-foreground font-medium'>
                  {t('User group')}
                </span>
                {': '}
                {t(
                  'Assigned by administrators and used to represent a user level, such as default or vip.'
                )}
              </p>
              <p>
                <span className='text-foreground font-medium'>
                  {t('Token group')}
                </span>
                {': '}
                {t(
                  'Selected when creating a token and used as the default billing group for API calls.'
                )}
              </p>
              <p>
                <span className='text-foreground font-medium'>
                  {t('Ratio')}
                </span>
                {': '}
                {t(
                  'A billing multiplier. Lower ratios mean lower API call costs.'
                )}
              </p>
              <p>
                <span className='text-foreground font-medium'>
                  {t('User selectable')}
                </span>
                {': '}
                {t(
                  'When enabled, users can pick this group when creating tokens.'
                )}
              </p>
            </div>
          </section>

          <Accordion className='rounded-lg border px-3'>
            <AccordionItem value='groups'>
              <AccordionTrigger>{t('Pricing group example')}</AccordionTrigger>
              <AccordionContent className='space-y-3'>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'Use the pricing group table to manage the ratio and whether the group appears in the token creation dropdown.'
                  )}
                </p>
                <GuideCodeBlock>
                  {`${t('Group name')}   ${t('Ratio')}   ${t('User selectable')}   ${t('Description')}
standard     1.0     ${t('Yes')}               ${t('Standard price')}
premium      0.5     ${t('Yes')}               ${t('Premium plan, half price')}
vip          0.5     ${t('No')}                ${t('Assigned by administrator only')}`}
                </GuideCodeBlock>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'Users only see groups marked as user selectable. Non-selectable groups can still be assigned by administrators.'
                  )}
                </p>
              </AccordionContent>
            </AccordionItem>

            <AccordionItem value='auto'>
              <AccordionTrigger>{t('Auto group behavior')}</AccordionTrigger>
              <AccordionContent className='space-y-3'>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'When a token uses the auto group, the system tries groups from top to bottom until it finds an available group.'
                  )}
                </p>
                <GuideCodeBlock>{`["default", "vip"]`}</GuideCodeBlock>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'If default auto group is enabled, newly created tokens start with auto instead of an empty group.'
                  )}
                </p>
              </AccordionContent>
            </AccordionItem>

            <AccordionItem value='special-ratio'>
              <AccordionTrigger>{t('Special ratio rules')}</AccordionTrigger>
              <AccordionContent className='space-y-3'>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'Special ratios override the token group ratio for specific user group and token group combinations.'
                  )}
                </p>
                <GuideCodeBlock>{`{
  "vip": {
    "standard": 0.8,
    "premium": 0.3
  }
}`}</GuideCodeBlock>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'Only configured combinations are overridden. All other calls keep the token group base ratio.'
                  )}
                </p>
              </AccordionContent>
            </AccordionItem>

            <AccordionItem value='usable'>
              <AccordionTrigger>
                {t('Special usable group rules')}
              </AccordionTrigger>
              <AccordionContent className='space-y-3'>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'Special usable group rules can add, remove, or append selectable token groups for a specific user group.'
                  )}
                </p>
                <GuideCodeBlock>{`{
  "vip": {
    "+:premium": "${t('Premium plan, half price')}",
    "-:default": "remove",
    "special": "${t('Special group')}"
  }
}`}</GuideCodeBlock>
                <p className='text-muted-foreground text-sm leading-6'>
                  {t(
                    'Use +: to add a group, -: to remove a default selectable group, or no prefix to append a group.'
                  )}
                </p>
              </AccordionContent>
            </AccordionItem>
          </Accordion>
        </div>
      </SheetContent>
    </Sheet>
  )
}
