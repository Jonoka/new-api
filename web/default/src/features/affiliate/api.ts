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
  ApiResponse,
  PageResponse,
  AdminBindAffiliateInviterRequest,
  AdminBindAffiliateInviterResult,
  AdminUnbindAffiliateInviterRequest,
  AdminUnbindAffiliateInviterResult,
  AffiliateAdminInvitation,
  AffiliateAdminRecord,
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

export async function getAffiliateLeaderboard(
  period = 'month',
  limit = 20,
  sort = 'commission'
) {
  const res = await api.get<ApiResponse<AffiliateLeaderboardItem[]>>(
    '/api/affiliate/leaderboard',
    { params: { period, limit, sort } }
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
  const res = await api.post<
    ApiResponse<{ path: string; account?: AffiliatePayoutAccount }>
  >('/api/affiliate/upload-qr', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return res.data
}

export async function deleteAffiliateQr(method: string) {
  const res = await api.delete<ApiResponse<AffiliatePayoutAccount>>(
    '/api/affiliate/qr',
    { params: { method } }
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

export async function getAdminAffiliateInvitations(
  keyword = '',
  page = 1,
  pageSize = 50
) {
  const res = await api.get<
    ApiResponse<PageResponse<AffiliateAdminInvitation>>
  >('/api/affiliate/admin/invitations', {
    params: { keyword, p: page, page_size: pageSize },
  })
  return res.data
}

export async function getAdminAffiliateRecords(
  sourceType = '',
  status = '',
  page = 1,
  pageSize = 50
) {
  const res = await api.get<ApiResponse<PageResponse<AffiliateAdminRecord>>>(
    '/api/affiliate/admin/records',
    {
      params: { source_type: sourceType, status, p: page, page_size: pageSize },
    }
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

export async function bindAdminAffiliateInviter(
  payload: AdminBindAffiliateInviterRequest
) {
  const res = await api.post<ApiResponse<AdminBindAffiliateInviterResult>>(
    '/api/affiliate/admin/bind-inviter',
    payload
  )
  return res.data
}

export async function unbindAdminAffiliateInviter(
  payload: AdminUnbindAffiliateInviterRequest
) {
  const res = await api.post<ApiResponse<AdminUnbindAffiliateInviterResult>>(
    '/api/affiliate/admin/unbind-inviter',
    payload
  )
  return res.data
}

export async function getAffiliateAgreement() {
  const res = await api.get<
    ApiResponse<{
      agreement_enabled: boolean
      agreement_text: string
      review_enabled: boolean
    }>
  >('/api/affiliate/agreement')
  return res.data
}

export async function getAffiliateApplicationStatus() {
  const res = await api.get<ApiResponse<any>>(
    '/api/affiliate/application-status'
  )
  return res.data
}

export async function applyAffiliate(agreementAccepted: boolean) {
  const res = await api.post<ApiResponse<null>>('/api/affiliate/apply', {
    agreement_accepted: agreementAccepted,
  })
  return res.data
}

export async function getAdminAffiliateApplications(
  status = '',
  page = 1,
  pageSize = 50
) {
  const res = await api.get<ApiResponse<PageResponse<any>>>(
    '/api/affiliate/admin/applications',
    { params: { status, p: page, page_size: pageSize } }
  )
  return res.data
}

export async function updateAdminAffiliateApplication(
  id: number,
  action: 'approve' | 'reject',
  payload: { remark?: string; reason?: string } = {}
) {
  const res = await api.post<ApiResponse<null>>(
    `/api/affiliate/admin/applications/${id}/${action}`,
    payload
  )
  return res.data
}

export async function getAdminFraudAlerts({
  status = '',
  keyword = '',
  ip = '',
  page = 1,
  pageSize = 50,
}: {
  status?: string
  keyword?: string
  ip?: string
  page?: number
  pageSize?: number
} = {}) {
  const res = await api.get<ApiResponse<PageResponse<any>>>(
    '/api/affiliate/admin/fraud-alerts',
    { params: { status, keyword, ip, p: page, page_size: pageSize } }
  )
  return res.data
}

export async function adminScanFraud(days = 30) {
  const res = await api.post<ApiResponse<{ new_alerts: number }>>(
    '/api/affiliate/admin/fraud-alerts/scan',
    null,
    { params: { days } }
  )
  return res.data
}

export async function adminScanFraudDeep(days = 30) {
  const res = await api.post<ApiResponse<{ new_alerts: number }>>(
    '/api/affiliate/admin/fraud-alerts/scan-deep',
    null,
    { params: { days } }
  )
  return res.data
}

export async function adminDeleteFraudAlert(id: number) {
  const res = await api.delete<ApiResponse<null>>(
    `/api/affiliate/admin/fraud-alerts/${id}`
  )
  return res.data
}

export async function adminResolveFraudAlert(
  id: number,
  action: 'unbind' | 'clawback' | 'dismiss',
  remark = ''
) {
  const res = await api.post<ApiResponse<null>>(
    `/api/affiliate/admin/fraud-alerts/${id}/${action}`,
    { remark }
  )
  return res.data
}
