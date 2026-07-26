/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export type AnalyticsRange = '1h' | 'today' | 'yesterday' | '7d' | 'custom'
export type ModelDimension = 'requested' | 'upstream'
export type StatusScope = 'upstream' | 'client'

export type AnalyticsFilters = {
  range: AnalyticsRange
  customStart: number
  customEnd: number
  granularity: string
  channelId: string
  channelType: string
  group: string
  requestedModel: string
  requestedModelHash: string
  upstreamModel: string
  upstreamModelHash: string
  outcome: string
  statusCode: string
  stream: string
  trafficSource: string
  dataOrigin: string
}

export type AnalyticsMeta = {
  generated_at: number
  reliable_from_ts: number
  data_start_ts: number
  data_end_ts: number
  last_flushed_at: number
  runtime_pending_batch_count: number
  runtime_flush_failure_count: number
  runtime_last_flush_error_at: number
  bucket_level: string
  bucket_seconds: number
  retention_days: number
  partial: boolean
  detail_available: boolean
  uncovered_channel_types: number[]
  invalid_sample_count: number
  dimension_overflow_count: number
  dropped_metric_event_count: number
  dropped_failure_event_count: number
  dimension_hash_collision_count: number
  backfill?: {
    status: string
    total_rows: number
    scanned_rows: number
    converted_rows: number
    skipped_rows: number
    last_error?: string
  }
}

export type AnalyticsSummary = {
  final_request_count: number
  channel_attempt_count: number
  upstream_call_count: number
  failed_attempt_count: number
  retry_count: number
  client_success_rate: number | null
  channel_quality_success_rate: number | null
  attempt_success_rate: number | null
  retry_rate: number | null
  usage_sample_count: number
  input_tokens_total: number
  uncached_input_tokens: number
  output_tokens: number
  total_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cache_request_hit_rate: number | null
  cache_token_hit_rate: number | null
  charged_quota: number
  charged_micro_usd: number
  avg_latency_ms: number | null
  p95_latency_ms: number | null
  avg_ttft_ms: number | null
  p95_ttft_ms: number | null
}

export type SummaryResponse = {
  summary: AnalyticsSummary
  meta: AnalyticsMeta
}

export type TrendPoint = {
  bucket_ts: number
  final_request_count: number
  channel_attempt_count: number
  upstream_call_count: number
  failed_attempt_count: number
  total_tokens: number
}

export type TrendResponse = {
  points: TrendPoint[]
  meta: AnalyticsMeta
}

export type StatusCodeItem = {
  status_present: boolean
  status_code: number
  label: string
  count: number
}

export type ErrorStageItem = {
  error_stage: string
  count: number
}

export type StatusResponse = {
  items: StatusCodeItem[]
  error_stages: ErrorStageItem[]
  meta: AnalyticsMeta
}

export type ChannelAnalyticsItem = {
  channel_id: number
  channel_name: string
  channel_type: number
  channel_type_name: string
  group?: string
  channel_attempt_count: number
  failure_count: number
  retry_count: number
  channel_quality_success_rate: number | null
  attempt_success_rate: number | null
  usage_sample_count: number
  input_tokens_total: number
  uncached_input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cache_request_hit_rate: number | null
  cache_token_hit_rate: number | null
  avg_latency_ms: number | null
  p95_latency_ms: number | null
  avg_ttft_ms: number | null
  p95_ttft_ms: number | null
  charged_quota: number
  charged_micro_usd: number
  last_failure_at: number
  top_status_codes: StatusCodeItem[]
}

export type ModelAnalyticsItem = ChannelAnalyticsItem & {
  requested_model?: string
  upstream_model?: string
  model_hash?: string
}

export type PagedResponse<T> = {
  items: T[]
  total: number
  page: number
  page_size: number
  meta: AnalyticsMeta
}

export type StabilityWindow = {
  window_seconds: number
  channel_attempt_count: number
  failure_count: number
  quality_eligible_count: number
  quality_success_rate: number | null
  attempt_success_rate: number | null
  retry_count: number
  retry_rate: number | null
  partial_response_count: number
  upstream_call_count: number
  upstream_status_coverage_rate: number | null
  upstream_429_count: number
  upstream_4xx_count: number
  upstream_5xx_count: number
  live_event_count: number
  legacy_event_count: number
  live_event_rate: number | null
  minimum_sample_count: number
  sample_sufficient: boolean
  total_tokens: number
  cache_token_hit_rate: number | null
  p95_latency_ms: number | null
  p95_ttft_ms: number | null
  last_failure_bucket_ts: number
}

export type StabilityItem = {
  key: string
  group?: string
  group_name?: string
  channel_id?: number
  channel_name?: string
  channel_type?: number
  channel_type_name?: string
  requested_model?: string
  upstream_model?: string
  model_hash?: string
  windows: StabilityWindow[]
}

export type StabilityResponse = PagedResponse<StabilityItem> & {
  dimension: string
}

export type FailureItem = {
  event_id: string
  created_at: number
  request_id: string
  attempt_seq: number
  retry_planned: boolean
  channel_id: number
  channel_name: string
  channel_type: number
  requested_model: string
  upstream_model: string
  group: string
  traffic_source: string
  data_origin: string
  outcome: string
  failure_owner: string
  partial_response: boolean
  error_stage: string
  upstream_status_present: boolean
  upstream_status_code: number
  normalized_status_present: boolean
  normalized_status_code: number
  client_status_present: boolean
  client_status_code: number
  latency_ms: number
  ttft_present: boolean
  ttft_ms: number
  retry_reason: string
  error_summary: string
}

export type FilterChannel = {
  channel_id: number
  channel_name: string
  channel_type: number
}

export type FilterType = { value: number; label: string }
export type FilterGroup = { code: string; name: string }
export type FilterModel = {
  value: string
  label: string
  model: string
  model_hash: string
}

export type FilterResponse = {
  channels: FilterChannel[]
  channel_types: FilterType[]
  groups: FilterGroup[]
  requested_models: string[]
  upstream_models: string[]
  requested_model_options: FilterModel[]
  upstream_model_options: FilterModel[]
  outcomes: string[]
  traffic_sources: string[]
  data_origins: string[]
  meta: AnalyticsMeta
}

export type ProbeChannel = {
  id: number
  name: string
  type: number
  status: number
  models: string | string[]
  response_time?: number
  test_time?: number
}

export type ProbeResult = {
  success: boolean
  duration?: number
  message?: string
}
