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
import { useEffect } from 'react'
import { z } from 'zod'
import { useForm, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { updateSystemOption } from '../api'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPage } from '../components/settings-page'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const gameSettingsDefaults = {
  'game_setting.token_exchange_rate': 100,
  'game_setting.award_fee_rate': 0.05,
  'game_setting.auto_judge_enabled': false,
  'game_setting.judge_poll_interval_seconds': 60,
  'game_setting.judge_provider': '',
  'game_setting.judge_model': '',
  'game_setting.judge_prompt': '',
}

const schema = z.object({
  tokenExchangeRate: z.coerce.number().int().min(1),
  awardFeeRate: z.coerce.number().min(0).max(1),
  autoJudgeEnabled: z.boolean(),
  judgePollIntervalSeconds: z.coerce.number().int().min(10),
  judgeProvider: z.string(),
  judgeModel: z.string(),
  judgePrompt: z.string(),
})

type Values = z.infer<typeof schema>

type GameSettingsDefaults = typeof gameSettingsDefaults

function GameSettingsForm({ settings }: { settings: GameSettingsDefaults }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const defaultValues: Values = {
    tokenExchangeRate: settings['game_setting.token_exchange_rate'],
    awardFeeRate: settings['game_setting.award_fee_rate'],
    autoJudgeEnabled: settings['game_setting.auto_judge_enabled'],
    judgePollIntervalSeconds:
      settings['game_setting.judge_poll_interval_seconds'],
    judgeProvider: settings['game_setting.judge_provider'],
    judgeModel: settings['game_setting.judge_model'],
    judgePrompt: settings['game_setting.judge_prompt'],
  }

  const form = useForm<Values>({
    resolver: zodResolver(schema) as unknown as Resolver<Values>,
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings])

  const { isDirty, isSubmitting } = form.formState
  const autoJudgeEnabled = form.watch('autoJudgeEnabled')

  async function onSubmit(values: Values) {
    const updates = [
      {
        key: 'game_setting.token_exchange_rate',
        value: String(values.tokenExchangeRate),
      },
      {
        key: 'game_setting.award_fee_rate',
        value: String(values.awardFeeRate),
      },
      {
        key: 'game_setting.auto_judge_enabled',
        value: String(values.autoJudgeEnabled),
      },
      {
        key: 'game_setting.judge_poll_interval_seconds',
        value: String(values.judgePollIntervalSeconds),
      },
      { key: 'game_setting.judge_provider', value: values.judgeProvider },
      { key: 'game_setting.judge_model', value: values.judgeModel },
      { key: 'game_setting.judge_prompt', value: values.judgePrompt },
    ]

    for (const update of updates) {
      const res = await updateSystemOption(update)
      if (!res.success) {
        throw new Error(res.message || t('Failed to update setting'))
      }
    }
    await queryClient.invalidateQueries({ queryKey: ['system-options'] })
    toast.success(t('Game settings saved successfully'))
    form.reset(values)
  }

  return (
    <SettingsSection title={t('Game Center Settings')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={isSubmitting}
            isSaveDisabled={!isDirty}
            saveLabel='Save game settings'
          />
          <div className='grid gap-6 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='tokenExchangeRate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Token exchange rate')}</FormLabel>
                  <FormControl>
                    <Input type='number' min={1} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Game Tokens received for each quota unit spent')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='awardFeeRate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Award fee rate')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      step='0.01'
                      min={0}
                      max={1}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Fee rate charged only on winning profit')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='autoJudgeEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable automatic judging')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Scan due prediction rounds and hand them to a judge provider'
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

          {autoJudgeEnabled && (
            <div className='grid gap-6 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='judgePollIntervalSeconds'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Judge poll interval seconds')}</FormLabel>
                    <FormControl>
                      <Input type='number' min={10} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='judgeProvider'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Judge provider')}</FormLabel>
                    <FormControl>
                      <Input placeholder='llm' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='judgeModel'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Judge model')}</FormLabel>
                    <FormControl>
                      <Input placeholder='gpt-4.1' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='judgePrompt'
                render={({ field }) => (
                  <FormItem className='sm:col-span-2'>
                    <FormLabel>{t('Judge prompt')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t(
                          'Return JSON with answer_index and reason'
                        )}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

export function GameSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/games/'
      defaultSettings={gameSettingsDefaults}
      defaultSection='general'
      getSectionMeta={() => ({ titleKey: 'Game Center Settings' })}
      getSectionContent={(_sectionId, settings) => (
        <GameSettingsForm settings={settings} />
      )}
    />
  )
}
