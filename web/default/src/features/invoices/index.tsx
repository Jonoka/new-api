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
  Check,
  ChevronLeft,
  ChevronRight,
  Download,
  ExternalLink,
  FileText,
  RefreshCw,
  XCircle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import {
  createOrderInvoice,
  createOrderInvoicePayment,
  cancelOrderInvoicePayment,
  getAdminInvoices,
  getInvoiceConfig,
  getInvoiceEligibleOrders,
  getUserInvoices,
  updateInvoiceRecord,
} from './api'
import { OrderInvoiceRequest } from './components/order-invoice-request'
import { openInvoicePaymentCheckout } from './payment'
import type {
  InvoiceConfig,
  InvoiceEligibleOrder,
  InvoiceRecord,
  InvoiceRequest,
  InvoiceStatus,
} from './types'

interface InvoicesProps {
  admin?: boolean
}

const PAGE_SIZE = 10

function formatCny(amount: number) {
  return `¥${Number(amount || 0).toFixed(2)}`
}

function formatTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

function getStatusConfig(
  status: InvoiceStatus,
  t: (key: string) => string
): { label: string; variant: StatusBadgeProps['variant'] } {
  switch (status) {
    case 'payment_pending':
      return { label: t('Pending payment'), variant: 'warning' }
    case 'issued':
      return { label: t('Issued'), variant: 'success' }
    case 'closed':
      return { label: t('Closed'), variant: 'neutral' }
    default:
      return { label: t('Pending issue'), variant: 'warning' }
  }
}

function getInvoiceTypeLabel(type: string, t: (key: string) => string) {
  return type === 'company' ? t('Company invoice') : t('Personal invoice')
}

function getInvoiceKindLabel(kind: string, t: (key: string) => string) {
  return kind === 'special'
    ? t('VAT special invoice')
    : t('VAT general invoice')
}

function getSourceLabel(type: string, t: (key: string) => string) {
  switch (type) {
    case 'subscription':
      return t('Subscription purchase')
    case 'batch':
      return t('Combined orders')
    default:
      return t('Top-up')
  }
}

