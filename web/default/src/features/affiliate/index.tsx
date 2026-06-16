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
import {
  Banknote,
  ChevronDown,
  Link2,
  RefreshCw,
  Send,
  Trash2,
  Upload,
  WalletCards,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  formatQuota,
  formatCurrencyUSD,
  formatTimestampToDate,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
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
import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import {
  createAffiliateWithdrawal,
  deleteAffiliateQr,
  getAffiliateLeaderboard,
  getAffiliatePayoutAccount,
  getAffiliateRecords,
  getAffiliateSummary,
  getAffiliateWithdrawals,
  transferAffiliateToBalance,
  updateAffiliatePayoutAccount,
  uploadAffiliateQr,
  getAffiliateApplicationStatus,
  getAffiliateAgreement,
  applyAffiliate,
} from './api'
import type {
  AffiliatePayoutAccount,
  AffiliateLeaderboardItem,
  AffiliateRecord,
  AffiliateSummary,
  AffiliateWithdrawal,
} from './types'

const EMPTY_ACCOUNT: AffiliatePayoutAccount = {
  user_id: 0,
  usdt_address: '',
  usdt_chain: 'TRC20',
  alipay_account: '',
  alipay_name: '',
  alipay_qr_path: '',
  wechat_account: '',
  wechat_name: '',
  wechat_qr_path: '',
}

const DEFAULT_PAYOUT_METHODS = ['usdt', 'alipay', 'wechat']

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline'

function statusVariant(status: string): BadgeVariant {
  if (status === 'paid' || status === 'available') return 'default'
  if (status === 'rejected') return 'destructive'
  if (status === 'approved') return 'secondary'
  return 'outline'
}

function methodLabel(method: string) {
  if (method === 'usdt') return 'USDT'
  if (method === 'alipay') return 'Alipay'
  if (method === 'wechat') return 'WeChat'
  return method
}

function sourceFallbackText(record: AffiliateRecord) {
  return `${record.source_type} #${record.source_id}`
}

function sourceDetailTitle(
  t: (key: string) => string,
  record: AffiliateRecord
) {
  const detail = record.detail
  if (record.source_type === 'topup') return t('Wallet top-up')
  if (record.source_type === 'subscription') {
    return detail?.plan_title
      ? `${t('Subscription purchase')}: ${detail.plan_title}`
      : t('Subscription purchase')
  }
  if (record.source_type === 'redemption') {
    return detail?.redemption_name
      ? `${t('Redemption code')}: ${detail.redemption_name}`
      : t('Redemption code')
  }
  if (detail?.title) return detail.title
  return sourceFallbackText(record)
}

function SourceDetailCell({
  record,
  t,
}: {
  record: AffiliateRecord
  t: (key: string) => string
}) {
  const detail = record.detail
  const parts: string[] = []
  if (record.source_quota > 0) {
    parts.push(`${t('Commission base')} ${formatQuota(record.source_quota)}`)
  }
  if (detail?.original_amount && detail.original_amount > 0) {
    parts.push(
      `${t('Original price')} ${formatCurrencyUSD(detail.original_amount)}`
    )
  }
  if (detail?.discount_amount && detail.discount_amount > 0) {
    const promo = detail.promo_code ? `${detail.promo_code} ` : ''
    parts.push(
      `${t('Discount applied')} ${promo}-${formatCurrencyUSD(detail.discount_amount)}`
    )
  }
  if ((detail?.paid_amount ?? 0) > 0 || (detail?.discount_amount ?? 0) > 0) {
    parts.push(
      `${t('Paid amount')} ${formatCurrencyUSD(detail?.paid_amount ?? 0)}`
    )
  }
  if (
    record.source_type === 'redemption' &&
    ((detail?.quota ?? 0) > 0 || record.source_quota > 0)
  ) {
    parts.push(
      `${t('Redeemed quota')} ${formatQuota(detail?.quota ?? record.source_quota)}`
    )
  }
  if (detail?.payment_provider || detail?.payment_method) {
    parts.push(
      `${t('Payment method')} ${[detail.payment_provider, detail.payment_method]
        .filter(Boolean)
        .join('/')}`
    )
  }

  return (
    <div className='min-w-0 space-y-1'>
      <div className='font-medium'>{sourceDetailTitle(t, record)}</div>
      <div className='text-muted-foreground text-xs leading-5'>
        {parts.length > 0 ? parts.join(' · ') : sourceFallbackText(record)}
      </div>
    </div>
  )
}

export function Affiliate() {
  const { t } = useTranslation()
  const [summary, setSummary] = useState<AffiliateSummary | null>(null)
  const [account, setAccount] = useState<AffiliatePayoutAccount>(EMPTY_ACCOUNT)
  const [records, setRecords] = useState<AffiliateRecord[]>([])
  const [withdrawals, setWithdrawals] = useState<AffiliateWithdrawal[]>([])
  const [leaderboard, setLeaderboard] = useState<AffiliateLeaderboardItem[]>([])
  const [leaderboardPeriod, setLeaderboardPeriod] = useState('month')
  const [leaderboardSort, setLeaderboardSort] = useState('commission')
  const [loading, setLoading] = useState(true)
  const [savingAccount, setSavingAccount] = useState(false)
  const [uploadingMethod, setUploadingMethod] = useState<string | null>(null)
  const [withdrawOpen, setWithdrawOpen] = useState(false)
  const [withdrawMethod, setWithdrawMethod] = useState('alipay')
  const [accountOpen, setAccountOpen] = useState(false)
  const [withdrawAmount, setWithdrawAmount] = useState('')
  const [submittingWithdraw, setSubmittingWithdraw] = useState(false)
  const [transferring, setTransferring] = useState(false)

  // Anti-fraud: Application state
  const [appStatus, setAppStatus] = useState<string>('loading')
  const [appRejectedReason, setAppRejectedReason] = useState('')
  const [agreementText, setAgreementText] = useState('')
  const [agreementEnabled, setAgreementEnabled] = useState(false)
  const [confirmInput, setConfirmInput] = useState('')
  const [applying, setApplying] = useState(false)
  const [eligibility, setEligibility] = useState<{eligible: boolean; reason?: string} | null>(null)

  useEffect(() => {
    async function checkAppStatus() {
      try {
        const [statusRes, agreementRes] = await Promise.all([
          getAffiliateApplicationStatus(),
          getAffiliateAgreement(),
        ])
        if (statusRes.success) {
          if (!statusRes.data.review_enabled) {
            setAppStatus('not_required')
          } else {
            setAppStatus(statusRes.data.status || 'none')
            if (statusRes.data.rejected_reason) setAppRejectedReason(statusRes.data.rejected_reason)
            if (statusRes.data.eligibility) setEligibility(statusRes.data.eligibility)
          }
        }
        if (agreementRes.success) {
          setAgreementEnabled(agreementRes.data.agreement_enabled)
          setAgreementText(agreementRes.data.agreement_text || '')
        }
      } catch {
        setAppStatus('not_required')
      }
    }
    checkAppStatus()
  }, [])

  const handleApply = async () => {
    const confirmText = t('agreement_confirm_text')
    if (agreementEnabled && confirmInput !== confirmText) {
      toast.error(t('Please type the exact confirmation text'))
      return
    }
    try {
      setApplying(true)
      const res = await applyAffiliate(true)
      if (res.success) {
        toast.success(t('Application submitted successfully'))
        setAppStatus('pending')
      } else {
        toast.error(res.message || t('Application failed'))
      }
    } catch {
      toast.error(t('Application failed'))
    } finally {
      setApplying(false)
    }
  }

  const needsApplication = appStatus !== 'not_required' && appStatus !== 'approved' && appStatus !== 'loading'

  const refresh = useCallback(async () => {
    try {
      setLoading(true)
      const [
        summaryRes,
        accountRes,
        recordsRes,
        withdrawalsRes,
        leaderboardRes,
      ] = await Promise.all([
        getAffiliateSummary(),
        getAffiliatePayoutAccount(),
        getAffiliateRecords(),
        getAffiliateWithdrawals(),
        getAffiliateLeaderboard(leaderboardPeriod, 20, leaderboardSort),
      ])

      if (summaryRes.success) setSummary(summaryRes.data)
      if (accountRes.success) setAccount(accountRes.data)
      if (recordsRes.success) setRecords(recordsRes.data.items || [])
      if (withdrawalsRes.success)
        setWithdrawals(withdrawalsRes.data.items || [])
      if (leaderboardRes.success) setLeaderboard(leaderboardRes.data || [])
    } finally {
      setLoading(false)
    }
  }, [leaderboardPeriod, leaderboardSort])

  useEffect(() => {
    refresh()
  }, [refresh])

  const availableDisplayAmount = useMemo(() => {
    return quotaUnitsToDollars(summary?.balance.available_quota ?? 0)
  }, [summary?.balance.available_quota])

  const payoutMethods = useMemo(() => {
    const methods = summary?.setting.payout_methods || []
    return methods.length > 0 ? methods : DEFAULT_PAYOUT_METHODS
  }, [summary?.setting.payout_methods])

  useEffect(() => {
    if (!payoutMethods.includes(withdrawMethod)) {
      setWithdrawMethod(payoutMethods[0] || 'usdt')
    }
  }, [payoutMethods, withdrawMethod])

  const isPayoutMethodEnabled = (method: string) =>
    payoutMethods.includes(method)

  const handleAccountChange = (
    key: keyof AffiliatePayoutAccount,
    value: string
  ) => {
    setAccount((current) => ({ ...current, [key]: value }))
  }

  const saveAccount = async () => {
    try {
      setSavingAccount(true)
      const payload = {
        ...account,
        usdt_chain: summary?.setting.usdt_chain || account.usdt_chain,
      }
      const res = await updateAffiliatePayoutAccount(payload)
      if (res.success) {
        setAccount(res.data)
        toast.success(t('Payout account saved'))
      }
    } finally {
      setSavingAccount(false)
    }
  }

  const handleQrUpload = async (method: 'alipay' | 'wechat', file?: File) => {
    if (!file) return
    try {
      setUploadingMethod(method)
      const res = await uploadAffiliateQr(method, file)
      if (res.success) {
        const pathKey =
          method === 'alipay' ? 'alipay_qr_path' : 'wechat_qr_path'
        setAccount(
          (current) =>
            res.data.account || { ...current, [pathKey]: res.data.path }
        )
        toast.success(t('QR code uploaded'))
      }
    } finally {
      setUploadingMethod(null)
    }
  }

  const handleQrDelete = async (method: 'alipay' | 'wechat') => {
    try {
      const res = await deleteAffiliateQr(method)
      if (res.success) {
        setAccount(res.data)
        toast.success(t('QR code deleted'))
      }
    } catch {
      toast.error(t('Delete failed'))
    }
  }

  const submitWithdraw = async () => {
    const amount = Number(withdrawAmount)
    const quota = parseQuotaFromDollars(amount)
    if (!Number.isFinite(amount) || amount <= 0 || quota <= 0) {
      toast.error(t('Enter a valid withdrawal amount'))
      return
    }
    try {
      setSubmittingWithdraw(true)
      const res = await createAffiliateWithdrawal(withdrawMethod, quota)
      if (res.success) {
        toast.success(t('Withdrawal submitted'))
        setWithdrawOpen(false)
        setWithdrawAmount('')
        await refresh()
      }
    } finally {
      setSubmittingWithdraw(false)
    }
  }

  const transferAllToBalance = async () => {
    const quota = summary?.balance.available_quota ?? 0
    if (quota <= 0) return
    try {
      setTransferring(true)
      const res = await transferAffiliateToBalance(quota)
      if (res.success) {
        toast.success(t('Transferred to balance'))
        await refresh()
      }
    } finally {
      setTransferring(false)
    }
  }

  const balance = summary?.balance

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Affiliate Commission')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          {needsApplication ? (
            <div className='mx-auto flex w-full max-w-2xl flex-col gap-4 sm:gap-5'>
              <Card className='py-0'>
                <CardHeader>
                  <CardTitle className='text-base'>
                    {appStatus === 'pending'
                      ? t('Application Pending Review')
                      : appStatus === 'rejected'
                        ? t('Application Rejected')
                        : t('Apply for Affiliate Program')}
                  </CardTitle>
                </CardHeader>
                <CardContent className='space-y-4 p-4 pt-0'>
                  {appStatus === 'pending' && (
                    <p className='text-muted-foreground text-sm'>
                      {t('Your affiliate application is being reviewed. You will be notified once it is approved.')}
                    </p>
                  )}
                  {appStatus === 'rejected' && (
                    <div className='space-y-2'>
                      <p className='text-destructive text-sm'>
                        {t('Your application was rejected.')}
                        {appRejectedReason && ` ${t('Reason')}: ${appRejectedReason}`}
                      </p>
                      <p className='text-muted-foreground text-sm'>
                        {t('You can submit a new application below.')}
                      </p>
                    </div>
                  )}
                  {(appStatus === 'none' || appStatus === 'rejected') && (
                    <>
                      {eligibility && !eligibility.eligible && (
                        <div className='bg-destructive/10 text-destructive rounded-md p-3 text-sm'>
                          {t('You do not meet the eligibility requirements yet.')}
                          {eligibility.reason && ` ${t(eligibility.reason)}`}
                        </div>
                      )}
                      {agreementEnabled && agreementText && (
                        <div className='space-y-3'>
                          <div className='bg-muted rounded-md p-4 text-sm whitespace-pre-wrap'>
                            {agreementText}
                          </div>
                          <div className='space-y-2'>
                            <Label>{t('Type the confirmation text to agree')}</Label>
                            <Input
                              value={confirmInput}
                              onChange={(e) => setConfirmInput(e.target.value)}
                              placeholder={t('agreement_confirm_text')}
                            />
                          </div>
                        </div>
                      )}
                      <Button
                        onClick={handleApply}
                        disabled={
                          applying ||
                          (eligibility !== null && !eligibility.eligible) ||
                          (agreementEnabled && confirmInput !== t('agreement_confirm_text'))
                        }
                      >
                        {applying ? t('Submitting...') : t('Submit Application')}
                      </Button>
                    </>
                  )}
                </CardContent>
              </Card>
            </div>
          ) : (
          <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
            <Card className='py-0'>
              <CardHeader className='pb-2'>
                <div className='flex items-center justify-between gap-3'>
                  <CardTitle className='flex items-center gap-2 text-base'>
                    <WalletCards className='size-4' />
                    {t('Overview')}
                  </CardTitle>
                  <Button
                    variant='ghost'
                    size='icon'
                    onClick={refresh}
                    disabled={loading}
                    aria-label={t('Refresh')}
                  >
                    <RefreshCw className='size-4' />
                  </Button>
                </div>
              </CardHeader>
              <CardContent className='grid gap-4 p-4 pt-0 xl:grid-cols-[minmax(0,1fr)_280px] xl:items-end'>
                <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
                  {[
                    [t('Available'), balance?.available_quota ?? 0],
                    [t('Pending Settlement'), balance?.pending_quota ?? 0],
                    [t('Frozen'), balance?.frozen_quota ?? 0],
                    [t('Total Commission'), balance?.total_quota ?? 0],
                  ].map(([label, value], index) => (
                    <div key={String(label)} className='min-w-0'>
                      <div className='text-muted-foreground text-xs font-medium'>
                        {label}
                      </div>
                      <div
                        className={`mt-1 font-semibold tabular-nums ${
                          index === 0 ? 'text-2xl' : 'text-lg'
                        }`}
                      >
                        {formatQuota(Number(value))}
                      </div>
                    </div>
                  ))}
                </div>
                <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-1'>
                  <Button
                    onClick={() => {
                      if (!payoutMethods.length) {
                        toast.error(t('No payout methods are available'))
                        return
                      }
                      setWithdrawAmount(
                        availableDisplayAmount > 0
                          ? String(Number(availableDisplayAmount.toFixed(4)))
                          : ''
                      )
                      setWithdrawOpen(true)
                    }}
                    disabled={!balance?.available_quota || loading}
                  >
                    <Banknote className='size-4' />
                    {t('Withdraw')}
                  </Button>
                  <Button
                    variant='outline'
                    onClick={transferAllToBalance}
                    disabled={!balance?.available_quota || transferring}
                  >
                    <Send className='size-4' />
                    {transferring
                      ? t('Transferring...')
                      : t('Transfer to Balance')}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card className='py-0'>
              <CardHeader className='pb-2'>
                <CardTitle className='flex items-center gap-2 text-base'>
                  <Link2 className='size-4' />
                  {t('Promotion Copy')}
                </CardTitle>
              </CardHeader>
              <CardContent className='space-y-3 p-4 pt-0'>
                <div className='grid gap-2 sm:grid-cols-[1fr_auto]'>
                  <Input
                    readOnly
                    value={summary?.invite_link ?? ''}
                    className='font-mono text-xs'
                  />
                  <CopyButton
                    value={summary?.invite_link ?? ''}
                    tooltip={t('Copy referral link')}
                    aria-label={t('Copy referral link')}
                  />
                </div>
                <div className='grid gap-2 sm:grid-cols-[1fr_auto] sm:items-start'>
                  <Textarea
                    readOnly
                    value={summary?.promotion_text ?? ''}
                    className='min-h-20 text-sm'
                  />
                  <CopyButton
                    value={summary?.promotion_text ?? ''}
                    tooltip={t('Copy promotion copy')}
                    aria-label={t('Copy promotion copy')}
                  />
                </div>
                <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
                  <span>
                    {t('Invites')}: {summary?.aff_count ?? 0}
                  </span>
                  <span>
                    {t('Level 1')}: {summary?.setting.first_level_ratio ?? 0}%
                    {summary?.setting.first_level_enabled
                      ? ''
                      : ` ${t('Disabled')}`}
                  </span>
                  <span>
                    {t('Level 2')}: {summary?.setting.second_level_ratio ?? 0}%
                    {summary?.setting.second_level_enabled
                      ? ''
                      : ` ${t('Disabled')}`}
                  </span>
                </div>
              </CardContent>
            </Card>

            <Collapsible open={accountOpen} onOpenChange={setAccountOpen}>
              <Card className='py-0'>
                <CollapsibleTrigger className='hover:bg-muted/40 flex w-full items-center justify-between px-4 py-3 text-left'>
                  <div>
                    <CardTitle className='text-base'>
                      {t('Payout Account')}
                    </CardTitle>
                    <div className='text-muted-foreground mt-1 flex flex-wrap gap-2 text-xs'>
                      {payoutMethods.map((method) => (
                        <Badge key={method} variant='outline'>
                          {methodLabel(method)}
                        </Badge>
                      ))}
                    </div>
                  </div>
                  <ChevronDown
                    className={`size-4 transition-transform ${
                      accountOpen ? 'rotate-180' : ''
                    }`}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <CardContent className='space-y-4 p-4 pt-0'>
                    <div className='grid gap-4 lg:grid-cols-3'>
                      {isPayoutMethodEnabled('usdt') && (
                        <div className='space-y-2'>
                          <Label>{t('USDT Address')}</Label>
                          <Input
                            value={account.usdt_address || ''}
                            onChange={(event) =>
                              handleAccountChange(
                                'usdt_address',
                                event.target.value
                              )
                            }
                            placeholder={t('Enter USDT address')}
                          />
                          <p className='text-muted-foreground text-xs'>
                            {t('USDT withdrawals use the configured chain')}
                            :&nbsp;
                            <span className='font-medium'>
                              {summary?.setting.usdt_chain ||
                                account.usdt_chain}
                            </span>
                          </p>
                        </div>
                      )}
                      {isPayoutMethodEnabled('alipay') && (
                        <div className='space-y-2'>
                          <Label>{t('Alipay Account')}</Label>
                          <Input
                            value={account.alipay_account || ''}
                            onChange={(event) =>
                              handleAccountChange(
                                'alipay_account',
                                event.target.value
                              )
                            }
                            placeholder={t('Account or phone number')}
                          />
                          <Input
                            value={account.alipay_name || ''}
                            onChange={(event) =>
                              handleAccountChange(
                                'alipay_name',
                                event.target.value
                              )
                            }
                            placeholder={t('Recipient name')}
                          />
                          <Label className='border-input flex h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border text-sm'>
                            <Upload className='size-4' />
                            {uploadingMethod === 'alipay'
                              ? t('Uploading...')
                              : t('Upload QR code')}
                            <input
                              type='file'
                              accept='image/*'
                              className='sr-only'
                              onChange={(event) => {
                                void handleQrUpload(
                                  'alipay',
                                  event.target.files?.[0]
                                )
                                event.currentTarget.value = ''
                              }}
                            />
                          </Label>
                          {account.alipay_qr_path && (
                            <div className='flex items-center gap-2'>
                              <img
                                src={account.alipay_qr_path}
                                alt={t('Alipay QR code')}
                                className='border-border size-12 rounded-md border object-cover'
                              />
                              <Button
                                type='button'
                                variant='outline'
                                size='sm'
                                onClick={() => handleQrDelete('alipay')}
                              >
                                <Trash2 className='size-4' />
                                {t('Delete QR code')}
                              </Button>
                            </div>
                          )}
                        </div>
                      )}
                      {isPayoutMethodEnabled('wechat') && (
                        <div className='space-y-2'>
                          <Label>{t('WeChat Account')}</Label>
                          <Input
                            value={account.wechat_account || ''}
                            onChange={(event) =>
                              handleAccountChange(
                                'wechat_account',
                                event.target.value
                              )
                            }
                            placeholder={t('Account or phone number')}
                          />
                          <Input
                            value={account.wechat_name || ''}
                            onChange={(event) =>
                              handleAccountChange(
                                'wechat_name',
                                event.target.value
                              )
                            }
                            placeholder={t('Recipient name')}
                          />
                          <Label className='border-input flex h-9 cursor-pointer items-center justify-center gap-2 rounded-lg border text-sm'>
                            <Upload className='size-4' />
                            {uploadingMethod === 'wechat'
                              ? t('Uploading...')
                              : t('Upload QR code')}
                            <input
                              type='file'
                              accept='image/*'
                              className='sr-only'
                              onChange={(event) => {
                                void handleQrUpload(
                                  'wechat',
                                  event.target.files?.[0]
                                )
                                event.currentTarget.value = ''
                              }}
                            />
                          </Label>
                          {account.wechat_qr_path && (
                            <div className='flex items-center gap-2'>
                              <img
                                src={account.wechat_qr_path}
                                alt={t('WeChat QR code')}
                                className='border-border size-12 rounded-md border object-cover'
                              />
                              <Button
                                type='button'
                                variant='outline'
                                size='sm'
                                onClick={() => handleQrDelete('wechat')}
                              >
                                <Trash2 className='size-4' />
                                {t('Delete QR code')}
                              </Button>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                    <Button onClick={saveAccount} disabled={savingAccount}>
                      {savingAccount
                        ? t('Saving...')
                        : t('Save payout account')}
                    </Button>
                  </CardContent>
                </CollapsibleContent>
              </Card>
            </Collapsible>

            <Card className='py-0'>
              <CardHeader className='pb-2'>
                <CardTitle className='text-base'>
                  {t('Affiliate Activity')}
                </CardTitle>
              </CardHeader>
              <CardContent className='p-4 pt-0'>
                <Tabs defaultValue='leaderboard' className='gap-4'>
                  <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
                    <TabsTrigger value='leaderboard'>
                      {t('Leaderboard')}
                    </TabsTrigger>
                    <TabsTrigger value='records'>{t('Records')}</TabsTrigger>
                    <TabsTrigger value='withdrawals'>
                      {t('Withdrawals')}
                    </TabsTrigger>
                  </TabsList>

                  <TabsContent value='leaderboard' className='space-y-3'>
                    <div className='grid gap-2 sm:w-fit sm:grid-cols-2'>
                      <NativeSelect
                        value={leaderboardPeriod}
                        onChange={(event) =>
                          setLeaderboardPeriod(event.target.value)
                        }
                        className='w-full sm:w-40'
                      >
                        <NativeSelectOption value='day'>
                          {t('Today')}
                        </NativeSelectOption>
                        <NativeSelectOption value='week'>
                          {t('This week')}
                        </NativeSelectOption>
                        <NativeSelectOption value='month'>
                          {t('This month')}
                        </NativeSelectOption>
                      </NativeSelect>
                      <NativeSelect
                        value={leaderboardSort}
                        onChange={(event) =>
                          setLeaderboardSort(event.target.value)
                        }
                        className='w-full sm:w-44'
                      >
                        <NativeSelectOption value='commission'>
                          {t('Sort by commission')}
                        </NativeSelectOption>
                        <NativeSelectOption value='invites'>
                          {t('Sort by invites')}
                        </NativeSelectOption>
                      </NativeSelect>
                    </div>
                    <Table className='min-w-[520px]'>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Rank')}</TableHead>
                          <TableHead>{t('User')}</TableHead>
                          <TableHead>{t('Invites')}</TableHead>
                          <TableHead>{t('Commission')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {leaderboard.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={4} className='h-24 text-center'>
                              {t('No leaderboard data')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          leaderboard.map((item) => (
                            <TableRow key={item.user_id}>
                              <TableCell className='font-medium'>
                                #{item.rank}
                              </TableCell>
                              <TableCell>
                                {item.masked_name || `User #${item.user_id}`}
                              </TableCell>
                              <TableCell>{item.invite_count}</TableCell>
                              <TableCell>
                                {formatQuota(item.commission_quota)}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TabsContent>

                  <TabsContent value='records'>
                    <Table className='min-w-[680px]'>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Level')}</TableHead>
                          <TableHead>{t('Purchase details')}</TableHead>
                          <TableHead>{t('Commission')}</TableHead>
                          <TableHead>{t('Status')}</TableHead>
                          <TableHead>{t('Available Time')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {records.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={5} className='h-24 text-center'>
                              {t('No commission records')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          records.map((record) => (
                            <TableRow key={record.id}>
                              <TableCell>{record.level}</TableCell>
                              <TableCell>
                                <SourceDetailCell record={record} t={t} />
                              </TableCell>
                              <TableCell>
                                {formatQuota(record.reward_quota)}
                              </TableCell>
                              <TableCell>
                                <Badge variant={statusVariant(record.status)}>
                                  {t(record.status)}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                {formatTimestampToDate(record.available_time)}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TabsContent>

                  <TabsContent value='withdrawals'>
                    <Table className='min-w-[560px]'>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t('Method')}</TableHead>
                          <TableHead>{t('Amount')}</TableHead>
                          <TableHead>{t('Status')}</TableHead>
                          <TableHead>{t('Created At')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {withdrawals.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={4} className='h-24 text-center'>
                              {t('No withdrawal records')}
                            </TableCell>
                          </TableRow>
                        ) : (
                          withdrawals.map((withdrawal) => (
                            <TableRow key={withdrawal.id}>
                              <TableCell>
                                {methodLabel(withdrawal.method)}
                              </TableCell>
                              <TableCell>
                                {formatQuota(withdrawal.quota)}
                              </TableCell>
                              <TableCell>
                                <Badge
                                  variant={statusVariant(withdrawal.status)}
                                >
                                  {t(withdrawal.status)}
                                </Badge>
                              </TableCell>
                              <TableCell>
                                {formatTimestampToDate(withdrawal.created_at)}
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TabsContent>
                </Tabs>
              </CardContent>
            </Card>
          </div>
          )}
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog open={withdrawOpen} onOpenChange={setWithdrawOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Withdraw commission')}</DialogTitle>
            <DialogDescription>
              {t('Submitted withdrawals are frozen until admin review')}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='space-y-2'>
              <Label>{t('Payout method')}</Label>
              <NativeSelect
                value={withdrawMethod}
                onChange={(event) => setWithdrawMethod(event.target.value)}
                className='w-full'
              >
                {payoutMethods.map((method) => (
                  <NativeSelectOption key={method} value={method}>
                    {methodLabel(method)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className='space-y-2'>
              <Label>{t('Withdrawal amount')}</Label>
              <Input
                type='number'
                min='0'
                step='0.0001'
                value={withdrawAmount}
                onChange={(event) => setWithdrawAmount(event.target.value)}
              />
              <p className='text-muted-foreground text-xs'>
                {t('Available')}: {formatQuota(balance?.available_quota ?? 0)}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setWithdrawOpen(false)}
              disabled={submittingWithdraw}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={submitWithdraw} disabled={submittingWithdraw}>
              {submittingWithdraw ? t('Submitting...') : t('Submit withdrawal')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
