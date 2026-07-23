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
  InformationCircleIcon,
  Loading03Icon,
  Search01Icon,
  TestTube01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
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
import { getProbeChannels, testProbeChannel } from './api'
import {
  formatDateTime,
  formatDuration,
  PageControls,
  ViewError,
  ViewSkeleton,
} from './shared'
import type { AnalyticsFilters, ProbeChannel, ProbeResult } from './types'

function channelModels(channel: ProbeChannel) {
  return Array.isArray(channel.models)
    ? channel.models.map(String).filter(Boolean)
    : String(channel.models || '')
        .split(',')
        .map((model) => model.trim())
        .filter(Boolean)
}

function statusLabel(status: number, t: (key: string) => string) {
  if (status === 1) return { label: t('Enabled'), variant: 'default' as const }
  if (status === 2)
    return { label: t('Manually disabled'), variant: 'secondary' as const }
  if (status === 3)
    return {
      label: t('Automatically disabled'),
      variant: 'destructive' as const,
    }
  return { label: t('Disabled'), variant: 'outline' as const }
}

export function ProbeView({
  filters,
  refreshKey,
}: {
  filters: AnalyticsFilters
  refreshKey: number
}) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [selectedModels, setSelectedModels] = useState<Record<number, string>>(
    {}
  )
  const [testing, setTesting] = useState<Set<number>>(new Set())
  const [results, setResults] = useState<Record<number, ProbeResult>>({})
  const query = useQuery({
    queryKey: ['channel-observability', 'probe-channels', refreshKey],
    queryFn: getProbeChannels,
    staleTime: 30_000,
  })

  const filtered = useMemo(() => {
    const needle = keyword.trim().toLocaleLowerCase()
    return (query.data ?? []).filter((channel) => {
      const models = channelModels(channel)
      if (filters.channelId && String(channel.id) !== filters.channelId)
        return false
      if (filters.channelType && String(channel.type) !== filters.channelType)
        return false
      if (filters.requestedModel && !models.includes(filters.requestedModel))
        return false
      if (!needle) return true
      return `${channel.name} ${channel.id} ${channel.type}`
        .toLocaleLowerCase()
        .includes(needle)
    })
  }, [
    filters.channelId,
    filters.channelType,
    filters.requestedModel,
    keyword,
    query.data,
  ])
  const pageItems = filtered.slice((page - 1) * 30, page * 30)

  useEffect(
    () => setPage(1),
    [filters.channelId, filters.channelType, filters.requestedModel, keyword]
  )

  const runProbe = async (channel: ProbeChannel) => {
    const models = channelModels(channel)
    const model = selectedModels[channel.id] || models[0]
    if (!model || testing.has(channel.id)) return
    setTesting((current) => new Set(current).add(channel.id))
    setResults((current) => {
      const next = { ...current }
      delete next[channel.id]
      return next
    })
    try {
      const duration = await testProbeChannel(channel.id, model)
      setResults((current) => ({
        ...current,
        [channel.id]: { success: true, duration },
      }))
      toast.success(
        t('Channel #{{id}} probe succeeded in {{duration}}', {
          id: channel.id,
          duration: formatDuration(duration),
        })
      )
    } catch (error) {
      const message = error instanceof Error ? error.message : t('Probe failed')
      setResults((current) => ({
        ...current,
        [channel.id]: { success: false, message },
      }))
      toast.error(
        t('Channel #{{id}} probe failed: {{message}}', {
          id: channel.id,
          message,
        })
      )
    } finally {
      setTesting((current) => {
        const next = new Set(current)
        next.delete(channel.id)
        return next
      })
    }
  }

  if (query.isLoading) return <ViewSkeleton />
  if (query.error)
    return (
      <ViewError error={query.error} retry={() => void query.refetch()} t={t} />
    )

  return (
    <div className='flex min-w-0 flex-col gap-3 sm:gap-4'>
      <Alert className='py-2'>
        <HugeiconsIcon icon={InformationCircleIcon} strokeWidth={2} />
        <AlertTitle>
          {t('Active probes are separate from production analytics')}
        </AlertTitle>
        <AlertDescription className='text-xs'>
          {t(
            'A probe answers whether a channel is reachable now; it does not prove sustained production stability.'
          )}
        </AlertDescription>
      </Alert>
      <Card className='min-w-0 overflow-hidden'>
        <CardHeader className='bg-muted/20 gap-3 border-b sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <CardTitle>{t('Channel active probes')}</CardTitle>
            <CardDescription className='max-w-2xl'>
              {t(
                'Select one supported model and run a single connectivity test.'
              )}
            </CardDescription>
          </div>
          <InputGroup className='w-full sm:max-w-xs'>
            <InputGroupAddon>
              <HugeiconsIcon icon={Search01Icon} strokeWidth={2} />
            </InputGroupAddon>
            <InputGroupInput
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              placeholder={t('Search channel, ID, or type')}
            />
          </InputGroup>
        </CardHeader>
        <CardContent className='p-0'>
          <Table className='min-w-[760px]'>
            <TableHeader>
              <TableRow className='bg-muted/20 hover:bg-muted/20'>
                <TableHead>{t('Channel')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Type')}</TableHead>
                <TableHead>{t('Test model')}</TableHead>
                <TableHead>{t('Last test')}</TableHead>
                <TableHead>{t('Result')}</TableHead>
                <TableHead>{t('Action')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pageItems.length ? (
                pageItems.map((channel) => {
                  const models = channelModels(channel)
                  const selected = selectedModels[channel.id] || models[0] || ''
                  const result = results[channel.id]
                  const isTesting = testing.has(channel.id)
                  const status = statusLabel(channel.status, t)
                  const items = models.map((model) => ({
                    value: model,
                    label: model,
                  }))
                  return (
                    <TableRow key={channel.id}>
                      <TableCell>
                        <div
                          className='max-w-72 truncate font-medium'
                          title={channel.name}
                        >
                          {channel.name}
                        </div>
                        <div className='text-muted-foreground text-xs'>
                          #{channel.id}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge className='text-xs' variant={status.variant}>
                          {status.label}
                        </Badge>
                      </TableCell>
                      <TableCell>{channel.type}</TableCell>
                      <TableCell>
                        {models.length ? (
                          <Select
                            items={items}
                            value={selected}
                            onValueChange={(value) =>
                              value &&
                              setSelectedModels((current) => ({
                                ...current,
                                [channel.id]: value,
                              }))
                            }
                          >
                            <SelectTrigger className='max-w-64'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent alignItemWithTrigger={false}>
                              <SelectGroup>
                                {models.map((model) => (
                                  <SelectItem key={model} value={model}>
                                    {model}
                                  </SelectItem>
                                ))}
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        ) : (
                          <span className='text-muted-foreground'>
                            {t('No available models')}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>{formatDateTime(channel.test_time)}</TableCell>
                      <TableCell>
                        {result ? (
                          <Badge
                            className='max-w-64 truncate text-xs'
                            variant={result.success ? 'default' : 'destructive'}
                          >
                            {result.success
                              ? formatDuration(result.duration)
                              : result.message}
                          </Badge>
                        ) : channel.response_time !== undefined ? (
                          formatDuration(channel.response_time * 1000)
                        ) : (
                          <span className='text-muted-foreground'>
                            {t('Not tested')}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={isTesting || !selected}
                          onClick={() => void runProbe(channel)}
                        >
                          <HugeiconsIcon
                            icon={isTesting ? Loading03Icon : TestTube01Icon}
                            strokeWidth={2}
                            data-icon='inline-start'
                            className={isTesting ? 'animate-spin' : undefined}
                          />
                          {isTesting ? t('Testing...') : t('Run test')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })
              ) : (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className='text-muted-foreground h-32 text-center'
                  >
                    {t('No matching channels')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
          <PageControls
            page={page}
            total={filtered.length}
            onPage={setPage}
            t={t}
          />
        </CardContent>
      </Card>
    </div>
  )
}
