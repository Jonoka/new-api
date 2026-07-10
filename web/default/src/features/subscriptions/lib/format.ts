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
import type { TFunction } from 'i18next'
import dayjs from '@/lib/dayjs'
import { formatQuota } from '@/lib/format'
import type { SubscriptionPlan } from '../types'

export function normalizePlanCurrency(currency?: string): 'USD' | 'CNY' {
  return currency === 'CNY' ? 'CNY' : 'USD'
}

export function formatPlanCurrencyAmount(
  amount: number,
  currency?: string
): string {
  const normalizedCurrency = normalizePlanCurrency(currency)
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: normalizedCurrency,
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(Number(amount || 0))
  } catch {
    return `${normalizedCurrency} ${Number(amount || 0).toFixed(2)}`
  }
}

export function formatDuration(
  plan: Partial<SubscriptionPlan>,
  t: TFunction
): string {
  const unit = plan?.duration_unit || 'month'
  const value = plan?.duration_value || 1
  const unitLabels: Record<string, string> = {
    year: t('years'),
    month: t('months'),
    day: t('days'),
    hour: t('hours'),
    custom: t('Custom (seconds)'),
  }
  if (unit === 'custom') {
    const seconds = plan?.custom_seconds || 0
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    return `${seconds} ${t('seconds')}`
  }
  return `${value} ${unitLabels[unit] || unit}`
}

export function formatResetPeriod(
  plan: Partial<SubscriptionPlan>,
  t: TFunction
): string {
  const period = plan?.quota_reset_period || 'never'
  if (period === 'daily') return t('Daily')
  if (period === 'weekly') return t('Weekly')
  if (period === 'monthly') return t('Monthly')
  if (period === 'custom') {
    const seconds = Number(plan?.quota_reset_custom_seconds || 0)
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    if (seconds >= 60) return `${Math.floor(seconds / 60)} ${t('minutes')}`
    return `${seconds} ${t('seconds')}`
  }
  return t('No Reset')
}

function getDurationSeconds(plan: Partial<SubscriptionPlan>): number {
  const value = Number(plan?.duration_value || 1)
  switch (plan?.duration_unit || 'month') {
    case 'year':
      return value * 365 * 86400
    case 'month':
      return value * 30 * 86400
    case 'day':
      return value * 86400
    case 'hour':
      return value * 3600
    case 'custom':
      return Number(plan?.custom_seconds || 0)
    default:
      return 0
  }
}

function getResetSeconds(plan: Partial<SubscriptionPlan>): number {
  switch (plan?.quota_reset_period || 'never') {
    case 'daily':
      return 86400
    case 'weekly':
      return 7 * 86400
    case 'monthly':
      return 30 * 86400
    case 'custom':
      return Number(plan?.quota_reset_custom_seconds || 0)
    default:
      return 0
  }
}

export function getQuotaResetCycleCount(
  plan: Partial<SubscriptionPlan>
): number {
  const durationSeconds = getDurationSeconds(plan)
  const resetSeconds = getResetSeconds(plan)
  if (durationSeconds <= 0 || resetSeconds <= 0) return 1
  return Math.max(1, Math.floor(durationSeconds / resetSeconds))
}

export function formatPlanQuotaAllowance(
  plan: Partial<SubscriptionPlan>,
  t: TFunction
): string[] {
  const totalAmount = Number(plan?.total_amount || 0)
  if (totalAmount <= 0) {
    return [`${t('Total Quota')}: ${t('Unlimited')}`]
  }

  const resetPeriod = plan?.quota_reset_period || 'never'
  if (resetPeriod === 'never') {
    return [`${t('Total Quota')}: ${formatQuota(totalAmount)}`]
  }

  const cycles = getQuotaResetCycleCount(plan)
  const estimatedTotal = totalAmount * cycles
  return [
    `${t('Per Reset Quota')}: ${formatQuota(totalAmount)}`,
    `${t('Estimated Quota During Validity')}: ${t('about')} ${formatQuota(estimatedTotal)} (${cycles} ${t('cycles')})`,
  ]
}

export function formatTimestamp(ts: number): string {
  if (!ts) return '-'
  return dayjs(ts * 1000).format('YYYY-MM-DD HH:mm:ss')
}
