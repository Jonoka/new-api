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
export type ApiResponse<T> = {
  success: boolean
  message: string
  data: T
}

export type PageResponse<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type AffiliateBalance = {
  user_id: number
  pending_quota: number
  available_quota: number
  frozen_quota: number
  withdrawn_quota: number
  transferred_quota: number
  total_quota: number
}

export type AffiliateSetting = {
  first_level_enabled: boolean
  first_level_ratio: number
  second_level_enabled: boolean
  second_level_ratio: number
  settlement_delay_seconds: number
  min_withdrawal_amount: number
  trigger_topup_enabled: boolean
  trigger_subscription_enabled: boolean
  filter_redemption_topup_enabled: boolean
  payout_methods: string[]
  usdt_chain: string
}

export type AffiliateSummary = {
  balance: AffiliateBalance
  aff_code: string
  aff_count: number
  invite_link: string
  promotion_text: string
  setting: AffiliateSetting
}

export type AffiliateRecord = {
  id: number
  user_id: number
  invitee_id: number
  level: number
  source_type: string
  source_id: string
  source_quota: number
  reward_quota: number
  ratio: number
  status: 'pending' | 'available'
  available_time: number
  settled_time: number
  created_at: number
  detail?: AffiliateSourceDetail
}

export type AffiliateSourceDetail = {
  source_type: string
  title: string
  plan_id?: number
  plan_title?: string
  redemption_id?: number
  redemption_name?: string
  original_amount?: number
  discount_amount?: number
  paid_amount?: number
  promo_code?: string
  payment_provider?: string
  payment_method?: string
  quota?: number
}

export type AffiliatePayoutAccount = {
  user_id: number
  usdt_address: string
  usdt_chain: string
  alipay_account: string
  alipay_name: string
  alipay_qr_path: string
  wechat_account: string
  wechat_name: string
  wechat_qr_path: string
}

export type AffiliateWithdrawal = {
  id: number
  user_id: number
  quota: number
  display_amount: number
  display_currency: string
  method: 'usdt' | 'alipay' | 'wechat'
  payout_snapshot: string
  status: 'pending' | 'approved' | 'paid' | 'rejected'
  admin_remark: string
  created_at: number
  approved_time: number
  paid_time: number
  rejected_time: number
}

export type AffiliateLeaderboardItem = {
  rank: number
  user_id: number
  username: string
  display_name: string
  invite_count: number
  commission_quota: number
}

export type AdminBindAffiliateInviterRequest = {
  user_id?: number
  user_identifier?: string
  aff_code: string
  force?: boolean
}

export type AdminBindAffiliateInviterResult = {
  user_id: number
  username: string
  display_name: string
  inviter_id: number
  inviter_username: string
  inviter_aff_code: string
  previous_inviter_id: number
  updated: boolean
}
