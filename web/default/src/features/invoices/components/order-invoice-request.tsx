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
import { Invoice03Icon, RefreshIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { previewOrderInvoice } from '../api'
import {
  createEmptyInvoiceRequest,
  isInvoiceRequestValid,
  normalizeInvoiceConfig,
  type InvoiceConfig,
  type InvoiceEligibleOrder,
  type InvoiceOrderPreview,
  type InvoiceRequest,
} from '../types'
import { InvoicePaymentSelector } from './invoice-payment-selector'
import { InvoiceRequestForm } from './invoice-request-form'

interface OrderInvoiceRequestProps {
  config?: InvoiceConfig | null
  orders: InvoiceEligibleOrder[]
  loading: boolean
  submitting: boolean
  onRefresh: () => void
  onSubmit: (
    orders: InvoiceEligibleOrder[],
    invoice: InvoiceRequest,
    paymentMethod: string,
    tradeType?: string
  ) => Promise<boolean>
}

const MAX_SELECTED_ORDERS = 100

function orderKey(order: InvoiceEligibleOrder) {
  return `${order.source_type}:${order.source_id}`
}

function isOrderSelectable(order: InvoiceEligibleOrder) {
  return order.invoice_eligible && !order.invoiced
}

function formatCny(amount: number) {
  return `¥${Number(amount || 0).toFixed(2)}`
}

function formatTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

function getSourceLabel(type: string, t: (key: string) => string) {
  return type === 'subscription' ? t('Subscription purchase') : t('Top-up')
}

export function OrderInvoiceRequest({
  config,
  orders,
  loading,
  submitting,
  onRefresh,
  onSubmit,
}: OrderInvoiceRequestProps) {
  const { t } = useTranslation()
  const normalizedConfig = useMemo(
    () => normalizeInvoiceConfig(config),
    [config]
  )
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set())
  const [dialogOpen, setDialogOpen] = useState(false)
  const [preview, setPreview] = useState<InvoiceOrderPreview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState('')
  const [paymentMethod, setPaymentMethod] = useState('balance')
  const [bepusdtTradeType, setBepusdtTradeType] = useState(
    normalizedConfig.bepusdt_chains[0]?.trade_type || ''
  )
  const [invoice, setInvoice] = useState<InvoiceRequest>(() => ({
    ...createEmptyInvoiceRequest(
      normalizedConfig.types[0],
      normalizedConfig.kinds[0]
    ),
    required: true,
  }))

  useEffect(() => {
    const availableKeys = new Set(
      orders.filter(isOrderSelectable).map(orderKey)
    )
    setSelectedKeys((current) => {
      const next = new Set([...current].filter((key) => availableKeys.has(key)))
      return next.size === current.size ? current : next
    })
  }, [orders])

  useEffect(() => {
    if (
      normalizedConfig.types.includes(invoice.type) &&
      normalizedConfig.kinds.includes(invoice.kind)
    ) {
      return
    }
    setInvoice((current) => ({
      ...current,
      type: normalizedConfig.types.includes(current.type)
        ? current.type
        : normalizedConfig.types[0],
      kind: normalizedConfig.kinds.includes(current.kind)
        ? current.kind
        : normalizedConfig.kinds[0],
    }))
  }, [
    invoice.kind,
    invoice.type,
    normalizedConfig.kinds,
    normalizedConfig.types,
  ])

  useEffect(() => {
    if (
      paymentMethod === 'balance' ||
      normalizedConfig.pay_methods.some(
        (method) => method.type === paymentMethod
      )
    ) {
      return
    }
    setPaymentMethod('balance')
  }, [normalizedConfig.pay_methods, paymentMethod])

  useEffect(() => {
    if (
      normalizedConfig.bepusdt_chains.some(
        (chain) => chain.trade_type === bepusdtTradeType
      )
    ) {
      return
    }
    setBepusdtTradeType(normalizedConfig.bepusdt_chains[0]?.trade_type || '')
  }, [bepusdtTradeType, normalizedConfig.bepusdt_chains])

  const selectedOrders = useMemo(
    () =>
      orders.filter(
        (order) => isOrderSelectable(order) && selectedKeys.has(orderKey(order))
      ),
    [orders, selectedKeys]
  )
  const selectedOrderRefs = useMemo(
    () =>
      selectedOrders.map((order) => ({
        source_type: order.source_type,
        source_id: order.source_id,
      })),
    [selectedOrders]
  )
  const selectableOrders = useMemo(
    () => orders.filter(isOrderSelectable),
    [orders]
  )
  const bulkSelectableOrders = useMemo(
    () => selectableOrders.slice(0, MAX_SELECTED_ORDERS),
    [selectableOrders]
  )
  const allSelected =
    bulkSelectableOrders.length > 0 &&
    bulkSelectableOrders.every((order) => selectedKeys.has(orderKey(order)))
  const selectedPaymentMethod = normalizedConfig.pay_methods.find(
    (method) => method.type === paymentMethod
  )
  const invoiceFee = Number(preview?.invoice_fee || 0)
  const isBepusdtPayment =
    selectedPaymentMethod?.provider === 'bepusdt' ||
    selectedPaymentMethod?.type === 'bepusdt'
  let submitLabel = t('Submit invoice request')
  if (invoiceFee > 0 && paymentMethod === 'balance') {
    submitLabel = t('Pay {{amount}} with balance and submit', {
      amount: formatCny(invoiceFee),
    })
  } else if (invoiceFee > 0 && selectedPaymentMethod) {
    submitLabel = t('Use {{method}} to pay {{amount}}', {
      method: selectedPaymentMethod.name,
      amount: formatCny(invoiceFee),
    })
  }

  useEffect(() => {
    if (selectedOrderRefs.length === 0) {
      setPreview(null)
      setPreviewError('')
      setPreviewLoading(false)
      return
    }

    let cancelled = false
    setPreview(null)
    setPreviewError('')
    setPreviewLoading(true)
    const timer = window.setTimeout(async () => {
      try {
        const response = await previewOrderInvoice({
          orders: selectedOrderRefs,
        })
        if (cancelled) return
        if (!response.success || !response.data) {
          throw new Error(
            response.message || t('Failed to calculate invoice fee')
          )
        }
        setPreview(response.data)
      } catch (error) {
        if (cancelled) return
        setPreview(null)
        setPreviewError(
          error instanceof Error
            ? error.message
            : t('Failed to calculate invoice fee')
        )
      } finally {
        if (!cancelled) setPreviewLoading(false)
      }
    }, 200)

    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [selectedOrderRefs, t])

  const toggleOrder = (order: InvoiceEligibleOrder, checked: boolean) => {
    if (!isOrderSelectable(order)) return
    setSelectedKeys((current) => {
      const next = new Set(current)
      if (checked) {
        if (next.size >= MAX_SELECTED_ORDERS) {
          toast.warning(
            t('You can select up to {{count}} orders at a time', {
              count: MAX_SELECTED_ORDERS,
            })
          )
          return current
        }
        next.add(orderKey(order))
      } else {
        next.delete(orderKey(order))
      }
      return next
    })
  }

  const submit = async () => {
    if (!isInvoiceRequestValid(normalizedConfig, invoice)) {
      toast.error(t('Please complete the invoice information'))
      return
    }
    if (selectedOrders.length === 0) {
      toast.error(t('Please select at least one order'))
      return
    }
    if (!preview || previewLoading) {
      toast.error(t('Please wait for invoice fee calculation'))
      return
    }
    if (invoiceFee > 0 && isBepusdtPayment && !bepusdtTradeType) {
      toast.error(t('Please select a USDT network'))
      return
    }
    const selectedMethod = invoiceFee > 0 ? paymentMethod : 'balance'
    const tradeType =
      invoiceFee > 0 && isBepusdtPayment ? bepusdtTradeType : undefined
    const success = await onSubmit(
      selectedOrders,
      invoice,
      selectedMethod,
      tradeType
    )
    if (!success) return
    setDialogOpen(false)
    setSelectedKeys(new Set())
    setPreview(null)
    setPaymentMethod('balance')
    setInvoice({
      ...createEmptyInvoiceRequest(
        normalizedConfig.types[0],
        normalizedConfig.kinds[0]
      ),
      required: true,
    })
  }

  if (!normalizedConfig.enabled) return null

  return (
    <>
      <Card>
        <CardHeader className='border-b'>
          <div>
            <CardTitle>{t('Apply for invoice')}</CardTitle>
            <CardDescription>
              {t(
                'Select paid orders from the last 30 days. Orders already used for an invoice cannot be selected again.'
              )}
            </CardDescription>
          </div>
          <CardAction>
            <Button
              variant='outline'
              size='sm'
              onClick={onRefresh}
              disabled={loading}
            >
              {loading ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <HugeiconsIcon icon={RefreshIcon} data-icon='inline-start' />
              )}
              {t('Refresh')}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className='flex flex-col gap-4'>
          {loading ? (
            <div className='flex flex-col gap-2'>
              {Array.from({ length: 3 }).map((_, index) => (
                <Skeleton key={index} className='h-12 w-full' />
              ))}
            </div>
          ) : orders.length === 0 ? (
            <Empty className='min-h-28'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <HugeiconsIcon icon={Invoice03Icon} />
                </EmptyMedia>
                <EmptyTitle>{t('No invoiceable orders')}</EmptyTitle>
              </EmptyHeader>
            </Empty>
          ) : (
            <>
              <div className='rounded-lg border'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-10'>
                        <Checkbox
                          checked={allSelected}
                          aria-label={t('Select all')}
                          onCheckedChange={(checked) =>
                            setSelectedKeys(
                              checked
                                ? new Set(bulkSelectableOrders.map(orderKey))
                                : new Set()
                            )
                          }
                        />
                      </TableHead>
                      <TableHead>{t('Source')}</TableHead>
                      <TableHead>{t('Order number')}</TableHead>
                      <TableHead>{t('Paid at')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Paid amount')}
                      </TableHead>
                      <TableHead>{t('Invoice status')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {orders.map((order) => {
                      const checked = selectedKeys.has(orderKey(order))
                      return (
                        <TableRow
                          key={orderKey(order)}
                          data-state={checked ? 'selected' : undefined}
                          className={cn(
                            'cursor-pointer',
                            !isOrderSelectable(order) &&
                              'text-muted-foreground cursor-not-allowed'
                          )}
                          onClick={() => toggleOrder(order, !checked)}
                        >
                          <TableCell>
                            <Checkbox
                              checked={checked}
                              disabled={!isOrderSelectable(order)}
                              aria-label={t('Select order')}
                              onClick={(event) => event.stopPropagation()}
                              onCheckedChange={(value) =>
                                toggleOrder(order, Boolean(value))
                              }
                            />
                          </TableCell>
                          <TableCell>
                            {getSourceLabel(order.source_type, t)}
                          </TableCell>
                          <TableCell className='max-w-56 truncate font-mono text-xs'>
                            {order.source_id}
                          </TableCell>
                          <TableCell>
                            {formatTime(order.complete_time)}
                          </TableCell>
                          <TableCell className='text-right font-medium'>
                            {formatCny(order.paid_amount)}
                          </TableCell>
                          <TableCell>
                            <Badge variant='secondary'>
                              {order.invoiced
                                ? t('Invoice requested')
                                : order.invoice_eligible
                                  ? t('Invoiceable')
                                  : t('Not invoiceable')}
                            </Badge>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>

              <div className='bg-muted/40 flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
                <div className='text-sm'>
                  <span className='font-medium'>
                    {t('Selected {{count}} orders', {
                      count: selectedOrders.length,
                    })}
                  </span>
                  <span className='text-muted-foreground ml-3'>
                    {t('Invoice amount')}:{' '}
                    {formatCny(preview?.order_amount || 0)}
                  </span>
                  <span className='text-muted-foreground ml-3'>
                    {t('Invoice fee')}: {formatCny(preview?.invoice_fee || 0)}
                  </span>
                </div>
                <Button
                  disabled={
                    selectedOrders.length === 0 ||
                    previewLoading ||
                    !preview ||
                    Boolean(previewError)
                  }
                  onClick={() => setDialogOpen(true)}
                >
                  {previewLoading
                    ? t('Calculating invoice fee')
                    : t('Continue to invoice')}
                </Button>
              </div>
              {previewError && (
                <Alert variant='destructive'>
                  <AlertDescription>{previewError}</AlertDescription>
                </Alert>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-xl'>
          <DialogHeader>
            <DialogTitle>{t('Confirm invoice request')}</DialogTitle>
            <DialogDescription>
              {t(
                'The historical order amount has already been paid. You are only paying the invoice service fee this time.'
              )}
            </DialogDescription>
          </DialogHeader>

          <div className='bg-muted/40 grid grid-cols-3 gap-3 rounded-lg border p-3 text-sm'>
            <div>
              <Label className='text-muted-foreground text-xs'>
                {t('Orders')}
              </Label>
              <div className='font-medium'>{selectedOrders.length}</div>
            </div>
            <div>
              <Label className='text-muted-foreground text-xs'>
                {t('Invoice amount')}
              </Label>
              <div className='font-medium'>
                {formatCny(preview?.order_amount || 0)}
              </div>
            </div>
            <div>
              <Label className='text-muted-foreground text-xs'>
                {t('Invoice fee')}
              </Label>
              <div className='font-medium'>
                {formatCny(preview?.invoice_fee || 0)}
              </div>
            </div>
          </div>

          {preview && (
            <Alert>
              <AlertDescription>
                {t(
                  'The historical order amount has already been paid. You are only paying the invoice service fee this time.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {invoiceFee > 0 && (
            <InvoicePaymentSelector
              methods={normalizedConfig.pay_methods}
              value={paymentMethod}
              onValueChange={setPaymentMethod}
              bepusdtChains={normalizedConfig.bepusdt_chains}
              tradeType={bepusdtTradeType}
              onTradeTypeChange={setBepusdtTradeType}
              disabled={submitting}
            />
          )}

          <InvoiceRequestForm
            config={normalizedConfig}
            value={invoice}
            onChange={setInvoice}
            invoiceFee={preview?.invoice_fee || 0}
            disabled={submitting}
            showRequiredToggle={false}
          />

          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDialogOpen(false)}
              disabled={submitting}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={submit} disabled={submitting}>
              {submitting && <Spinner data-icon='inline-start' />}
              {submitLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
