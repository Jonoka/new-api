import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import { DollarSign, Ticket, TrendingUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  formatLocalCurrencyAmount,
  formatQuotaWithCurrency,
  getCurrencyDisplay,
} from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { getRevenueStats } from '@/features/dashboard/api'
import { TIME_RANGE_PRESETS } from '@/features/dashboard/constants'

function getChartThemeTokens(resolvedTheme: string) {
  return {
    textColor:
      resolvedTheme === 'dark'
        ? 'rgba(255, 255, 255, 0.68)'
        : 'rgba(15, 23, 42, 0.58)',
    gridColor:
      resolvedTheme === 'dark'
        ? 'rgba(255, 255, 255, 0.12)'
        : 'rgba(15, 23, 42, 0.12)',
  }
}

function quotaToLocalCurrency(quota: number): number {
  const { config } = getCurrencyDisplay()
  const amountUSD = quota / config.quotaPerUnit
  return amountUSD * config.usdExchangeRate
}

export function RevenuePanel() {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const { textColor, gridColor } = getChartThemeTokens(resolvedTheme)
  const [selectedDays, setSelectedDays] = useState(7)

  const granularity = selectedDays <= 1 ? 'hour' : 'day'
  const { start_timestamp, end_timestamp } = useMemo(
    () => computeTimeRange(selectedDays, undefined, undefined, true),
    [selectedDays]
  )

  const { data, isLoading } = useQuery({
    queryKey: ['revenue-stats', start_timestamp, end_timestamp, granularity],
    queryFn: () =>
      getRevenueStats({
        start_timestamp,
        end_timestamp,
        granularity,
      }),
    staleTime: 60 * 1000,
  })

  const stats = data?.data
  const summary = stats?.summary

  const totalOnline = summary?.total_online_money ?? 0
  const totalRedemptionQuota = summary?.total_redemption_quota ?? 0
  const redemptionEquivalent = quotaToLocalCurrency(totalRedemptionQuota)
  const totalRevenue = totalOnline + redemptionEquivalent

  return (
    <div className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      {/* Header */}
      <div className='flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5'>
        <div className='flex items-center gap-2'>
          <TrendingUp className='text-muted-foreground size-4' />
          <h3 className='text-sm font-semibold'>{t('Revenue Statistics')}</h3>
        </div>
        <div className='flex items-center gap-1'>
          {TIME_RANGE_PRESETS.map((preset) => (
            <Button
              key={preset.days}
              variant={selectedDays === preset.days ? 'default' : 'ghost'}
              size='sm'
              className='h-7 px-2.5 text-xs'
              onClick={() => setSelectedDays(preset.days)}
            >
              {t(preset.label)}
            </Button>
          ))}
        </div>
      </div>

      {/* Stats cards */}
      <div className='grid grid-cols-1 gap-px border-b sm:grid-cols-3'>
        <StatMiniCard
          icon={TrendingUp}
          label={t('Total Revenue')}
          value={formatLocalCurrencyAmount(totalRevenue)}
          loading={isLoading}
        />
        <StatMiniCard
          icon={DollarSign}
          label={t('Online Recharge')}
          value={formatLocalCurrencyAmount(totalOnline)}
          sub={t('{{count}} transactions', {
            count: formatNumber(summary?.total_online_count ?? 0),
          })}
          loading={isLoading}
        />
        <StatMiniCard
          icon={Ticket}
          label={t('Redemption Code')}
          value={formatQuotaWithCurrency(totalRedemptionQuota)}
          sub={t('{{count}} redemptions', {
            count: formatNumber(summary?.total_redemption_count ?? 0),
          })}
          loading={isLoading}
        />
      </div>

      {/* Chart */}
      <div className='h-56 px-3 py-2 sm:h-64 sm:px-4'>
        <RevenueChart
          dataPoints={stats?.data_points ?? []}
          granularity={granularity}
          themeReady={themeReady}
          resolvedTheme={resolvedTheme}
          textColor={textColor}
          gridColor={gridColor}
          loading={isLoading}
        />
      </div>
    </div>
  )
}

