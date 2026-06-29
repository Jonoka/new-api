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
export type InvoiceStatus = 'pending' | 'issued' | 'closed'
export type InvoiceSourceType = 'topup' | 'subscription'
export type InvoiceFeeRuleType = 'fixed' | 'percent'

export interface InvoiceFeeRule {
  min: number
  max?: number
  type: InvoiceFeeRuleType
  value: number
}

export interface InvoiceConfig {
  enabled: boolean
  types: InvoiceType[]
  fee_rules?: InvoiceFeeRule[]
  currency: 'CNY' | string
}

export interface InvoiceRequest {
  required: boolean
  type: InvoiceType
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
  title: string
  tax_no: string
  email: string
  phone: string
  remark: string
  base_amount: number
  fee_amount: number
  total_amount: number
  status: InvoiceStatus
  download_url: string
  admin_remark: string
  create_time: number
  update_time: number
  issued_time: number
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
  fee_rules: [],
  currency: 'CNY',
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

  return {
    enabled: Boolean(config.enabled),
    types: types.length > 0 ? types : DEFAULT_INVOICE_CONFIG.types,
    fee_rules: Array.isArray(config.fee_rules) ? config.fee_rules : [],
    currency: config.currency || 'CNY',
  }
}

export function createEmptyInvoiceRequest(
  defaultType: InvoiceType = 'personal'
): InvoiceRequest {
  return {
    required: false,
    type: defaultType,
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
  if (!request.title.trim()) return false
  if (request.type === 'company' && !request.tax_no.trim()) return false
  return true
}

export function getInvoicePayload(request: InvoiceRequest | undefined | null): {
  invoice?: InvoiceRequest
} {
  return request?.required ? { invoice: request } : {}
}
