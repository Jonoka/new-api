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
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Plus, Trash2 } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getUpstreamChannels } from '../api'
import {
  SettingsForm,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { UpstreamChannel } from '../types'

const ACTION_MASK = 'mask'
const ACTION_BLOCK = 'block'
const DEFAULT_REPLACEMENT = '[REDACTED]'

type SensitiveRuleAction = typeof ACTION_MASK | typeof ACTION_BLOCK

type SensitiveRule = {
  id: string
  name: string
  enabled: boolean
  action: SensitiveRuleAction
  replacement?: string
  keywords: string[]
}

type SensitiveRuleDraft = Omit<SensitiveRule, 'keywords'> & {
  keywordsText: string
}

type SensitiveRulesConfig = {
  rules?: SensitiveRule[]
}

type SensitiveFormValues = {
  CheckSensitiveEnabled: boolean
  CheckSensitiveOnPromptEnabled: boolean
  SensitiveWords?: string
  SensitiveRules?: string
  SensitiveRuleChannelIds?: string
}

type SensitiveWordsSectionProps = {
  defaultValues: SensitiveFormValues
}

function splitKeywords(value: string) {
  const seen = new Set<string>()
  const keywords: string[] = []

  value
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase()
      if (seen.has(key)) return
      seen.add(key)
      keywords.push(item)
    })

  return keywords
}

function normalizeChannelIds(channelIds: number[]) {
  const seen = new Set<number>()
  const normalized: number[] = []

  channelIds.forEach((id) => {
    if (!Number.isInteger(id) || id <= 0 || seen.has(id)) return
    seen.add(id)
    normalized.push(id)
  })

  return normalized.sort((a, b) => a - b)
}

function parseChannelIds(raw?: string) {
  const trimmed = raw?.trim()
  if (!trimmed) return []

  try {
    const parsed = JSON.parse(trimmed)
    if (!Array.isArray(parsed)) return []
    return normalizeChannelIds(
      parsed
        .map((item) =>
          typeof item === 'number' ? item : Number.parseInt(String(item), 10)
        )
        .filter((item) => Number.isFinite(item))
    )
  } catch {
    return []
  }
}

function serializeChannelIds(channelIds: number[]) {
  return JSON.stringify(normalizeChannelIds(channelIds))
}

function createRule(): SensitiveRuleDraft {
  return {
    id: nanoid(),
    name: '',
    enabled: true,
    action: ACTION_MASK,
    replacement: DEFAULT_REPLACEMENT,
    keywordsText: '',
  }
}

function normalizeRule(rule: SensitiveRuleDraft): SensitiveRule | null {
  const keywords = splitKeywords(rule.keywordsText)
  if (keywords.length === 0) return null

  const action = rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK
  const fallbackName = keywords[0] ?? ''

  return {
    id: rule.id || fallbackName.toLowerCase() || nanoid(),
    name: rule.name.trim() || fallbackName,
    enabled: rule.enabled,
    action,
    replacement:
      action === ACTION_MASK
        ? rule.replacement?.trim() || DEFAULT_REPLACEMENT
        : undefined,
    keywords,
  }
}

function rulesToDrafts(rules: SensitiveRule[]): SensitiveRuleDraft[] {
  return rules.map((rule) => ({
    id: rule.id || nanoid(),
    name: rule.name ?? '',
    enabled: rule.enabled ?? true,
    action: rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK,
    replacement: rule.replacement || DEFAULT_REPLACEMENT,
    keywordsText: (rule.keywords ?? []).join('\n'),
  }))
}

function serializeRules(rules: SensitiveRuleDraft[]) {
  return JSON.stringify(
    {
      rules: rules
        .map((rule) => normalizeRule(rule))
        .filter((rule): rule is SensitiveRule => rule !== null),
    },
    null,
    2
  )
}

function parseRulesConfig(raw?: string, legacyWords?: string) {
  const trimmed = raw?.trim()
  if (trimmed) {
    try {
      const parsed = JSON.parse(trimmed) as SensitiveRulesConfig
      if (Array.isArray(parsed.rules) && parsed.rules.length > 0) {
        return rulesToDrafts(parsed.rules)
      }
    } catch {
      return []
    }
  }

  const legacyKeywords = splitKeywords(legacyWords ?? '')
  if (legacyKeywords.length === 0) return []

  return rulesToDrafts([
    {
      id: 'legacy-sensitive-words',
      name: 'Legacy sensitive words',
      enabled: true,
      action: ACTION_BLOCK,
      keywords: legacyKeywords,
    },
  ])
}

