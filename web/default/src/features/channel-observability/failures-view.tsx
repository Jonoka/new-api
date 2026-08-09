/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { createAnalyticsParams, getFailures, getStatusCodes } from './api'
import {
  formatDateTime,
  formatDuration,
  formatInteger,
  getStatusLabel,
  PageControls,
  QualityNotice,
  statusBadgeVariant,
  ViewEmpty,
  ViewError,
  ViewSkeleton,
} from './shared'
import type { AnalyticsFilters, FailureItem, StatusScope } from './types'

const STAGE_LABELS: Record<string, string> = {
  auth: 'Authentication',
  authentication: 'Authentication',
  dispatch: 'Dispatch',
  channel_selection: 'Channel selection',
  pre_upstream: 'Before upstream request',
  connect: 'Upstream connection',
  upstream_response: 'Upstream response',
  stream: 'Streaming',
  local: 'Local processing',
}

export function FailuresView({
  filters,
  refreshKey,
  onReset,
}: {
  filters: AnalyticsFilters
  refreshKey: number
  onReset: () => void
}) {
  const { t } = useTranslation()
  const [scope, setScope] = useState<StatusScope>('upstream')
  const [page, setPage] = useState(1)
  const base = useMemo(
    () => createAnalyticsParams(filters, {}, { statusScope: scope }).toString(),
    [filters, scope]
  )
  const statusParams = useMemo(() => {
    const params = new URLSearchParams(base)
    params.set(
      'metric_scope',
      scope === 'client' ? 'final_request' : 'upstream_call'
    )
    return params.toString()
  }, [base, scope])
  const failureParams = useMemo(() => {
    const params = createAnalyticsParams(
      filters,
      { page, page_size: 30 },
      { statusScope: scope, includeStream: false }
    )
    return params.toString()
  }, [filters, page, scope])
  const statusQuery = useQuery({
    queryKey: [
      'channel-observability',
      'status-codes',
      statusParams,
      refreshKey,
    ],
    queryFn: () => getStatusCodes(new URLSearchParams(statusParams)),
  })
  const failuresQuery = useQuery({
    queryKey: ['channel-observability', 'failures', failureParams, refreshKey],
    queryFn: () => getFailures(new URLSearchParams(failureParams)),
    placeholderData: (previous) => previous,
  })

  useEffect(() => setPage(1), [filters, scope])

  if (
    statusQuery.isLoading ||
    (failuresQuery.isLoading && !failuresQuery.data)
  ) {
    return <ViewSkeleton />
  }
  const error = statusQuery.error ?? failuresQuery.error
  if (error) {
    return (
      <ViewError
        error={error}
        retry={() => {
          void statusQuery.refetch()
          void failuresQuery.refetch()
        }}
        t={t}
      />
    )
  }
  const statuses = statusQuery.data?.items ?? []
  const failures = failuresQuery.data?.items ?? []
  if (!statuses.length && !failures.length) {
    return (
      <ViewEmpty
        reset={onReset}
        t={t}
        meta={statusQuery.data?.meta ?? failuresQuery.data?.meta}
      />
    )
  }

  const maxStatus = Math.max(1, ...statuses.map((item) => item.count))
  const stages = statusQuery.data?.error_stages ?? []
  const maxStage = Math.max(1, ...stages.map((item) => item.count))
  const statusGroups = statuses.reduce(
    (groups, item) => {
      if (!item.status_present) groups.unknown += item.count
      else if (item.status_code === 0) groups.noResponse += item.count
      else if (item.status_code >= 200 && item.status_code < 300)
        groups.success += item.count
      else if (item.status_code >= 400 && item.status_code < 500)
        groups.client += item.count
      else if (item.status_code >= 500) groups.server += item.count
      else groups.unknown += item.count
      return groups
    },
    { success: 0, client: 0, server: 0, noResponse: 0, unknown: 0 }
  )

  return (
    <div className='flex min-w-0 flex-col gap-3 sm:gap-4'>
      <QualityNotice
        meta={statusQuery.data?.meta ?? failuresQuery.data?.meta}
      />
      <div className='flex justify-end'>
        <ToggleGroup
          variant='outline'
          size='sm'
          value={[scope]}
          onValueChange={(values) => {
            const next = values[0] as StatusScope | undefined
            if (next) setScope(next)
          }}
        >
          <ToggleGroupItem value='upstream'>
            {t('Upstream status')}
          </ToggleGroupItem>
          <ToggleGroupItem value='client'>{t('Client status')}</ToggleGroupItem>
        </ToggleGroup>
      </div>

      <div className='grid min-w-0 gap-3 lg:grid-cols-2'>
        <Card size='sm' className='min-w-0 shadow-none'>
          <CardHeader className='bg-muted/20 border-b pb-3'>
            <CardTitle>{t('Status-code distribution')}</CardTitle>
            <CardDescription>
              {scope === 'client'
                ? t('Final status returned to logical client requests')
                : t('Original status returned by upstream calls')}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-3 pt-3'>
            <div className='flex flex-wrap gap-2'>
              <Badge>{`2xx · ${formatInteger(statusGroups.success)}`}</Badge>
              <Badge variant='secondary'>{`4xx · ${formatInteger(statusGroups.client)}`}</Badge>
              <Badge variant='destructive'>{`5xx · ${formatInteger(statusGroups.server)}`}</Badge>
              <Badge variant='destructive'>{`${t('No response')} · ${formatInteger(statusGroups.noResponse)}`}</Badge>
              <Badge variant='outline'>{`${t('Unknown')} · ${formatInteger(statusGroups.unknown)}`}</Badge>
            </div>
            {statuses.slice(0, 12).map((item) => (
              <Progress
                key={`${item.status_present}-${item.status_code}`}
                value={(item.count / maxStatus) * 100}
              >
                <ProgressLabel>{getStatusLabel(item)}</ProgressLabel>
                <ProgressValue>{() => formatInteger(item.count)}</ProgressValue>
              </Progress>
            ))}
          </CardContent>
        </Card>

        <Card size='sm' className='min-w-0 shadow-none'>
          <CardHeader className='bg-muted/20 border-b pb-3'>
            <CardTitle>{t('Failure stages')}</CardTitle>
            <CardDescription>
              {t('Locate where failures occur in the request path.')}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-3 pt-3'>
            {stages.length ? (
              stages.slice(0, 12).map((stage) => (
                <Progress
                  key={stage.error_stage}
                  value={(stage.count / maxStage) * 100}
                >
                  <ProgressLabel>
                    {t(STAGE_LABELS[stage.error_stage] ?? stage.error_stage)}
                  </ProgressLabel>
                  <ProgressValue>
                    {() => formatInteger(stage.count)}
                  </ProgressValue>
                </Progress>
              ))
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No failure-stage samples')}
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      <section className='flex min-w-0 flex-col gap-2'>
        <div>
          <h3 className='text-sm font-semibold'>
            {t('Recent failed requests')}
          </h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Only sanitized summaries are retained; complete successful requests remain in usage logs.'
            )}
          </p>
        </div>
        {failures.map((failure) => (
          <FailureCard key={failure.event_id} failure={failure} scope={scope} />
        ))}
        {failuresQuery.data && (
          <PageControls
            page={page}
            total={failuresQuery.data.total}
            pageSize={failuresQuery.data.page_size}
            onPage={setPage}
            t={t}
          />
        )}
      </section>
    </div>
  )
}

