import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { PAYMENT_ICON_COLORS, PAYMENT_TYPES } from '../../constants'
import type { BepusdtChain } from '../../types'

interface BepusdtChainDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  chains: BepusdtChain[]
  topupAmount: number
  onConfirm: (tradeType: string) => void
  processing: boolean
}

export function BepusdtChainDialog({
  open,
  onOpenChange,
  chains,
  topupAmount,
  onConfirm,
  processing,
}: BepusdtChainDialogProps) {
  const { t } = useTranslation()
  const [selectedChain, setSelectedChain] = useState<string | null>(null)

  const handleConfirm = () => {
    if (selectedChain) {
      onConfirm(selectedChain)
    }
  }

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setSelectedChain(null)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-[460px]'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <svg
              className='h-5 w-5'
              viewBox='0 0 32 32'
              fill='none'
              xmlns='http://www.w3.org/2000/svg'
            >
              <circle
                cx='16'
                cy='16'
                r='16'
                fill={PAYMENT_ICON_COLORS[PAYMENT_TYPES.BEPUSDT]}
              />
              <path
                d='M17.922 17.383v-.002c-.11.008-.677.042-1.942.042-1.01 0-1.721-.03-1.971-.042v.003c-3.888-.171-6.79-.848-6.79-1.658 0-.809 2.902-1.486 6.79-1.66v2.644c.254.018.982.061 1.988.061 1.207 0 1.812-.05 1.925-.06v-2.643c3.88.173 6.775.851 6.775 1.658 0 .81-2.895 1.485-6.775 1.657m0-3.59v-2.366h5.414V7.819H8.595v3.608h5.414v2.365c-4.4.202-7.709 1.074-7.709 2.118 0 1.044 3.309 1.915 7.709 2.118v7.582h3.913v-7.584c4.393-.202 7.694-1.073 7.694-2.116 0-1.043-3.301-1.914-7.694-2.117'
                fill='#fff'
              />
            </svg>
            {t('Select USDT Network')}
          </DialogTitle>
          <DialogDescription>
            {t('Choose the blockchain network for your USDT payment.')}
          </DialogDescription>
        </DialogHeader>

        <div className='grid grid-cols-2 gap-2 py-3 sm:grid-cols-3 sm:gap-3 sm:py-4'>
          {chains.map((chain) => (
            <Button
              key={chain.trade_type}
              variant={
                selectedChain === chain.trade_type ? 'default' : 'outline'
              }
              className='h-auto flex-col gap-1 py-3'
              onClick={() => setSelectedChain(chain.trade_type)}
              disabled={processing}
            >
              <span className='text-sm font-semibold'>{chain.name}</span>
              <span className='text-muted-foreground text-[10px] opacity-70'>
                {chain.trade_type}
              </span>
            </Button>
          ))}
        </div>

        {topupAmount > 0 && (
          <div className='text-muted-foreground text-center text-sm'>
            {t('Topup Amount')}: {topupAmount}
          </div>
        )}

        <DialogFooter className='grid grid-cols-2 gap-2 sm:flex'>
          <Button
            variant='outline'
            onClick={() => handleOpenChange(false)}
            disabled={processing}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={processing || !selectedChain}
          >
            {processing && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Confirm Payment')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
