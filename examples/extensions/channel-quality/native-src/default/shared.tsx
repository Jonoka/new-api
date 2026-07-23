/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { ReactNode } from 'react'
import { Alert02Icon, Database01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import type { AnalyticsMeta, StatusCodeItem } from './types'

export const PAGE_SIZE = 30
export const STABILITY_WINDOWS = [900, 3600, 21600, 86400, 604800]

export function formatInteger(value: number | null | undefined) {
  return Number(value ?? 0).toLocaleString()
}

export function formatCompact(value: number | null | undefined) {
  const number = Number(value ?? 0)
  return new Intl.NumberFormat(undefined, {
    notation: Math.abs(number) >= 1000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(number)
}

export function formatPercent(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value))
    return '-'
  const percent = Math.abs(value) <= 1 ? value * 100 : value
  return `${percent.toFixed(1)}%`
}

export function percentValue(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, Math.abs(value) <= 1 ? value * 100 : value))
}

export function formatDuration(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value))
    return '-'
  if (value < 1000) return `${Math.round(value)} ms`
  return `${(value / 1000).toFixed(value < 10_000 ? 2 : 1)} s`
}

export function formatDateTime(timestamp: number | null | undefined) {
  if (!timestamp) return '-'
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function formatMoney(microUsd: number | null | undefined) {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(Number(microUsd ?? 0) / 1_000_000)
}

export function getStatusLabel(item: StatusCodeItem) {
  if (!item.status_present) return item.label || '-'
  if (item.status_code === 0) return item.label || 'No response'
  return item.label || String(item.status_code)
}

export function statusBadgeVariant(
  code: number,
  present = true
): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (!present || code === 0 || code >= 500) return 'destructive'
  if (code >= 400) return 'secondary'
  if (code >= 200 && code < 300) return 'default'
  return 'outline'
}

export function windowLabel(seconds: number, t: TFunction) {
  const labels: Record<number, string> = {
    900: t('15 minutes'),
    3600: t('1 hour'),
    21600: t('6 hours'),
    86400: t('24 hours'),
    604800: t('7 days'),
  }
  return labels[seconds] ?? `${formatCompact(seconds)} ${t('seconds')}`
}

export function QualityNotice({ meta }: { meta?: AnalyticsMeta }) {
  const { t } = useTranslation()
  const messages: ReactNode[] = []
  if (!meta) return null
  if (meta.partial) messages.push(t('Current range contains partial data'))
  if (meta.invalid_sample_count > 0) {
    messages.push(
      t('{{count}} invalid samples excluded', {
        count: formatInteger(meta.invalid_sample_count),
      })
    )
  }
  if (meta.dimension_overflow_count > 0) {
    messages.push(
      t('{{count}} dimensions merged into Other', {
        count: formatInteger(meta.dimension_overflow_count),
      })
    )
  }
  if (meta.dimension_hash_collision_count > 0) {
    messages.push(
      t('{{count}} dimension hash collisions isolated', {
        count: formatInteger(meta.dimension_hash_collision_count),
      })
    )
  }
  if (meta.dropped_metric_event_count > 0) {
    messages.push(
      t('{{count}} metric events dropped', {
        count: formatInteger(meta.dropped_metric_event_count),
      })
    )
  }
  if (meta.dropped_failure_event_count > 0) {
    messages.push(
      t('{{count}} failure details dropped', {
        count: formatInteger(meta.dropped_failure_event_count),
      })
    )
  }
  if (meta.runtime_pending_batch_count > 0) {
    messages.push(
      t('{{count}} metric batches pending', {
        count: formatInteger(meta.runtime_pending_batch_count),
      })
    )
  }
  if (
    meta.runtime_last_flush_error_at > 0 &&
    meta.runtime_last_flush_error_at > meta.last_flushed_at
  ) {
    messages.push(
      t('Metric write last failed at {{time}}', {
        time: formatDateTime(meta.runtime_last_flush_error_at),
      })
    )
  }
  if (
    Array.isArray(meta.uncovered_channel_types) &&
    meta.uncovered_channel_types.length > 0
  ) {
    messages.push(
      t('Some channel types are not fully covered: {{types}}', {
        types: meta.uncovered_channel_types.join(', '),
      })
    )
  }
  if (meta.detail_available === false) {
    messages.push(t('Detailed failure records are currently unavailable'))
  }
  if (meta.backfill && meta.backfill.status !== 'completed') {
    messages.push(
      t('Historical backfill: {{status}}', { status: meta.backfill.status })
    )
  }
  if (!messages.length) return null

  return (
    <Alert className='py-2'>
      <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
      <AlertTitle>{t('Data quality notice')}</AlertTitle>
      <AlertDescription className='flex flex-wrap gap-x-3 gap-y-0.5 text-xs'>
        {messages.map((message, index) => (
          <span key={index}>{message}</span>
        ))}
      </AlertDescription>
    </Alert>
  )
}

export function ViewError({
  error,
  retry,
  t,
}: {
  error: Error
  retry: () => void
  t: TFunction
}) {
  return (
    <Alert variant='destructive' className='py-2'>
      <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
      <AlertTitle>{t('Failed to load analytics')}</AlertTitle>
      <AlertDescription className='flex flex-wrap items-center justify-between gap-3'>
        <span>{error.message}</span>
        <Button variant='outline' size='sm' onClick={retry}>
          {t('Retry')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}

export function ViewEmpty({
  reset,
  t,
  meta,
}: {
  reset?: () => void
  t: TFunction
  meta?: AnalyticsMeta
}) {
  return (
    <div className='flex flex-col gap-4'>
      <QualityNotice meta={meta} />
      <Empty className='bg-muted/10 min-h-48 border sm:min-h-56'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={Database01Icon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>
            {t('No call data matches the current filters')}
          </EmptyTitle>
          <EmptyDescription>
            {t('Expand the time range or clear filters to see more data.')}
          </EmptyDescription>
        </EmptyHeader>
        {reset && (
          <EmptyContent>
            <Button variant='outline' onClick={reset}>
              {t('Reset filters')}
            </Button>
          </EmptyContent>
        )}
      </Empty>
    </div>
  )
}

export function ViewSkeleton() {
  return (
    <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
      {Array.from({ length: 8 }).map((_, index) => (
        <Skeleton key={index} className='h-24 w-full' />
      ))}
      <Skeleton className='h-64 w-full md:col-span-2 xl:col-span-4' />
    </div>
  )
}

export function PageControls({
  page,
  total,
  pageSize = PAGE_SIZE,
  onPage,
  t,
}: {
  page: number
  total: number
  pageSize?: number
  onPage: (page: number) => void
  t: TFunction
}) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  return (
    <div className='bg-muted/20 flex flex-wrap items-center justify-between gap-2 border-t px-3 py-2'>
      <span className='text-muted-foreground text-xs tabular-nums'>
        {t('Total {{total}}, page {{page}} of {{pages}}', {
          total: formatInteger(total),
          page,
          pages,
        })}
      </span>
      <div className='flex gap-1.5'>
        <Button
          variant='outline'
          size='sm'
          disabled={page <= 1}
          onClick={() => onPage(page - 1)}
        >
          {t('Previous')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          disabled={page >= pages}
          onClick={() => onPage(page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </div>
  )
}

export function MetricBadge({
  value,
  label,
}: {
  value: string
  label: string
}) {
  return (
    <Badge variant='outline' className='font-normal'>
      {label} {value}
    </Badge>
  )
}
