import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { requestOkpayPayment, isApiSuccess } from '../api'

function getPaymentErrorMessage(data: unknown, fallback?: string): string {
  return typeof data === 'string' && data.trim() ? data : fallback || ''
}

/**
 * Hook for handling OKPay payment processing
 */
export function useOkpayPayment() {
  const [processing, setProcessing] = useState(false)

  const processOkpayPayment = useCallback(
    async (topupAmount: number, promoCode?: string) => {
      setProcessing(true)

      try {
        const response = await requestOkpayPayment({
          amount: Math.floor(topupAmount),
          promo_code: promoCode,
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

  return { processing, processOkpayPayment }
}
