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
import { z } from 'zod'

// ============================================================================
// Redemption Schema & Types
// ============================================================================

export const redemptionSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  name: z.string(),
  key: z.string(),
  status: z.number(), // 1: enabled, 2: disabled, 3: used
  quota: z.number(),
  created_time: z.number(),
  redeemed_time: z.number(),
  expired_time: z.number(), // 0 for never expires
  used_user_id: z.number(),
  max_redeem_count: z.number().optional(),
  redeemed_count: z.number().optional(),
})

export type Redemption = z.infer<typeof redemptionSchema>

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetRedemptionsParams {
  p?: number
  page_size?: number
}

export interface GetRedemptionsResponse {
  success: boolean
  message?: string
  data?: {
    items: Redemption[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchRedemptionsParams {
  keyword?: string
  p?: number
  page_size?: number
}

export interface RedemptionFormData {
  id?: number
  name: string
  quota: number
  expired_time: number
  max_redeem_count?: number
  count?: number // Only for create
  status?: number // Only for status update
}

export const promoCodeSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  name: z.string(),
  code: z.string(),
  status: z.number(),
  discount_type: z.enum(['percent', 'fixed']),
  discount_value: z.number(),
  applies_to_topup: z.boolean(),
  applies_to_all_subscription: z.boolean(),
  subscription_plan_ids: z.string(),
  max_redeem_count: z.number(),
  redeemed_count: z.number(),
  created_time: z.number(),
  updated_time: z.number(),
  expired_time: z.number(),
})

export type PromoCode = z.infer<typeof promoCodeSchema>

export interface PromoCodeFormData {
  id?: number
  name: string
  code: string
  status?: number
  discount_type: 'percent' | 'fixed'
  discount_value: number
  applies_to_topup: boolean
  applies_to_all_subscription: boolean
  subscription_plan_ids: string
  max_redeem_count: number
  expired_time: number
}

export interface GetPromoCodesParams {
  p?: number
  page_size?: number
}

export interface GetPromoCodesResponse {
  success: boolean
  message?: string
  data?: {
    items: PromoCode[]
    total: number
    page: number
    page_size: number
  }
}

// ============================================================================
// Dialog Types
// ============================================================================

export type RedemptionsDialogType = 'create' | 'update' | 'delete' | 'view'
