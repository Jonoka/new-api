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
import {
  isSafeHttpPaymentUrl,
  submitPaymentForm,
} from '@/features/wallet/lib/payment'
import type { InvoicePaymentCheckout } from './types'

/**
 * 安全拉起开票服务费收银台，仅接受后端契约约定的 HTTP(S) 表单或跳转。
 */
export function openInvoicePaymentCheckout(
  checkout: InvoicePaymentCheckout
): boolean {
  if (!isSafeHttpPaymentUrl(checkout.url)) return false

  if (checkout.type === 'form') {
    if (
      checkout.params !== undefined &&
      (!checkout.params ||
        typeof checkout.params !== 'object' ||
        Array.isArray(checkout.params))
    ) {
      return false
    }

    try {
      submitPaymentForm(checkout.url, checkout.params || {})
      return true
    } catch {
      return false
    }
  }

  if (checkout.type !== 'redirect') return false

  try {
    const paymentWindow = window.open(checkout.url, '_blank')
    if (!paymentWindow) return false
    paymentWindow.opener = null
    return true
  } catch {
    return false
  }
}
