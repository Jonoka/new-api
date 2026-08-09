/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { createAnalyticsParams, getChannelModels, getChannels } from './api'
import {
  formatCompact,
  formatDateTime,
  formatDuration,
  formatInteger,
  formatMoney,
  formatPercent,
  getStatusLabel,
  PageControls,
  QualityNotice,
  statusBadgeVariant,
  ViewEmpty,
  ViewError,
  ViewSkeleton,
} from './shared'
import type {
  AnalyticsFilters,
  ModelAnalyticsItem,
  ModelDimension,
  PagedResponse,
} from './types'

type ModelEntry = {
  loading: boolean
  error?: string
  data?: PagedResponse<ModelAnalyticsItem>
  page: number
}

function CompactSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string
  options: Array<{ value: string; label: string }>
  onChange: (value: string) => void
}) {
  return (
    <Field orientation='horizontal' className='w-auto min-w-0'>
      <FieldLabel className='text-muted-foreground text-xs'>{label}</FieldLabel>
      <Select
        items={options}
        value={value}
        onValueChange={(next) => next && onChange(next)}
      >
        <SelectTrigger className='h-8 min-w-28'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  )
}

export function ChannelsView({
  filters,
  refreshKey,
  onReset,
  onOpenGrouped,
}: {
  filters: AnalyticsFilters
  refreshKey: number
  onReset: () => void
  onOpenGrouped: () => void
}) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [sortBy, setSortBy] = useState('channel_name')
  const [modelDimension, setModelDimension] =
    useState<ModelDimension>('requested')
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const [models, setModels] = useState<Record<number, ModelEntry>>({})
  const modelControllers = useRef<Map<number, AbortController>>(new Map())
  const modelGeneration = useRef(0)
  const filterParams = useMemo(
    () =>
      createAnalyticsParams(filters, {}, { includeStatus: false }).toString(),
    [filters]
  )
  const queryParams = useMemo(() => {
    const params = new URLSearchParams(filterParams)
    params.set('page', String(page))
    params.set('page_size', '30')
    params.set('sort_by', sortBy)
    params.set('sort_order', sortBy === 'channel_name' ? 'asc' : 'desc')
    return params.toString()
  }, [filterParams, page, sortBy])
  const query = useQuery({
    queryKey: ['channel-observability', 'channels', queryParams, refreshKey],
    queryFn: () => getChannels(new URLSearchParams(queryParams)),
    placeholderData: (previous) => previous,
  })

  useEffect(() => {
    setPage(1)
  }, [filterParams])

  useEffect(() => {
    modelGeneration.current += 1
    modelControllers.current.forEach((controller) => controller.abort())
    modelControllers.current.clear()
    setExpanded(new Set())
    setModels({})
  }, [modelDimension, queryParams, refreshKey])

  useEffect(
    () => () => {
      modelGeneration.current += 1
      modelControllers.current.forEach((controller) => controller.abort())
      modelControllers.current.clear()
    },
    []
  )

  const loadModels = async (channelId: number, requestedPage = 1) => {
    modelControllers.current.get(channelId)?.abort()
    const controller = new AbortController()
    const generation = modelGeneration.current
    modelControllers.current.set(channelId, controller)
    setModels((current) => ({
      ...current,
      [channelId]: { loading: true, page: requestedPage },
    }))
    try {
      const params = new URLSearchParams(filterParams)
      params.set('page', String(requestedPage))
      params.set('page_size', '30')
      params.set('sort_by', 'request_count')
      params.set('sort_order', 'desc')
      params.set('model_dimension', modelDimension)
      const data = await getChannelModels(channelId, params, controller.signal)
      if (controller.signal.aborted || generation !== modelGeneration.current) {
        return
      }
      setModels((current) => ({
        ...current,
        [channelId]: { loading: false, data, page: requestedPage },
      }))
    } catch (error) {
      if (controller.signal.aborted || generation !== modelGeneration.current) {
        return
      }
      setModels((current) => ({
        ...current,
        [channelId]: {
          loading: false,
          error: error instanceof Error ? error.message : t('Request failed'),
          page: requestedPage,
        },
      }))
    } finally {
      if (modelControllers.current.get(channelId) === controller) {
        modelControllers.current.delete(channelId)
      }
    }
  }

  const toggle = (channelId: number) => {
    const opening = !expanded.has(channelId)
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(channelId)) next.delete(channelId)
      else next.add(channelId)
      return next
    })
    if (!opening) {
      modelControllers.current.get(channelId)?.abort()
      modelControllers.current.delete(channelId)
      setModels((current) => {
        const entry = current[channelId]
        if (!entry?.loading) return current
        return {
          ...current,
          [channelId]: { ...entry, loading: false },
        }
      })
    } else if (!models[channelId]?.data) {
      void loadModels(channelId)
    }
  }

  if (query.isLoading && !query.data) return <ViewSkeleton />
  if (query.error) {
    return (
      <ViewError error={query.error} retry={() => void query.refetch()} t={t} />
    )
  }
  const data = query.data
  if (!data?.items.length) {
    return <ViewEmpty reset={onReset} t={t} meta={data?.meta} />
  }

  return (
    <div className='flex min-w-0 flex-col gap-3 sm:gap-4'>
      <QualityNotice meta={data.meta} />
      <Card className='min-w-0 overflow-hidden'>
        <CardHeader className='bg-muted/20 gap-3 border-b sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <CardTitle>{t('Channels and models')}</CardTitle>
            <CardDescription className='max-w-2xl'>
              {t(
                'Expand a channel to inspect model-level calls, cache usage, latency, and failures.'
              )}
            </CardDescription>
          </div>
          <div className='flex shrink-0 flex-wrap items-end gap-2'>
            <CompactSelect
              label={t('Model dimension')}
              value={modelDimension}
              options={[
                { value: 'requested', label: t('Requested model') },
                { value: 'upstream', label: t('Upstream model') },
              ]}
              onChange={(value) => setModelDimension(value as ModelDimension)}
            />
            <CompactSelect
              label={t('Sort channels')}
              value={sortBy}
              options={[
                { value: 'channel_name', label: t('Channel name') },
                { value: 'request_count', label: t('Call volume') },
                {
                  value: 'quality_success_rate',
                  label: t('Quality success rate'),
                },
                { value: 'p95_latency_ms', label: t('P95 latency') },
                { value: 'charged_quota', label: t('Cost') },
                { value: 'failure_count', label: t('Failures') },
              ]}
              onChange={(value) => {
                setPage(1)
                setSortBy(value)
              }}
            />
            <Button variant='outline' size='sm' onClick={onOpenGrouped}>
              {t('Group view')}
            </Button>
          </div>
        </CardHeader>
        <CardContent className='p-0'>
          <Table className='min-w-[1120px]'>
            <TableHeader>
              <TableRow className='bg-muted/20 hover:bg-muted/20'>
                <TableHead>{t('Channel')}</TableHead>
                <TableHead>{t('Calls / retries')}</TableHead>
                <TableHead>{t('Quality success rate')}</TableHead>
                <TableHead>{t('Status codes')}</TableHead>
                <TableHead>{t('Input / output')}</TableHead>
                <TableHead>{t('Cache read / write')}</TableHead>
                <TableHead>{t('Latency average / P95')}</TableHead>
                <TableHead>{t('Cost')}</TableHead>
                <TableHead>{t('Last failure')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.map((channel) => {
                const isExpanded = expanded.has(channel.channel_id)
                const modelEntry = models[channel.channel_id]
                return (
                  <Fragment key={channel.channel_id}>
                    <TableRow>
                      <TableCell>
                        <div className='flex min-w-52 items-center gap-2'>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={
                              isExpanded ? t('Collapse') : t('Expand')
                            }
                            aria-expanded={isExpanded}
                            onClick={() => toggle(channel.channel_id)}
                          >
                            <HugeiconsIcon
                              icon={
                                isExpanded ? ArrowDown01Icon : ArrowRight01Icon
                              }
                              strokeWidth={2}
                            />
                          </Button>
                          <div className='min-w-0'>
                            <div
                              className='max-w-64 truncate font-medium'
                              title={channel.channel_name}
                            >
                              {channel.channel_name || `#${channel.channel_id}`}
                            </div>
                            <div className='text-muted-foreground text-xs'>
                              #{channel.channel_id} ·{' '}
                              {channel.channel_type_name}
                            </div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <span className='font-medium'>
                          {formatInteger(channel.channel_attempt_count)}
                        </span>
                        <div className='text-muted-foreground text-xs'>
                          {t('{{count}} retries', {
                            count: formatInteger(channel.retry_count),
                          })}
                        </div>
                      </TableCell>
                      <TableCell>
                        {formatPercent(channel.channel_quality_success_rate)}
                      </TableCell>
                      <TableCell>
                        <div className='flex max-w-48 flex-wrap gap-1'>
                          {channel.top_status_codes
                            ?.slice(0, 3)
                            .map((status) => (
                              <Badge
                                key={`${status.status_present}-${status.status_code}`}
                                className='text-xs'
                                variant={statusBadgeVariant(
                                  status.status_code,
                                  status.status_present
                                )}
                              >
                                {status.status_present
                                  ? status.status_code || t('No response')
                                  : t('Unknown')}{' '}
                                · {formatCompact(status.count)}
                              </Badge>
                            ))}
                        </div>
                      </TableCell>
                      <TableCell>
                        {formatCompact(channel.input_tokens_total)} /{' '}
                        {formatCompact(channel.output_tokens)}
                      </TableCell>
                      <TableCell>
                        {formatCompact(channel.cache_read_tokens)} /{' '}
                        {formatCompact(channel.cache_write_tokens)}
                        <div className='text-muted-foreground text-xs'>
                          {t('{{rate}} token hit rate', {
                            rate: formatPercent(channel.cache_token_hit_rate),
                          })}
                        </div>
                      </TableCell>
                      <TableCell>
                        {formatDuration(channel.avg_latency_ms)} /{' '}
                        {formatDuration(channel.p95_latency_ms)}
                      </TableCell>
                      <TableCell>
                        {formatMoney(channel.charged_micro_usd)}
                      </TableCell>
                      <TableCell>
                        {formatDateTime(channel.last_failure_at)}
                      </TableCell>
                    </TableRow>
                    {isExpanded && (
                      <ModelRows
                        channelId={channel.channel_id}
                        entry={modelEntry}
                        dimension={modelDimension}
                        onRetry={() =>
                          void loadModels(
                            channel.channel_id,
                            modelEntry?.page ?? 1
                          )
                        }
                        onPage={(nextPage) =>
                          void loadModels(channel.channel_id, nextPage)
                        }
                      />
                    )}
                  </Fragment>
                )
              })}
            </TableBody>
          </Table>
          <PageControls page={page} total={data.total} onPage={setPage} t={t} />
        </CardContent>
      </Card>
    </div>
  )
}

