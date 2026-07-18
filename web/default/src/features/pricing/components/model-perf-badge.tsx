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
import {
  formatLatency,
  formatThroughput,
} from '@/features/performance-metrics/lib/format'

export type ModelPerfBadgeData = {
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
}

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

function formatCompactThroughput(tps: number): string {
  return formatThroughput(tps).replace(' t/s', 'tps')
}

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()

  if (!props.perf) {
    return null
  }

  const { avg_latency_ms, avg_tps, success_rate } = props.perf
  const formattedSuccessRate = Number.isFinite(success_rate)
    ? `${success_rate.toFixed(1)}%`
    : '—'

  let statusColor = 'bg-emerald-500'
  if (success_rate < 99) {
    statusColor = 'bg-red-500'
  } else if (success_rate < 99.9) {
    statusColor = 'bg-amber-500'
  }

  return (
    <div
      className={cn(
        'bg-muted/30 grid w-full grid-cols-3 gap-x-1 rounded-md px-2 py-1 text-left tabular-nums min-[460px]:w-[132px] min-[460px]:grid-cols-[38px_48px_30px] min-[460px]:gap-x-2 min-[460px]:rounded-none min-[460px]:bg-transparent min-[460px]:p-0 min-[460px]:text-right',
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
      <div
        title={`${t('Success rate')}: ${formattedSuccessRate}`}
        className='min-w-0'
      >
        <div className='text-muted-foreground/55 truncate text-[10px] leading-4'>
          {t('Status short')}
        </div>
        <div className='flex h-4 items-center justify-start gap-1 min-[460px]:justify-end min-[460px]:gap-0.5'>
          <span className='text-muted-foreground/80 font-mono text-xs leading-4 whitespace-nowrap min-[460px]:hidden'>
            {formattedSuccessRate}
          </span>
          <span aria-hidden='true' className='flex items-center gap-0.5'>
            <span className='bg-muted-foreground/10 h-2 w-1 rounded-full' />
            <span className='bg-muted-foreground/15 h-2.5 w-1 rounded-full' />
            <span className={cn('h-3 w-1 rounded-full', statusColor)} />
          </span>
        </div>
      </div>
    </div>
  )
})
