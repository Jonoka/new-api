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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  buildStatusSegments,
  type StatusSegment,
  type StatusSeriesPoint,
} from '../lib/status-segments'

type StatusSegmentsSize = 'sm' | 'md'

export type StatusSegmentsProps = {
  series: StatusSeriesPoint[]
  overallRate?: number
  size?: StatusSegmentsSize
  showOverall?: boolean
  emptyLabel?: string
  endTs?: number
  className?: string
}

function getSegmentColor(successRate: number | null): string {
  if (successRate == null) return 'bg-muted-foreground/15'
  if (successRate >= 99.9) return 'bg-success'
  if (successRate >= 99) return 'bg-warning'
  return 'bg-destructive'
}

function getRateTextColor(successRate: number | null): string {
  if (successRate == null) return 'text-muted-foreground'
  if (successRate >= 99.9) return 'text-success'
  if (successRate >= 99) return 'text-warning'
  return 'text-destructive'
}

function getAverageRate(series: StatusSeriesPoint[]): number | null {
  const rates = series
    .map((point) => point.success_rate)
    .filter((rate) => Number.isFinite(rate) && rate >= 0 && rate <= 100)
  if (rates.length === 0) return null
  return rates.reduce((sum, rate) => sum + rate, 0) / rates.length
}

function formatSegmentRange(
  segment: StatusSegment,
  formatter: Intl.DateTimeFormat
): string {
  return `${formatter.format(segment.startTs * 1000)} – ${formatter.format(segment.endTs * 1000)}`
}

export function StatusSegments(props: StatusSegmentsProps) {
  const { t, i18n } = useTranslation()
  const size = props.size ?? 'md'
  const showOverall = props.showOverall ?? true
  const endTs = Number.isFinite(props.endTs)
    ? Number(props.endTs)
    : Math.trunc(Date.now() / 1000)
  const segments = useMemo(
    () => buildStatusSegments(props.series, endTs),
    [endTs, props.series]
  )
  const formatter = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.language, {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [i18n.language]
  )

  if (props.series.length === 0) {
    return (
      <span className={cn('text-muted-foreground text-xs', props.className)}>
        {props.emptyLabel ?? '—'}
      </span>
    )
  }

  const fallbackOverall = getAverageRate(props.series)
  const overallRate = Number.isFinite(props.overallRate)
    ? Number(props.overallRate)
    : fallbackOverall
  const segmentSize = size === 'sm' ? 'h-3 w-1.5' : 'h-4 w-2'
  const overallTextSize = size === 'sm' ? 'text-[11px]' : 'text-xs'
  const ariaSummary = segments
    .map((segment) => {
      const rate =
        segment.successRate == null
          ? t('No data')
          : `${segment.successRate.toFixed(2)}%`
      return `${formatSegmentRange(segment, formatter)} ${rate}`
    })
    .join(', ')

  return (
    <div className={cn('flex items-center gap-2', props.className)}>
      <div
        className='flex items-center gap-1'
        role='img'
        aria-label={`${t('Success rate')}: ${ariaSummary}`}
      >
        {segments.map((segment) => {
          const rangeLabel = formatSegmentRange(segment, formatter)
          return (
            <Tooltip key={segment.startTs}>
              <TooltipTrigger
                render={
                  <span
                    className={cn(
                      'rounded-sm transition-opacity hover:opacity-80',
                      segmentSize,
                      getSegmentColor(segment.successRate)
                    )}
                  />
                }
              />
              <TooltipContent side='top' className='font-mono text-xs'>
                <div className='font-medium'>{rangeLabel}</div>
                <div>
                  {segment.successRate == null
                    ? t('No data')
                    : `${segment.successRate.toFixed(2)}%`}
                </div>
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
      {showOverall && (
        <span
          className={cn(
            'font-mono leading-none font-semibold whitespace-nowrap tabular-nums',
            overallTextSize,
            getRateTextColor(overallRate)
          )}
        >
          {overallRate == null ? '—' : `${overallRate.toFixed(1)}%`}
        </span>
      )}
    </div>
  )
}
