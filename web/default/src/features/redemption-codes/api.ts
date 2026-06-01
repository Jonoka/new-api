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
  Redemption,
  ApiResponse,
  GetRedemptionsParams,
  GetRedemptionsResponse,
  SearchRedemptionsParams,
  RedemptionFormData,
  PromoCode,
  PromoCodeFormData,
  GetPromoCodesParams,
  GetPromoCodesResponse,
} from './types'

// ============================================================================
// Redemption Code Management
// ============================================================================

// Get paginated redemption codes list
export async function getRedemptions(
  params: GetRedemptionsParams = {}
): Promise<GetRedemptionsResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/redemption/?p=${p}&page_size=${page_size}`)
  return res.data
}

// Search redemption codes by keyword
export async function searchRedemptions(
  params: SearchRedemptionsParams
): Promise<GetRedemptionsResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const res = await api.get(
    `/api/redemption/search?keyword=${keyword}&p=${p}&page_size=${page_size}`
  )
  return res.data
}

// Get single redemption code by ID
export async function getRedemption(
  id: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.get(`/api/redemption/${id}`)
  return res.data
}

// Create redemption code(s)
export async function createRedemption(
  data: RedemptionFormData
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/redemption/', data)
  return res.data
}

// Update redemption code
export async function updateRedemption(
  data: RedemptionFormData & { id: number }
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/', data)
  return res.data
}

// Update redemption code status (enable/disable)
export async function updateRedemptionStatus(
  id: number,
  status: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/?status_only=true', { id, status })
  return res.data
}

// Delete a single redemption code
export async function deleteRedemption(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/redemption/${id}/`)
  return res.data
}

// Delete invalid redemption codes (used, disabled, expired)
export async function deleteInvalidRedemptions(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/redemption/invalid')
  return res.data
}

export async function getPromoCodes(
  params: GetPromoCodesParams = {}
): Promise<GetPromoCodesResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/promo-code/?p=${p}&page_size=${page_size}`)
  return res.data
}

export async function searchPromoCodes(params: {
  keyword?: string
  p?: number
  page_size?: number
}): Promise<GetPromoCodesResponse> {
  const { keyword = '', p = 1, page_size = 10 } = params
  const res = await api.get(
    `/api/promo-code/search?keyword=${keyword}&p=${p}&page_size=${page_size}`
  )
  return res.data
}

export async function getPromoCode(
  id: number
): Promise<ApiResponse<PromoCode>> {
  const res = await api.get(`/api/promo-code/${id}`)
  return res.data
}

export async function createPromoCode(
  data: PromoCodeFormData
): Promise<ApiResponse<PromoCode>> {
  const res = await api.post('/api/promo-code/', data)
  return res.data
}

export async function updatePromoCode(
  data: PromoCodeFormData & { id: number }
): Promise<ApiResponse<PromoCode>> {
  const res = await api.put('/api/promo-code/', data)
  return res.data
}

export async function updatePromoCodeStatus(
  id: number,
  status: number
): Promise<ApiResponse<PromoCode>> {
  const res = await api.put('/api/promo-code/?status_only=true', { id, status })
  return res.data
}

export async function deletePromoCode(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/promo-code/${id}/`)
  return res.data
}
