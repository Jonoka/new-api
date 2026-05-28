import { api } from '@/lib/api'
import type {
  ApiResponse,
  PageResponse,
  AffiliatePayoutAccount,
  AffiliateLeaderboardItem,
  AffiliateRecord,
  AffiliateSummary,
  AffiliateWithdrawal,
} from './types'

export async function getAffiliateSummary() {
  const res = await api.get<ApiResponse<AffiliateSummary>>(
    '/api/affiliate/summary'
  )
  return res.data
}

export async function getAffiliateRecords(page = 1, pageSize = 20) {
  const res = await api.get<ApiResponse<PageResponse<AffiliateRecord>>>(
    '/api/affiliate/records',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function getAffiliateWithdrawals(page = 1, pageSize = 20) {
  const res = await api.get<ApiResponse<PageResponse<AffiliateWithdrawal>>>(
    '/api/affiliate/withdrawals',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function getAffiliateLeaderboard(period = 'month', limit = 20) {
  const res = await api.get<ApiResponse<AffiliateLeaderboardItem[]>>(
    '/api/affiliate/leaderboard',
    { params: { period, limit } }
  )
  return res.data
}

export async function getAffiliatePayoutAccount() {
  const res = await api.get<ApiResponse<AffiliatePayoutAccount>>(
    '/api/affiliate/payout-account'
  )
  return res.data
}

export async function updateAffiliatePayoutAccount(
  account: Partial<AffiliatePayoutAccount>
) {
  const res = await api.put<ApiResponse<AffiliatePayoutAccount>>(
    '/api/affiliate/payout-account',
    account
  )
  return res.data
}

export async function createAffiliateWithdrawal(method: string, quota: number) {
  const res = await api.post<ApiResponse<AffiliateWithdrawal>>(
    '/api/affiliate/withdraw',
    { method, quota }
  )
  return res.data
}

export async function transferAffiliateToBalance(quota: number) {
  const res = await api.post<ApiResponse<null>>(
    '/api/affiliate/transfer-to-balance',
    { quota }
  )
  return res.data
}

export async function uploadAffiliateQr(method: string, file: File) {
  const form = new FormData()
  form.append('method', method)
  form.append('file', file)
  const res = await api.post<ApiResponse<{ path: string }>>(
    '/api/affiliate/upload-qr',
    form,
    { headers: { 'Content-Type': 'multipart/form-data' } }
  )
  return res.data
}

export async function getAdminAffiliateWithdrawals(
  status = '',
  page = 1,
  pageSize = 50
) {
  const res = await api.get<ApiResponse<PageResponse<AffiliateWithdrawal>>>(
    '/api/affiliate/admin/withdrawals',
    { params: { status, p: page, page_size: pageSize } }
  )
  return res.data
}

export async function updateAdminAffiliateWithdrawal(
  id: number,
  action: 'approve' | 'reject' | 'paid',
  remark = ''
) {
  const res = await api.post<ApiResponse<null>>(
    `/api/affiliate/admin/withdrawals/${id}/${action}`,
    { remark }
  )
  return res.data
}
