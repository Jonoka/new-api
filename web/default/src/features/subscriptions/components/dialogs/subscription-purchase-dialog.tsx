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
import { useState, useEffect, useMemo } from 'react'
import { Crown, CalendarClock, Loader2, Package } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'
import { formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useSystemConfig } from '@/hooks/use-system-config'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { GroupBadge } from '@/components/group-badge'
import { InvoiceRequestForm } from '@/features/invoices/components/invoice-request-form'
import {
  createEmptyInvoiceRequest,
  getInvoicePayload,
  isInvoicePreviewRequestEnabled,
  isInvoiceRequestValid,
  normalizeInvoiceConfig,
  type InvoiceConfig,
  type InvoiceRequest,
} from '@/features/invoices/types'
import { BepusdtChainDialog } from '@/features/wallet/components/dialogs/bepusdt-chain-dialog'
import { getPaymentIcon } from '@/features/wallet/lib'
import type { BepusdtChain } from '@/features/wallet/types'
import {
  paySubscriptionStripe,
  paySubscriptionCreem,
  paySubscriptionEpay,
  paySubscriptionWaffoPancake,
  paySubscriptionBalance,
  paySubscriptionBepusdt,
  paySubscriptionOkpay,
  previewSubscriptionAmount,
} from '../../api'
import {
  formatDuration,
  formatPlanCurrencyAmount,
  formatPlanQuotaAllowance,
  formatResetPeriod,
} from '../../lib'
import type {
  PlanRecord,
  SubscriptionAmountPreview,
  SubscriptionAmountResponse,
} from '../../types'

interface PaymentMethod {
  type: string
  name?: string
  icon?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  enableStripe?: boolean
  enableCreem?: boolean
  enableWaffoPancake?: boolean
  enableBepusdt?: boolean
  enableOkpay?: boolean
  bepusdtChains?: BepusdtChain[]
  enableOnlineTopUp?: boolean
  epayMethods?: PaymentMethod[]
  invoiceConfig?: InvoiceConfig | null
  purchaseLimit?: number
  purchaseCount?: number
  userQuota?: number
  onPurchaseSuccess?: () => void | Promise<void>
}

