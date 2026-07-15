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
import { FileText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  createEmptyInvoiceRequest,
  normalizeInvoiceConfig,
  type InvoiceConfig,
  type InvoiceRequest,
  type InvoiceType,
} from '../types'

interface InvoiceRequestFormProps {
  config?: InvoiceConfig | null
  value: InvoiceRequest
  onChange: (value: InvoiceRequest) => void
  invoiceFee?: number
  disabled?: boolean
  showRequiredToggle?: boolean
}

function getTypeLabel(type: InvoiceType, t: (key: string) => string) {
  return type === 'company' ? t('Company invoice') : t('Personal invoice')
}

export function InvoiceRequestForm({
  config,
  value,
  onChange,
  invoiceFee = 0,
  disabled = false,
  showRequiredToggle = true,
}: InvoiceRequestFormProps) {
  const { t } = useTranslation()
  const invoiceConfig = normalizeInvoiceConfig(config)
  const invoice =
    value || createEmptyInvoiceRequest(invoiceConfig.types[0] || 'personal')
  const currentType = invoiceConfig.types.includes(invoice.type)
    ? invoice.type
    : invoiceConfig.types[0] || 'personal'
  const invoiceTypeItems = invoiceConfig.types.map((type) => ({
    value: type,
    label: getTypeLabel(type, t),
  }))

  const patchInvoice = (patch: Partial<InvoiceRequest>) => {
    onChange({
      ...invoice,
      type: currentType,
      ...patch,
    })
  }

  if (!invoiceConfig.enabled) {
    return (
      <div className='bg-muted/40 rounded-lg border p-3 text-sm'>
        <div className='flex items-center gap-2 font-medium'>
          <FileText className='h-4 w-4' />
          {t('Invoice')}
        </div>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Invoices are not supported for this payment.')}
        </p>
      </div>
    )
  }

  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <div className='flex items-start gap-2'>
        {showRequiredToggle && (
          <Checkbox
            checked={invoice.required}
            disabled={disabled}
            onCheckedChange={(checked) =>
              patchInvoice({
                required: Boolean(checked),
                type: currentType,
              })
            }
            className='mt-0.5'
          />
        )}
        <div className='min-w-0 flex-1'>
          <div className='text-sm font-medium'>{t('Need invoice')}</div>
          <p className='text-muted-foreground text-xs'>
            {t('Invoice service is available for {{types}}.', {
              types: invoiceConfig.types
                .map((type) => getTypeLabel(type, t))
                .join(' / '),
            })}
          </p>
          {invoice.required && Number(invoiceFee || 0) > 0 && (
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Invoice fee is charged in CNY')}: ¥
              {Number(invoiceFee).toFixed(2)}
            </p>
          )}
        </div>
      </div>

      {invoice.required && (
        <div className='grid gap-3'>
          <div className='grid gap-2'>
            <Label>{t('Invoice type')}</Label>
            <Select
              items={invoiceTypeItems}
              value={currentType}
              onValueChange={(nextType) => {
                if (!nextType) return
                patchInvoice({ type: nextType as InvoiceType })
              }}
              disabled={disabled}
            >
              <SelectTrigger>
                <SelectValue>{getTypeLabel(currentType, t)}</SelectValue>
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {invoiceConfig.types.map((type) => (
                    <SelectItem key={type} value={type}>
                      {getTypeLabel(type, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>

          <div className='grid gap-2'>
            <Label>{t('Invoice title')}</Label>
            <Input
              value={invoice.title}
              onChange={(event) => patchInvoice({ title: event.target.value })}
              disabled={disabled}
              placeholder={t('Enter invoice title')}
            />
          </div>

          <div className='grid gap-2'>
            <Label>{t('Tax number')}</Label>
            <Input
              value={invoice.tax_no}
              onChange={(event) => patchInvoice({ tax_no: event.target.value })}
              disabled={disabled}
              placeholder={t('Enter taxpayer identification number')}
            />
          </div>

          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='grid gap-2'>
              <Label>{t('Email')}</Label>
              <Input
                value={invoice.email}
                onChange={(event) =>
                  patchInvoice({ email: event.target.value })
                }
                disabled={disabled}
                placeholder='name@example.com'
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Phone')}</Label>
              <Input
                value={invoice.phone}
                onChange={(event) =>
                  patchInvoice({ phone: event.target.value })
                }
                disabled={disabled}
                placeholder={t('Phone number')}
              />
            </div>
          </div>

          <div className='grid gap-2'>
            <Label>{t('Remark')}</Label>
            <Textarea
              value={invoice.remark}
              onChange={(event) => patchInvoice({ remark: event.target.value })}
              disabled={disabled}
              placeholder={t('Optional invoice remark')}
              rows={2}
            />
          </div>
        </div>
      )}
    </div>
  )
}
