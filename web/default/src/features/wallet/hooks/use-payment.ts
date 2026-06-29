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
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  getInvoicePayload,
  type InvoiceRequest,
} from '@/features/invoices/types'
import {
  calculateAmount,
  calculateBepusdtAmount,
  calculateOkpayAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isBepusdtPayment,
  isOkpayPayment,
  isWaffoPayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'

// ============================================================================
// Payment Hook
// ============================================================================

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [amountText, setAmountText] = useState<string>('')
  const [invoiceFee, setInvoiceFee] = useState(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      try {
        setCalculating(true)

        const isStripe = isStripePayment(paymentType)
        const isWaffo = isWaffoPayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)
        const isBepusdt = isBepusdtPayment(paymentType)
        const isOkpay = isOkpayPayment(paymentType)
        const invoicePayload = getInvoicePayload(invoiceRequest)

        const response = isStripe
          ? await calculateStripeAmount({
              amount: topupAmount,
              promo_code: promoCode,
              ...invoicePayload,
            })
          : isWaffo
            ? await calculateWaffoAmount({
                amount: topupAmount,
                promo_code: promoCode,
                ...invoicePayload,
              })
            : isPancake
              ? await calculateWaffoPancakeAmount({
                  amount: topupAmount,
                  promo_code: promoCode,
                  ...invoicePayload,
                })
              : isBepusdt
                ? await calculateBepusdtAmount({
                    amount: topupAmount,
                    promo_code: promoCode,
                    ...invoicePayload,
                  })
                : isOkpay
                  ? await calculateOkpayAmount({
                      amount: topupAmount,
                      promo_code: promoCode,
                      ...invoicePayload,
                    })
                  : await calculateAmount({
                      amount: topupAmount,
                      promo_code: promoCode,
                      ...invoicePayload,
                    })

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = parseFloat(response.data)
          setAmount(calculatedAmount)
          setAmountText(response.amount_text || '')
          setInvoiceFee(Number(response.invoice_fee || 0))
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        setAmountText('')
        setInvoiceFee(0)
        return 0
      } catch (_error) {
        setAmount(0)
        setAmountText('')
        setInvoiceFee(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const amount = Math.floor(topupAmount)
        const invoicePayload = getInvoicePayload(invoiceRequest)

        const response = isStripe
          ? await requestStripePayment({
              amount,
              payment_method: 'stripe',
              promo_code: promoCode,
              ...invoicePayload,
            })
          : await requestPayment({
              amount,
              payment_method: paymentType,
              promo_code: promoCode,
              ...invoicePayload,
            })

        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }

        if (
          (response as { completed?: boolean }).completed ||
          (response.data &&
            typeof response.data === 'object' &&
            'completed' in response.data &&
            response.data.completed)
        ) {
          toast.success(i18next.t('Order completed successfully'))
          return true
        }

        // Handle Stripe payment
        if (isStripe && response.data?.pay_link) {
          window.open(response.data.pay_link as string, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        // Handle non-Stripe payment
        if (!isStripe && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        return false
      } catch (_error) {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    amountText,
    invoiceFee,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