export function SubscriptionPurchaseDialog(props: Props) {
  const { t } = useTranslation()
  const { currency } = useSystemConfig()
  const [paying, setPaying] = useState(false)
  const [amountLoading, setAmountLoading] = useState(false)
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('')
  const [promoCode, setPromoCode] = useState('')
  const [promoDiscount, setPromoDiscount] =
    useState<SubscriptionAmountPreview | null>(null)
  const [amountPreview, setAmountPreview] =
    useState<SubscriptionAmountResponse | null>(null)
  const [bepusdtChainOpen, setBepusdtChainOpen] = useState(false)
  const [bepusdtConfirmOpen, setBepusdtConfirmOpen] = useState(false)
  const [selectedBepusdtTradeType, setSelectedBepusdtTradeType] = useState('')
  const [selectedPaymentKind, setSelectedPaymentKind] = useState('')
  const normalizedInvoiceConfig = useMemo(
    () => normalizeInvoiceConfig(props.invoiceConfig),
    [props.invoiceConfig]
  )
  const [invoiceRequest, setInvoiceRequest] = useState<InvoiceRequest>(
    createEmptyInvoiceRequest()
  )

  const plan = props.plan?.plan
  const planId = plan?.id || 0
  const hasConfiguredBepusdt =
    !!props.enableBepusdt && (props.bepusdtChains || []).length > 0

  function getPreviewPaymentMethod() {
    if (selectedPaymentKind === 'bepusdt' && hasConfiguredBepusdt) {
      return 'bepusdt'
    }
    if (selectedPaymentKind === 'epay' && selectedEpayMethod) {
      return selectedEpayMethod
    }
    if (selectedPaymentKind) return selectedPaymentKind
    if (selectedEpayMethod) return selectedEpayMethod
    return 'balance'
  }

  async function loadAmountPreview(
    paymentMethod = getPreviewPaymentMethod(),
    code = promoCode.trim(),
    silent = false,
    request = invoiceRequest
  ) {
    if (!planId) return null
    const previewRequest = isInvoicePreviewRequestEnabled(
      normalizedInvoiceConfig,
      request
    )
      ? request
      : createEmptyInvoiceRequest(request.type)
    setAmountLoading(true)
    try {
      const res = await previewSubscriptionAmount({
        plan_id: planId,
        promo_code: code,
        payment_method: paymentMethod,
        ...getInvoicePayload(previewRequest),
      })
      if (res.message === 'success') {
        setAmountPreview(res)
        setPromoDiscount(res.discount || null)
        return res
      }
      setAmountPreview(null)
      setPromoDiscount(null)
      if (!silent) {
        toast.error(res.data || res.message || t('Payment request failed'))
      }
      return null
    } catch {
      setAmountPreview(null)
      setPromoDiscount(null)
      if (!silent) {
        toast.error(t('Payment request failed'))
      }
      return null
    } finally {
      setAmountLoading(false)
    }
  }

  useEffect(() => {
    if (props.open) {
      const firstEpayMethod = props.epayMethods?.[0]?.type || ''
      setSelectedEpayMethod(firstEpayMethod)
      setSelectedPaymentKind(
        props.enableOnlineTopUp && firstEpayMethod
          ? 'epay'
          : hasConfiguredBepusdt
            ? 'bepusdt'
            : props.enableStripe && props.plan?.plan?.stripe_price_id
              ? 'stripe'
              : props.enableCreem && props.plan?.plan?.creem_product_id
                ? 'creem'
                : props.enableWaffoPancake &&
                    props.plan?.plan?.waffo_pancake_product_id
                  ? 'waffo_pancake'
                  : ''
      )
    } else if (!props.open) {
      setSelectedEpayMethod('')
      setSelectedPaymentKind('')
      setPromoCode('')
      setPromoDiscount(null)
      setAmountPreview(null)
      setBepusdtChainOpen(false)
      setBepusdtConfirmOpen(false)
      setSelectedBepusdtTradeType('')
      setInvoiceRequest(
        createEmptyInvoiceRequest(normalizedInvoiceConfig.types[0])
      )
      setAmountLoading(false)
    }
  }, [
    props.open,
    props.epayMethods,
    hasConfiguredBepusdt,
    normalizedInvoiceConfig.types,
    props.enableStripe,
    props.enableCreem,
    props.enableWaffoPancake,
    props.plan?.plan?.stripe_price_id,
    props.plan?.plan?.creem_product_id,
    props.plan?.plan?.waffo_pancake_product_id,
  ])

  useEffect(() => {
    if (!props.open || !planId) return
    void loadAmountPreview(getPreviewPaymentMethod(), promoCode.trim(), true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, planId, selectedEpayMethod, selectedPaymentKind])

  if (!plan) return null

  const hasStripe = props.enableStripe && !!plan.stripe_price_id
  const hasCreem = props.enableCreem && !!plan.creem_product_id
  const hasWaffoPancake =
    props.enableWaffoPancake && !!plan.waffo_pancake_product_id
  const hasBepusdt = hasConfiguredBepusdt
  const hasOkpay = !!props.enableOkpay
  const hasEpay =
    props.enableOnlineTopUp && (props.epayMethods || []).length > 0
  const hasAnyPayment =
    hasStripe ||
    hasCreem ||
    hasWaffoPancake ||
    hasBepusdt ||
    hasOkpay ||
    hasEpay
  const selectedEpayMethodLabel =
    (props.epayMethods || []).find((m) => m.type === selectedEpayMethod)
      ?.name ||
    selectedEpayMethod ||
    t('Select payment method')
  const price = Number(plan.price_amount || 0)
  const hasPromoDiscount = Number(promoDiscount?.discount_amount || 0) > 0
  const paidDisplayAmount = hasPromoDiscount
    ? Number(promoDiscount?.paid_amount || 0)
    : price
  const originalPrice = Number(promoDiscount?.original_amount || price)
  const discountAmount = Number(promoDiscount?.discount_amount || 0)
  const previewAmount =
    typeof amountPreview?.amount === 'number'
      ? amountPreview.amount
      : Number(amountPreview?.data || Number.NaN)
  const amountDue = Number.isFinite(previewAmount)
    ? previewAmount
    : paidDisplayAmount
  const amountDueCurrency = amountPreview?.currency || plan.currency || 'USD'
  const amountDueText = formatPlanCurrencyAmount(amountDue, amountDueCurrency)
  const paidPriceUSD =
    amountDueCurrency === 'USD'
      ? amountDue
      : Number(amountPreview?.amount_usd || 0) ||
        (plan.currency === 'USD' ? paidDisplayAmount : 0)
  const quotaPerUnit =
    currency?.quotaPerUnit && currency.quotaPerUnit > 0
      ? currency.quotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const balanceCost = Math.max(0, Math.ceil(paidPriceUSD * quotaPerUnit))
  const userQuota = Math.max(0, Number(props.userQuota || 0))
  const balanceAmountReady = paidPriceUSD > 0 || amountDue <= 0
  const insufficientBalance = !balanceAmountReady || userQuota < balanceCost
  const limitReached =
    (props.purchaseLimit || 0) > 0 &&
    (props.purchaseCount || 0) >= (props.purchaseLimit || 0)
  const quotaAllowanceLines = formatPlanQuotaAllowance(plan, t)
  const selectedBepusdtChain = (props.bepusdtChains || []).find(
    (chain) => chain.trade_type === selectedBepusdtTradeType
  )
  const invoiceFee = Number(amountPreview?.invoice_fee || 0)
  const invoiceValid = isInvoiceRequestValid(
    normalizedInvoiceConfig,
    invoiceRequest
  )
  const invoiceDisabled = paying || amountLoading
  const externalPaymentOptions: Array<{
    key: string
    kind: string
    value: string
    label: string
    icon?: string
  }> = [
    ...(hasEpay
      ? (props.epayMethods || []).map((method) => ({
          key: `epay:${method.type}`,
          kind: 'epay',
          value: method.type,
          label: method.name || method.type,
          icon: method.icon,
        }))
      : []),
    ...(hasBepusdt
      ? [
          {
            key: 'bepusdt',
            kind: 'bepusdt',
            value: 'bepusdt',
            label: 'USDT',
          },
        ]
      : []),
    ...(hasOkpay
      ? [{ key: 'okpay', kind: 'okpay', value: 'okpay', label: 'OKPay' }]
      : []),
    ...(hasStripe
      ? [{ key: 'stripe', kind: 'stripe', value: 'stripe', label: 'Stripe' }]
      : []),
    ...(hasCreem
      ? [{ key: 'creem', kind: 'creem', value: 'creem', label: 'Creem' }]
      : []),
    ...(hasWaffoPancake
      ? [
          {
            key: 'waffo_pancake',
            kind: 'waffo_pancake',
            value: 'waffo_pancake',
            label: 'Waffo Pancake',
          },
        ]
      : []),
  ]
  const handleOpenBepusdtChains = async () => {
    if (!invoiceValid) return
    setSelectedPaymentKind('bepusdt')
    const preview = await loadAmountPreview(
      'bepusdt',
      promoCode.trim(),
      false,
      invoiceRequest
    )
    if (preview?.message === 'success') {
      setBepusdtChainOpen(true)
    }
  }

  const handleSelectPayment = (kind: string, value: string) => {
    setSelectedPaymentKind(kind)
    if (kind === 'epay') {
      setSelectedEpayMethod(value)
      void loadAmountPreview(value, promoCode.trim(), true, invoiceRequest)
      return
    }
    if (kind === 'bepusdt') {
      void handleOpenBepusdtChains()
      return
    }
    void loadAmountPreview(value, promoCode.trim(), true, invoiceRequest)
  }

  const handleCompletedPurchase = () => {
    toast.success(t('Subscription purchased successfully'))
    void props.onPurchaseSuccess?.()
    props.onOpenChange(false)
  }

  const handlePromoCodeBlur = async () => {
    const code = promoCode.trim()
    if (!code) {
      setPromoDiscount(null)
      void loadAmountPreview(
        getPreviewPaymentMethod(),
        '',
        true,
        invoiceRequest
      )
      return
    }
    await loadAmountPreview(
      getPreviewPaymentMethod(),
      code,
      false,
      invoiceRequest
    )
  }

  const handleInvoiceRequestChange = (request: InvoiceRequest) => {
    setInvoiceRequest(request)
    void loadAmountPreview(
      getPreviewPaymentMethod(),
      promoCode.trim(),
      true,
      request
    )
  }

  const handlePayStripe = async () => {
    if (!invoiceValid) return
    setPaying(true)
    try {
      const res = await paySubscriptionStripe({
        plan_id: plan.id,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.message === 'success' && res.data?.completed) {
        handleCompletedPurchase()
      } else if (res.message === 'success' && res.data?.pay_link) {
        window.open(res.data.pay_link, '_blank')
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayCreem = async () => {
    if (!invoiceValid) return
    if (promoCode.trim()) {
      toast.error(t('Creem does not support promo codes yet'))
      return
    }
    setPaying(true)
    try {
      const res = await paySubscriptionCreem({
        plan_id: plan.id,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.message === 'success' && res.data?.checkout_url) {
        window.open(res.data.checkout_url, '_blank')
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  // In-tab redirect (not window.open) — user-gesture context is lost
  // across the await, so a popup would be blocked. Same as the wallet hook.
  const handlePayWaffoPancake = async () => {
    if (!invoiceValid) return
    setPaying(true)
    try {
      const res = await paySubscriptionWaffoPancake({
        plan_id: plan.id,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.message === 'success' && res.data?.completed) {
        handleCompletedPurchase()
      } else if (res.message === 'success' && res.data?.checkout_url) {
        toast.success(t('Redirecting to payment page...'))
        window.location.href = res.data.checkout_url
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const isSafari =
    typeof navigator !== 'undefined' &&
    /^((?!chrome|android).)*safari/i.test(navigator.userAgent)

  const handlePayEpay = async () => {
    if (!invoiceValid) return
    if (!selectedEpayMethod) {
      toast.error(t('Please select a payment method'))
      return
    }
    setPaying(true)
    try {
      const res = await paySubscriptionEpay({
        plan_id: plan.id,
        payment_method: selectedEpayMethod,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.message === 'success' && res.data?.completed) {
        handleCompletedPurchase()
      } else if (res.message === 'success' && res.url) {
        const form = document.createElement('form')
        form.action = res.url
        form.method = 'POST'
        if (!isSafari) {
          form.target = '_blank'
        }
        Object.entries(res.data || {}).forEach(([key, value]) => {
          const input = document.createElement('input')
          input.type = 'hidden'
          input.name = key
          input.value = String(value)
          form.appendChild(input)
        })
        document.body.appendChild(form)
        form.submit()
        document.body.removeChild(form)
        toast.success(t('Payment initiated'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayBalance = async () => {
    if (!invoiceValid) return
    setPaying(true)
    try {
      const res = await paySubscriptionBalance({
        plan_id: plan.id,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.success) {
        toast.success(t('Subscription purchased successfully'))
        void props.onPurchaseSuccess?.()
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayOkpay = async () => {
    if (!invoiceValid) return
    setPaying(true)
    try {
      const res = await paySubscriptionOkpay({
        plan_id: plan.id,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.message === 'success' && res.data?.completed) {
        handleCompletedPurchase()
      } else if (res.message === 'success' && res.data?.payment_url) {
        toast.success(t('Redirecting to payment page...'))
        window.location.href = res.data.payment_url
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handleBepusdtChainConfirm = (tradeType: string) => {
    setSelectedBepusdtTradeType(tradeType)
    setBepusdtChainOpen(false)
    setBepusdtConfirmOpen(true)
  }

  const handlePayBepusdt = async () => {
    if (!invoiceValid) return
    if (!selectedBepusdtTradeType) {
      toast.error(t('Please select a payment network'))
      return
    }
    setPaying(true)
    try {
      const res = await paySubscriptionBepusdt({
        plan_id: plan.id,
        trade_type: selectedBepusdtTradeType,
        promo_code: promoCode,
        ...getInvoicePayload(invoiceRequest),
      })
      if (res.message === 'success' && res.data?.completed) {
        setBepusdtConfirmOpen(false)
        handleCompletedPurchase()
      } else if (res.message === 'success' && res.data?.payment_url) {
        window.open(res.data.payment_url, '_blank')
        toast.success(t('Redirecting to payment page...'))
        setBepusdtConfirmOpen(false)
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              <Crown className='h-5 w-5' />
              {t('Purchase Subscription')}
            </DialogTitle>
          </DialogHeader>

          <div className='space-y-3 sm:space-y-4'>
            <div className='bg-muted/50 space-y-2.5 rounded-lg border p-3 sm:space-y-3 sm:p-4'>
              <div className='flex justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Plan Name')}
                </span>
                <span className='max-w-[200px] truncate text-sm font-medium'>
                  {plan.title}
                </span>
              </div>
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Validity Period')}
                </span>
                <span className='flex items-center gap-1 text-sm'>
                  <CalendarClock className='h-3.5 w-3.5' />
                  {formatDuration(plan, t)}
                </span>
              </div>
              {formatResetPeriod(plan, t) !== t('No Reset') && (
                <div className='flex justify-between'>
                  <span className='text-muted-foreground text-sm'>
                    {t('Reset Period')}
                  </span>
                  <span className='text-sm'>{formatResetPeriod(plan, t)}</span>
                </div>
              )}
              <div className='flex items-start justify-between gap-3'>
                <span className='text-muted-foreground text-sm'>
                  {t('Received amount')}
                </span>
                <span className='flex min-w-0 flex-col items-end gap-1 text-right text-sm'>
                  {quotaAllowanceLines.map((line) => (
                    <span key={line} className='flex items-center gap-1'>
                      <Package className='h-3.5 w-3.5 shrink-0' />
                      {line}
                    </span>
                  ))}
                </span>
              </div>
              {plan.upgrade_group && (
                <div className='flex items-center justify-between'>
                  <span className='text-muted-foreground text-sm'>
                    {t('Upgrade Group')}
                  </span>
                  <GroupBadge group={plan.upgrade_group} />
                </div>
              )}
              <Separator />
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Plan Price')}
                </span>
                <span className='text-sm font-medium'>
                  {formatPlanCurrencyAmount(price, plan.currency)}
                </span>
              </div>
              <div className='flex items-center justify-between'>
                <span className='text-sm font-medium'>{t('Amount Due')}</span>
                <div className='flex items-baseline gap-2'>
                  <span className='text-lg font-bold text-red-600 dark:text-red-400'>
                    {amountLoading && !amountPreview ? '...' : amountDueText}
                  </span>
                  {hasPromoDiscount && !amountLoading && (
                    <span className='text-xs text-green-600'>
                      {t('Discount')}
                    </span>
                  )}
                </div>
              </div>
              {hasPromoDiscount && !amountLoading && (
                <div className='flex justify-between text-xs'>
                  <span className='text-muted-foreground'>
                    {formatPlanCurrencyAmount(originalPrice, amountDueCurrency)}
                  </span>
                  <span className='text-green-600'>
                    -
                    {formatPlanCurrencyAmount(
                      discountAmount,
                      amountDueCurrency
                    )}
                  </span>
                </div>
              )}
              {invoiceFee > 0 && !amountLoading && (
                <div className='flex justify-between text-xs'>
                  <span className='text-muted-foreground'>
                    {t('Invoice fee')}
                  </span>
                  <span>{`¥${invoiceFee.toFixed(2)}`}</span>
                </div>
              )}
            </div>

            {limitReached && (
              <Alert variant='destructive'>
                <AlertDescription>
                  {t('Purchase limit reached')} ({props.purchaseCount}/
                  {props.purchaseLimit})
                </AlertDescription>
              </Alert>
            )}

            <div className='flex flex-col gap-2 rounded-md border p-3'>
              <div className='flex items-center justify-between gap-2 text-xs'>
                <span className='text-muted-foreground'>{t('Required')}</span>
                <span>{formatQuota(balanceCost)}</span>
              </div>
              <div className='flex items-center justify-between gap-2 text-xs'>
                <span className='text-muted-foreground'>{t('Available')}</span>
                <span>{formatQuota(userQuota)}</span>
              </div>
              {balanceAmountReady && insufficientBalance && (
                <Alert variant='destructive'>
                  <AlertDescription>
                    {t('Insufficient balance')}
                  </AlertDescription>
                </Alert>
              )}
              <Button
                variant='outline'
                onClick={handlePayBalance}
                disabled={
                  paying ||
                  amountLoading ||
                  limitReached ||
                  !balanceAmountReady ||
                  insufficientBalance ||
                  !invoiceValid
                }
              >
                {t('Pay with Balance')}
              </Button>
            </div>

            <div className='space-y-2'>
              <Input
                value={promoCode}
                onChange={(event) => {
                  setPromoCode(event.target.value)
                  setPromoDiscount(null)
                  setAmountPreview(null)
                }}
                onBlur={handlePromoCodeBlur}
                placeholder={t('Enter promo code')}
              />
            </div>

            <InvoiceRequestForm
              config={normalizedInvoiceConfig}
              value={invoiceRequest}
              onChange={handleInvoiceRequestChange}
              invoiceFee={invoiceFee}
              disabled={invoiceDisabled}
            />

            {hasAnyPayment && (
              <div className='space-y-3'>
                <p className='text-muted-foreground text-xs'>
                  {t('Select payment method')}
                </p>
                {externalPaymentOptions.length > 0 && (
                  <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
                    {externalPaymentOptions.map((option) => {
                      const selected =
                        option.kind === 'epay'
                          ? selectedPaymentKind === 'epay' &&
                            selectedEpayMethod === option.value
                          : selectedPaymentKind === option.kind
                      return (
                        <Button
                          key={option.key}
                          type='button'
                          variant={selected ? 'default' : 'outline'}
                          className={cn(
                            'min-w-0 justify-center gap-2 truncate'
                          )}
                          onClick={() =>
                            handleSelectPayment(option.kind, option.value)
                          }
                          disabled={paying || amountLoading || limitReached}
                        >
                          {getPaymentIcon(
                            option.value,
                            'h-4 w-4 shrink-0',
                            option.icon,
                            option.label
                          )}
                          <span className='truncate'>{option.label}</span>
                        </Button>
                      )
                    })}
                  </div>
                )}

                {selectedPaymentKind === 'epay' && hasEpay && (
                  <div className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2'>
                    <div className='bg-muted/40 flex items-center gap-2 rounded-md border px-3 py-2 text-sm'>
                      {getPaymentIcon(
                        selectedEpayMethod,
                        'h-4 w-4 shrink-0',
                        (props.epayMethods || []).find(
                          (m) => m.type === selectedEpayMethod
                        )?.icon,
                        selectedEpayMethodLabel
                      )}
                      {selectedEpayMethodLabel}
                    </div>
                    <Button
                      onClick={handlePayEpay}
                      disabled={
                        paying ||
                        !selectedEpayMethod ||
                        limitReached ||
                        !invoiceValid
                      }
                    >
                      {t('Pay')}
                    </Button>
                  </div>
                )}
                {selectedPaymentKind === 'okpay' && hasOkpay && (
                  <Button
                    className='w-full'
                    onClick={handlePayOkpay}
                    disabled={
                      paying || amountLoading || limitReached || !invoiceValid
                    }
                  >
                    OKPay
                  </Button>
                )}
                {selectedPaymentKind === 'stripe' && hasStripe && (
                  <Button
                    className='w-full'
                    onClick={handlePayStripe}
                    disabled={
                      paying || amountLoading || limitReached || !invoiceValid
                    }
                  >
                    Stripe
                  </Button>
                )}
                {selectedPaymentKind === 'creem' && hasCreem && (
                  <Button
                    className='w-full'
                    onClick={handlePayCreem}
                    disabled={
                      paying || amountLoading || limitReached || !invoiceValid
                    }
                  >
                    Creem
                  </Button>
                )}
                {selectedPaymentKind === 'waffo_pancake' && hasWaffoPancake && (
                  <Button
                    className='w-full'
                    onClick={handlePayWaffoPancake}
                    disabled={
                      paying || amountLoading || limitReached || !invoiceValid
                    }
                  >
                    Waffo Pancake
                  </Button>
                )}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      <BepusdtChainDialog
        open={bepusdtChainOpen}
        onOpenChange={setBepusdtChainOpen}
        chains={props.bepusdtChains || []}
        amountLabel={t('Amount Due')}
        amountText={amountDueText}
        onConfirm={handleBepusdtChainConfirm}
        processing={paying}
      />

      <AlertDialog
        open={bepusdtConfirmOpen}
        onOpenChange={setBepusdtConfirmOpen}
      >
        <AlertDialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Confirm USDT Payment')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('Review your payment details before continuing.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className='space-y-3 py-2 text-sm'>
            <div className='flex items-center justify-between gap-3'>
              <span className='text-muted-foreground'>{t('Network')}</span>
              <span className='font-medium'>
                {selectedBepusdtChain?.name || selectedBepusdtTradeType}
              </span>
            </div>
            <div className='flex items-center justify-between gap-3'>
              <span className='text-muted-foreground'>{t('Amount Due')}</span>
              <span className='text-lg font-semibold'>{amountDueText}</span>
            </div>
          </div>
          <AlertDialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
            <AlertDialogCancel disabled={paying}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handlePayBepusdt}
              disabled={paying || !invoiceValid}
            >
              {paying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              {t('Confirm Payment')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
