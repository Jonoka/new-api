/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'
import type {
  AnalyticsFilters,
  ChannelAnalyticsItem,
  FailureItem,
  FilterModel,
  FilterResponse,
  ModelAnalyticsItem,
  PagedResponse,
  ProbeChannel,
  StabilityResponse,
  StatusResponse,
  SummaryResponse,
  TrendResponse,
} from './types'

const API_ROOT = '/api/channel-analytics'

type ApiEnvelope<T> = {
  success: boolean
  message?: string
  data?: T
}

export type AnalyticsQueryOptions = {
  includeOutcome?: boolean
  includeStatus?: boolean
  includeStream?: boolean
  statusScope?: 'upstream' | 'client'
}

function unwrap<T>(response: ApiEnvelope<T>): T {
  if (response.success === false || response.data === undefined) {
    throw new Error(response.message || 'Request failed')
  }
  return response.data
}

export function getRangeTimestamps(
  filters: AnalyticsFilters
): [number, number] {
  const now = new Date()
  const end = Math.floor(now.getTime() / 1000)
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const today = Math.floor(todayStart.getTime() / 1000)

  switch (filters.range) {
    case '1h':
      return [end - 3600, end]
    case 'yesterday':
      return [today - 86400, today]
    case '7d':
      return [end - 7 * 86400, end]
    case 'custom': {
      const customEnd = Math.min(filters.customEnd || end, end)
      return [filters.customStart || customEnd - 3600, customEnd]
    }
    default:
      return [today, end]
  }
}

export function createAnalyticsParams(
  filters: AnalyticsFilters,
  extra: Record<string, string | number | boolean | undefined> = {},
  options: AnalyticsQueryOptions = {}
) {
  const [start, end] = getRangeTimestamps(filters)
  const params = new URLSearchParams({
    start_timestamp: String(start),
    end_timestamp: String(end),
    granularity: filters.granularity,
    traffic_source: filters.trafficSource,
    data_origin: filters.dataOrigin,
  })

  if (filters.channelId) params.set('channel_ids', filters.channelId)
  if (filters.channelType) params.set('channel_types', filters.channelType)
  if (filters.group) params.set('groups', filters.group)
  if (filters.requestedModelHash) {
    params.set('requested_model_hashes', filters.requestedModelHash)
  } else if (filters.requestedModel) {
    params.set('requested_models', filters.requestedModel)
  }
  if (filters.upstreamModelHash) {
    params.set('upstream_model_hashes', filters.upstreamModelHash)
  } else if (filters.upstreamModel) {
    params.set('upstream_models', filters.upstreamModel)
  }
  if (filters.outcome && options.includeOutcome !== false) {
    params.set('outcome', filters.outcome)
  }
  if (filters.stream && options.includeStream !== false) {
    params.set('stream', filters.stream)
  }
  if (filters.statusCode && options.includeStatus !== false) {
    params.set(
      options.statusScope === 'client'
        ? 'client_status_codes'
        : 'upstream_status_codes',
      filters.statusCode
    )
  }
  Object.entries(extra).forEach(([key, value]) => {
    if (value !== undefined && value !== '') params.set(key, String(value))
  })
  return params
}

async function getAnalytics<T>(
  path: string,
  params?: URLSearchParams,
  signal?: AbortSignal
) {
  const response = await api.get<ApiEnvelope<T>>(`${API_ROOT}${path}`, {
    params,
    signal,
    skipErrorHandler: true,
  })
  return unwrap(response.data)
}

export const getAnalyticsFilters = () =>
  getAnalytics<FilterResponse>('/filters')

export const searchAnalyticsModels = (
  dimension: 'requested' | 'upstream',
  query: string
) =>
  getAnalytics<{ items: FilterModel[] }>(
    '/filters/models',
    new URLSearchParams({
      model_dimension: dimension,
      q: query,
      page: '1',
      page_size: '100',
    })
  )

export const getSummary = (params: URLSearchParams) =>
  getAnalytics<SummaryResponse>('/summary', params)

export const getTrend = (params: URLSearchParams) =>
  getAnalytics<TrendResponse>('/trend', params)

export const getStatusCodes = (params: URLSearchParams) =>
  getAnalytics<StatusResponse>('/status-codes', params)

export const getChannels = (params: URLSearchParams) =>
  getAnalytics<PagedResponse<ChannelAnalyticsItem>>('/channels', params)

export const getChannelModels = (
  channelId: number,
  params: URLSearchParams,
  signal?: AbortSignal
) =>
  getAnalytics<PagedResponse<ModelAnalyticsItem>>(
    `/channels/${channelId}/models`,
    params,
    signal
  )

export const getStability = (params: URLSearchParams, signal?: AbortSignal) =>
  getAnalytics<StabilityResponse>('/stability', params, signal)

export const getFailures = (params: URLSearchParams) =>
  getAnalytics<PagedResponse<FailureItem>>('/failures', params)

export async function getProbeChannels(): Promise<ProbeChannel[]> {
  const result: ProbeChannel[] = []
  let page = 1
  let total = 0
  do {
    const response = await api.get<
      ApiEnvelope<{ items: ProbeChannel[]; total: number }>
    >('/api/channel/search', {
      params: {
        keyword: '',
        group: '',
        model: '',
        id_sort: true,
        tag_mode: false,
        p: page,
        page_size: 100,
      },
      skipErrorHandler: true,
    })
    const payload = unwrap(response.data)
    result.push(...(payload.items ?? []))
    total = payload.total ?? result.length
    page += 1
    if (!payload.items?.length) break
  } while (result.length < total && page <= 101)

  return result.sort((left, right) =>
    left.name.localeCompare(right.name, undefined, { sensitivity: 'base' })
  )
}

export async function testProbeChannel(channelId: number, model: string) {
  const startedAt = performance.now()
  const response = await api.get<
    ApiEnvelope<{ response_time?: number; time?: number; error?: string }>
  >(`/api/channel/test/${channelId}`, {
    params: { model },
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  if (!response.data.success) {
    throw new Error(
      response.data.message || response.data.data?.error || 'Probe failed'
    )
  }
  const seconds = response.data.data?.time ?? response.data.data?.response_time
  return typeof seconds === 'number'
    ? seconds * 1000
    : performance.now() - startedAt
}
