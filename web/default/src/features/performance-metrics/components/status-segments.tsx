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
  getAvailabilityStatusLevel,
  type StatusSegment,
  type StatusSeriesPoint,
} from '../lib/status-segments'

type StatusSegmentsSize = 'sm' | 'md'
type StatusSegmentsTone = 'success-rate' | 'availability'
type StatusSegmentsShape = 'blocks' | 'signal'

export type StatusSegmentsProps = {
  series: StatusSeriesPoint[]
  overallRate?: number
  size?: StatusSegmentsSize
  showOverall?: boolean
  emptyLabel?: string
  endTs?: number
  className?: string
  tone?: StatusSegmentsTone
  shape?: StatusSegmentsShape
  segmentCount?: number
}

function getSegmentColor(
  successRate: number | null,
  tone: StatusSegmentsTone
): string {
  if (successRate == null) return 'bg-muted-foreground/15'
  if (tone === 'availability') {
    const level = getAvailabilityStatusLevel(successRate)
    if (level === 'healthy') return 'bg-success'
    if (level === 'degraded') return 'bg-warning'
    return 'bg-destructive'
  }
  if (successRate >= 99.9) return 'bg-success'
  if (successRate >= 99) return 'bg-warning'
  return 'bg-destructive'
}

function getRateTextColor(
  successRate: number | null,
  tone: StatusSegmentsTone
): string {
  if (successRate == null) return 'text-muted-foreground'
  if (tone === 'availability') {
    const level = getAvailabilityStatusLevel(successRate)
    if (level === 'healthy') return 'text-success'
    if (level === 'degraded') return 'text-warning'
    return 'text-destructive'
  }
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
  const tone = props.tone ?? 'success-rate'
  const shape = props.shape ?? 'blocks'
  const showOverall = props.showOverall ?? true
  const endTs = Number.isFinite(props.endTs)
    ? Number(props.endTs)
    : Math.trunc(Date.now() / 1000)
  const segments = useMemo(
    () => buildStatusSegments(props.series, endTs, props.segmentCount),
    [endTs, props.segmentCount, props.series]
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
  const statusLabel = tone === 'availability' ? t('Status') : t('Success rate')

  return (
    <div className={cn('flex items-center gap-2', props.className)}>
      <div
        className={cn(
          'flex',
          shape === 'signal' ? 'h-4 items-end gap-0.5' : 'items-center gap-1'
        )}
        role='img'
        aria-label={`${statusLabel}: ${ariaSummary}`}
      >
        {segments.map((segment, index) => {
          const rangeLabel = formatSegmentRange(segment, formatter)
          return (
            <Tooltip key={segment.startTs}>
              <TooltipTrigger
                render={
                  <span
                    className={cn(
                      'transition-opacity hover:opacity-80',
                      shape === 'signal' ? 'w-1 rounded-full' : 'rounded-sm',
                      shape === 'signal'
                        ? ['h-2', 'h-2.5', 'h-3'][Math.min(index, 2)]
                        : segmentSize,
                      getSegmentColor(segment.successRate, tone)
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
            getRateTextColor(overallRate, tone)
          )}
        >
          {overallRate == null ? '—' : `${overallRate.toFixed(1)}%`}
        </span>
      )}
    </div>
  )
}
