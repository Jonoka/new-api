/*
Copyright (C) 2023-2026 QuantumNous

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

export type InvoiceType = 'personal' | 'company'
export type InvoiceKind = 'normal' | 'special'
export type InvoiceStatus = 'payment_pending' | 'pending' | 'issued' | 'closed'
export type InvoiceSourceType = 'topup' | 'subscription' | 'batch'
export type InvoiceFeeRuleType = 'fixed' | 'percent'

export interface InvoicePaymentMethod {
  name: string
  type: string
  provider: string
  color: string
}

export interface InvoiceBepusdtChain {
  name: string
  trade_type: string
}

export interface InvoiceFeeRule {
  min: number
  max?: number
  type: InvoiceFeeRuleType
  value: number
  max_fee?: number
}

export interface InvoiceConfig {
  enabled: boolean
  types: InvoiceType[]
  kinds: InvoiceKind[]
  fee_rules?: InvoiceFeeRule[]
  currency: 'CNY' | string
  pay_methods: InvoicePaymentMethod[]
  bepusdt_chains: InvoiceBepusdtChain[]
}

export interface InvoiceRequest {
  required: boolean
  type: InvoiceType
  kind: InvoiceKind
  title: string
  tax_no: string
  email: string
  phone: string
  remark: string
}

export interface InvoiceRecord {
  id: number
  user_id: number
  source_type: InvoiceSourceType
  source_id: string
  payment_method: string
  invoice_type: InvoiceType
  invoice_kind: InvoiceKind | ''
  title: string
  tax_no: string
  email: string
  phone: string
  remark: string
  base_amount: number
  fee_amount: number
  total_amount: number
  payment_provider?: string
  payment_status?:
    | 'pending'
    | 'success'
    | 'failed'
    | 'expired'
    | 'canceled'
    | ''
  provider_order_id?: string
  provider_amount?: string
  provider_currency?: string
  payment_amount_minor?: number
  request_ip?: string
  paid_time?: number
  status: InvoiceStatus
  download_url: string
  admin_remark: string
  create_time: number
  update_time: number
  issued_time: number
}

export interface InvoiceEligibleOrder {
  source_type: InvoiceSourceType
  source_id: string
  payment_method: string
  payment_provider: string
  paid_amount: number
  invoice_amount: number
  currency: string
  complete_time: number
  invoiced: boolean
  invoice_eligible: boolean
  invoice_status: string
}

export interface InvoiceEligibleOrderList {
  orders: InvoiceEligibleOrder[]
  window_days: number
  currency: string
}

export interface InvoiceOrderReference {
  source_type: InvoiceSourceType
  source_id: string
}

export interface CreateOrderInvoiceRequest {
  orders: InvoiceOrderReference[]
  invoice: InvoiceRequest
}

export interface CreateOrderInvoicePaymentRequest extends CreateOrderInvoiceRequest {
  payment_method: string
  trade_type?: string
}

export interface InvoicePaymentCheckout {
  type: 'form' | 'redirect'
  url: string
  params?: Record<string, unknown>
}

export interface InvoiceSubmissionResult {
  completed: boolean
  trade_no: string
  invoice: InvoiceRecord
  checkout?: InvoicePaymentCheckout
  amount_text?: string
}

export interface InvoiceOrderSelectionRequest {
  orders: InvoiceOrderReference[]
}

export interface InvoiceOrderPreview {
  order_amount: number
  invoice_fee: number
  invoice_total_amount: number
  fee_quota: number
  currency: string
}

export interface InvoicePageData {
  page: number
  page_size: number
  total: number
  items: InvoiceRecord[]
}

export interface InvoiceApiResponse<T = unknown> {
  success?: boolean
  message?: string
  data?: T
}

export interface AdminUpdateInvoiceRequest {
  download_url: string
  status: InvoiceStatus
  admin_remark: string
}

export const DEFAULT_INVOICE_CONFIG: InvoiceConfig = {
  enabled: false,
  types: ['personal', 'company'],
  kinds: ['normal'],
  fee_rules: [],
  currency: 'CNY',
  pay_methods: [],
  bepusdt_chains: [],
}

function normalizeInvoicePaymentMethods(
  value: unknown
): InvoicePaymentMethod[] {
  if (!Array.isArray(value)) return []

  const seenTypes = new Set<string>()
  const methods: InvoicePaymentMethod[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) continue
    const method = item as Record<string, unknown>
    const name = typeof method.name === 'string' ? method.name.trim() : ''
    const type = typeof method.type === 'string' ? method.type.trim() : ''
    const provider =
      typeof method.provider === 'string' ? method.provider.trim() : ''
    if (!name || !type || !provider || seenTypes.has(type)) continue

    seenTypes.add(type)
    methods.push({
      name,
      type,
      provider,
      color: typeof method.color === 'string' ? method.color.trim() : '',
    })
  }
  return methods
}

function normalizeInvoiceBepusdtChains(value: unknown): InvoiceBepusdtChain[] {
  if (!Array.isArray(value)) return []

  const seenTradeTypes = new Set<string>()
  const chains: InvoiceBepusdtChain[] = []
  for (const item of value) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) continue
    const chain = item as Record<string, unknown>
    const name = typeof chain.name === 'string' ? chain.name.trim() : ''
    const tradeType =
      typeof chain.trade_type === 'string' ? chain.trade_type.trim() : ''
    if (!name || !tradeType || seenTradeTypes.has(tradeType)) continue

    seenTradeTypes.add(tradeType)
    chains.push({ name, trade_type: tradeType })
  }
  return chains
}

export function normalizeInvoiceConfig(
  config?: Partial<InvoiceConfig> | null
): InvoiceConfig {
  if (!config) return DEFAULT_INVOICE_CONFIG
  const types = Array.isArray(config.types)
    ? config.types.filter(
        (type): type is InvoiceType => type === 'personal' || type === 'company'
      )
    : []
  const kinds = Array.isArray(config.kinds)
    ? config.kinds.filter(
        (kind): kind is InvoiceKind => kind === 'normal' || kind === 'special'
      )
    : []

  return {
    enabled: Boolean(config.enabled),
    types: types.length > 0 ? types : DEFAULT_INVOICE_CONFIG.types,
    kinds: kinds.length > 0 ? kinds : DEFAULT_INVOICE_CONFIG.kinds,
    fee_rules: Array.isArray(config.fee_rules) ? config.fee_rules : [],
    currency: config.currency || 'CNY',
    pay_methods: normalizeInvoicePaymentMethods(config.pay_methods),
    bepusdt_chains: normalizeInvoiceBepusdtChains(config.bepusdt_chains),
  }
}

export function createEmptyInvoiceRequest(
  defaultType: InvoiceType = 'personal',
  defaultKind: InvoiceKind = 'normal'
): InvoiceRequest {
  return {
    required: false,
    type: defaultType,
    kind: defaultKind,
    title: '',
    tax_no: '',
    email: '',
    phone: '',
    remark: '',
  }
}

export function isInvoiceRequestValid(
  config: InvoiceConfig | undefined | null,
  request: InvoiceRequest | undefined | null
): boolean {
  if (!request?.required) return true
  const normalizedConfig = normalizeInvoiceConfig(config)
  if (!normalizedConfig.enabled) return false
  if (!normalizedConfig.types.includes(request.type)) return false
  if (!normalizedConfig.kinds.includes(request.kind)) return false
  if (!request.title.trim()) return false
  if (request.type === 'company' && !request.tax_no.trim()) return false
  return true
}

export function isInvoicePreviewRequestEnabled(
  config: InvoiceConfig | undefined | null,
  request: InvoiceRequest | undefined | null
): boolean {
  if (!request?.required) return false
  const normalizedConfig = normalizeInvoiceConfig(config)
  return (
    normalizedConfig.enabled &&
    normalizedConfig.types.includes(request.type) &&
    normalizedConfig.kinds.includes(request.kind)
  )
}

export function getInvoicePayload(request: InvoiceRequest | undefined | null): {
  invoice?: InvoiceRequest
} {
  return request?.required ? { invoice: request } : {}
}
