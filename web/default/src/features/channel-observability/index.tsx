/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Alert02Icon,
  Analytics01Icon,
  ChartRelationshipIcon,
  RefreshIcon,
  Router01Icon,
  TestTube01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import { getAnalyticsFilters } from './api'
import { ChannelsView } from './channels-view'
import { FailuresView } from './failures-view'
import { FilterBar } from './filter-bar'
import { OverviewView } from './overview-view'
import { ProbeView } from './probe-view'
import { formatDateTime } from './shared'
import { StabilityView } from './stability-view'
import type { AnalyticsFilters } from './types'

type View = 'overview' | 'operations' | 'channels' | 'failures' | 'probe'

function defaultFilters(): AnalyticsFilters {
  const now = Math.floor(Date.now() / 1000)
  return {
    range: 'today',
    customStart: now - 3600,
    customEnd: now,
    granularity: 'auto',
    channelId: '',
    channelType: '',
    group: '',
    requestedModel: '',
    requestedModelHash: '',
    upstreamModel: '',
    upstreamModelHash: '',
    outcome: '',
    statusCode: '',
    stream: '',
    trafficSource: 'relay',
    dataOrigin: 'live,legacy',
  }
}

export function ChannelObservability() {
  const { t } = useTranslation()
  const [view, setView] = useState<View>('overview')
  const [filters, setFilters] = useState<AnalyticsFilters>(defaultFilters)
  const [refreshKey, setRefreshKey] = useState(0)
  const filtersQuery = useQuery({
    queryKey: ['channel-observability', 'filters', refreshKey],
    queryFn: getAnalyticsFilters,
    staleTime: 30_000,
  })
  const reset = () => setFilters(defaultFilters())
  const tabs: Array<{
    value: View
    label: string
    icon: typeof Analytics01Icon
  }> = [
    { value: 'overview', label: t('Overview'), icon: Analytics01Icon },
    {
      value: 'operations',
      label: t('Operations matrix'),
      icon: ChartRelationshipIcon,
    },
    { value: 'channels', label: t('Channels and models'), icon: Router01Icon },
    {
      value: 'failures',
      label: t('Status codes and failures'),
      icon: Alert02Icon,
    },
    { value: 'probe', label: t('Active probes'), icon: TestTube01Icon },
  ]

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Channel Observability')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {filtersQuery.data?.meta.generated_at ? (
          <Badge variant='outline' className='text-xs'>
            {t('Updated {{time}}', {
              time: formatDateTime(filtersQuery.data.meta.generated_at),
            })}
          </Badge>
        ) : null}
        <Button
          variant='outline'
          size='sm'
          onClick={() => setRefreshKey((value) => value + 1)}
        >
          <HugeiconsIcon
            icon={RefreshIcon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <Tabs
          value={view}
          onValueChange={(value) => setView(value as View)}
          className='min-w-0'
        >
          <div className='flex min-w-0 flex-col gap-3 sm:gap-4'>
            <div className='-mx-1 max-w-full overflow-x-auto px-1 pb-1'>
              <TabsList variant='line' aria-label={t('Channel Observability')}>
                {tabs.map((tab) => (
                  <TabsTrigger key={tab.value} value={tab.value}>
                    <HugeiconsIcon
                      icon={tab.icon}
                      strokeWidth={2}
                      data-icon='inline-start'
                    />
                    {tab.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>
            <FilterBar
              filters={filters}
              options={filtersQuery.data}
              onChange={setFilters}
              onReset={reset}
            />
            <TabsContent value='overview'>
              <OverviewView
                filters={filters}
                refreshKey={refreshKey}
                onReset={reset}
              />
            </TabsContent>
            <TabsContent value='operations'>
              <StabilityView
                filters={filters}
                refreshKey={refreshKey}
                retentionDays={filtersQuery.data?.meta.retention_days}
                onReset={reset}
              />
            </TabsContent>
            <TabsContent value='channels'>
              <ChannelsView
                filters={filters}
                refreshKey={refreshKey}
                onReset={reset}
                onOpenGrouped={() => setView('operations')}
              />
            </TabsContent>
            <TabsContent value='failures'>
              <FailuresView
                filters={filters}
                refreshKey={refreshKey}
                onReset={reset}
              />
            </TabsContent>
            <TabsContent value='probe'>
              <ProbeView filters={filters} refreshKey={refreshKey} />
            </TabsContent>
          </div>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
