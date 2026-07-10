import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import {
  getInvoicePayload,
  type InvoiceRequest,
} from '@/features/invoices/types'
import { requestBepusdtPayment, isApiSuccess } from '../api'

function getPaymentErrorMessage(data: unknown, fallback?: string): string {
  return typeof data === 'string' && data.trim() ? data : fallback || ''
}

/**
 * Hook for handling Bepusdt (USDT) payment processing
 */
export function useBepusdtPayment() {
  const [processing, setProcessing] = useState(false)

  const processBepusdtPayment = useCallback(
    async (
      topupAmount: number,
      tradeType: string,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      setProcessing(true)

      try {
        const response = await requestBepusdtPayment({
          amount: Math.floor(topupAmount),
          trade_type: tradeType,
          promo_code: promoCode,
          ...getInvoicePayload(invoiceRequest),
        })

        if (isApiSuccess(response)) {
          if (
            response.data &&
            typeof response.data === 'object' &&
            'completed' in response.data &&
            (response.data as Record<string, unknown>).completed
          ) {
            toast.success(i18next.t('Order completed successfully'))
            return true
          }

          const paymentUrl =
            response.data &&
            typeof response.data === 'object' &&
            'payment_url' in response.data
              ? (response.data as { payment_url: string }).payment_url
              : null

          if (paymentUrl) {
            window.open(paymentUrl, '_blank')
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        const errorMsg =
          getPaymentErrorMessage(response.data, response.message) ||
          i18next.t('Payment request failed')
        toast.error(errorMsg)
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

  return { processing, processBepusdtPayment }
}
