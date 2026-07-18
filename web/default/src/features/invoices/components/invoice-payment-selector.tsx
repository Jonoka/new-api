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
import { useMemo } from 'react'
import { WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getPaymentIcon } from '@/features/wallet/lib'
import type { InvoiceBepusdtChain, InvoicePaymentMethod } from '../types'

interface InvoicePaymentSelectorProps {
  methods: InvoicePaymentMethod[]
  value: string
  onValueChange: (value: string) => void
  bepusdtChains: InvoiceBepusdtChain[]
  tradeType: string
  onTradeTypeChange: (value: string) => void
  disabled?: boolean
}

export function InvoicePaymentSelector(props: InvoicePaymentSelectorProps) {
  const { t } = useTranslation()
  const methods = useMemo(
    () => [
      {
        name: t('Account balance'),
        type: 'balance',
        provider: 'balance',
        color: '',
      },
      ...props.methods.filter(
        (method) => method.type !== 'balance' && method.provider !== 'balance'
      ),
    ],
    [props.methods, t]
  )
  const selectedMethod = methods.find((method) => method.type === props.value)
  const isBepusdt =
    selectedMethod?.provider === 'bepusdt' || selectedMethod?.type === 'bepusdt'
  const selectedChain = props.bepusdtChains.find(
    (chain) => chain.trade_type === props.tradeType
  )

  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <Label>{t('Payment method')}</Label>
      <RadioGroup
        value={props.value}
        onValueChange={props.onValueChange}
        className='grid gap-2 sm:grid-cols-2'
      >
        {methods.map((method) => {
          const selected = method.type === props.value
          const id = `invoice-payment-${method.type}`
          return (
            <Label
              key={method.type}
              htmlFor={id}
              className={cn(
                'hover:bg-muted/50 flex cursor-pointer items-center gap-3 rounded-lg border p-3 transition-colors',
                selected && 'border-primary bg-primary/5',
                props.disabled && 'cursor-not-allowed opacity-60'
              )}
            >
              <RadioGroupItem
                id={id}
                value={method.type}
                disabled={props.disabled}
              />
              <span
                className='flex size-5 items-center justify-center'
                style={method.color ? { color: method.color } : undefined}
              >
                {method.provider === 'balance' ? (
                  <WalletCards className='size-5' aria-hidden='true' />
                ) : (
                  getPaymentIcon(method.type, 'size-5')
                )}
              </span>
              <span className='min-w-0 truncate text-sm font-medium'>
                {method.name}
              </span>
            </Label>
          )
        })}
      </RadioGroup>

      {isBepusdt && props.bepusdtChains.length > 0 && (
        <div className='grid gap-2 border-t pt-3'>
          <Label>{t('USDT network')}</Label>
          <Select
            value={props.tradeType}
            onValueChange={(value) => {
              if (value) props.onTradeTypeChange(value)
            }}
            disabled={props.disabled}
          >
            <SelectTrigger>
              <SelectValue placeholder={t('Select a USDT network')}>
                {selectedChain?.name || props.tradeType}
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {props.bepusdtChains.map((chain) => (
                  <SelectItem key={chain.trade_type} value={chain.trade_type}>
                    {chain.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )
}
