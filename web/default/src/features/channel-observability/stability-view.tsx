/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useRef, useState, type ReactElement } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  InformationCircleIcon,
  Loading03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { createAnalyticsParams, getStability } from './api'
import {
  formatCompact,
  formatDuration,
  formatInteger,
  formatPercent,
  PageControls,
  QualityNotice,
  STABILITY_WINDOWS,
  ViewEmpty,
  ViewError,
  ViewSkeleton,
  windowLabel,
} from './shared'
import type {
  AnalyticsFilters,
  ModelDimension,
  StabilityItem,
  StabilityResponse,
  StabilityWindow,
} from './types'

type Level = 'group' | 'channel' | 'model'
type TreePlan = { levels: Level[] }
type ChildEntry = {
  loading: boolean
  error?: string
  data?: StabilityResponse
  page: number
}

const TREE_PLANS: Record<string, TreePlan> = {
  group_model: { levels: ['group', 'model'] },
  group_channel: { levels: ['group', 'channel'] },
  channel_model: { levels: ['channel', 'model'] },
  group_channel_model: { levels: ['group', 'channel', 'model'] },
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

function identityValue(
  item: StabilityItem,
  level: Level,
  modelDimension: ModelDimension
) {
  if (level === 'group') return item.group || item.group_name || ''
  if (level === 'channel') return String(item.channel_id || '')
  return modelDimension === 'upstream'
    ? item.upstream_model || item.requested_model || item.model_hash || ''
    : item.requested_model || item.upstream_model || item.model_hash || ''
}

function nodeKey(
  item: StabilityItem,
  levelIndex: number,
  plan: TreePlan,
  modelDimension: ModelDimension
) {
  return plan.levels
    .slice(0, levelIndex + 1)
    .map((level) => identityValue(item, level, modelDimension))
    .join('\u0001')
}

function windowFor(
  item: StabilityItem,
  seconds: number
): StabilityWindow | undefined {
  return item.windows?.find((window) => window.window_seconds === seconds)
}

function WindowCell({ window }: { window?: StabilityWindow }) {
  const { t } = useTranslation()
  if (!window) return <span className='text-muted-foreground'>-</span>
  return (
    <div className='min-w-32'>
      <div className='flex items-center gap-1.5'>
        <span className='font-semibold tabular-nums'>
          {formatPercent(window.quality_success_rate)}
        </span>
        {!window.sample_sufficient && (
          <Badge variant='outline'>{t('Low sample')}</Badge>
        )}
      </div>
      <div className='text-muted-foreground mt-1 text-xs'>
        {t('{{calls}} calls · {{failures}} failures', {
          calls: formatCompact(window.channel_attempt_count),
          failures: formatCompact(window.failure_count),
        })}
      </div>
      <div className='text-muted-foreground text-xs'>
        P95 {formatDuration(window.p95_latency_ms)} ·{' '}
        {formatCompact(window.total_tokens)} Token
      </div>
    </div>
  )
}

export function StabilityView({
  filters,
  refreshKey,
  retentionDays = 7,
  initialDimension = 'group_channel_model',
  onReset,
}: {
  filters: AnalyticsFilters
  refreshKey: number
  retentionDays?: number
  initialDimension?: string
  onReset: () => void
}) {
  const { t } = useTranslation()
  const [dimension, setDimension] = useState(initialDimension)
  const [modelDimension, setModelDimension] =
    useState<ModelDimension>('requested')
  const [sortBy, setSortBy] = useState('failure_count')
  const [page, setPage] = useState(1)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [children, setChildren] = useState<Record<string, ChildEntry>>({})
  const childControllers = useRef<Map<string, AbortController>>(new Map())
  const childGeneration = useRef(0)
  const plan = TREE_PLANS[dimension]
  const windows = useMemo(() => {
    const maxSeconds = Math.max(300, retentionDays * 86400)
    const supported = STABILITY_WINDOWS.filter(
      (seconds) => seconds <= maxSeconds
    )
    return supported.length ? supported : [maxSeconds]
  }, [retentionDays])
  const baseFilterParams = useMemo(
    () =>
      createAnalyticsParams(filters, {}, { includeStatus: false }).toString(),
    [filters]
  )
  const rootParams = useMemo(() => {
    const params = new URLSearchParams(baseFilterParams)
    params.set('dimension', plan?.levels[0] ?? dimension)
    params.set('model_dimension', modelDimension)
    params.set('windows', windows.join(','))
    params.set('page', String(page))
    params.set('page_size', '30')
    params.set('sort_by', sortBy)
    params.set('sort_order', sortBy === 'quality_success_rate' ? 'asc' : 'desc')
    return params.toString()
  }, [baseFilterParams, dimension, modelDimension, page, plan, sortBy, windows])
  const query = useQuery({
    queryKey: ['channel-observability', 'stability', rootParams, refreshKey],
    queryFn: () => getStability(new URLSearchParams(rootParams)),
    placeholderData: (previous) => previous,
  })

  useEffect(() => {
    setPage(1)
  }, [baseFilterParams, dimension, modelDimension, sortBy])

  useEffect(() => {
    childGeneration.current += 1
    childControllers.current.forEach((controller) => controller.abort())
    childControllers.current.clear()
    setExpanded(new Set())
    setChildren({})
  }, [refreshKey, rootParams])

  useEffect(
    () => () => {
      childGeneration.current += 1
      childControllers.current.forEach((controller) => controller.abort())
      childControllers.current.clear()
    },
    []
  )

  const loadChildren = async (
    item: StabilityItem,
    levelIndex: number,
    key: string,
    append = false
  ) => {
    if (!plan || levelIndex >= plan.levels.length - 1) return
    const current = children[key]
    if (current?.loading) return
    const requestedPage = append ? (current?.page ?? 0) + 1 : 1
    childControllers.current.get(key)?.abort()
    const controller = new AbortController()
    const generation = childGeneration.current
    childControllers.current.set(key, controller)
    setChildren((entries) => ({
      ...entries,
      [key]: {
        ...entries[key],
        loading: true,
        error: undefined,
        page: requestedPage,
      },
    }))
    try {
      const params = new URLSearchParams(rootParams)
      params.set('dimension', plan.levels.slice(0, levelIndex + 2).join('_'))
      if (item.group) params.set('groups', item.group)
      if (item.channel_id) params.set('channel_ids', String(item.channel_id))
      params.set('page', String(requestedPage))
      params.set('page_size', '30')
      const data = await getStability(params, controller.signal)
      if (controller.signal.aborted || generation !== childGeneration.current) {
        return
      }
      setChildren((entries) => {
        const previousItems = append ? (entries[key]?.data?.items ?? []) : []
        const seen = new Set(
          previousItems.map((child) =>
            nodeKey(child, levelIndex + 1, plan, modelDimension)
          )
        )
        const merged = [
          ...previousItems,
          ...data.items.filter((child) => {
            const childKey = nodeKey(
              child,
              levelIndex + 1,
              plan,
              modelDimension
            )
            if (seen.has(childKey)) return false
            seen.add(childKey)
            return true
          }),
        ]
        return {
          ...entries,
          [key]: {
            loading: false,
            page: requestedPage,
            data: { ...data, items: merged },
          },
        }
      })
    } catch (error) {
      if (controller.signal.aborted || generation !== childGeneration.current) {
        return
      }
      setChildren((entries) => ({
        ...entries,
        [key]: {
          ...entries[key],
          loading: false,
          page: requestedPage,
          error: error instanceof Error ? error.message : t('Request failed'),
        },
      }))
    } finally {
      if (childControllers.current.get(key) === controller) {
        childControllers.current.delete(key)
      }
    }
  }

  const toggle = (item: StabilityItem, levelIndex: number, key: string) => {
    const opening = !expanded.has(key)
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
    if (!opening) {
      childControllers.current.get(key)?.abort()
      childControllers.current.delete(key)
      setChildren((entries) => {
        const entry = entries[key]
        if (!entry?.loading) return entries
        return { ...entries, [key]: { ...entry, loading: false } }
      })
    } else if (!children[key]?.data) {
      void loadChildren(item, levelIndex, key)
    }
  }

  const renderRows = (
    items: StabilityItem[],
    levelIndex: number,
    parentKey = ''
  ): ReactElement[] => {
    if (!plan) return []
    return items.flatMap((item) => {
      const ownKey = `${parentKey}/${nodeKey(item, levelIndex, plan, modelDimension)}`
      const level = plan.levels[levelIndex]
      const canExpand = levelIndex < plan.levels.length - 1
      const isExpanded = expanded.has(ownKey)
      const entry = children[ownKey]
      const modelName =
        modelDimension === 'upstream'
          ? item.upstream_model || item.requested_model
          : item.requested_model || item.upstream_model
      const title =
        level === 'group'
          ? item.group_name || item.group || t('Unlabelled group')
          : level === 'channel'
            ? item.channel_name || `#${item.channel_id}`
            : modelName || t('Unknown model')
      const subtitle =
        level === 'group'
          ? item.group_name && item.group_name !== item.group
            ? item.group
            : ''
          : level === 'channel'
            ? `#${item.channel_id} · ${item.channel_type_name || t('Unknown type')}`
            : item.requested_model &&
                item.upstream_model &&
                item.requested_model !== item.upstream_model
              ? `${modelDimension === 'upstream' ? t('Requested model') : t('Upstream model')}: ${modelDimension === 'upstream' ? item.requested_model : item.upstream_model}`
              : ''
      const row = (
        <TableRow
          key={ownKey}
          className={
            levelIndex > 0 ? 'bg-muted/20 hover:bg-muted/30' : undefined
          }
        >
          <TableCell
            className={cn(levelIndex > 0 && 'border-primary/30 border-l-2')}
          >
            <div
              className='flex min-w-60 items-start gap-2'
              style={{ paddingInlineStart: `${levelIndex * 1.5}rem` }}
            >
              {canExpand ? (
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-expanded={isExpanded}
                  aria-label={isExpanded ? t('Collapse') : t('Expand')}
                  onClick={() => toggle(item, levelIndex, ownKey)}
                >
                  <HugeiconsIcon
                    icon={isExpanded ? ArrowDown01Icon : ArrowRight01Icon}
                    strokeWidth={2}
                  />
                </Button>
              ) : (
                <span className='size-7 shrink-0' />
              )}
              <div className='min-w-0'>
                <div className='max-w-72 truncate font-medium' title={title}>
                  {title}
                </div>
                {subtitle && (
                  <div className='text-muted-foreground text-xs'>
                    {subtitle}
                  </div>
                )}
              </div>
            </div>
          </TableCell>
          {windows.map((seconds) => (
            <TableCell key={seconds}>
              <WindowCell window={windowFor(item, seconds)} />
            </TableCell>
          ))}
        </TableRow>
      )
      if (!canExpand || !isExpanded) return [row]

      const stateRows: ReactElement[] = [row]
      if (entry?.data?.items.length) {
        stateRows.push(...renderRows(entry.data.items, levelIndex + 1, ownKey))
      }
      if (!entry || entry.loading) {
        stateRows.push(
          <TableRow key={`${ownKey}-loading`} className='bg-muted/20'>
            <TableCell colSpan={windows.length + 1}>
              <div className='flex items-center gap-2 pl-12'>
                <HugeiconsIcon
                  icon={Loading03Icon}
                  strokeWidth={2}
                  className='size-4 animate-spin'
                />
                {t('Loading child metrics...')}
              </div>
            </TableCell>
          </TableRow>
        )
      } else if (entry.error) {
        stateRows.push(
          <TableRow key={`${ownKey}-error`} className='bg-muted/20'>
            <TableCell colSpan={windows.length + 1}>
              <div className='text-destructive flex items-center justify-between gap-3 pl-12'>
                <span>{entry.error}</span>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void loadChildren(item, levelIndex, ownKey)}
                >
                  {t('Retry')}
                </Button>
              </div>
            </TableCell>
          </TableRow>
        )
      } else if (!entry.data?.items.length) {
        stateRows.push(
          <TableRow key={`${ownKey}-empty`} className='bg-muted/20'>
            <TableCell
              colSpan={windows.length + 1}
              className='text-muted-foreground pl-20'
            >
              {t('No child samples match the current filters')}
            </TableCell>
          </TableRow>
        )
      } else if (entry.data.items.length < entry.data.total) {
        stateRows.push(
          <TableRow key={`${ownKey}-more`} className='bg-muted/20'>
            <TableCell colSpan={windows.length + 1}>
              <div className='flex items-center justify-between gap-3 pl-12'>
                <span className='text-muted-foreground text-xs'>
                  {t('Loaded {{loaded}} of {{total}}', {
                    loaded: formatInteger(entry.data.items.length),
                    total: formatInteger(entry.data.total),
                  })}
                </span>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    void loadChildren(item, levelIndex, ownKey, true)
                  }
                >
                  {t('Load more')}
                </Button>
              </div>
            </TableCell>
          </TableRow>
        )
      }
      return stateRows
    })
  }

  if (query.isLoading && !query.data) return <ViewSkeleton />
  if (query.error)
    return (
      <ViewError error={query.error} retry={() => void query.refetch()} t={t} />
    )
  const data = query.data
  if (!data?.items.length) {
    return <ViewEmpty reset={onReset} t={t} meta={data?.meta} />
  }

  return (
    <div className='flex min-w-0 flex-col gap-3 sm:gap-4'>
      <Alert className='py-2'>
        <HugeiconsIcon icon={InformationCircleIcon} strokeWidth={2} />
        <AlertTitle>{t('Multi-window stability matrix')}</AlertTitle>
        <AlertDescription className='text-xs'>
          {t(
            'Compare short-term incidents with long-term degradation. Expand each row to load the next level only when needed.'
          )}
        </AlertDescription>
      </Alert>
      <QualityNotice meta={data.meta} />
      <Card className='min-w-0 overflow-hidden'>
        <CardHeader className='bg-muted/20 gap-3 border-b sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <CardTitle>{t('Group, channel, and model stability')}</CardTitle>
            <CardDescription className='max-w-2xl'>
              {t(
                'Quality success rate uses attributable samples and shows low-sample windows explicitly.'
              )}
            </CardDescription>
          </div>
          <div className='flex shrink-0 flex-wrap items-end gap-2'>
            <CompactSelect
              label={t('Analysis dimension')}
              value={dimension}
              options={[
                {
                  value: 'group_channel_model',
                  label: t('Group → channel → model'),
                },
                { value: 'group_model', label: t('Group → model') },
                { value: 'channel_model', label: t('Channel → model') },
                { value: 'group_channel', label: t('Group → channel') },
                { value: 'model', label: t('Models only') },
                { value: 'channel', label: t('Channels only') },
                { value: 'group', label: t('Groups only') },
              ]}
              onChange={setDimension}
            />
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
              label={t('Sort')}
              value={sortBy}
              options={[
                { value: 'failure_count', label: t('Recent failures') },
                {
                  value: 'quality_success_rate',
                  label: t('Recent quality rate'),
                },
                { value: 'request_count', label: t('Recent call volume') },
                { value: 'p95_latency_ms', label: t('Recent P95 latency') },
                { value: 'retry_rate', label: t('Recent retry rate') },
                { value: 'total_tokens', label: t('Recent tokens') },
              ]}
              onChange={setSortBy}
            />
          </div>
        </CardHeader>
        <CardContent className='p-0'>
          <Table className='min-w-[760px]'>
            <TableHeader>
              <TableRow className='bg-muted/20 hover:bg-muted/20'>
                <TableHead>{t('Entity')}</TableHead>
                {windows.map((seconds) => (
                  <TableHead key={seconds}>{windowLabel(seconds, t)}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>{renderRows(data.items, 0)}</TableBody>
          </Table>
          <PageControls page={page} total={data.total} onPage={setPage} t={t} />
        </CardContent>
      </Card>
    </div>
  )
}
