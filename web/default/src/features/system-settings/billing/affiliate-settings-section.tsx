import * as z from 'zod'
import { useCallback, useEffect, useState } from 'react'
import type { ChangeEvent } from 'react'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import {
  getAdminAffiliateWithdrawals,
  updateAdminAffiliateWithdrawal,
} from '@/features/affiliate/api'
import type { AffiliateWithdrawal } from '@/features/affiliate/types'
import { Badge } from '@/components/ui/badge'
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
import { Input } from '@/components/ui/input'
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const affiliateSchema = z.object({
  affiliate_setting: z.object({
    first_level_enabled: z.boolean(),
    first_level_ratio: z.coerce.number().min(0).max(100),
    second_level_enabled: z.boolean(),
    second_level_ratio: z.coerce.number().min(0).max(100),
    settlement_delay_seconds: z.coerce.number().min(0),
    min_withdrawal_amount: z.coerce.number().min(0),
    trigger_topup_enabled: z.boolean(),
    trigger_subscription_enabled: z.boolean(),
    usdt_chain: z.string().min(1),
    promotion_template: z.string().min(1),
  }),
})

type AffiliateFormValues = z.infer<typeof affiliateSchema>

type Props = {
  defaultValues: AffiliateFormValues
}

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline'

function withdrawalStatusVariant(status: string): BadgeVariant {
  if (status === 'paid') return 'default'
  if (status === 'rejected') return 'destructive'
  if (status === 'approved') return 'secondary'
  return 'outline'
}

function withdrawalMethodLabel(method: string) {
  if (method === 'usdt') return 'USDT'
  if (method === 'alipay') return 'Alipay'
  if (method === 'wechat') return 'WeChat'
  return method
}

