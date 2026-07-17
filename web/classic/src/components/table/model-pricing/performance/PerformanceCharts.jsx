/*
Copyright (C) 2025 QuantumNous

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

import React, { useEffect, useMemo, useState } from 'react';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';

import { CHART_CONFIG } from '../../../../constants/dashboard.constants';
import { useActualTheme } from '../../../../context/Theme';
import { formatBucketTime, getSuccessRateHex, getUptimeAxisMin } from './utils';

let chartThemeInitialized = false;

const usePerformanceChartTheme = () => {
  const actualTheme = useActualTheme();
  const [themeReady, setThemeReady] = useState(chartThemeInitialized);

  useEffect(() => {
    if (!chartThemeInitialized) {
      initVChartSemiTheme({ isWatchingThemeSwitch: true });
      chartThemeInitialized = true;
    }
    setThemeReady(true);
  }, []);

  return {
    actualTheme,
    themeReady,
    theme: actualTheme === 'dark' ? 'semiDesignDark' : 'semiDesignLight',
    textColor:
      actualTheme === 'dark'
        ? 'rgba(255, 255, 255, 0.68)'
        : 'rgba(15, 23, 42, 0.58)',
    gridColor:
      actualTheme === 'dark'
        ? 'rgba(255, 255, 255, 0.12)'
        : 'rgba(15, 23, 42, 0.12)',
  };
};

const EmptyChart = ({ children }) => (
  <div
    className='flex h-52 items-center justify-center rounded-xl border text-xs'
    style={{
      borderColor: 'var(--semi-color-border)',
      color: 'var(--semi-color-text-2)',
    }}
  >
    {children}
  </div>
);

export const LatencyTrendChart = ({ series, t }) => {
  const { actualTheme, themeReady, theme, textColor, gridColor } =
    usePerformanceChartTheme();

  const spec = useMemo(() => {
    if (!Array.isArray(series) || series.length === 0) return null;
    const values = series.map((point) => ({
      time: formatBucketTime(point.ts, false),
      ttft: point.avg_ttft_ms,
    }));

    return {
      type: 'line',
      data: [{ id: 'latency', values }],
      xField: 'time',
      yField: 'ttft',
      line: {
        style: { lineWidth: 2, curveType: 'monotone', stroke: '#4f6ef7' },
      },
      point: {
        visible: true,
        style: {
          size: 5,
          fill: '#4f6ef7',
          stroke: actualTheme === 'dark' ? '#1f2937' : '#ffffff',
          lineWidth: 1.5,
        },
      },
      legends: { visible: false },
      tooltip: {
        mark: {
          title: { value: (datum) => datum.time },
          content: [
            {
              key: t('平均首 Token 延迟'),
              value: (datum) => `${Math.round(datum.ttft)} ms`,
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fill: textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          label: {
            formatMethod: (value) => `${value} ms`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
      ],
      theme,
      background: 'transparent',
    };
  }, [actualTheme, gridColor, series, t, textColor, theme]);

  if (!spec) return <EmptyChart>{t('暂无延迟数据')}</EmptyChart>;

  return (
    <div className='h-64'>
      {themeReady && (
        <VChart
          key={`latency-${actualTheme}`}
          spec={spec}
          options={CHART_CONFIG}
        />
      )}
    </div>
  );
};

export const UptimeTrendChart = ({ series, t }) => {
  const { actualTheme, themeReady, theme, textColor, gridColor } =
    usePerformanceChartTheme();

  const spec = useMemo(() => {
    if (!Array.isArray(series) || series.length === 0) return null;
    const values = series.map((point) => ({
      time: formatBucketTime(point.ts, false),
      uptime: point.success_rate,
      incidents: point.incidents,
    }));
    const minimum = getUptimeAxisMin(values.map((point) => point.uptime));

    return {
      type: 'line',
      data: [{ id: 'uptime', values }],
      xField: 'time',
      yField: 'uptime',
      line: {
        style: { lineWidth: 2, curveType: 'monotone', stroke: '#10b981' },
      },
      point: {
        visible: true,
        style: {
          size: 5,
          fill: (datum) => getSuccessRateHex(datum.uptime),
          stroke: actualTheme === 'dark' ? '#1f2937' : '#ffffff',
          lineWidth: 1.5,
        },
      },
      legends: { visible: false },
      tooltip: {
        mark: {
          title: { value: (datum) => datum.time },
          content: [
            {
              key: t('成功率'),
              value: (datum) => `${Number(datum.uptime).toFixed(2)}%`,
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          label: {
            autoLimit: true,
            style: { fill: textColor, fontSize: 10 },
          },
          tick: { visible: false },
        },
        {
          orient: 'left',
          min: minimum,
          max: 100,
          label: {
            formatMethod: (value) => `${value}%`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
      ],
      theme,
      background: 'transparent',
    };
  }, [actualTheme, gridColor, series, t, textColor, theme]);

  if (!spec) return <EmptyChart>{t('暂无可用率数据')}</EmptyChart>;

  return (
    <div className='h-64'>
      {themeReady && (
        <VChart
          key={`uptime-${actualTheme}`}
          spec={spec}
          options={CHART_CONFIG}
        />
      )}
    </div>
  );
};
