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
import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { StatusSegments } from '@/features/performance-metrics/components/status-segments'
import {
  formatLatency,
  formatThroughput,
} from '@/features/performance-metrics/lib/format'
import type { PerfModelSummary } from '@/features/performance-metrics/types'

export type ModelPerfBadgeData = Pick<
  PerfModelSummary,
  'avg_latency_ms' | 'success_rate' | 'status_rate' | 'avg_tps' | 'series'
>

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

function formatCompactThroughput(tps: number): string {
  return formatThroughput(tps).replace(' t/s', 't')
}

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()

  if (!props.perf) {
    return null
  }

  const { avg_latency_ms, avg_tps, success_rate } = props.perf
  const statusSeries = (props.perf.series ?? []).map((point) => ({
    ...point,
    success_rate: Number.isFinite(point.status_rate)
      ? Number(point.status_rate)
      : point.success_rate,
  }))
  const statusRate = Number.isFinite(props.perf.status_rate)
    ? Number(props.perf.status_rate)
    : success_rate

  return (
    <div
      className={cn(
        'bg-muted/30 grid w-full grid-cols-3 gap-x-3 rounded-md px-2 py-1 text-left tabular-nums min-[460px]:w-[132px] min-[460px]:grid-cols-[38px_48px_30px] min-[460px]:gap-x-2 min-[460px]:rounded-none min-[460px]:bg-transparent min-[460px]:p-0 min-[460px]:text-right',
        props.className
      )}
    >
      <div title={t('Average latency')} className='min-w-0'>
        <div className='text-muted-foreground/55 text-[10px] leading-4'>
          {t('Latency short')}
        </div>
        <div className='text-muted-foreground/80 font-mono text-xs leading-4 whitespace-nowrap'>
          {avg_latency_ms > 0 ? formatLatency(avg_latency_ms) : '—'}
        </div>
      </div>
      <div title={t('Throughput')} className='min-w-0'>
        <div className='text-muted-foreground/55 truncate text-[10px] leading-4'>
          {t('Throughput short')}
        </div>
        <div className='text-muted-foreground/80 font-mono text-xs leading-4 whitespace-nowrap'>
          {formatCompactThroughput(avg_tps)}
        </div>
      </div>
      <div title={t('Status')} className='min-w-0'>
        <div className='text-muted-foreground/55 truncate text-[10px] leading-4'>
          {t('Status short')}
        </div>
        <StatusSegments
          series={statusSeries}
          overallRate={statusRate}
          size='sm'
          showOverall={false}
          tone='availability'
          shape='signal'
          segmentCount={3}
          className='h-4 justify-start min-[460px]:justify-end'
        />
      </div>
    </div>
  )
})