export function AffiliateSettingsSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [withdrawals, setWithdrawals] = useState<AffiliateWithdrawal[]>([])
  const [withdrawalStatus, setWithdrawalStatus] = useState('')
  const [withdrawalsLoading, setWithdrawalsLoading] = useState(false)
  const [actionLoadingId, setActionLoadingId] = useState<number | null>(null)
  const handleNumberChange =
    (onChange: (value: number | string) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === '' ? '' : event.currentTarget.valueAsNumber
      )
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<AffiliateFormValues>({
      resolver: zodResolver(affiliateSchema) as Resolver<
        AffiliateFormValues,
        unknown,
        AffiliateFormValues
      >,
      defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key,
            value: value as string | number | boolean,
          })
        }
      },
    })

  const loadWithdrawals = useCallback(async () => {
    try {
      setWithdrawalsLoading(true)
      const res = await getAdminAffiliateWithdrawals(withdrawalStatus)
      if (res.success) setWithdrawals(res.data.items || [])
    } finally {
      setWithdrawalsLoading(false)
    }
  }, [withdrawalStatus])

  useEffect(() => {
    loadWithdrawals()
  }, [loadWithdrawals])

  const updateWithdrawal = async (
    id: number,
    action: 'approve' | 'reject' | 'paid'
  ) => {
    if (!window.confirm(t('Confirm withdrawal action?'))) return
    try {
      setActionLoadingId(id)
      const res = await updateAdminAffiliateWithdrawal(id, action)
      if (res.success) {
        toast.success(t('Withdrawal updated'))
        await loadWithdrawals()
      }
    } finally {
      setActionLoadingId(null)
    }
  }

  return (
    <SettingsSection
      title={t('Affiliate Commission')}
      description={t(
        'Configure paid-referral commission levels, settlement delay, payout threshold, and promotion copy.'
      )}
    >
      <FormNavigationGuard when={isDirty} />
      <Form {...form}>
        <form onSubmit={handleSubmit} className='space-y-6'>
          <FormDirtyIndicator isDirty={isDirty} />

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='affiliate_setting.first_level_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Enable level 1 commission')}</FormLabel>
                    <FormDescription>
                      {t('Reward the direct inviter after a paid order.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='affiliate_setting.second_level_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Enable level 2 commission')}</FormLabel>
                    <FormDescription>
                      {t('Reward the inviter above the direct inviter.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-4 md:grid-cols-2'>
            {[
              ['affiliate_setting.first_level_ratio', 'Level 1 ratio (%)'],
              ['affiliate_setting.second_level_ratio', 'Level 2 ratio (%)'],
              [
                'affiliate_setting.settlement_delay_seconds',
                'Settlement delay seconds',
              ],
              ['affiliate_setting.min_withdrawal_amount', 'Minimum withdrawal'],
            ].map(([name, label]) => (
              <FormField
                key={name}
                control={form.control}
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                name={name as any}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t(label)}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        value={field.value ?? ''}
                        onChange={handleNumberChange(field.onChange)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='affiliate_setting.trigger_topup_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel>{t('Top-up orders trigger commission')}</FormLabel>
                    <FormDescription>
                      {t('Generate commission after successful top-ups.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='affiliate_setting.trigger_subscription_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel>
                      {t('Subscription orders trigger commission')}
                    </FormLabel>
                    <FormDescription>
                      {t('Generate commission after subscription purchases.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='affiliate_setting.usdt_chain'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('USDT payout chain')}</FormLabel>
                <FormControl>
                  <Input placeholder='TRC20' {...field} />
                </FormControl>
                <FormDescription>
                  {t('Users see this chain when saving a USDT address.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='affiliate_setting.promotion_template'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Promotion copy template')}</FormLabel>
                <FormControl>
                  <Textarea className='min-h-28' {...field} />
                </FormControl>
                <FormDescription>
                  {t('Use {invite_link} where the referral link should appear.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type='submit' disabled={isSubmitting || updateOption.isPending}>
            {isSubmitting || updateOption.isPending
              ? t('Saving...')
              : t('Save affiliate settings')}
          </Button>
        </form>
      </Form>

      <div className='mt-8 space-y-3'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <h3 className='text-base font-semibold'>
              {t('Affiliate Withdrawals')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t('Review withdrawals and mark offline payouts as paid.')}
            </p>
          </div>
          <div className='flex gap-2'>
            <NativeSelect
              value={withdrawalStatus}
              onChange={(event) => setWithdrawalStatus(event.target.value)}
              className='w-36'
            >
              <NativeSelectOption value=''>{t('All statuses')}</NativeSelectOption>
              <NativeSelectOption value='pending'>{t('pending')}</NativeSelectOption>
              <NativeSelectOption value='approved'>
                {t('approved')}
              </NativeSelectOption>
              <NativeSelectOption value='paid'>{t('paid')}</NativeSelectOption>
              <NativeSelectOption value='rejected'>
                {t('rejected')}
              </NativeSelectOption>
            </NativeSelect>
            <Button variant='outline' onClick={loadWithdrawals}>
              {withdrawalsLoading ? t('Refreshing...') : t('Refresh')}
            </Button>
          </div>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t('User')}</TableHead>
              <TableHead>{t('Method')}</TableHead>
              <TableHead>{t('Amount')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Created At')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {withdrawals.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className='h-24 text-center'>
                  {withdrawalsLoading
                    ? t('Loading...')
                    : t('No withdrawal records')}
                </TableCell>
              </TableRow>
            ) : (
              withdrawals.map((withdrawal) => (
                <TableRow key={withdrawal.id}>
                  <TableCell>{withdrawal.id}</TableCell>
                  <TableCell>{withdrawal.user_id}</TableCell>
                  <TableCell>
                    {withdrawalMethodLabel(withdrawal.method)}
                  </TableCell>
                  <TableCell>{formatQuota(withdrawal.quota)}</TableCell>
                  <TableCell>
                    <Badge variant={withdrawalStatusVariant(withdrawal.status)}>
                      {t(withdrawal.status)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {formatTimestampToDate(withdrawal.created_at)}
                  </TableCell>
                  <TableCell>
                    <div className='flex justify-end gap-2'>
                      {withdrawal.status === 'pending' && (
                        <>
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={actionLoadingId === withdrawal.id}
                            onClick={() =>
                              updateWithdrawal(withdrawal.id, 'approve')
                            }
                          >
                            {t('Approve')}
                          </Button>
                          <Button
                            size='sm'
                            variant='destructive'
                            disabled={actionLoadingId === withdrawal.id}
                            onClick={() =>
                              updateWithdrawal(withdrawal.id, 'reject')
                            }
                          >
                            {t('Reject')}
                          </Button>
                        </>
                      )}
                      {(withdrawal.status === 'pending' ||
                        withdrawal.status === 'approved') && (
                        <Button
                          size='sm'
                          disabled={actionLoadingId === withdrawal.id}
                          onClick={() =>
                            updateWithdrawal(withdrawal.id, 'paid')
                          }
                        >
                          {t('Mark Paid')}
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </SettingsSection>
  )
}
