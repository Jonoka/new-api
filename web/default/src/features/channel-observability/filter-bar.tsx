/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  FilterHorizontalIcon,
  FilterResetIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Collapsible, CollapsibleContent } from '@/components/ui/collapsible'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { searchAnalyticsModels } from './api'
import type {
  AnalyticsFilters,
  AnalyticsRange,
  FilterModel,
  FilterResponse,
} from './types'

type Option = { value: string; label: string }

function SelectField({
  label,
  value,
  options,
  placeholder,
  onChange,
  className,
}: {
  label: string
  value: string
  options: Option[]
  placeholder: string
  onChange: (value: string) => void
  className?: string
}) {
  const items = useMemo(
    () => [{ value: null, label: placeholder }, ...options],
    [options, placeholder]
  )
  return (
    <Field className={cn('min-w-0', className)}>
      <FieldLabel>{label}</FieldLabel>
      <Select
        items={items}
        value={value || null}
        onValueChange={(next) => onChange(next ?? '')}
      >
        <SelectTrigger className='w-full'>
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

function useDebouncedValue(value: string, delay = 260) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay)
    return () => window.clearTimeout(timer)
  }, [delay, value])
  return debounced
}

function ModelField({
  dimension,
  label,
  value,
  model,
  initial,
  onChange,
}: {
  dimension: 'requested' | 'upstream'
  label: string
  value: string
  model: string
  initial: FilterModel[]
  onChange: (model: string, hash: string) => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const debounced = useDebouncedValue(search.trim())
  const searchQuery = useQuery({
    queryKey: ['channel-observability', 'model-filter', dimension, debounced],
    queryFn: () => searchAnalyticsModels(dimension, debounced),
    enabled: debounced.length > 0,
    staleTime: 30_000,
  })
  const options = useMemo(() => {
    const source = debounced ? (searchQuery.data?.items ?? []) : initial
    const next = [...source]
    if (value && !next.some((item) => item.model_hash === value)) {
      next.unshift({ value, model_hash: value, model, label: model || value })
    }
    return next
  }, [debounced, initial, model, searchQuery.data?.items, value])
  const items = useMemo(
    () => [
      { value: null, label: t('All models') },
      ...options.map((item) => ({ value: item.model_hash, label: item.label })),
    ],
    [options, t]
  )

  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <Input
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        placeholder={t('Search models')}
      />
      <Select
        items={items}
        value={value || null}
        onValueChange={(next) => {
          const selected = options.find((item) => item.model_hash === next)
          onChange(selected?.model ?? '', selected?.model_hash ?? '')
        }}
      >
        <SelectTrigger className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {options.map((item) => (
              <SelectItem key={item.model_hash} value={item.model_hash}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  )
}

function toLocalInput(timestamp: number) {
  const date = new Date(timestamp * 1000)
  const adjusted = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return adjusted.toISOString().slice(0, 16)
}

function fromLocalInput(value: string) {
  const timestamp = Math.floor(new Date(value).getTime() / 1000)
  return Number.isFinite(timestamp) ? timestamp : 0
}

export function FilterBar({
  filters,
  options,
  onChange,
  onReset,
}: {
  filters: AnalyticsFilters
  options?: FilterResponse
  onChange: (filters: AnalyticsFilters) => void
  onReset: () => void
}) {
  const { t } = useTranslation()
  const [advanced, setAdvanced] = useState(false)
  const update = <K extends keyof AnalyticsFilters>(
    key: K,
    value: AnalyticsFilters[K]
  ) => onChange({ ...filters, [key]: value })

  const ranges: Array<{ value: AnalyticsRange; label: string }> = [
    { value: '1h', label: t('Last hour') },
    { value: 'today', label: t('Today') },
    { value: 'yesterday', label: t('Yesterday') },
    { value: '7d', label: t('Last 7 days') },
    { value: 'custom', label: t('Custom') },
  ]
  const outcomeLabels: Record<string, string> = {
    success: t('Success'),
    http_error: t('HTTP error'),
    transport_error: t('Connection error'),
    protocol_error: t('Protocol error'),
    stream_error: t('Stream error'),
    local_error: t('Local error'),
    dispatch_error: t('Dispatch error'),
    client_cancelled: t('Client cancelled'),
  }

  return (
    <Card size='sm' className='shadow-none'>
      <CardContent className='flex flex-col gap-3 p-3 sm:p-4'>
        <div className='flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between'>
          <div className='max-w-full min-w-0 overflow-x-auto pb-0.5'>
            <ToggleGroup
              variant='outline'
              size='sm'
              value={[filters.range]}
              onValueChange={(values) => {
                const range = values[0] as AnalyticsRange | undefined
                if (range) update('range', range)
              }}
            >
              {ranges.map((range) => (
                <ToggleGroupItem key={range.value} value={range.value}>
                  {range.label}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          </div>
          <div className='flex flex-wrap items-end gap-2'>
            <SelectField
              label={t('Granularity')}
              value={filters.granularity}
              placeholder={t('Automatic')}
              options={[
                { value: 'auto', label: t('Automatic') },
                { value: '5m', label: t('5 minutes') },
              ]}
              onChange={(value) => update('granularity', value || 'auto')}
              className='w-full sm:w-36'
            />
            <Button
              variant='outline'
              size='sm'
              className='shrink-0'
              aria-expanded={advanced}
              aria-controls='channel-observability-advanced-filters'
              onClick={() => setAdvanced((value) => !value)}
            >
              <HugeiconsIcon
                icon={FilterHorizontalIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('Advanced filters')}
            </Button>
            <Button variant='ghost' size='sm' onClick={onReset}>
              <HugeiconsIcon
                icon={FilterResetIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('Reset')}
            </Button>
          </div>
        </div>

        <Collapsible open={filters.range === 'custom'}>
          <CollapsibleContent>
            <FieldGroup className='grid gap-3 border-t pt-3 sm:grid-cols-2'>
              <Field>
                <FieldLabel>{t('Start time')}</FieldLabel>
                <Input
                  type='datetime-local'
                  value={toLocalInput(filters.customStart)}
                  onChange={(event) =>
                    update('customStart', fromLocalInput(event.target.value))
                  }
                />
              </Field>
              <Field>
                <FieldLabel>{t('End time')}</FieldLabel>
                <Input
                  type='datetime-local'
                  value={toLocalInput(filters.customEnd)}
                  onChange={(event) =>
                    update('customEnd', fromLocalInput(event.target.value))
                  }
                />
              </Field>
            </FieldGroup>
          </CollapsibleContent>
        </Collapsible>

        <Collapsible open={advanced}>
          <CollapsibleContent id='channel-observability-advanced-filters'>
            <FieldGroup className='grid gap-3 border-t pt-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'>
              <SelectField
                label={t('Request group')}
                value={filters.group}
                placeholder={t('All groups')}
                options={(options?.groups ?? []).map((item) => ({
                  value: item.code,
                  label: item.name,
                }))}
                onChange={(value) => update('group', value)}
              />
              <SelectField
                label={t('Channel')}
                value={filters.channelId}
                placeholder={t('All channels')}
                options={(options?.channels ?? []).map((item) => ({
                  value: String(item.channel_id),
                  label: `${item.channel_name} (#${item.channel_id})`,
                }))}
                onChange={(value) => update('channelId', value)}
              />
              <SelectField
                label={t('Channel type')}
                value={filters.channelType}
                placeholder={t('All types')}
                options={(options?.channel_types ?? []).map((item) => ({
                  value: String(item.value),
                  label: item.label,
                }))}
                onChange={(value) => update('channelType', value)}
              />
              <ModelField
                dimension='requested'
                label={t('Requested model')}
                value={filters.requestedModelHash}
                model={filters.requestedModel}
                initial={options?.requested_model_options ?? []}
                onChange={(model, hash) =>
                  onChange({
                    ...filters,
                    requestedModel: model,
                    requestedModelHash: hash,
                  })
                }
              />
              <ModelField
                dimension='upstream'
                label={t('Upstream model')}
                value={filters.upstreamModelHash}
                model={filters.upstreamModel}
                initial={options?.upstream_model_options ?? []}
                onChange={(model, hash) =>
                  onChange({
                    ...filters,
                    upstreamModel: model,
                    upstreamModelHash: hash,
                  })
                }
              />
              <SelectField
                label={t('Outcome')}
                value={filters.outcome}
                placeholder={t('All outcomes')}
                options={(options?.outcomes ?? []).map((value) => ({
                  value,
                  label: outcomeLabels[value] ?? value,
                }))}
                onChange={(value) => update('outcome', value)}
              />
              <Field>
                <FieldLabel>{t('Status code')}</FieldLabel>
                <Input
                  value={filters.statusCode}
                  onChange={(event) => update('statusCode', event.target.value)}
                  placeholder={t('For example: 429 or 5xx')}
                />
              </Field>
              <SelectField
                label={t('Response mode')}
                value={filters.stream}
                placeholder={t('All')}
                options={[
                  { value: 'true', label: t('Streaming') },
                  { value: 'false', label: t('Non-streaming') },
                ]}
                onChange={(value) => update('stream', value)}
              />
              <SelectField
                label={t('Data origin')}
                value={filters.dataOrigin}
                placeholder={t('All')}
                options={[
                  { value: 'live,legacy', label: t('Live and historical') },
                  { value: 'live', label: t('Live only') },
                  { value: 'legacy', label: t('Historical only') },
                ]}
                onChange={(value) => update('dataOrigin', value)}
              />
              <SelectField
                label={t('Traffic source')}
                value={filters.trafficSource}
                placeholder={t('All')}
                options={[
                  { value: 'relay', label: t('Production relay') },
                  { value: 'playground', label: t('Playground') },
                  { value: 'task', label: t('Async task') },
                ]}
                onChange={(value) => update('trafficSource', value)}
              />
            </FieldGroup>
          </CollapsibleContent>
        </Collapsible>
      </CardContent>
    </Card>
  )
}
