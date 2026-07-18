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
import { api } from '@/lib/api'
import type {
  AdminUpdateInvoiceRequest,
  CreateOrderInvoiceRequest,
  CreateOrderInvoicePaymentRequest,
  InvoiceApiResponse,
  InvoiceConfig,
  InvoiceEligibleOrderList,
  InvoiceOrderPreview,
  InvoiceOrderSelectionRequest,
  InvoicePageData,
  InvoiceRecord,
  InvoiceSubmissionResult,
} from './types'

export async function getInvoiceConfig(): Promise<
  InvoiceApiResponse<InvoiceConfig>
> {
  const res = await api.get('/api/user/invoice/config')
  return res.data
}

export async function getUserInvoices(
  page: number,
  pageSize: number
): Promise<InvoiceApiResponse<InvoicePageData>> {
  const params = new URLSearchParams({
    p: String(page),
    page_size: String(pageSize),
  })
  const res = await api.get(`/api/user/invoice/self?${params.toString()}`)
  return res.data
}

export async function getInvoiceEligibleOrders(): Promise<
  InvoiceApiResponse<InvoiceEligibleOrderList>
> {
  const res = await api.get('/api/user/invoice/orders')
  return res.data
}

export async function previewOrderInvoice(
  request: InvoiceOrderSelectionRequest
): Promise<InvoiceApiResponse<InvoiceOrderPreview>> {
  const res = await api.post('/api/user/invoice/preview', request)
  return res.data
}

export async function createOrderInvoice(
  request: CreateOrderInvoiceRequest
): Promise<InvoiceApiResponse<InvoiceRecord>> {
  const res = await api.post('/api/user/invoice/request', request)
  return res.data
}

export async function createOrderInvoicePayment(
  request: CreateOrderInvoicePaymentRequest
): Promise<InvoiceApiResponse<InvoiceSubmissionResult>> {
  const res = await api.post('/api/user/invoice/payment', request)
  return res.data
}

export async function cancelOrderInvoicePayment(
  tradeNo: string
): Promise<InvoiceApiResponse<InvoiceRecord>> {
  const res = await api.post(
    `/api/user/invoice/payment/${encodeURIComponent(tradeNo)}/cancel`
  )
  return res.data
}

export async function getAdminInvoices(
  page: number,
  pageSize: number,
  status?: string
): Promise<InvoiceApiResponse<InvoicePageData>> {
  const params = new URLSearchParams({
    p: String(page),
    page_size: String(pageSize),
  })
  if (status) {
    params.set('status', status)
  }
  const res = await api.get(`/api/user/invoice?${params.toString()}`)
  return res.data
}

export async function updateInvoiceRecord(
  id: number,
  request: AdminUpdateInvoiceRequest
): Promise<InvoiceApiResponse> {
  const res = await api.put(`/api/user/invoice/${id}`, request)
  return res.data
}
