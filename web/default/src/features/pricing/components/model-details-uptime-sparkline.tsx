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
import { useMemo } from 'react'
import { Activity, AlertCircle, CheckCircle2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { StatusSegments } from '@/features/performance-metrics/components/status-segments'
import { formatUptimePct } from '@/features/performance-metrics/lib/format'
import type { StatusSeriesPoint } from '@/features/performance-metrics/lib/status-segments'
import { aggregateUptime, type UptimeDayPoint } from '../lib/mock-stats'

// ---------------------------------------------------------------------------
// Uptime sparkline
// ---------------------------------------------------------------------------
//
// 将模型性能数据转换为统一的最近 24 小时四段式状态条。

type SparklineSize = 'sm' | 'md'

type UptimeSparklineProps = {
  series: UptimeDayPoint[]
  size?: SparklineSize
  showOverall?: boolean
  overallRate?: number
  emptyLabel?: string
  className?: string
}

export function UptimeSparkline(props: UptimeSparklineProps) {
  const statusSeries = useMemo<StatusSeriesPoint[]>(
    () =>
      props.series.flatMap((point) => {
        const ts = Date.parse(point.date) / 1000
        if (!Number.isFinite(ts)) return []
        return [{ ts, success_rate: point.uptime_pct }]
      }),
    [props.series]
  )

  return (
    <StatusSegments
      series={statusSeries}
      overallRate={props.overallRate}
      size={props.size}
      showOverall={props.showOverall}
      emptyLabel={props.emptyLabel}
      className={props.className}
    />
  )
}

// ---------------------------------------------------------------------------
// Uptime status row — sparkline + summary text + status icon
// ---------------------------------------------------------------------------

export function UptimeStatusRow(props: {
  series: UptimeDayPoint[]
  className?: string
}) {
  const { t } = useTranslation()
  const summary = useMemo(() => aggregateUptime(props.series), [props.series])
  const status = useMemo(() => {
    if (summary.uptime_pct >= 99.9) return 'operational'
    if (summary.uptime_pct >= 99.0) return 'minor'
    if (summary.uptime_pct >= 95.0) return 'degraded'
    return 'major'
  }, [summary.uptime_pct])

  const StatusIcon =
    status === 'operational'
      ? CheckCircle2
      : status === 'minor'
        ? Activity
        : AlertCircle

  const statusColour =
    status === 'operational'
      ? 'text-success'
      : status === 'minor'
        ? 'text-warning'
        : status === 'degraded'
          ? 'text-warning'
          : 'text-destructive'

  const statusLabel =
    status === 'operational'
      ? t('All systems operational')
      : status === 'minor'
        ? t('Minor blips in the last 30 days')
        : status === 'degraded'
          ? t('Degraded performance recently')
          : t('Significant outages detected')

  return (
    <div
      className={cn(
        'border-border/60 bg-muted/30 flex flex-wrap items-center gap-3 rounded-lg border px-3 py-2 sm:gap-4 sm:px-4',
        props.className
      )}
    >
      <div className='flex items-center gap-2'>
        <StatusIcon className={cn('size-4 shrink-0', statusColour)} />
        <span className='text-sm font-medium'>
          {t('Availability (last 24h)')}
        </span>
      </div>

      <UptimeSparkline
        series={props.series}
        overallRate={summary.uptime_pct}
        className='ml-auto'
      />

      <div className='flex items-center gap-3 text-xs'>
        <span className={cn('font-medium', statusColour)}>{statusLabel}</span>
        {summary.incidents > 0 && (
          <span className='text-muted-foreground'>
            {summary.incidents}{' '}
            {summary.incidents === 1 ? t('incident') : t('incidents')}
          </span>
        )}
        {summary.outage_minutes > 0 && (
          <span className='text-muted-foreground'>
            {summary.outage_minutes} {t('min downtime')}
          </span>
        )}
        <span className='text-muted-foreground hidden sm:inline'>
          {formatUptimePct(summary.uptime_pct)} {t('overall')}
        </span>
      </div>
    </div>
  )
}
