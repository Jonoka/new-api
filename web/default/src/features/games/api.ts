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
  CreatePredictionRequest,
  GamePrediction,
  GamePredictionBet,
  GameSettleResult,
  GameWallet,
  GameWalletTransaction,
  PageInfo,
} from './types'

export const gameQueryKeys = {
  wallet: ['game-wallet'] as const,
  transactions: ['game-transactions'] as const,
  predictions: ['game-predictions'] as const,
  prediction: (id: number) => ['game-prediction', id] as const,
  adminPredictions: ['admin-game-predictions'] as const,
}

export async function getGameWallet() {
  const res = await api.get<ApiResponse<GameWallet>>('/api/game/wallet')
  return res.data
}

export async function getGameTransactions() {
  const res = await api.get<ApiResponse<PageInfo<GameWalletTransaction>>>(
    '/api/game/transactions'
  )
  return res.data
}

export async function exchangeQuotaToToken(quota: number) {
  const res = await api.post<ApiResponse<GameWalletTransaction>>(
    '/api/game/exchange/quota-to-token',
    { quota }
  )
  return res.data
}

export async function exchangeTokenToQuota(tokens: number) {
  const res = await api.post<ApiResponse<GameWalletTransaction>>(
    '/api/game/exchange/token-to-quota',
    { tokens }
  )
  return res.data
}

export async function getGamePredictions() {
  const res = await api.get<ApiResponse<PageInfo<GamePrediction>>>(
    '/api/game/predictions'
  )
  return res.data
}

export async function getGamePrediction(id: number) {
  const res = await api.get<ApiResponse<GamePrediction>>(
    `/api/game/predictions/${id}`
  )
  return res.data
}

export async function placePredictionBet(
  predictionId: number,
  optionId: number,
  amount: number
) {
  const res = await api.post<ApiResponse<GamePredictionBet>>(
    `/api/game/predictions/${predictionId}/bets`,
    { option_id: optionId, amount }
  )
  return res.data
}

export async function adminGetGamePredictions() {
  const res = await api.get<ApiResponse<PageInfo<GamePrediction>>>(
    '/api/game/admin/predictions'
  )
  return res.data
}

export async function adminCreatePrediction(request: CreatePredictionRequest) {
  const res = await api.post<ApiResponse<GamePrediction>>(
    '/api/game/admin/predictions',
    request
  )
  return res.data
}

export async function adminSetPredictionAnswer(
  predictionId: number,
  answerIndex: number
) {
  const res = await api.put<ApiResponse<GamePrediction>>(
    `/api/game/admin/predictions/${predictionId}/answer`,
    { answer_index: answerIndex }
  )
  return res.data
}

export async function adminSettlePrediction(predictionId: number) {
  const res = await api.post<ApiResponse<GameSettleResult>>(
    `/api/game/admin/predictions/${predictionId}/settle`
  )
  return res.data
}
