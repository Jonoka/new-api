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
*/
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  CUSTOM_NAV_ICON_OPTIONS,
  getCustomNavIcon,
  type CustomNavSection,
  type CustomMenuItemConfig,
} from '@/lib/custom-nav'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  SettingsControlChildren,
  SettingsControlGroup,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'

type CustomMenuItemsEditorProps = {
  items: CustomMenuItemConfig[]
  onChange: (items: CustomMenuItemConfig[]) => void
  showSection?: boolean
  showRequireAuth?: boolean
}

const SECTION_OPTIONS: Array<{ value: CustomNavSection; labelKey: string }> = [
  { value: 'chat', labelKey: 'Chat area' },
  { value: 'console', labelKey: 'Console area' },
  { value: 'personal', labelKey: 'Personal area' },
  { value: 'admin', labelKey: 'Admin area' },
]

const createCustomItem = (showSection: boolean): CustomMenuItemConfig => {
  const id = `custom-${Date.now().toString(36)}`
  return {
    id,
    title: '',
    url: '',
    enabled: true,
    icon: 'ExternalLink',
    order: 0,
    requireAuth: false,
    openInNewTab: false,
    section: showSection ? 'chat' : undefined,
  }
}

export function CustomMenuItemsEditor({
  items,
  onChange,
  showSection = false,
  showRequireAuth = false,
}: CustomMenuItemsEditorProps) {
  const { t } = useTranslation()

  const setItem = (
    index: number,
    patch: Partial<CustomMenuItemConfig>
  ): void => {
    onChange(
      items.map((item, currentIndex) =>
        currentIndex === index ? { ...item, ...patch } : item
      )
    )
  }

  const addItem = (): void => {
    onChange([...items, createCustomItem(showSection)])
  }

  const removeItem = (index: number): void => {
    onChange(items.filter((_, currentIndex) => currentIndex !== index))
  }

  return (
    <SettingsControlGroup>
      <div className='flex items-center justify-between gap-3'>
        <SettingsSwitchContent>
          <div className='text-sm font-medium'>{t('Custom menu items')}</div>
          <p className='text-muted-foreground text-xs'>
            {t('Add managed links without changing frontend code.')}
          </p>
        </SettingsSwitchContent>
        <Button type='button' variant='outline' size='sm' onClick={addItem}>
          <Plus className='size-4' />
          {t('Add')}
        </Button>
      </div>

      {items.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('No custom menu items configured.')}
        </p>
      ) : (
        <SettingsControlChildren className='space-y-3'>
          {items.map((item, index) => {
            const Icon = getCustomNavIcon(item.icon)
            return (
              <div
                key={item.id || index}
                className='bg-background/80 space-y-3 rounded-lg border p-3'
              >
                <div className='flex items-center justify-between gap-3'>
                  <div className='flex min-w-0 items-center gap-2'>
                    {Icon ? <Icon className='text-muted-foreground size-4' /> : null}
                    <span className='truncate text-sm font-medium'>
                      {item.title || t('Untitled menu item')}
                    </span>
                  </div>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    onClick={() => removeItem(index)}
                    aria-label={t('Remove')}
                  >
                    <Trash2 className='size-4' />
                  </Button>
                </div>

                <div className='grid gap-3 md:grid-cols-2'>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('Title')}</span>
                    <Input
                      value={item.title}
                      onChange={(event) =>
                        setItem(index, { title: event.target.value })
                      }
                    />
                  </label>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('URL')}</span>
                    <Input
                      value={item.url}
                      placeholder='/canvas or https://example.com'
                      onChange={(event) =>
                        setItem(index, { url: event.target.value })
                      }
                    />
                  </label>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('Icon')}</span>
                    <NativeSelect
                      className='w-full'
                      value={item.icon ?? ''}
                      onChange={(event) =>
                        setItem(index, { icon: event.target.value || undefined })
                      }
                    >
                      <NativeSelectOption value=''>{t('No icon')}</NativeSelectOption>
                      {CUSTOM_NAV_ICON_OPTIONS.map((iconName) => (
                        <NativeSelectOption key={iconName} value={iconName}>
                          {iconName}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </label>
                  {showSection ? (
                    <label className='grid gap-1.5 text-sm'>
                      <span className='font-medium'>{t('Sidebar area')}</span>
                      <NativeSelect
                        className='w-full'
                        value={item.section ?? 'chat'}
                        onChange={(event) =>
                          setItem(index, { section: event.target.value })
                        }
                      >
                        {SECTION_OPTIONS.map((option) => (
                          <NativeSelectOption
                            key={option.value}
                            value={option.value}
                          >
                            {t(option.labelKey)}
                          </NativeSelectOption>
                        ))}
                      </NativeSelect>
                    </label>
                  ) : null}
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('Order')}</span>
                    <Input
                      type='number'
                      value={item.order}
                      onChange={(event) =>
                        setItem(index, { order: Number(event.target.value) || 0 })
                      }
                    />
                  </label>
                </div>

                <div className='grid gap-2 md:grid-cols-3'>
                  <SettingsSwitchItem className='border-b-0 py-1.5'>
                    <SettingsSwitchContent>
                      <div className='text-sm font-medium'>{t('Enabled')}</div>
                    </SettingsSwitchContent>
                    <Switch
                      checked={item.enabled}
                      onCheckedChange={(enabled) => setItem(index, { enabled })}
                    />
                  </SettingsSwitchItem>
                  <SettingsSwitchItem className='border-b-0 py-1.5'>
                    <SettingsSwitchContent>
                      <div className='text-sm font-medium'>
                        {t('Open in new tab')}
                      </div>
                    </SettingsSwitchContent>
                    <Switch
                      checked={Boolean(item.openInNewTab)}
                      onCheckedChange={(openInNewTab) =>
                        setItem(index, { openInNewTab })
                      }
                    />
                  </SettingsSwitchItem>
                  {showRequireAuth ? (
                    <SettingsSwitchItem className='border-b-0 py-1.5'>
                      <SettingsSwitchContent>
                        <div className='text-sm font-medium'>
                          {t('Require login')}
                        </div>
                      </SettingsSwitchContent>
                      <Switch
                        checked={Boolean(item.requireAuth)}
                        onCheckedChange={(requireAuth) =>
                          setItem(index, { requireAuth })
                        }
                      />
                    </SettingsSwitchItem>
                  ) : null}
                </div>
              </div>
            )
          })}
        </SettingsControlChildren>
      )}
    </SettingsControlGroup>
  )
}