interface StatMiniCardProps {
  icon: React.ComponentType<{ className?: string }>
  label: string
  value: string
  sub?: string
  loading?: boolean
}

function StatMiniCard(props: StatMiniCardProps) {
  const Icon = props.icon
  return (
    <div className='flex items-center gap-3 px-4 py-3 sm:px-5'>
      <span className='bg-muted flex size-8 shrink-0 items-center justify-center rounded-lg'>
        <Icon className='size-3.5' aria-hidden='true' />
      </span>
      <div className='min-w-0'>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        {props.loading ? (
          <Skeleton className='mt-1 h-5 w-20' />
        ) : (
          <>
            <div className='truncate text-base font-semibold tabular-nums'>
              {props.value}
            </div>
            {props.sub && (
              <div className='text-muted-foreground truncate text-xs'>
                {props.sub}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

interface RevenueChartProps {
  dataPoints: Array<{
    timestamp: number
    online_money: number
    redemption_quota: number
  }>
  granularity: string
  themeReady: boolean
  resolvedTheme: string
  textColor: string
  gridColor: string
  loading?: boolean
}

function formatTimeLabel(timestamp: number, granularity: string): string {
  const date = new Date(timestamp * 1000)
  if (granularity === 'hour') {
    return `${String(date.getHours()).padStart(2, '0')}:00`
  }
  return `${date.getMonth() + 1}/${date.getDate()}`
}

function RevenueChart(props: RevenueChartProps) {
  const { t } = useTranslation()

  const spec = useMemo(() => {
    if (props.dataPoints.length === 0) return null

    const chartData: Array<{
      time: string
      value: number
      type: string
    }> = []

    for (const point of props.dataPoints) {
      const timeLabel = formatTimeLabel(point.timestamp, props.granularity)
      chartData.push({
        time: timeLabel,
        value: point.online_money,
        type: t('Online Recharge'),
      })
      chartData.push({
        time: timeLabel,
        value: quotaToLocalCurrency(point.redemption_quota),
        type: t('Redemption Code'),
      })
    }

    return {
      type: 'area' as const,
      data: [{ id: 'revenue', values: chartData }],
      xField: 'time',
      yField: 'value',
      seriesField: 'type',
      stack: true,
      line: { style: { lineWidth: 2, curveType: 'monotone' } },
      area: { style: { fillOpacity: 0.15 } },
      point: { visible: false },
      legends: {
        visible: true,
        orient: 'bottom' as const,
        position: 'middle' as const,
        item: {
          label: { style: { fill: props.textColor, fontSize: 11 } },
        },
      },
      tooltip: {
        mark: {
          content: [
            {
              key: (d: { type: string }) => d.type,
              value: (d: { value: number }) =>
                formatLocalCurrencyAmount(d.value),
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom' as const,
          label: { style: { fill: props.textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left' as const,
          label: {
            formatMethod: (val: number | string) =>
              formatLocalCurrencyAmount(Number(val)),
            style: { fill: props.textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: props.gridColor },
          },
        },
      ],
      color: ['hsl(var(--chart-1))', 'hsl(var(--chart-2))'],
    }
  }, [props.dataPoints, props.granularity, props.textColor, props.gridColor, t])

  if (props.loading) {
    return <Skeleton className='h-full w-full rounded-lg' />
  }

  if (!spec || props.dataPoints.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
        {t('No revenue data in this period')}
      </div>
    )
  }

  if (!props.themeReady) {
    return <Skeleton className='h-full w-full rounded-lg' />
  }

  return (
    <VChart
      key={`revenue-${props.resolvedTheme}`}
      spec={{
        ...spec,
        theme: props.resolvedTheme === 'dark' ? 'dark' : 'light',
        background: 'transparent',
      }}
      option={VCHART_OPTION}
    />
  )
}
