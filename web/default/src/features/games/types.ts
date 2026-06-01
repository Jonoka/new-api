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

export type PageInfo<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type GameWallet = {
  id: number
  user_id: number
  balance: number
  created_at: number
  updated_at: number
}

export type GameWalletTransaction = {
  id: number
  user_id: number
  wallet_id: number
  type: string
  token_amount: number
  quota_amount: number
  fee_amount: number
  balance_after: number
  prediction_id: number
  prediction_bet_id: number
  content: string
  created_at: number
}

export type GamePredictionOption = {
  id: number
  prediction_id: number
  index: number
  title: string
  pool_amount: number
  bet_count: number
  created_at: number
  updated_at: number
}

export type GamePrediction = {
  id: number
  title: string
  description: string
  status: 'open' | 'answered' | 'settling' | 'settled' | 'failed'
  judge_mode: 'manual' | 'auto'
  close_time: number
  settle_time: number
  answer_option_id: number
  total_pool: number
  winning_pool: number
  total_payout: number
  total_fee: number
  winner_count: number
  created_at: number
  updated_at: number
  options: GamePredictionOption[]
}

export type GamePredictionBet = {
  id: number
  prediction_id: number
  option_id: number
  user_id: number
  wallet_id: number
  amount: number
  status: string
  gross_payout: number
  fee_amount: number
  net_payout: number
  created_at: number
  updated_at: number
}

export type GameSettleResult = {
  prediction_id: number
  total_pool: number
  winning_pool: number
  total_payout: number
  total_fee: number
  winner_count: number
}

export type CreatePredictionRequest = {
  title: string
  description: string
  options: string[]
  close_time: number
  settle_time: number
  judge_mode: 'manual' | 'auto'
}
