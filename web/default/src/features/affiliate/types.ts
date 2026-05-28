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
