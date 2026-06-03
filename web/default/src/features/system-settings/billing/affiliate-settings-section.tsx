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
import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ChangeEvent } from 'react'
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Search } from 'lucide-react'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  bindAdminAffiliateInviter,
  getAdminAffiliateInvitations,
  getAdminAffiliateRecords,
  getAdminAffiliateWithdrawals,
  updateAdminAffiliateWithdrawal,
} from '@/features/affiliate/api'
import type {
  AdminBindAffiliateInviterResult,
  AffiliateAdminInvitation,
  AffiliateAdminRecord,
  AffiliateWithdrawal,
} from '@/features/affiliate/types'
import { searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
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
    filter_redemption_topup_enabled: z.boolean(),
    payout_methods: z.string().min(1),
    usdt_chain: z.string().min(1),
    promotion_template: z.string().min(1),
  }),
})

type AffiliateFormValues = z.infer<typeof affiliateSchema>

type Props = {
  defaultValues: AffiliateFormValues
}

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline'

const SETTLEMENT_DELAY_KEY = 'affiliate_setting.settlement_delay_seconds'
const PAYOUT_METHODS_KEY = 'affiliate_setting.payout_methods'
const PAYOUT_METHOD_OPTIONS = [
  ['usdt', 'USDT'],
  ['alipay', 'Alipay'],
  ['wechat', 'WeChat'],
] as const

function secondsToMinutes(value: number) {
  return Math.max(0, Math.round((Number(value) || 0) / 60))
}

function minutesToSeconds(value: number | string | boolean) {
  return Math.max(0, Math.round(Number(value) || 0)) * 60
}

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

function sourceTypeLabel(t: (key: string) => string, sourceType: string) {
  if (sourceType === 'topup') return t('Wallet top-up')
  if (sourceType === 'subscription') return t('Subscription purchase')
  if (sourceType === 'redemption') return t('Redemption code')
  return sourceType || '-'
}

function adminUserLine(
  userId: number,
  username?: string,
  displayName?: string,
  email?: string
) {
  const name = displayName || username || '-'
  return (
    <div className='min-w-0'>
      <div className='truncate font-medium'>
        #{userId} {name}
      </div>
      <div className='text-muted-foreground truncate text-xs'>
        {email || username || '-'}
      </div>
    </div>
  )
}

function adminRecordDetailLine(record: AffiliateAdminRecord) {
  const detail = record.detail
  const title = detail?.title || `${record.source_type} #${record.source_id}`
  const paidAmount =
    detail?.paid_amount && detail.paid_amount > 0
      ? `$${detail.paid_amount.toFixed(2)}`
      : ''
  return (
    <div className='min-w-0'>
      <div className='truncate font-medium'>{title}</div>
      <div className='text-muted-foreground truncate text-xs'>
        {[record.source_id, paidAmount, detail?.payment_method]
          .filter(Boolean)
          .join(' · ') || '-'}
      </div>
    </div>
  )
}

