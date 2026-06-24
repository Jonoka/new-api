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
  risk_frozen_quota: number
  confiscated_quota: number
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
  can_invite: boolean
}

export type AffiliateApplicationStatus = {
  review_enabled: boolean
  agreement_enabled: boolean
  status: string
  can_invite: boolean
  rejected_reason?: string
  eligibility?: {
    eligible: boolean
    reason?: string
    conditions?: AffiliateEligibilityCondition[]
  }
}

export type AffiliateEligibilityCondition = {
  type: string
  required: number
  current: number
  unit: string
  met: boolean
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
  status: 'pending' | 'available' | 'confiscated'
  available_time: number
  settled_time: number
  balance_after_quota: number
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
  masked_name: string
  invite_count: number
  commission_quota: number
}

export type AffiliateAdminUserInfo = {
  id: number
  username: string
  display_name: string
  email: string
  aff_code: string
  status: number
  inviter_id: number
  created_at: number
}

export type AffiliateAdminInvitation = {
  inviter_id: number
  inviter_username: string
  inviter_name: string
  inviter_email: string
  inviter_aff_code: string
  invitee_id: number
  invitee_username: string
  invitee_name: string
  invitee_email: string
  invitee_status: number
  invitee_created_at: number
  topup_count: number
  topup_quota: number
  last_topup_time: number
  commission_quota: number
}

export type AffiliateAdminRecord = AffiliateRecord & {
  inviter: AffiliateAdminUserInfo
  invitee: AffiliateAdminUserInfo
}

export type AffiliateRiskUser = {
  id: number
  user_id: number
  status: 'active' | 'removed'
  freeze_assets: boolean
  block_invite_code: boolean
  detached_invitees: boolean
  cleared_quota: number
  reason: string
  admin_id: number
  removed_by: number
  remove_remark: string
  removed_at: number
  created_at: number
  updated_at: number
}

export type AffiliateRiskUserWithDetail = AffiliateRiskUser & {
  user: AffiliateAdminUserInfo
  balance: AffiliateBalance
  direct_invitee_count: number
  restorable_invitee_count: number
}

export type AffiliateRiskPreview = {
  user: AffiliateAdminUserInfo
  balance: AffiliateBalance
  active_risk?: AffiliateRiskUser
  direct_invitee_count: number
  restorable_invitee_count: number
  clearable_quota: number
}

export type AffiliateRiskApplyRequest = {
  freeze_assets: boolean
  block_invite_code: boolean
  detach_invitees: boolean
  clear_assets: boolean
  reason: string
}

export type AffiliateRiskRemoveRequest = {
  restore_detached_invitees: boolean
  remark: string
}

export type AffiliateRiskApplyResult = {
  risk_user: AffiliateRiskUser
  frozen_quota: number
  detached_count: number
  cleared_quota: number
  rejected_withdrawals: number
}

export type AffiliateRiskRemoveResult = {
  restored_invitees: number
  unfrozen_quota: number
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

export type AdminUnbindAffiliateInviterRequest = {
  user_id?: number
  user_identifier?: string
}

export type AdminUnbindAffiliateInviterResult = {
  user_id: number
  username: string
  display_name: string
  previous_inviter_id: number
  updated: boolean
}
