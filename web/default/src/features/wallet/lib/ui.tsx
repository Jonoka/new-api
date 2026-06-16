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
import { type ReactNode } from 'react'
import i18next from 'i18next'
import { CreditCard, Landmark } from 'lucide-react'
import { SiAlipay, SiWechat, SiStripe } from 'react-icons/si'
import { PAYMENT_TYPES, PAYMENT_ICON_COLORS } from '../constants'

// ============================================================================
// UI Helper Functions
// ============================================================================

const HAS_LOCATION =
  typeof globalThis !== 'undefined' && 'location' in globalThis

/**
 * Resolves a backend-provided image URL to http(s) only. Rejects javascript:,
 * data:, blob:, file:, and URLs with userinfo, which are unsafe in <img src/>.
 */
function normalizeHttpIconUrl(raw: string | undefined | null): string | null {
  if (!raw) return null
  const s = raw.trim()
  if (!s) return null
  let url: URL
  try {
    url = HAS_LOCATION
      ? new URL(s, (globalThis as { location: Location }).location.href)
      : new URL(s)
  } catch {
    return null
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return null
  }
  if (url.username || url.password) {
    return null
  }
  return url.toString()
}

/**
 * Get payment method icon component
 *
 * When iconUrl is provided, render an <img/> with that URL so custom
 * gateway logos can be configured per-method.
 */
export function getPaymentIcon(
  paymentType: string | undefined,
  className: string = 'h-4 w-4',
  iconUrl?: string,
  altName?: string
): ReactNode {
  const safeIconUrl = normalizeHttpIconUrl(iconUrl)
  if (safeIconUrl) {
    return (
      <img
        src={safeIconUrl}
        alt={altName || paymentType || 'payment'}
        className={className}
        style={{ objectFit: 'contain' }}
        loading='lazy'
        decoding='async'
        referrerPolicy='no-referrer'
      />
    )
  }

  if (!paymentType) {
    return <CreditCard className={className} />
  }

  switch (paymentType) {
    case PAYMENT_TYPES.ALIPAY:
      return (
        <SiAlipay
          className={className}
          style={{ color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.ALIPAY] }}
        />
      )
    case PAYMENT_TYPES.WECHAT:
      return (
        <SiWechat
          className={className}
          style={{ color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.WECHAT] }}
        />
      )
    case PAYMENT_TYPES.STRIPE:
      return (
        <SiStripe
          className={className}
          style={{ color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.STRIPE] }}
        />
      )
    case PAYMENT_TYPES.CREEM:
      return (
        <Landmark
          className={className}
          style={{ color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.CREEM] }}
        />
      )
    case PAYMENT_TYPES.WAFFO:
      return (
        <CreditCard
          className={className}
          style={{ color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.WAFFO] }}
        />
      )
    case PAYMENT_TYPES.WAFFO_PANCAKE:
      // The W glyph fills only ~40% of its viewBox vertically (wide and
      // short letterform); scale(2) brings its rendered height in line
      // with Stripe's S and Creem's Landmark.
      return (
        <span
          className={`inline-flex items-center justify-center leading-none ${className}`}
          style={{ transform: 'scale(2)' }}
        >
          <img
            src='/waffo-logo-light.svg'
            alt={i18next.t('Waffo')}
            className='block h-full w-full object-contain dark:hidden'
          />
          <img
            src='/waffo-logo-dark.svg'
            alt={i18next.t('Waffo')}
            className='hidden h-full w-full object-contain dark:block'
          />
        </span>
      )
    case PAYMENT_TYPES.BEPUSDT:
      return (
        <svg
          className={className}
          viewBox='0 0 32 32'
          fill='none'
          xmlns='http://www.w3.org/2000/svg'
        >
          <circle cx='16' cy='16' r='16' fill={PAYMENT_ICON_COLORS[PAYMENT_TYPES.BEPUSDT]} />
          <path
            d='M17.922 17.383v-.002c-.11.008-.677.042-1.942.042-1.01 0-1.721-.03-1.971-.042v.003c-3.888-.171-6.79-.848-6.79-1.658 0-.809 2.902-1.486 6.79-1.66v2.644c.254.018.982.061 1.988.061 1.207 0 1.812-.05 1.925-.06v-2.643c3.88.173 6.775.851 6.775 1.658 0 .81-2.895 1.485-6.775 1.657m0-3.59v-2.366h5.414V7.819H8.595v3.608h5.414v2.365c-4.4.202-7.709 1.074-7.709 2.118 0 1.044 3.309 1.915 7.709 2.118v7.582h3.913v-7.584c4.393-.202 7.694-1.073 7.694-2.116 0-1.043-3.301-1.914-7.694-2.117'
            fill='#fff'
          />
        </svg>
      )
    case PAYMENT_TYPES.OKPAY:
      return (
        <CreditCard
          className={className}
          style={{ color: PAYMENT_ICON_COLORS[PAYMENT_TYPES.OKPAY] }}
        />
      )
    default:
      return <CreditCard className={className} />
  }
}