export function AffiliateSettingsSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [invitations, setInvitations] = useState<AffiliateAdminInvitation[]>([])
  const [invitationKeyword, setInvitationKeyword] = useState('')
  const [invitationSearch, setInvitationSearch] = useState('')
  const [invitationsLoading, setInvitationsLoading] = useState(false)
  const [records, setRecords] = useState<AffiliateAdminRecord[]>([])
  const [recordSourceType, setRecordSourceType] = useState('topup')
  const [recordStatus, setRecordStatus] = useState('')
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [withdrawals, setWithdrawals] = useState<AffiliateWithdrawal[]>([])
  const [withdrawalStatus, setWithdrawalStatus] = useState('')
  const [withdrawalsLoading, setWithdrawalsLoading] = useState(false)
  const [actionLoadingId, setActionLoadingId] = useState<number | null>(null)
  const [bindUserKeyword, setBindUserKeyword] = useState('')
  const [bindUserCandidates, setBindUserCandidates] = useState<User[]>([])
  const [selectedBindUser, setSelectedBindUser] = useState<User | null>(null)
  const [bindUserSearching, setBindUserSearching] = useState(false)
  const [bindAffCode, setBindAffCode] = useState('')
  const [bindForce, setBindForce] = useState(false)
  const [bindLoading, setBindLoading] = useState(false)
  const [bindResult, setBindResult] =
    useState<AdminBindAffiliateInviterResult | null>(null)
  const displayDefaultValues = useMemo<AffiliateFormValues>(
    () => ({
      affiliate_setting: {
        ...defaultValues.affiliate_setting,
        settlement_delay_seconds: secondsToMinutes(
          defaultValues.affiliate_setting.settlement_delay_seconds
        ),
      },
    }),
    [defaultValues]
  )
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
      defaultValues: displayDefaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key: key === SETTLEMENT_DELAY_KEY ? SETTLEMENT_DELAY_KEY : key,
            value:
              key === SETTLEMENT_DELAY_KEY
                ? minutesToSeconds(value as string | number | boolean)
                : (value as string | number | boolean),
          })
        }
      },
    })

  const selectedPayoutMethods = (
    form.watch('affiliate_setting.payout_methods') || ''
  )
    .split(',')
    .map((method) => method.trim())
    .filter(Boolean)

  const togglePayoutMethod = (method: string, checked: boolean) => {
    if (!checked && selectedPayoutMethods.length <= 1) {
      toast.error(t('At least one payout method must remain enabled'))
      return
    }
    const next = checked
      ? [...selectedPayoutMethods, method]
      : selectedPayoutMethods.filter((item) => item !== method)
    const unique = PAYOUT_METHOD_OPTIONS.map(([value]) => value).filter(
      (value) => next.includes(value)
    )
    form.setValue(PAYOUT_METHODS_KEY, unique.join(','), {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const loadInvitations = useCallback(async () => {
    try {
      setInvitationsLoading(true)
      const res = await getAdminAffiliateInvitations(invitationSearch)
      if (res.success) setInvitations(res.data.items || [])
    } finally {
      setInvitationsLoading(false)
    }
  }, [invitationSearch])

  const loadRecords = useCallback(async () => {
    try {
      setRecordsLoading(true)
      const res = await getAdminAffiliateRecords(recordSourceType, recordStatus)
      if (res.success) setRecords(res.data.items || [])
    } finally {
      setRecordsLoading(false)
    }
  }, [recordSourceType, recordStatus])

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
    loadInvitations()
  }, [loadInvitations])

  useEffect(() => {
    loadRecords()
  }, [loadRecords])

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

  const searchBindUsers = async () => {
    const keyword = bindUserKeyword.trim()
    if (!keyword) {
      toast.error(t('Enter a user keyword first'))
      return
    }
    try {
      setBindUserSearching(true)
      setSelectedBindUser(null)
      const res = await searchUsers({ keyword, p: 1, page_size: 10 })
      if (res.success) {
        setBindUserCandidates(res.data?.items || [])
      }
    } finally {
      setBindUserSearching(false)
    }
  }

  const bindInviter = async () => {
    const affCode = bindAffCode.trim()
    if (!selectedBindUser || !affCode) {
      toast.error(t('Search and select target user first'))
      return
    }
    try {
      setBindLoading(true)
      const res = await bindAdminAffiliateInviter({
        user_id: selectedBindUser.id,
        aff_code: affCode,
        force: bindForce,
      })
      if (res.success) {
        setBindResult(res.data)
        toast.success(t('Referral binding saved'))
      }
    } finally {
      setBindLoading(false)
    }
  }

  return (
    <SettingsSection title={t('Affiliate Commission')}>
      <Tabs defaultValue='rules' className='space-y-6'>
        <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
          <TabsTrigger value='rules'>{t('Commission rules')}</TabsTrigger>
          <TabsTrigger value='manual-bind'>{t('Manual referral')}</TabsTrigger>
          <TabsTrigger value='invitations'>{t('User Invitations')}</TabsTrigger>
          <TabsTrigger value='records'>{t('Commission Records')}</TabsTrigger>
          <TabsTrigger value='withdrawals'>{t('Withdrawals')}</TabsTrigger>
        </TabsList>

        <TabsContent value='rules'>
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
                    'Settlement delay minutes',
                  ],
                  [
                    'affiliate_setting.min_withdrawal_amount',
                    'Minimum withdrawal',
                  ],
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
                        <FormLabel>
                          {t('Top-up orders trigger commission')}
                        </FormLabel>
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
                          {t(
                            'Generate commission after subscription purchases.'
                          )}
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
                name='affiliate_setting.filter_redemption_topup_enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel>
                        {t('Filter redemption-code top-ups')}
                      </FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, quota added by redemption codes does not generate commission.'
                        )}
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
                name='affiliate_setting.payout_methods'
                render={() => (
                  <FormItem>
                    <FormLabel>{t('Supported payout methods')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Only enabled methods are shown on the affiliate page.'
                      )}
                    </FormDescription>
                    <div className='grid gap-3 md:grid-cols-3'>
                      {PAYOUT_METHOD_OPTIONS.map(([method, label]) => (
                        <label
                          key={method}
                          className='flex items-center justify-between rounded-lg border p-4 text-sm'
                        >
                          <span>{t(label)}</span>
                          <Switch
                            checked={selectedPayoutMethods.includes(method)}
                            onCheckedChange={(checked) =>
                              togglePayoutMethod(method, checked)
                            }
                          />
                        </label>
                      ))}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

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
                      {t(
                        'Use {invite_link} where the referral link should appear.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <Button
                type='submit'
                disabled={isSubmitting || updateOption.isPending}
              >
                {isSubmitting || updateOption.isPending
                  ? t('Saving...')
                  : t('Save affiliate settings')}
              </Button>
            </form>
          </Form>
        </TabsContent>

        <TabsContent value='manual-bind' className='space-y-4'>
          <div className='space-y-1'>
            <h3 className='text-base font-semibold'>
              {t('Manual referral binding')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t('Bind a user to a missed ?aff=xxxx referral code.')}
            </p>
          </div>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='space-y-2'>
              <FormLabel>{t('Target user')}</FormLabel>
              <div className='flex gap-2'>
                <Input
                  value={bindUserKeyword}
                  onChange={(event) => {
                    setBindUserKeyword(event.target.value)
                    setSelectedBindUser(null)
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.preventDefault()
                      searchBindUsers()
                    }
                  }}
                  placeholder={t('User ID, username, email, or display name')}
                />
                <Button
                  type='button'
                  variant='outline'
                  onClick={searchBindUsers}
                  disabled={bindUserSearching}
                >
                  <Search className='size-4' />
                  {bindUserSearching ? t('Searching...') : t('Search users')}
                </Button>
              </div>
            </div>
            <div className='space-y-2'>
              <FormLabel>{t('Affiliate code')}</FormLabel>
              <Input
                value={bindAffCode}
                onChange={(event) => setBindAffCode(event.target.value)}
                placeholder={t('Code or URL containing ?aff=')}
              />
            </div>
          </div>
          <div className='rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Display name')}</TableHead>
                  <TableHead>{t('Email')}</TableHead>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {bindUserCandidates.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className='h-20 text-center'>
                      {t('No matched users')}
                    </TableCell>
                  </TableRow>
                ) : (
                  bindUserCandidates.map((user) => {
                    const selected = selectedBindUser?.id === user.id
                    return (
                      <TableRow key={user.id}>
                        <TableCell>#{user.id}</TableCell>
                        <TableCell>{user.username || '-'}</TableCell>
                        <TableCell>{user.display_name || '-'}</TableCell>
                        <TableCell>{user.email || '-'}</TableCell>
                        <TableCell>{user.group || '-'}</TableCell>
                        <TableCell className='text-right'>
                          <Button
                            type='button'
                            size='sm'
                            variant={selected ? 'default' : 'outline'}
                            onClick={() => setSelectedBindUser(user)}
                          >
                            {selected ? t('Selected') : t('Select')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
          {selectedBindUser && (
            <div className='bg-muted/40 rounded-lg border p-3 text-sm'>
              <span className='font-medium'>{t('Selected target user')}:</span>{' '}
              #{selectedBindUser.id} {selectedBindUser.display_name || selectedBindUser.username}
              {selectedBindUser.email ? ` (${selectedBindUser.email})` : ''}
            </div>
          )}
          <label className='flex items-start gap-3 rounded-lg border p-4 text-sm'>
            <Checkbox
              checked={bindForce}
              onCheckedChange={(checked) => setBindForce(Boolean(checked))}
            />
            <span className='space-y-1'>
              <span className='block font-medium'>
                {t('Force overwrite existing inviter')}
              </span>
              <span className='text-muted-foreground block'>
                {t(
                  'By default existing inviters are not overwritten; force mode also adjusts old and new invite counts.'
                )}
              </span>
            </span>
          </label>
          <Button onClick={bindInviter} disabled={bindLoading}>
            {bindLoading ? t('Binding...') : t('Bind inviter')}
          </Button>
          {bindResult && (
            <div className='bg-muted/40 space-y-2 rounded-lg border p-4 text-sm'>
              <div className='font-medium'>{t('Binding result')}</div>
              <div>
                {t('Target user')}: #{bindResult.user_id}{' '}
                {bindResult.display_name || bindResult.username || ''}
              </div>
              <div>
                {t('Inviter')}: #{bindResult.inviter_id}{' '}
                {bindResult.inviter_username} ({bindResult.inviter_aff_code})
              </div>
              <div className='text-muted-foreground'>
                {t('Previous inviter')}:{' '}
                {bindResult.previous_inviter_id || t('No Inviter')}
              </div>
            </div>
          )}
        </TabsContent>

        <TabsContent value='invitations' className='space-y-3'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('User Invitations')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Review inviter relationships and downstream top-ups.')}
              </p>
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Input
                value={invitationKeyword}
                onChange={(event) => setInvitationKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    setInvitationSearch(invitationKeyword.trim())
                  }
                }}
                placeholder={t('Search inviter or invitee')}
                className='sm:w-64'
              />
              <Button
                variant='outline'
                onClick={() => setInvitationSearch(invitationKeyword.trim())}
                disabled={invitationsLoading}
              >
                <Search className='size-4' />
                {invitationsLoading ? t('Searching...') : t('Search')}
              </Button>
              <Button variant='outline' onClick={loadInvitations}>
                {invitationsLoading ? t('Refreshing...') : t('Refresh')}
              </Button>
            </div>
          </div>

          <div className='overflow-x-auto rounded-lg border'>
            <Table className='min-w-[920px]'>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Inviter')}</TableHead>
                  <TableHead>{t('Invitee')}</TableHead>
                  <TableHead>{t('Affiliate code')}</TableHead>
                  <TableHead>{t('Top-up count')}</TableHead>
                  <TableHead>{t('Top-up quota')}</TableHead>
                  <TableHead>{t('Commission')}</TableHead>
                  <TableHead>{t('Invited At')}</TableHead>
                  <TableHead>{t('Last top-up')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {invitations.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={8} className='h-24 text-center'>
                      {invitationsLoading
                        ? t('Loading...')
                        : t('No invitation records')}
                    </TableCell>
                  </TableRow>
                ) : (
                  invitations.map((item) => (
                    <TableRow key={`${item.inviter_id}-${item.invitee_id}`}>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          item.inviter_id,
                          item.inviter_username,
                          item.inviter_name,
                          item.inviter_email
                        )}
                      </TableCell>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          item.invitee_id,
                          item.invitee_username,
                          item.invitee_name,
                          item.invitee_email
                        )}
                      </TableCell>
                      <TableCell className='font-mono text-xs'>
                        {item.inviter_aff_code || '-'}
                      </TableCell>
                      <TableCell>{item.topup_count}</TableCell>
                      <TableCell>{formatQuota(item.topup_quota)}</TableCell>
                      <TableCell>
                        {formatQuota(item.commission_quota)}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(item.invitee_created_at)}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(item.last_topup_time)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </TabsContent>

        <TabsContent value='records' className='space-y-3'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('Commission Records')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Audit downstream orders that generated commission.')}
              </p>
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <NativeSelect
                value={recordSourceType}
                onChange={(event) => setRecordSourceType(event.target.value)}
                className='sm:w-44'
              >
                <NativeSelectOption value=''>
                  {t('All sources')}
                </NativeSelectOption>
                <NativeSelectOption value='topup'>
                  {t('Wallet top-up')}
                </NativeSelectOption>
                <NativeSelectOption value='subscription'>
                  {t('Subscription purchase')}
                </NativeSelectOption>
                <NativeSelectOption value='redemption'>
                  {t('Redemption code')}
                </NativeSelectOption>
              </NativeSelect>
              <NativeSelect
                value={recordStatus}
                onChange={(event) => setRecordStatus(event.target.value)}
                className='sm:w-36'
              >
                <NativeSelectOption value=''>
                  {t('All statuses')}
                </NativeSelectOption>
                <NativeSelectOption value='pending'>
                  {t('pending')}
                </NativeSelectOption>
                <NativeSelectOption value='available'>
                  {t('available')}
                </NativeSelectOption>
              </NativeSelect>
              <Button variant='outline' onClick={loadRecords}>
                {recordsLoading ? t('Refreshing...') : t('Refresh')}
              </Button>
            </div>
          </div>

          <div className='overflow-x-auto rounded-lg border'>
            <Table className='min-w-[1040px]'>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Inviter')}</TableHead>
                  <TableHead>{t('Invitee')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                  <TableHead>{t('Order details')}</TableHead>
                  <TableHead>{t('Level')}</TableHead>
                  <TableHead>{t('Ratio')}</TableHead>
                  <TableHead>{t('Commission')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Created At')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={10} className='h-24 text-center'>
                      {recordsLoading
                        ? t('Loading...')
                        : t('No commission records')}
                    </TableCell>
                  </TableRow>
                ) : (
                  records.map((record) => (
                    <TableRow key={record.id}>
                      <TableCell>{record.id}</TableCell>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          record.inviter.id,
                          record.inviter.username,
                          record.inviter.display_name,
                          record.inviter.email
                        )}
                      </TableCell>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          record.invitee.id,
                          record.invitee.username,
                          record.invitee.display_name,
                          record.invitee.email
                        )}
                      </TableCell>
                      <TableCell>
                        {sourceTypeLabel(t, record.source_type)}
                      </TableCell>
                      <TableCell className='max-w-[240px]'>
                        {adminRecordDetailLine(record)}
                      </TableCell>
                      <TableCell>{record.level}</TableCell>
                      <TableCell>{record.ratio}%</TableCell>
                      <TableCell>{formatQuota(record.reward_quota)}</TableCell>
                      <TableCell>
                        <Badge variant={withdrawalStatusVariant(record.status)}>
                          {t(record.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(record.created_at)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </TabsContent>

        <TabsContent value='withdrawals' className='space-y-3'>
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
                <NativeSelectOption value=''>
                  {t('All statuses')}
                </NativeSelectOption>
                <NativeSelectOption value='pending'>
                  {t('pending')}
                </NativeSelectOption>
                <NativeSelectOption value='approved'>
                  {t('approved')}
                </NativeSelectOption>
                <NativeSelectOption value='paid'>
                  {t('paid')}
                </NativeSelectOption>
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
                      <Badge
                        variant={withdrawalStatusVariant(withdrawal.status)}
                      >
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
        </TabsContent>
      </Tabs>
    </SettingsSection>
  )
}
