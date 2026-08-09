/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from 'recharts'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import {
  createAnalyticsParams,
  getStatusCodes,
  getSummary,
  getTrend,
} from './api'
import {
  formatCompact,
  formatDuration,
  formatInteger,
  formatMoney,
  formatPercent,
  getStatusLabel,
  percentValue,
  QualityNotice,
  ViewEmpty,
  ViewError,
  ViewSkeleton,
} from './shared'
import type { AnalyticsFilters } from './types'

export function OverviewView({
  filters,
  refreshKey,
  onReset,
}: {
  filters: AnalyticsFilters
  refreshKey: number
  onReset: () => void
}) {
  const { t } = useTranslation()
  const baseParams = useMemo(
    () =>
      createAnalyticsParams(filters, {}, { includeStatus: false }).toString(),
    [filters]
  )
  const statusParams = useMemo(() => {
    const params = new URLSearchParams(baseParams)
    params.set('metric_scope', 'upstream_call')
    return params.toString()
  }, [baseParams])

  const summaryQuery = useQuery({
    queryKey: ['channel-observability', 'summary', baseParams, refreshKey],
    queryFn: () => getSummary(new URLSearchParams(baseParams)),
  })
  const trendQuery = useQuery({
    queryKey: ['channel-observability', 'trend', baseParams, refreshKey],
    queryFn: () => getTrend(new URLSearchParams(baseParams)),
  })
  const statusQuery = useQuery({
    queryKey: [
      'channel-observability',
      'overview-status',
      statusParams,
      refreshKey,
    ],
    queryFn: () => getStatusCodes(new URLSearchParams(statusParams)),
  })

  if (summaryQuery.isLoading || trendQuery.isLoading) return <ViewSkeleton />
  const error = summaryQuery.error ?? trendQuery.error
  if (error) {
    return (
      <ViewError
        error={error}
        retry={() => {
          void summaryQuery.refetch()
          void trendQuery.refetch()
          void statusQuery.refetch()
        }}
        t={t}
      />
    )
  }

  const summary = summaryQuery.data?.summary
  if (!summary) {
    return <ViewEmpty reset={onReset} t={t} meta={summaryQuery.data?.meta} />
  }
  const empty =
    summary.final_request_count === 0 &&
    summary.channel_attempt_count === 0 &&
    summary.upstream_call_count === 0
  if (empty) {
    return <ViewEmpty reset={onReset} t={t} meta={summaryQuery.data?.meta} />
  }

  const metrics = [
    {
      label: t('Client requests'),
      value: formatInteger(summary.final_request_count),
      detail: t('Counted once per logical request'),
    },
    {
      label: t('Channel attempts'),
      value: formatInteger(summary.channel_attempt_count),
      detail: t('{{count}} upstream calls', {
        count: formatInteger(summary.upstream_call_count),
      }),
    },
    {
      label: t('Client success rate'),
      value: formatPercent(summary.client_success_rate),
      detail: t('Final response delivered to the client'),
    },
    {
      label: t('Channel quality success rate'),
      value: formatPercent(summary.channel_quality_success_rate),
      detail: t('Only attributable channel samples'),
    },
    {
      label: t('Failed attempts'),
      value: formatInteger(summary.failed_attempt_count),
      detail: t('{{rate}} retry rate', {
        rate: formatPercent(summary.retry_rate),
      }),
    },
    {
      label: t('Recorded tokens'),
      value: formatCompact(summary.total_tokens),
      detail: t('{{count}} usage samples', {
        count: formatInteger(summary.usage_sample_count),
      }),
    },
    {
      label: t('Cache reads'),
      value: formatCompact(summary.cache_read_tokens),
      detail: t('{{rate}} token hit rate', {
        rate: formatPercent(summary.cache_token_hit_rate),
      }),
    },
    {
      label: t('Estimated cost'),
      value: formatMoney(summary.charged_micro_usd),
      detail: t('{{average}} average / {{p95}} P95 latency', {
        average: formatDuration(summary.avg_latency_ms),
        p95: formatDuration(summary.p95_latency_ms),
      }),
    },
  ]

  const trendData = (trendQuery.data?.points ?? []).map((point) => ({
    ...point,
    time: new Intl.DateTimeFormat(undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(point.bucket_ts * 1000)),
  }))
  const chartConfig = {
    final_request_count: {
      label: t('Client requests'),
      color: 'var(--chart-1)',
    },
    failed_attempt_count: {
      label: t('Failed attempts'),
      color: 'var(--chart-2)',
    },
    total_tokens: { label: t('Recorded tokens'), color: 'var(--chart-3)' },
  } satisfies ChartConfig
  const tokenParts = [
    [t('Uncached input'), summary.uncached_input_tokens],
    [t('Cache reads'), summary.cache_read_tokens],
    [t('Cache writes'), summary.cache_write_tokens],
    [t('Output tokens'), summary.output_tokens],
  ] as const
  const statuses = statusQuery.data?.items ?? []
  const statusMax = Math.max(1, ...statuses.map((item) => item.count))

  return (
    <div className='flex min-w-0 flex-col gap-3 sm:gap-4'>
      <QualityNotice meta={summaryQuery.data?.meta} />
      <div className='grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {metrics.map((metric) => (
          <Card key={metric.label} size='sm' className='shadow-none'>
            <CardHeader className='gap-1 pb-1'>
              <CardTitle className='text-muted-foreground text-xs font-medium'>
                {metric.label}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className='text-xl font-semibold tabular-nums sm:text-2xl'>
                {metric.value}
              </div>
              <p className='text-muted-foreground mt-1 line-clamp-2 text-xs'>
                {metric.detail}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>

      <div className='grid min-w-0 gap-3 xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]'>
        <Card className='min-w-0'>
          <CardHeader className='pb-2'>
            <CardTitle>{t('Traffic and quality trend')}</CardTitle>
          </CardHeader>
          <CardContent className='pt-0'>
            <ChartContainer
              config={chartConfig}
              className='aspect-auto h-64 w-full sm:h-72'
            >
              <LineChart data={trendData} margin={{ left: 4, right: 12 }}>
                <CartesianGrid vertical={false} />
                <XAxis
                  dataKey='time'
                  tickLine={false}
                  axisLine={false}
                  minTickGap={48}
                />
                <YAxis tickLine={false} axisLine={false} width={46} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <ChartLegend content={<ChartLegendContent />} />
                <Line
                  dataKey='final_request_count'
                  type='monotone'
                  stroke='var(--color-final_request_count)'
                  strokeWidth={2}
                  dot={false}
                />
                <Line
                  dataKey='failed_attempt_count'
                  type='monotone'
                  stroke='var(--color-failed_attempt_count)'
                  strokeWidth={2}
                  dot={false}
                />
                <Line
                  dataKey='total_tokens'
                  type='monotone'
                  stroke='var(--color-total_tokens)'
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ChartContainer>
          </CardContent>
        </Card>

        <Card size='sm' className='min-w-0 shadow-none'>
          <CardHeader className='pb-2'>
            <CardTitle>{t('Token composition')}</CardTitle>
          </CardHeader>
          <CardContent className='flex flex-col gap-3 pt-0'>
            {tokenParts.map(([label, value]) => (
              <Progress
                key={label}
                value={percentValue(value / Math.max(1, summary.total_tokens))}
              >
                <ProgressLabel>{label}</ProgressLabel>
                <ProgressValue>{() => formatCompact(value)}</ProgressValue>
              </Progress>
            ))}
          </CardContent>
        </Card>
      </div>

      <div className='grid min-w-0 gap-3 lg:grid-cols-2'>
        <Card size='sm' className='shadow-none'>
          <CardHeader className='pb-2'>
            <CardTitle>{t('Success-rate definitions')}</CardTitle>
          </CardHeader>
          <CardContent className='flex flex-col gap-3 pt-0'>
            {[
              [
                t('Client success rate'),
                summary.client_success_rate,
                t('One final result per client request'),
              ],
              [
                t('Channel quality success rate'),
                summary.channel_quality_success_rate,
                t('Excludes non-channel attributable failures'),
              ],
              [
                t('Attempt success rate'),
                summary.attempt_success_rate,
                t('Business outcome for every channel attempt'),
              ],
            ].map(([label, value, detail]) => (
              <div
                key={String(label)}
                className='flex items-start justify-between gap-4'
              >
                <div>
                  <div className='font-medium'>{label}</div>
                  <p className='text-muted-foreground text-xs'>{detail}</p>
                </div>
                <span className='font-semibold tabular-nums'>
                  {formatPercent(value as number | null)}
                </span>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card size='sm' className='shadow-none'>
          <CardHeader className='pb-2'>
            <CardTitle>{t('Upstream status overview')}</CardTitle>
          </CardHeader>
          <CardContent className='flex flex-col gap-3 pt-0'>
            {statusQuery.isError ? (
              <p className='text-destructive text-sm'>
                {statusQuery.error.message}
              </p>
            ) : statuses.length ? (
              statuses.slice(0, 8).map((item) => (
                <Progress
                  key={`${item.status_present}-${item.status_code}`}
                  value={(item.count / statusMax) * 100}
                >
                  <ProgressLabel>{getStatusLabel(item)}</ProgressLabel>
                  <ProgressValue>
                    {() => formatInteger(item.count)}
                  </ProgressValue>
                </Progress>
              ))
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No status-code samples')}
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
