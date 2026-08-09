import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { Card, Tabs, TabPane, Spin } from '@douyinfe/semi-ui';
import { TrendingUp, DollarSign, Ticket } from 'lucide-react';
import { VChart } from '@visactor/react-vchart';
import { useTranslation } from 'react-i18next';
import { API, isAdmin, showError } from '../../helpers';
import { renderQuota, renderQuotaWithAmount } from '../../helpers/render';

const TIME_RANGE_OPTIONS = [
  { label: '1天', days: 1, granularity: 'hour' },
  { label: '7天', days: 7, granularity: 'day' },
  { label: '14天', days: 14, granularity: 'day' },
  { label: '29天', days: 29, granularity: 'day' },
];

function formatTimeLabel(timestamp, granularity) {
  const date = new Date(timestamp * 1000);
  if (granularity === 'hour') {
    return `${String(date.getHours()).padStart(2, '0')}:00`;
  }
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

function quotaToLocalCurrency(quota) {
  let quotaPerUnit =
    parseFloat(localStorage.getItem('quota_per_unit')) || 500000;
  const quotaDisplayType = localStorage.getItem('quota_display_type') || 'USD';
  const usdRate = parseFloat(localStorage.getItem('usd_exchange_rate')) || 1;
  const amountUSD = quota / quotaPerUnit;
  if (quotaDisplayType === 'CNY') {
    return amountUSD * usdRate;
  }
  return amountUSD;
}

// PLACEHOLDER_REST

const StatMiniCard = ({ icon: Icon, label, value, sub, loading }) => (
  <div className='flex items-center gap-3 px-4 py-3'>
    <span className='flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 dark:bg-gray-800'>
      <Icon size={14} />
    </span>
    <div className='min-w-0'>
      <div className='text-xs text-gray-500'>{label}</div>
      {loading ? (
        <div className='mt-1 h-5 w-20 animate-pulse rounded bg-gray-200 dark:bg-gray-700' />
      ) : (
        <>
          <div className='truncate text-base font-semibold tabular-nums'>
            {value}
          </div>
          {sub && <div className='truncate text-xs text-gray-500'>{sub}</div>}
        </>
      )}
    </div>
  </div>
);

const RevenuePanel = ({ CARD_PROPS, CHART_CONFIG }) => {
  const { t } = useTranslation();
  const [selectedDays, setSelectedDays] = useState(7);
  const [loading, setLoading] = useState(false);
  const [stats, setStats] = useState(null);

  const granularity = selectedDays <= 1 ? 'hour' : 'day';

  const loadRevenueData = useCallback(async () => {
    setLoading(true);
    try {
      const now = new Date();
      const todayStart = new Date(
        now.getFullYear(),
        now.getMonth(),
        now.getDate(),
      );
      const end = Math.floor(todayStart.getTime() / 1000) + 86400 - 1;
      const start =
        Math.floor(todayStart.getTime() / 1000) - (selectedDays - 1) * 86400;
      const tzOffset = now.getTimezoneOffset() * -60;
      const res = await API.get(
        `/api/data/revenue?start_timestamp=${start}&end_timestamp=${end}&granularity=${granularity}&timezone_offset=${tzOffset}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        setStats(data);
      } else {
        showError(message);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, [selectedDays, granularity]);

  useEffect(() => {
    if (isAdmin()) {
      loadRevenueData();
    }
  }, [loadRevenueData]);

  const summary = stats?.summary;
  const dataPoints = stats?.data_points || [];

  const totalOnline = summary?.total_online_money ?? 0;
  const totalRedemptionQuota = summary?.total_redemption_quota ?? 0;
  const redemptionEquivalent = quotaToLocalCurrency(totalRedemptionQuota);
  const totalRevenue = totalOnline + redemptionEquivalent;

  if (!isAdmin()) return null;

  return (
    <Card
      {...CARD_PROPS}
      className='!rounded-2xl'
      title={
        <div className='flex flex-col sm:flex-row sm:items-center sm:justify-between w-full gap-3'>
          <div className='flex items-center gap-2'>
            <TrendingUp size={16} />
            {t('收入统计')}
          </div>
          <div className='flex items-center gap-1'>
            {TIME_RANGE_OPTIONS.map((opt) => (
              <button
                key={opt.days}
                onClick={() => setSelectedDays(opt.days)}
                className={`rounded-md px-2.5 py-1 text-xs font-medium transition-colors ${
                  selectedDays === opt.days
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700'
                }`}
              >
                {t(opt.label)}
              </button>
            ))}
          </div>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      {/* Stats row */}
      <div className='grid grid-cols-1 sm:grid-cols-3 border-b'>
        <StatMiniCard
          icon={TrendingUp}
          label={t('总收入')}
          value={renderQuotaWithAmount(totalRevenue)}
          loading={loading}
        />
        <StatMiniCard
          icon={DollarSign}
          label={t('在线充值')}
          value={renderQuotaWithAmount(totalOnline)}
          sub={`${summary?.total_online_count ?? 0} ${t('笔')}`}
          loading={loading}
        />
        <StatMiniCard
          icon={Ticket}
          label={t('兑换码')}
          value={renderQuota(totalRedemptionQuota)}
          sub={`${summary?.total_redemption_count ?? 0} ${t('次')}`}
          loading={loading}
        />
      </div>

      {/* Chart */}
      <div className='h-72 p-2'>
        {loading ? (
          <div className='flex h-full items-center justify-center'>
            <Spin />
          </div>
        ) : dataPoints.length === 0 ? (
          <div className='flex h-full items-center justify-center text-sm text-gray-400'>
            {t('该时间段内无收入数据')}
          </div>
        ) : (
          <RevenueChart
            dataPoints={dataPoints}
            granularity={granularity}
            CHART_CONFIG={CHART_CONFIG}
            t={t}
          />
        )}
      </div>
    </Card>
  );
};

// PLACEHOLDER_CHART_COMPONENT

const RevenueChart = ({ dataPoints, granularity, CHART_CONFIG, t }) => {
  const spec = useMemo(() => {
    const chartData = [];
    for (const point of dataPoints) {
      const timeLabel = formatTimeLabel(point.timestamp, granularity);
      chartData.push({
        time: timeLabel,
        value: point.online_money,
        type: t('在线充值'),
      });
      chartData.push({
        time: timeLabel,
        value: quotaToLocalCurrency(point.redemption_quota),
        type: t('兑换码'),
      });
    }

    return {
      type: 'area',
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
        orient: 'bottom',
        position: 'middle',
      },
      tooltip: {
        mark: {
          content: [
            {
              key: (d) => d.type,
              value: (d) => renderQuotaWithAmount(d.value),
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          label: {
            formatMethod: (val) => renderQuotaWithAmount(Number(val)),
            style: { fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3] },
          },
        },
      ],
      color: ['#3b82f6', '#f59e0b'],
    };
  }, [dataPoints, granularity, t]);

  return <VChart spec={spec} option={CHART_CONFIG} />;
};

export default RevenuePanel;