function getChannelLabel(channel: UpstreamChannel) {
  return channel.name?.trim() || `#${channel.id}`
}

export function SensitiveWordsSection({
  defaultValues,
}: SensitiveWordsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { data: channelsData } = useQuery({
    queryKey: ['upstream-channels'],
    queryFn: getUpstreamChannels,
  })
  const channels = useMemo(() => {
    return [...(channelsData?.data ?? [])].sort((a, b) => {
      const nameCompare = getChannelLabel(a).localeCompare(getChannelLabel(b))
      return nameCompare === 0 ? a.id - b.id : nameCompare
    })
  }, [channelsData?.data])
  const [filterEnabled, setFilterEnabled] = useState(
    defaultValues.CheckSensitiveEnabled
  )
  const [promptEnabled, setPromptEnabled] = useState(
    defaultValues.CheckSensitiveOnPromptEnabled
  )
  const [rules, setRules] = useState<SensitiveRuleDraft[]>(() =>
    parseRulesConfig(defaultValues.SensitiveRules, defaultValues.SensitiveWords)
  )
  const [selectedChannelIds, setSelectedChannelIds] = useState<number[]>(() =>
    parseChannelIds(defaultValues.SensitiveRuleChannelIds)
  )

  const initialRulesValue = useMemo(
    () =>
      serializeRules(
        parseRulesConfig(
          defaultValues.SensitiveRules,
          defaultValues.SensitiveWords
        )
      ),
    [defaultValues.SensitiveRules, defaultValues.SensitiveWords]
  )
  const currentRulesValue = useMemo(() => serializeRules(rules), [rules])
  const initialChannelIdsValue = useMemo(
    () =>
      serializeChannelIds(
        parseChannelIds(defaultValues.SensitiveRuleChannelIds)
      ),
    [defaultValues.SensitiveRuleChannelIds]
  )
  const currentChannelIdsValue = useMemo(
    () => serializeChannelIds(selectedChannelIds),
    [selectedChannelIds]
  )
  const selectedChannelIdSet = useMemo(
    () => new Set(selectedChannelIds),
    [selectedChannelIds]
  )
  const selectedChannels = useMemo(
    () => channels.filter((channel) => selectedChannelIdSet.has(channel.id)),
    [channels, selectedChannelIdSet]
  )
  const selectedChannelSummary =
    selectedChannelIds.length === 0
      ? t('No channels selected')
      : selectedChannelIds.length === 1
        ? getChannelLabel(
            selectedChannels[0] ?? {
              id: selectedChannelIds[0],
              name: '',
              base_url: '',
              status: 0,
            }
          )
        : t('{{count}} channels selected', { count: selectedChannelIds.length })
  const hasChanges =
    filterEnabled !== defaultValues.CheckSensitiveEnabled ||
    promptEnabled !== defaultValues.CheckSensitiveOnPromptEnabled ||
    currentRulesValue !== initialRulesValue ||
    currentChannelIdsValue !== initialChannelIdsValue

  useEffect(() => {
    setFilterEnabled(defaultValues.CheckSensitiveEnabled)
    setPromptEnabled(defaultValues.CheckSensitiveOnPromptEnabled)
    setRules(
      parseRulesConfig(
        defaultValues.SensitiveRules,
        defaultValues.SensitiveWords
      )
    )
    setSelectedChannelIds(
      parseChannelIds(defaultValues.SensitiveRuleChannelIds)
    )
  }, [defaultValues])

  const updateRule = (id: string, patch: Partial<SensitiveRuleDraft>) => {
    setRules((prev) =>
      prev.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    )
  }

  const onSubmit = async () => {
    const updates: Array<{ key: string; value: string | boolean }> = []
    if (filterEnabled !== defaultValues.CheckSensitiveEnabled) {
      updates.push({ key: 'CheckSensitiveEnabled', value: filterEnabled })
    }
    if (promptEnabled !== defaultValues.CheckSensitiveOnPromptEnabled) {
      updates.push({
        key: 'CheckSensitiveOnPromptEnabled',
        value: promptEnabled,
      })
    }
    if (currentRulesValue !== initialRulesValue) {
      updates.push({ key: 'SensitiveRules', value: currentRulesValue })
    }
    if (currentChannelIdsValue !== initialChannelIdsValue) {
      updates.push({
        key: 'SensitiveRuleChannelIds',
        value: currentChannelIdsValue,
      })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const onReset = () => {
    setFilterEnabled(defaultValues.CheckSensitiveEnabled)
    setPromptEnabled(defaultValues.CheckSensitiveOnPromptEnabled)
    setRules(
      parseRulesConfig(
        defaultValues.SensitiveRules,
        defaultValues.SensitiveWords
      )
    )
    setSelectedChannelIds(
      parseChannelIds(defaultValues.SensitiveRuleChannelIds)
    )
  }

  const toggleChannel = (channelId: number, checked: boolean) => {
    setSelectedChannelIds((prev) =>
      checked
        ? normalizeChannelIds([...prev, channelId])
        : prev.filter((id) => id !== channelId)
    )
  }

  return (
    <SettingsSection title={t('Sensitive Words')}>
      <SettingsForm
        onSubmit={(event) => {
          event.preventDefault()
          void onSubmit()
        }}
      >
        <SettingsPageFormActions
          onSave={() => void onSubmit()}
          onReset={onReset}
          isSaving={updateOption.isPending}
          isSaveDisabled={!hasChanges}
          isResetDisabled={!hasChanges}
          saveLabel='Save sensitive rules'
        />

        <div data-settings-form-span='full' className='space-y-4'>
          <SettingsSwitchField
            checked={filterEnabled}
            onCheckedChange={setFilterEnabled}
            label={t('Enable filtering')}
            description={t(
              'Inspect request text before it reaches upstream models.'
            )}
          />
          <SettingsSwitchField
            checked={promptEnabled}
            onCheckedChange={setPromptEnabled}
            label={t('Inspect user prompts')}
            description={t(
              'Rules are applied to prompt-like text fields in supported request formats.'
            )}
          />

          <div className='space-y-1.5'>
            <Label>{t('Applied channels')}</Label>
            <Popover>
              <PopoverTrigger
                render={
                  <Button
                    type='button'
                    variant='outline'
                    className='h-auto min-h-9 w-full justify-between gap-3 px-3 py-2 text-start'
                  >
                    <span className='min-w-0 truncate'>
                      {selectedChannelSummary}
                    </span>
                    <ChevronDown className='text-muted-foreground size-4 shrink-0' />
                  </Button>
                }
              />
              <PopoverContent
                align='start'
                className='w-[min(520px,calc(100vw-2rem))] gap-2 p-2'
              >
                <div className='flex items-center justify-between gap-2 px-1'>
                  <div className='min-w-0'>
                    <p className='text-sm font-medium'>
                      {t('Applied channels')}
                    </p>
                    <p className='text-muted-foreground text-xs'>
                      {t('Empty selection means rules apply to no channels.')}
                    </p>
                  </div>
                  {selectedChannelIds.length > 0 ? (
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => setSelectedChannelIds([])}
                    >
                      {t('Clear')}
                    </Button>
                  ) : null}
                </div>

                <div className='max-h-72 space-y-1 overflow-y-auto pr-1'>
                  {channels.length === 0 ? (
                    <div className='text-muted-foreground rounded-md border border-dashed p-3 text-sm'>
                      {t('No channels available.')}
                    </div>
                  ) : (
                    channels.map((channel) => {
                      const checked = selectedChannelIdSet.has(channel.id)
                      return (
                        <label
                          key={channel.id}
                          className='hover:bg-muted/60 flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 text-sm'
                        >
                          <Checkbox
                            checked={checked}
                            onCheckedChange={(value) =>
                              toggleChannel(channel.id, !!value)
                            }
                          />
                          <span className='min-w-0 flex-1'>
                            <span className='block truncate font-medium'>
                              {getChannelLabel(channel)}
                            </span>
                            <span className='text-muted-foreground block truncate text-xs'>
                              #{channel.id}
                              {channel.base_url ? ` · ${channel.base_url}` : ''}
                            </span>
                          </span>
                        </label>
                      )
                    })
                  )}
                </div>
              </PopoverContent>
            </Popover>
            <p className='text-muted-foreground text-xs'>
              {t('Sensitive rules only run on checked channels.')}
            </p>
          </div>
        </div>

        <div data-settings-form-span='full' className='space-y-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div className='min-w-0'>
              <h3 className='text-sm font-medium'>{t('Filter rules')}</h3>
              <p className='text-muted-foreground text-xs'>
                {t('Each rule can mask or block matching request text.')}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => setRules((prev) => [...prev, createRule()])}
            >
              <Plus data-icon='inline-start' />
              <span>{t('Add rule')}</span>
            </Button>
          </div>

          {rules.length === 0 ? (
            <div className='text-muted-foreground rounded-lg border border-dashed p-4 text-sm'>
              {t('No sensitive rules configured.')}
            </div>
          ) : (
            <div className='space-y-3'>
              {rules.map((rule, index) => (
                <div
                  key={rule.id}
                  className='bg-card text-card-foreground space-y-3 rounded-lg border p-3'
                >
                  <div className='flex flex-wrap items-center justify-between gap-3'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <Badge variant={rule.enabled ? 'default' : 'outline'}>
                        {rule.enabled ? t('Enabled') : t('Disabled')}
                      </Badge>
                      <span className='text-muted-foreground text-xs'>
                        {t('Rule {{number}}', { number: index + 1 })}
                      </span>
                    </div>
                    <div className='flex items-center gap-2'>
                      <Switch
                        checked={rule.enabled}
                        onCheckedChange={(enabled) =>
                          updateRule(rule.id, { enabled })
                        }
                      />
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Delete rule')}
                        onClick={() =>
                          setRules((prev) =>
                            prev.filter((item) => item.id !== rule.id)
                          )
                        }
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>

                  <div className='grid gap-3 md:grid-cols-[minmax(0,1fr)_150px]'>
                    <div className='space-y-1.5'>
                      <Label htmlFor={`${rule.id}-name`}>
                        {t('Rule name')}
                      </Label>
                      <Input
                        id={`${rule.id}-name`}
                        value={rule.name}
                        placeholder={t('Rule name')}
                        onChange={(event) =>
                          updateRule(rule.id, { name: event.target.value })
                        }
                      />
                    </div>
                    <div className='space-y-1.5'>
                      <Label>{t('Action')}</Label>
                      <Select
                        value={rule.action}
                        onValueChange={(value) => {
                          if (value !== ACTION_MASK && value !== ACTION_BLOCK) {
                            return
                          }
                          updateRule(rule.id, {
                            action: value,
                            replacement:
                              value === ACTION_MASK
                                ? rule.replacement || DEFAULT_REPLACEMENT
                                : rule.replacement,
                          })
                        }}
                      >
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            <SelectItem value={ACTION_MASK}>
                              {t('Mask')}
                            </SelectItem>
                            <SelectItem value={ACTION_BLOCK}>
                              {t('Block')}
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  {rule.action === ACTION_MASK ? (
                    <div className='space-y-1.5'>
                      <Label htmlFor={`${rule.id}-replacement`}>
                        {t('Replacement text')}
                      </Label>
                      <Input
                        id={`${rule.id}-replacement`}
                        value={rule.replacement ?? ''}
                        placeholder={DEFAULT_REPLACEMENT}
                        onChange={(event) =>
                          updateRule(rule.id, {
                            replacement: event.target.value,
                          })
                        }
                      />
                    </div>
                  ) : null}

                  <div className='space-y-1.5'>
                    <Label htmlFor={`${rule.id}-keywords`}>
                      {t('Keywords')}
                    </Label>
                    <Textarea
                      id={`${rule.id}-keywords`}
                      rows={5}
                      value={rule.keywordsText}
                      placeholder={t('Enter one keyword per line')}
                      onChange={(event) =>
                        updateRule(rule.id, {
                          keywordsText: event.target.value,
                        })
                      }
                    />
                    <p className='text-muted-foreground text-xs'>
                      {t('Empty lines and duplicate keywords are ignored.')}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </SettingsForm>
    </SettingsSection>
  )
}