function ModelRows({
  channelId,
  entry,
  dimension,
  onRetry,
  onPage,
}: {
  channelId: number
  entry?: ModelEntry
  dimension: ModelDimension
  onRetry: () => void
  onPage: (page: number) => void
}) {
  const { t } = useTranslation()
  if (!entry || entry.loading) {
    return (
      <TableRow>
        <TableCell colSpan={9} className='bg-muted/20'>
          <div className='flex items-center gap-2 pl-10 text-sm'>
            <HugeiconsIcon
              icon={Loading03Icon}
              strokeWidth={2}
              className='size-4 animate-spin'
            />
            {t('Loading model analytics...')}
          </div>
        </TableCell>
      </TableRow>
    )
  }
  if (entry.error) {
    return (
      <TableRow>
        <TableCell colSpan={9} className='bg-muted/20'>
          <div className='text-destructive flex items-center justify-between gap-3 pl-10'>
            <span>{entry.error}</span>
            <Button variant='outline' size='sm' onClick={onRetry}>
              {t('Retry')}
            </Button>
          </div>
        </TableCell>
      </TableRow>
    )
  }
  if (!entry.data?.items.length) {
    return (
      <TableRow>
        <TableCell
          colSpan={9}
          className='bg-muted/20 text-muted-foreground pl-20'
        >
          {t('No model-level samples in this range')}
        </TableCell>
      </TableRow>
    )
  }
  return (
    <>
      {entry.data.items.map((model) => {
        const primary =
          dimension === 'upstream'
            ? model.upstream_model || model.requested_model
            : model.requested_model || model.upstream_model
        const mapped =
          model.requested_model &&
          model.upstream_model &&
          model.requested_model !== model.upstream_model
            ? dimension === 'upstream'
              ? `${t('Requested model')}: ${model.requested_model}`
              : `${t('Upstream model')}: ${model.upstream_model}`
            : ''
        return (
          <TableRow
            key={`${channelId}-${model.model_hash}-${primary}`}
            className='bg-muted/20 hover:bg-muted/30'
          >
            <TableCell className='border-primary/30 border-l-2 pl-14'>
              <div className='max-w-64 truncate font-medium' title={primary}>
                {primary || t('Unknown model')}
              </div>
              {mapped && (
                <div className='text-muted-foreground text-xs'>{mapped}</div>
              )}
            </TableCell>
            <TableCell>
              {formatInteger(model.channel_attempt_count)} /{' '}
              {formatInteger(model.retry_count)}
            </TableCell>
            <TableCell>
              {formatPercent(model.channel_quality_success_rate)}
            </TableCell>
            <TableCell>
              <div className='flex flex-wrap gap-1'>
                {model.top_status_codes?.slice(0, 2).map((status) => (
                  <Badge
                    key={`${status.status_present}-${status.status_code}`}
                    className='text-xs'
                    variant={statusBadgeVariant(
                      status.status_code,
                      status.status_present
                    )}
                  >
                    {getStatusLabel(status)} · {formatCompact(status.count)}
                  </Badge>
                ))}
              </div>
            </TableCell>
            <TableCell>
              {formatCompact(model.input_tokens_total)} /{' '}
              {formatCompact(model.output_tokens)}
            </TableCell>
            <TableCell>
              {formatCompact(model.cache_read_tokens)} /{' '}
              {formatCompact(model.cache_write_tokens)}
            </TableCell>
            <TableCell>
              {formatDuration(model.avg_latency_ms)} /{' '}
              {formatDuration(model.p95_latency_ms)}
            </TableCell>
            <TableCell>{formatMoney(model.charged_micro_usd)}</TableCell>
            <TableCell>{formatDateTime(model.last_failure_at)}</TableCell>
          </TableRow>
        )
      })}
      {entry.data.total > entry.data.page_size && (
        <TableRow className='bg-muted/20 hover:bg-muted/30'>
          <TableCell colSpan={9} className='p-0'>
            <PageControls
              page={entry.page}
              total={entry.data.total}
              pageSize={entry.data.page_size}
              onPage={onPage}
              t={t}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  )
}
