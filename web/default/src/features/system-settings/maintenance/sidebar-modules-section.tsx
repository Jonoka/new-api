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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { CUSTOM_NAV_ICON_OPTIONS, getCustomNavIcon } from '@/lib/custom-nav'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  SettingsControlChildren,
  SettingsForm,
  SettingsSwitchContent,
  SettingsControlGroup,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  SIDEBAR_MODULES_DEFAULT,
  type SidebarModulesAdminConfig,
  type SidebarSectionConfig,
  serializeSidebarModulesAdmin,
} from './config'
import { CustomMenuItemsEditor } from './custom-menu-items-editor'

type SidebarModulesSectionProps = {
  config: SidebarModulesAdminConfig
  initialSerialized: string
}

type SidebarFormValues = SidebarModulesAdminConfig

const toTitleCase = (value: string) =>
  value.replace(/[_-]+/g, ' ').replace(/\b\w/g, (char) => char.toUpperCase())

const isSidebarSectionConfig = (
  value: SidebarModulesAdminConfig[string]
): value is SidebarSectionConfig =>
  Boolean(value && typeof value === 'object' && !Array.isArray(value))

export function SidebarModulesSection({
  config,
  initialSerialized,
}: SidebarModulesSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [customItems, setCustomItems] = useState(config.customItems ?? [])

  const sectionMeta: Record<string, { title: string; description: string }> = {
    chat: {
      title: t('Chat area'),
      description: t('Playground experiments and live conversations.'),
    },
    console: {
      title: t('Console area'),
      description: t('Dashboards, tokens, and usage analytics.'),
    },
    personal: {
      title: t('Personal area'),
      description: t('Wallet management and personal preferences.'),
    },
    admin: {
      title: t('Admin area'),
      description: t('Global configuration and administrative tools.'),
    },
  }

  const moduleMeta: Record<
    string,
    Record<string, { title: string; description: string }>
  > = {
    chat: {
      playground: {
        title: t('Playground'),
        description: t('Experiment with prompts and models in real time.'),
      },
      canvas: {
        title: t('Infinite Canvas'),
        description: t(
          'Use the current login session to open Infinite Canvas.'
        ),
      },
      chat: {
        title: t('Chat'),
        description: t('Access previous conversations and start new ones.'),
      },
    },
    console: {
      detail: {
        title: t('Dashboard'),
        description: t('Aggregated usage metrics and trend charts.'),
      },
      token: {
        title: t('Token management'),
        description: t('Create, revoke, and audit API tokens.'),
      },
      log: {
        title: t('Usage logs'),
        description: t('Detailed request logs for investigations.'),
      },
      midjourney: {
        title: t('Drawing logs'),
        description: t('History of Midjourney-style image tasks.'),
      },
      task: {
        title: t('Task logs'),
        description: t('Background job tracker for queued work.'),
      },
      game: {
        title: t('Game Center'),
        description: t('Game wallet, prediction rounds, and participation.'),
      },
    },
    personal: {
      topup: {
        title: t('Wallet'),
        description: t('Top up balance and view billing history.'),
      },
      invoice: {
        title: t('Invoice Center'),
        description: t('View invoices for paid orders.'),
      },
      affiliate: {
        title: t('Affiliate Commission'),
        description: t('Referral commission and payout center.'),
      },
      personal: {
        title: t('Profile'),
        description: t('Personal settings and profile management.'),
      },
    },
    admin: {
      channel: {
        title: t('Channels'),
        description: t('Configure upstream providers and routing.'),
      },
      channel_observability: {
        title: t('Channel Observability'),
        description: t(
          'Analyze channel traffic, stability, failures, and cache efficiency.'
        ),
      },
      models: {
        title: t('Models'),
        description: t('Manage catalog visibility and pricing.'),
      },
      redemption: {
        title: t('Redeem codes'),
        description: t('Create and review invite or credit codes.'),
      },
      user: {
        title: t('Users'),
        description: t('Administer user accounts and roles.'),
      },
      affiliate_admin: {
        title: t('Affiliate Commission'),
        description: t('Configure paid-referral commission and payouts'),
      },
      setting: {
        title: t('System settings'),
        description: t('Advanced platform configuration.'),
      },
      subscription: {
        title: t('Subscription Management'),
        description: t('Manage subscription plans and pricing.'),
      },
      invoice_admin: {
        title: t('Invoice Management'),
        description: t('Review invoice requests and issued invoice files.'),
      },
      notification_center: {
        title: t('Notification Center'),
        description: t('Configure notification tasks and Telegram Bots.'),
      },
      extension_admin: {
        title: t('Extensions'),
        description: t('Manage extension modules and enable or disable them.'),
      },
      game: {
        title: t('Game Management'),
        description: t('Create prediction rounds and settle player rewards.'),
      },
    },
  }
  const formDefaults = useMemo(() => config, [config])

  const form = useForm<SidebarFormValues>({
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(formDefaults)
    setCustomItems(config.customItems ?? [])
  }, [config.customItems, formDefaults, form])

  const onSubmit = async (values: SidebarFormValues) => {
    const serialized = serializeSidebarModulesAdmin({
      ...values,
      customItems,
    })
    if (serialized === initialSerialized) {
      return
    }

    await updateOption.mutateAsync({
      key: 'SidebarModulesAdmin',
      value: serialized,
    })
  }

  const resetToDefault = () => {
    form.reset(SIDEBAR_MODULES_DEFAULT)
    setCustomItems([])
  }

  const sections = Object.entries(config).filter(
    ([sectionKey, sectionConfig]) =>
      sectionKey === 'customItems'
        ? false
        : isSidebarSectionConfig(sectionConfig)
  )

  return (
    <SettingsSection title={t('Sidebar modules')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={resetToDefault}
            isSaving={updateOption.isPending}
            resetLabel='Reset to default'
            saveLabel='Save sidebar modules'
          />
          {sections.map(([sectionKey, sectionConfig]) => {
            const sectionInfo = sectionMeta[sectionKey] ?? {
              title: toTitleCase(sectionKey),
              description: t('Custom sidebar section'),
            }
            if (!isSidebarSectionConfig(sectionConfig)) return null
            const modules = Object.entries(sectionConfig).filter(
              ([moduleKey, moduleValue]) =>
                moduleKey !== 'enabled' && typeof moduleValue === 'boolean'
            )
            const selectedCanvasIcon =
              sectionKey === 'chat' &&
              typeof sectionConfig.canvasIcon === 'string'
                ? sectionConfig.canvasIcon
                : undefined
            const CanvasIcon = getCustomNavIcon(selectedCanvasIcon)

            return (
              <SettingsControlGroup key={sectionKey}>
                <FormField
                  control={form.control}
                  // eslint-disable-next-line @typescript-eslint/no-explicit-any
                  name={`${sectionKey}.enabled` as any}
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{sectionInfo.title}</FormLabel>
                        <FormDescription>
                          {sectionInfo.description}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={Boolean(field.value)}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />

                <SettingsControlChildren className='grid gap-3 md:grid-cols-2'>
                  {modules.map(([moduleKey]) => {
                    const moduleInfo = moduleMeta[sectionKey]?.[moduleKey] ?? {
                      title: toTitleCase(moduleKey),
                      description: t('Custom module'),
                    }
                    return (
                      <FormField
                        key={`${sectionKey}.${moduleKey}`}
                        control={form.control}
                        // eslint-disable-next-line @typescript-eslint/no-explicit-any
                        name={`${sectionKey}.${moduleKey}` as any}
                        render={({ field }) => (
                          <SettingsSwitchItem className='border-b-0 py-2'>
                            <SettingsSwitchContent>
                              <FormLabel>{moduleInfo.title}</FormLabel>
                              <FormDescription>
                                {moduleInfo.description}
                              </FormDescription>
                            </SettingsSwitchContent>
                            <FormControl>
                              <Switch
                                checked={Boolean(field.value)}
                                onCheckedChange={field.onChange}
                                disabled={
                                  // eslint-disable-next-line @typescript-eslint/no-explicit-any
                                  !form.watch(`${sectionKey}.enabled` as any)
                                }
                              />
                            </FormControl>
                          </SettingsSwitchItem>
                        )}
                      />
                    )
                  })}
                </SettingsControlChildren>

                {sectionKey === 'chat' ? (
                  <SettingsControlChildren className='grid gap-3 md:grid-cols-2'>
                    <FormField
                      control={form.control}
                      // eslint-disable-next-line @typescript-eslint/no-explicit-any
                      name={'chat.canvasOrigin' as any}
                      render={({ field }) => (
                        <label className='grid gap-1.5 text-sm'>
                          <span className='font-medium'>
                            {t('Canvas app domain')}
                          </span>
                          <Input
                            value={String(field.value ?? '')}
                            placeholder='https://canvas.example.com'
                            onChange={field.onChange}
                            disabled={
                              // eslint-disable-next-line @typescript-eslint/no-explicit-any
                              !form.watch('chat.enabled' as any) ||
                              // eslint-disable-next-line @typescript-eslint/no-explicit-any
                              !form.watch('chat.canvas' as any)
                            }
                          />
                          <span className='text-muted-foreground text-xs'>
                            {t(
                              'Enter a domain or full origin, for example canvas.example.com.'
                            )}
                          </span>
                        </label>
                      )}
                    />

                    <FormField
                      control={form.control}
                      // eslint-disable-next-line @typescript-eslint/no-explicit-any
                      name={'chat.canvasIcon' as any}
                      render={({ field }) => {
                        const Icon = getCustomNavIcon(field.value)
                        return (
                          <label className='grid gap-1.5 text-sm'>
                            <span className='font-medium'>
                              {t('Canvas icon')}
                            </span>
                            <div className='flex items-center gap-2'>
                              <div className='bg-muted flex size-9 shrink-0 items-center justify-center rounded-md'>
                                {Icon ? (
                                  <Icon
                                    className='text-muted-foreground size-4'
                                    aria-hidden='true'
                                  />
                                ) : CanvasIcon ? (
                                  <CanvasIcon
                                    className='text-muted-foreground size-4'
                                    aria-hidden='true'
                                  />
                                ) : null}
                              </div>
                              <NativeSelect
                                className='w-full'
                                value={String(field.value ?? '')}
                                onChange={field.onChange}
                                disabled={
                                  // eslint-disable-next-line @typescript-eslint/no-explicit-any
                                  !form.watch('chat.enabled' as any) ||
                                  // eslint-disable-next-line @typescript-eslint/no-explicit-any
                                  !form.watch('chat.canvas' as any)
                                }
                              >
                                {CUSTOM_NAV_ICON_OPTIONS.map((iconName) => (
                                  <NativeSelectOption
                                    key={iconName}
                                    value={iconName}
                                  >
                                    {iconName}
                                  </NativeSelectOption>
                                ))}
                              </NativeSelect>
                            </div>
                            <span className='text-muted-foreground text-xs'>
                              {t(
                                'Select the sidebar and launcher icon for Infinite Canvas.'
                              )}
                            </span>
                          </label>
                        )
                      }}
                    />
                  </SettingsControlChildren>
                ) : null}
              </SettingsControlGroup>
            )
          })}

          <CustomMenuItemsEditor
            items={customItems}
            onChange={setCustomItems}
            showSection
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