function FailureCard({
  failure,
  scope,
}: {
  failure: FailureItem
  scope: StatusScope
}) {
  const { t } = useTranslation()
  const present =
    scope === 'client'
      ? failure.client_status_present
      : failure.upstream_status_present
  const code =
    scope === 'client'
      ? failure.client_status_code
      : failure.upstream_status_code
  const model =
    failure.requested_model || failure.upstream_model || t('Unknown model')
  return (
    <Card size='sm' className='shadow-none'>
      <CardHeader className='bg-muted/20 gap-2 border-b pb-3 sm:flex-row sm:items-start sm:justify-between'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge
              className='text-xs'
              variant={statusBadgeVariant(code, present)}
            >
              {present ? code || t('No response') : t('Unknown status')}
            </Badge>
            <CardTitle className='truncate text-sm'>
              {failure.channel_name || `#${failure.channel_id}`} · {model}
            </CardTitle>
          </div>
          <CardDescription className='mt-1'>
            {failure.error_summary || t('No error summary')}
          </CardDescription>
        </div>
        <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
          {formatDateTime(failure.created_at)}
        </span>
      </CardHeader>
      <CardContent className='flex flex-wrap gap-1.5 pt-3'>
        {failure.request_id && (
          <Badge variant='outline'>
            {t('Request ID')}: {failure.request_id}
          </Badge>
        )}
        <Badge variant='secondary'>{failure.outcome}</Badge>
        <Badge variant='outline'>
          {t(STAGE_LABELS[failure.error_stage] ?? failure.error_stage)}
        </Badge>
        <Badge variant='outline'>
          {t('Attempt')} #{failure.attempt_seq || '-'}
        </Badge>
        <Badge variant='outline'>
          {failure.retry_planned ? t('Retry planned') : t('No further retry')}
        </Badge>
        <Badge variant='outline'>
          {failure.data_origin === 'legacy'
            ? t('Historical inference')
            : t('Live telemetry')}
        </Badge>
        <Badge variant='outline'>{formatDuration(failure.latency_ms)}</Badge>
        {failure.partial_response && (
          <Badge variant='destructive'>{t('Partial response')}</Badge>
        )}
      </CardContent>
    </Card>
  )
}
