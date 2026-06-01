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
import { Edit, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getAdminPlans } from '@/features/subscriptions/api'
import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import {
  createPromoCode,
  deletePromoCode,
  getPromoCodes,
  updatePromoCode,
  updatePromoCodeStatus,
} from '../api'
import type { PromoCode, PromoCodeFormData } from '../types'

const PROMO_ENABLED = 1
const PROMO_DISABLED = 2

type PromoFormState = {
  id?: number
  name: string
  code: string
  discount_type: 'percent' | 'fixed'
  discount_value: number
  applies_to_topup: boolean
  applies_to_all_subscription: boolean
  subscription_plan_ids: number[]
  max_redeem_count: number
  expired_time_text: string
}

const defaultForm: PromoFormState = {
  name: '',
  code: '',
  discount_type: 'percent',
  discount_value: 10,
  applies_to_topup: true,
  applies_to_all_subscription: false,
  subscription_plan_ids: [],
  max_redeem_count: 0,
  expired_time_text: '',
}

function parsePlanIds(raw: string): number[] {
  return raw
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((id) => Number.isFinite(id) && id > 0)
}

function toForm(row: PromoCode): PromoFormState {
  return {
    id: row.id,
    name: row.name,
    code: row.code,
    discount_type: row.discount_type,
    discount_value:
      row.discount_type === 'fixed'
        ? quotaUnitsToDollars(row.discount_value)
        : row.discount_value,
    applies_to_topup: row.applies_to_topup,
    applies_to_all_subscription: row.applies_to_all_subscription,
    subscription_plan_ids: parsePlanIds(row.subscription_plan_ids || ''),
    max_redeem_count: row.max_redeem_count || 0,
    expired_time_text:
      row.expired_time > 0
        ? new Date(row.expired_time * 1000).toISOString().slice(0, 16)
        : '',
  }
}

function buildPayload(form: PromoFormState): PromoCodeFormData {
  return {
    name: form.name.trim(),
    code: form.code.trim().toUpperCase(),
    discount_type: form.discount_type,
    discount_value:
      form.discount_type === 'fixed'
        ? parseQuotaFromDollars(form.discount_value)
        : Math.round(form.discount_value),
    applies_to_topup: form.applies_to_topup,
    applies_to_all_subscription: form.applies_to_all_subscription,
    subscription_plan_ids: form.subscription_plan_ids.join(','),
    max_redeem_count: form.max_redeem_count,
    expired_time: form.expired_time_text
      ? Math.floor(new Date(form.expired_time_text).getTime() / 1000)
      : 0,
  }
}

function formatPromoDiscount(row: PromoCode) {
  if (row.discount_type === 'percent') return `${row.discount_value}%`
  return quotaUnitsToDollars(row.discount_value).toFixed(2)
}