export function Invoices({ admin = false }: InvoicesProps) {
  const { t } = useTranslation()
  const [records, setRecords] = useState<InvoiceRecord[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<InvoiceStatus | 'all'>('pending')
  const [loading, setLoading] = useState(false)
  const [invoiceConfig, setInvoiceConfig] = useState<InvoiceConfig | null>(null)
  const [invoiceOrders, setInvoiceOrders] = useState<InvoiceEligibleOrder[]>([])
  const [ordersLoading, setOrdersLoading] = useState(false)
  const [orderInvoiceSubmitting, setOrderInvoiceSubmitting] = useState(false)
  const [savingId, setSavingId] = useState<number | null>(null)
  const [cancelingId, setCancelingId] = useState<number | null>(null)
  const [editing, setEditing] = useState<
    Record<
      number,
      { download_url: string; status: InvoiceStatus; admin_remark: string }
    >
  >({})

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const loadInvoices = useCallback(async () => {
    setLoading(true)
    try {
      const response = admin
        ? await getAdminInvoices(
            page,
            PAGE_SIZE,
            status === 'all' ? undefined : status
          )
        : await getUserInvoices(page, PAGE_SIZE)
      if (response.success && response.data) {
        const items = response.data.items || []
        setRecords(items)
        setTotal(response.data.total || 0)
        setEditing((prev) => {
          const next = { ...prev }
          for (const record of items) {
            if (!next[record.id]) {
              next[record.id] = {
                download_url: record.download_url || '',
                status:
                  record.status === 'payment_pending'
                    ? 'closed'
                    : record.status || 'pending',
                admin_remark: record.admin_remark || '',
              }
            }
          }
          return next
        })
      } else {
        toast.error(response.message || t('Failed to load invoices'))
      }
    } catch {
      toast.error(t('Failed to load invoices'))
    } finally {
      setLoading(false)
    }
  }, [admin, page, status, t])

  const loadInvoiceOrders = useCallback(async () => {
    if (admin) return
    setOrdersLoading(true)
    try {
      const [configResponse, ordersResponse] = await Promise.all([
        getInvoiceConfig(),
        getInvoiceEligibleOrders(),
      ])
      if (!configResponse.success || !configResponse.data) {
        throw new Error(
          configResponse.message || t('Failed to load invoice configuration')
        )
      }
      if (!ordersResponse.success || !ordersResponse.data) {
        throw new Error(
          ordersResponse.message || t('Failed to load invoiceable orders')
        )
      }
      setInvoiceConfig(configResponse.data)
      setInvoiceOrders(ordersResponse.data.orders || [])
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to load invoiceable orders')
      )
      setInvoiceOrders([])
    } finally {
      setOrdersLoading(false)
    }
  }, [admin, t])

  useEffect(() => {
    void loadInvoices()
  }, [loadInvoices])

  useEffect(() => {
    void loadInvoiceOrders()
  }, [loadInvoiceOrders])

  const pageRange = useMemo(() => {
    if (total === 0) return '0-0'
    const start = (page - 1) * PAGE_SIZE + 1
    return `${start}-${Math.min(page * PAGE_SIZE, total)}`
  }, [page, total])

  const saveInvoice = async (record: InvoiceRecord) => {
    const draft = editing[record.id]
    if (!draft) return
    setSavingId(record.id)
    try {
      const response = await updateInvoiceRecord(record.id, draft)
      if (response.success) {
        toast.success(t('Saved successfully'))
        await loadInvoices()
      } else {
        toast.error(response.message || t('Save failed'))
      }
    } catch {
      toast.error(t('Save failed, please retry'))
    } finally {
      setSavingId(null)
    }
  }

  const cancelInvoicePayment = async (record: InvoiceRecord) => {
    if (
      record.status !== 'payment_pending' ||
      !window.confirm(
        t(
          'Cancel this unpaid invoice request only if you have not completed payment.'
        )
      )
    ) {
      return
    }
    setCancelingId(record.id)
    try {
      const response = await cancelOrderInvoicePayment(record.source_id)
      if (response.success) {
        toast.success(t('Pending payment request canceled'))
        await Promise.all([loadInvoices(), loadInvoiceOrders()])
      } else {
        toast.error(response.message || t('Failed to cancel invoice request'))
      }
    } catch {
      toast.error(t('Failed to cancel invoice request'))
    } finally {
      setCancelingId(null)
    }
  }

  const submitOrderInvoice = async (
    orders: InvoiceEligibleOrder[],
    invoice: InvoiceRequest,
    paymentMethod: string,
    tradeType?: string
  ) => {
    setOrderInvoiceSubmitting(true)
    try {
      const request = {
        orders: orders.map((order) => ({
          source_type: order.source_type,
          source_id: order.source_id,
        })),
        invoice: { ...invoice, required: true },
      }
      if (paymentMethod === 'balance') {
        const response = await createOrderInvoice(request)
        if (!response.success || !response.data) {
          toast.error(response.message || t('Failed to submit invoice request'))
          return false
        }
        toast.success(t('Invoice request submitted'))
        await Promise.all([loadInvoices(), loadInvoiceOrders()])
        return true
      }

      const response = await createOrderInvoicePayment({
        ...request,
        payment_method: paymentMethod,
        ...(tradeType ? { trade_type: tradeType } : {}),
      })
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to submit invoice request'))
        return false
      }
      if (response.data.completed) {
        toast.success(t('Invoice request submitted'))
      } else if (
        !response.data.checkout ||
        !openInvoicePaymentCheckout(response.data.checkout)
      ) {
        toast.error(t('Failed to open payment checkout'))
        await Promise.all([loadInvoices(), loadInvoiceOrders()])
        return true
      } else {
        toast.success(
          t(
            'Pending payment request created. It will automatically move to pending issue after successful payment.'
          )
        )
      }
      await Promise.all([loadInvoices(), loadInvoiceOrders()])
      return true
    } catch {
      toast.error(t('Failed to submit invoice request'))
      return false
    } finally {
      setOrderInvoiceSubmitting(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {admin ? t('Invoice Management') : t('Invoice Center')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-4'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <p className='text-muted-foreground text-sm'>
                {admin
                  ? t('Review invoice requests and attach issued invoice URLs.')
                  : t('View invoice requests created from paid orders.')}
              </p>
            </div>
            <div className='flex items-center gap-2'>
              {admin && (
                <Select
                  items={[
                    {
                      value: 'payment_pending',
                      label: t('Pending payment'),
                    },
                    { value: 'pending', label: t('Pending issue') },
                    { value: 'issued', label: t('Issued') },
                    { value: 'closed', label: t('Closed') },
                    { value: 'all', label: t('All') },
                  ]}
                  value={status}
                  onValueChange={(value) => {
                    if (!value) return
                    setPage(1)
                    setStatus(value as InvoiceStatus | 'all')
                  }}
                >
                  <SelectTrigger className='h-9 w-36'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='payment_pending'>
                        {t('Pending payment')}
                      </SelectItem>
                      <SelectItem value='pending'>
                        {t('Pending issue')}
                      </SelectItem>
                      <SelectItem value='issued'>{t('Issued')}</SelectItem>
                      <SelectItem value='closed'>{t('Closed')}</SelectItem>
                      <SelectItem value='all'>{t('All')}</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              )}
              <Button
                variant='outline'
                size='sm'
                onClick={() => loadInvoices()}
                disabled={loading}
              >
                <RefreshCw
                  className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`}
                />
                {t('Refresh')}
              </Button>
            </div>
          </div>

          {!admin && (
            <OrderInvoiceRequest
              config={invoiceConfig}
              orders={invoiceOrders}
              loading={ordersLoading}
              submitting={orderInvoiceSubmitting}
              onRefresh={loadInvoiceOrders}
              onSubmit={submitOrderInvoice}
            />
          )}

          {loading ? (
            <div className='space-y-3'>
              {Array.from({ length: 4 }).map((_, index) => (
                <Skeleton key={index} className='h-44 w-full' />
              ))}
            </div>
          ) : records.length === 0 ? (
            <Card>
              <CardContent className='flex h-64 flex-col items-center justify-center text-center'>
                <FileText className='text-muted-foreground mb-3 h-8 w-8' />
                <p className='text-sm font-medium'>
                  {t('No invoice records found')}
                </p>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {admin
                    ? t('Invoice requests will appear here after users pay.')
                    : t('Paid orders with invoice requests will appear here.')}
                </p>
              </CardContent>
            </Card>
          ) : (
            <div className='space-y-3'>
              {records.map((record) => {
                const statusConfig = getStatusConfig(record.status, t)
                const draft = editing[record.id] || {
                  download_url: record.download_url || '',
                  status:
                    record.status === 'payment_pending'
                      ? 'closed'
                      : record.status || 'pending',
                  admin_remark: record.admin_remark || '',
                }

                return (
                  <Card key={record.id}>
                    <CardHeader className='border-b pb-3'>
                      <div className='min-w-0'>
                        <CardTitle className='flex flex-wrap items-center gap-2'>
                          <span className='truncate'>
                            {record.title || t('Invoice')}
                          </span>
                          <StatusBadge
                            label={statusConfig.label}
                            variant={statusConfig.variant}
                            copyable={false}
                          />
                        </CardTitle>
                        <p className='text-muted-foreground mt-1 truncate text-xs'>
                          {getSourceLabel(record.source_type, t)} ·{' '}
                          {record.source_id}
                        </p>
                      </div>
                      {record.download_url && (
                        <CardAction>
                          <Button
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              window.open(record.download_url, '_blank')
                            }
                          >
                            <Download className='mr-2 h-4 w-4' />
                            {t('Download')}
                          </Button>
                        </CardAction>
                      )}
                    </CardHeader>
                    <CardContent className='space-y-4'>
                      <div className='grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4'>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Invoice kind')}
                          </Label>
                          <div>
                            {getInvoiceKindLabel(record.invoice_kind, t)}
                          </div>
                        </div>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Invoice title type')}
                          </Label>
                          <div>
                            {getInvoiceTypeLabel(record.invoice_type, t)}
                          </div>
                        </div>
                        {admin && (
                          <div>
                            <Label className='text-muted-foreground text-xs'>
                              {t('User ID')}
                            </Label>
                            <div>{record.user_id}</div>
                          </div>
                        )}
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Invoice amount')}
                          </Label>
                          <div>{formatCny(record.base_amount)}</div>
                        </div>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Invoice fee')}
                          </Label>
                          <div>{formatCny(record.fee_amount)}</div>
                        </div>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Total')}
                          </Label>
                          <div>{formatCny(record.total_amount)}</div>
                        </div>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Created at')}
                          </Label>
                          <div>{formatTime(record.create_time)}</div>
                        </div>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Issued at')}
                          </Label>
                          <div>{formatTime(record.issued_time)}</div>
                        </div>
                        <div>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Payment Method')}
                          </Label>
                          <div>{record.payment_method || '-'}</div>
                        </div>
                      </div>

                      {(record.tax_no || record.email || record.phone) && (
                        <div className='grid gap-3 text-sm sm:grid-cols-3'>
                          {record.tax_no && (
                            <div>
                              <Label className='text-muted-foreground text-xs'>
                                {t('Tax number')}
                              </Label>
                              <div className='break-all'>{record.tax_no}</div>
                            </div>
                          )}
                          {record.email && (
                            <div>
                              <Label className='text-muted-foreground text-xs'>
                                {t('Email')}
                              </Label>
                              <div className='break-all'>{record.email}</div>
                            </div>
                          )}
                          {record.phone && (
                            <div>
                              <Label className='text-muted-foreground text-xs'>
                                {t('Phone')}
                              </Label>
                              <div>{record.phone}</div>
                            </div>
                          )}
                        </div>
                      )}

                      {record.remark && (
                        <div className='text-sm'>
                          <Label className='text-muted-foreground text-xs'>
                            {t('Remark')}
                          </Label>
                          <p className='mt-1 whitespace-pre-wrap'>
                            {record.remark}
                          </p>
                        </div>
                      )}

                      {admin ? (
                        <div className='grid gap-3 border-t pt-4'>
                          <div className='grid gap-3 lg:grid-cols-[1fr_160px]'>
                            <div className='grid gap-2'>
                              <Label>{t('Invoice download URL')}</Label>
                              <Input
                                value={draft.download_url}
                                onChange={(event) =>
                                  setEditing((prev) => ({
                                    ...prev,
                                    [record.id]: {
                                      ...draft,
                                      download_url: event.target.value,
                                    },
                                  }))
                                }
                                placeholder='https://example.com/invoice.pdf'
                              />
                            </div>
                            <div className='grid gap-2'>
                              <Label>{t('Status')}</Label>
                              <Select
                                items={[
                                  {
                                    value: 'pending',
                                    label: t('Pending issue'),
                                  },
                                  { value: 'issued', label: t('Issued') },
                                  { value: 'closed', label: t('Closed') },
                                ]}
                                value={draft.status}
                                onValueChange={(value) => {
                                  if (!value) return
                                  setEditing((prev) => ({
                                    ...prev,
                                    [record.id]: {
                                      ...draft,
                                      status: value as InvoiceStatus,
                                    },
                                  }))
                                }}
                              >
                                <SelectTrigger>
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent alignItemWithTrigger={false}>
                                  <SelectGroup>
                                    <SelectItem value='pending'>
                                      {t('Pending issue')}
                                    </SelectItem>
                                    <SelectItem value='issued'>
                                      {t('Issued')}
                                    </SelectItem>
                                    <SelectItem value='closed'>
                                      {t('Closed')}
                                    </SelectItem>
                                  </SelectGroup>
                                </SelectContent>
                              </Select>
                            </div>
                          </div>
                          <div className='grid gap-2'>
                            <Label>{t('Admin remark')}</Label>
                            <Textarea
                              value={draft.admin_remark}
                              onChange={(event) =>
                                setEditing((prev) => ({
                                  ...prev,
                                  [record.id]: {
                                    ...draft,
                                    admin_remark: event.target.value,
                                  },
                                }))
                              }
                              rows={2}
                            />
                          </div>
                          <div className='flex justify-end'>
                            <Button
                              onClick={() => saveInvoice(record)}
                              disabled={savingId === record.id}
                            >
                              {savingId === record.id ? (
                                <RefreshCw className='mr-2 h-4 w-4 animate-spin' />
                              ) : (
                                <Check className='mr-2 h-4 w-4' />
                              )}
                              {t('Save')}
                            </Button>
                          </div>
                        </div>
                      ) : record.status === 'payment_pending' ? (
                        <div className='flex justify-end border-t pt-4'>
                          <Button
                            variant='outline'
                            onClick={() => cancelInvoicePayment(record)}
                            disabled={cancelingId === record.id}
                          >
                            <XCircle className='mr-2 h-4 w-4' />
                            {t('Cancel pending payment')}
                          </Button>
                        </div>
                      ) : record.download_url ? (
                        <div className='flex justify-end border-t pt-4'>
                          <Button
                            variant='outline'
                            onClick={() =>
                              window.open(record.download_url, '_blank')
                            }
                          >
                            <ExternalLink className='mr-2 h-4 w-4' />
                            {t('Open invoice')}
                          </Button>
                        </div>
                      ) : null}
                    </CardContent>
                  </Card>
                )
              })}
            </div>
          )}

          {!loading && total > 0 && (
            <div className='flex flex-col items-center gap-3 border-t pt-4 sm:flex-row sm:justify-between'>
              <div className='text-muted-foreground text-sm'>
                {t('Showing')} {pageRange} {t('of')} {total}
              </div>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                  disabled={page <= 1}
                  className='h-8 w-8 p-0'
                >
                  <ChevronLeft className='h-4 w-4' />
                </Button>
                <span className='text-muted-foreground text-sm'>
                  {page}/{totalPages}
                </span>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    setPage((prev) => Math.min(totalPages, prev + 1))
                  }
                  disabled={page >= totalPages}
                  className='h-8 w-8 p-0'
                >
                  <ChevronRight className='h-4 w-4' />
                </Button>
              </div>
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