export function PromoCodesPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<PromoFormState>(defaultForm)
  const pageSize = 20

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['promo-codes', page],
    queryFn: async () => {
      const result = await getPromoCodes({ p: page, page_size: pageSize })
      return {
        items: result.data?.items || [],
        total: result.data?.total || 0,
      }
    },
  })

  const { data: planData } = useQuery({
    queryKey: ['promo-code-subscription-plans'],
    queryFn: async () => {
      const result = await getAdminPlans()
      return result.data || []
    },
  })

  const plans = useMemo(() => planData?.map((item) => item.plan) || [], [
    planData,
  ])
  const items = data?.items || []
  const total = data?.total || 0
  const pageCount = Math.max(1, Math.ceil(total / pageSize))

  useEffect(() => {
    if (page > pageCount) setPage(pageCount)
  }, [page, pageCount])

  const openCreate = () => {
    setForm(defaultForm)
    setOpen(true)
  }

  const openEdit = (row: PromoCode) => {
    setForm(toForm(row))
    setOpen(true)
  }

  const submit = async () => {
    if (!form.name.trim()) {
      toast.error(t('Name is required'))
      return
    }
    if (!form.code.trim()) {
      toast.error(t('Promo code is required'))
      return
    }
    if (form.discount_value <= 0) {
      toast.error(t('Discount value must be greater than 0'))
      return
    }
    if (
      form.discount_type === 'percent' &&
      Math.round(form.discount_value) > 100
    ) {
      toast.error(t('Discount percentage cannot exceed 100'))
      return
    }
    if (
      !form.applies_to_topup &&
      !form.applies_to_all_subscription &&
      form.subscription_plan_ids.length === 0
    ) {
      toast.error(t('Please select at least one promo code scope'))
      return
    }
    const payload = buildPayload(form)
    const result = form.id
      ? await updatePromoCode({ ...payload, id: form.id })
      : await createPromoCode(payload)
    if (result.success) {
      toast.success(
        t(
          form.id
            ? 'Promo code updated successfully'
            : 'Promo code created successfully'
        )
      )
      setOpen(false)
      refetch()
    } else {
      toast.error(result.message || t('Failed to save promo code'))
    }
  }

  const toggleStatus = async (row: PromoCode) => {
    const nextStatus =
      row.status === PROMO_ENABLED ? PROMO_DISABLED : PROMO_ENABLED
    const result = await updatePromoCodeStatus(row.id, nextStatus)
    if (result.success) refetch()
  }

  const remove = async (row: PromoCode) => {
    if (!window.confirm(t('Delete this promo code?'))) return
    const result = await deletePromoCode(row.id)
    if (result.success) {
      toast.success(t('Promo code deleted successfully'))
      refetch()
    }
  }

  const togglePlan = (planId: number) => {
    setForm((prev) => {
      const exists = prev.subscription_plan_ids.includes(planId)
      return {
        ...prev,
        subscription_plan_ids: exists
          ? prev.subscription_plan_ids.filter((id) => id !== planId)
          : [...prev.subscription_plan_ids, planId],
      }
    })
  }

  const discountTypeItems = [
    { value: 'percent', label: t('Percentage') },
    { value: 'fixed', label: t('Fixed quota') },
  ]

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between gap-3'>
        <Input
          className='max-w-xs'
          placeholder={t('Promo code management')}
          disabled
        />
        <Button size='sm' onClick={openCreate}>
          <Plus className='h-4 w-4' />
          {t('Create Promo Code')}
        </Button>
      </div>

      <div className='rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('ID')}</TableHead>
              <TableHead>{t('Name')}</TableHead>
              <TableHead>{t('Promo Code')}</TableHead>
              <TableHead>{t('Discount')}</TableHead>
              <TableHead>{t('Scope')}</TableHead>
              <TableHead>{t('Redeemed')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className='h-24 text-center'>
                  {isFetching ? t('Loading...') : t('No Promo Codes Found')}
                </TableCell>
              </TableRow>
            ) : (
              items.map((row) => (
                <TableRow key={row.id}>
                  <TableCell>
                    <TableId value={row.id} />
                  </TableCell>
                  <TableCell className='font-medium'>{row.name}</TableCell>
                  <TableCell className='font-mono'>{row.code}</TableCell>
                  <TableCell>{formatPromoDiscount(row)}</TableCell>
                  <TableCell>
                    {row.applies_to_topup
                      ? t('Balance recharge')
                      : row.applies_to_all_subscription
                        ? t('All subscriptions')
                        : t('Selected subscriptions')}
                  </TableCell>
                  <TableCell className='font-mono'>
                    {row.redeemed_count}/{row.max_redeem_count || t('Unlimited')}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      label={t(row.status === PROMO_ENABLED ? 'Enabled' : 'Disabled')}
                      variant={row.status === PROMO_ENABLED ? 'success' : 'neutral'}
                      copyable={false}
                    />
                  </TableCell>
                  <TableCell>
                    <div className='flex justify-end gap-1'>
                      <Button
                        size='icon'
                        variant='ghost'
                        onClick={() => openEdit(row)}
                        aria-label={t('Edit')}
                      >
                        <Edit className='h-4 w-4' />
                      </Button>
                      <Button size='sm' variant='ghost' onClick={() => toggleStatus(row)}>
                        {t(row.status === PROMO_ENABLED ? 'Disable' : 'Enable')}
                      </Button>
                      <Button
                        size='icon'
                        variant='ghost'
                        onClick={() => remove(row)}
                        aria-label={t('Delete')}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <div className='flex justify-end gap-2'>
        <Button
          size='sm'
          variant='outline'
          disabled={page <= 1}
          onClick={() => setPage((value) => Math.max(1, value - 1))}
        >
          {t('Previous')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          disabled={page >= pageCount}
          onClick={() => setPage((value) => Math.min(pageCount, value + 1))}
        >
          {t('Next')}
        </Button>
      </div>

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className='sm:max-w-[600px]'>
          <SheetHeader>
            <SheetTitle>
              {t(form.id ? 'Update Promo Code' : 'Create Promo Code')}
            </SheetTitle>
          </SheetHeader>
          <div className='space-y-4 overflow-y-auto py-4'>
            <Input
              value={form.name}
              placeholder={t('Name')}
              onChange={(event) =>
                setForm((prev) => ({ ...prev, name: event.target.value }))
              }
            />
            <Input
              value={form.code}
              placeholder={t('Promo Code')}
              onChange={(event) =>
                setForm((prev) => ({ ...prev, code: event.target.value }))
              }
            />
            <Select
              items={discountTypeItems}
              value={form.discount_type}
              onValueChange={(value) => {
                if (value !== 'percent' && value !== 'fixed') return
                setForm((prev) => ({
                  ...prev,
                  discount_type: value,
                }))
              }}
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Discount Type')} />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value='percent'>{t('Percentage')}</SelectItem>
                  <SelectItem value='fixed'>{t('Fixed quota')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              type='number'
              min='0'
              step={form.discount_type === 'percent' ? 1 : 0.01}
              value={form.discount_value}
              placeholder={
                form.discount_type === 'percent'
                  ? t('Discount Value')
                  : t('Fixed quota discount amount')
              }
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  discount_value: Number(event.target.value) || 0,
                }))
              }
            />
            <Input
              type='number'
              min='0'
              value={form.max_redeem_count}
              placeholder={t('Usage limit, 0 for unlimited')}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  max_redeem_count: Number(event.target.value) || 0,
                }))
              }
            />
            <Input
              type='datetime-local'
              value={form.expired_time_text}
              onChange={(event) =>
                setForm((prev) => ({
                  ...prev,
                  expired_time_text: event.target.value,
                }))
              }
            />
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.applies_to_topup}
                onCheckedChange={(checked) =>
                  setForm((prev) => ({
                    ...prev,
                    applies_to_topup: !!checked,
                  }))
                }
              />
              {t('Balance recharge')}
            </label>
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.applies_to_all_subscription}
                onCheckedChange={(checked) =>
                  setForm((prev) => ({
                    ...prev,
                    applies_to_all_subscription: !!checked,
                    subscription_plan_ids: checked
                      ? []
                      : prev.subscription_plan_ids,
                  }))
                }
              />
              {t('All subscriptions')}
            </label>
            {!form.applies_to_all_subscription && (
              <div className='space-y-2 rounded-md border p-3'>
                <div className='text-sm font-medium'>
                  {t('Selected subscriptions')}
                </div>
                <div className='grid gap-2 sm:grid-cols-2'>
                  {plans.map((plan) => (
                    <label key={plan.id} className='flex items-center gap-2 text-sm'>
                      <Checkbox
                        checked={form.subscription_plan_ids.includes(plan.id)}
                        onCheckedChange={() => togglePlan(plan.id)}
                      />
                      <span className='truncate'>{plan.title}</span>
                    </label>
                  ))}
                </div>
              </div>
            )}
          </div>
          <SheetFooter>
            <SheetClose render={<Button variant='outline' />}>
              {t('Close')}
            </SheetClose>
            <Button onClick={submit}>{t('Save changes')}</Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  )
}
