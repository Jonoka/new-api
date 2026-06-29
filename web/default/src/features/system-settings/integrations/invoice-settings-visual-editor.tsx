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
import * as React from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

type InvoiceFeeRuleType = 'fixed' | 'percent'

type InvoiceFeeRule = {
  min: number
  max?: number
  type: InvoiceFeeRuleType
  value: number
}

type InvoiceSettingsVisualEditorProps = {
  typesValue: string
  feeRulesValue: string
  onTypesChange: (value: string) => void
  onFeeRulesChange: (value: string) => void
}

const DEFAULT_TYPES = ['personal', 'company']
const DEFAULT_RULES: InvoiceFeeRule[] = [
  { min: 0, max: 500, type: 'fixed', value: 50 },
  { min: 501, max: 2000, type: 'fixed', value: 100 },
  { min: 2001, max: 5000, type: 'fixed', value: 175 },
  { min: 5000, type: 'percent', value: 5 },
]

function parseTypes(value: string): string[] {
  try {
    const parsed = JSON.parse(value || '[]')
    if (!Array.isArray(parsed)) return DEFAULT_TYPES
    const types = parsed.filter(
      (item): item is string => item === 'personal' || item === 'company'
    )
    return types.length > 0 ? types : DEFAULT_TYPES
  } catch {
    return DEFAULT_TYPES
  }
}

function parseRules(value: string): InvoiceFeeRule[] {
  try {
    const parsed = JSON.parse(value || '[]')
    if (!Array.isArray(parsed)) return DEFAULT_RULES
    const rules = parsed
      .map((item): InvoiceFeeRule => ({
        min: Number(item?.min ?? 0),
        max:
          item?.max === undefined || item?.max === ''
            ? undefined
            : Number(item.max),
        type: item?.type === 'percent' ? 'percent' : 'fixed',
        value: Number(item?.value ?? 0),
      }))
      .filter(
        (rule) =>
          Number.isFinite(rule.min) &&
          (rule.max === undefined || Number.isFinite(rule.max)) &&
          Number.isFinite(rule.value)
      )
      .sort((a, b) => a.min - b.min)
    return rules.length > 0 ? rules : DEFAULT_RULES
  } catch {
    return DEFAULT_RULES
  }
}

function serializeTypes(types: string[]) {
  return JSON.stringify(types.length > 0 ? types : ['personal'], null, 2)
}

function serializeRules(rules: InvoiceFeeRule[]) {
  const normalized = rules
    .map((rule) => ({
      min: Number(rule.min) || 0,
      ...(rule.max !== undefined && rule.max > 0 ? { max: Number(rule.max) } : {}),
      type: rule.type === 'percent' ? 'percent' : 'fixed',
      value: Number(rule.value) || 0,
    }))
    .sort((a, b) => a.min - b.min)
  return JSON.stringify(normalized, null, 2)
}

export function InvoiceSettingsVisualEditor({
  typesValue,
  feeRulesValue,
  onTypesChange,
  onFeeRulesChange,
}: InvoiceSettingsVisualEditorProps) {
  const { t } = useTranslation()
  const types = React.useMemo(() => parseTypes(typesValue), [typesValue])
  const rules = React.useMemo(() => parseRules(feeRulesValue), [feeRulesValue])

  const toggleType = (type: string, checked: boolean) => {
    const next = checked
      ? Array.from(new Set([...types, type]))
      : types.filter((item) => item !== type)
    onTypesChange(serializeTypes(next.length > 0 ? next : [type]))
  }

  const patchRule = (
    index: number,
    patch: Partial<InvoiceFeeRule> & { maxText?: string }
  ) => {
    const next = rules.map((rule, idx) => {
      if (idx !== index) return rule
      const merged = { ...rule, ...patch }
      if ('maxText' in patch) {
        const text = patch.maxText ?? ''
        if (text === '') {
          delete merged.max
        } else {
          merged.max = Number(text)
        }
      }
      return merged
    })
    onFeeRulesChange(serializeRules(next))
  }

  const addRule = () => {
    const last = rules[rules.length - 1]
    const nextMin = last?.max ? last.max + 1 : (last?.min || 0) + 1
    onFeeRulesChange(
      serializeRules([...rules, { min: nextMin, type: 'fixed', value: 0 }])
    )
  }

  const deleteRule = (index: number) => {
    const next = rules.filter((_, idx) => idx !== index)
    onFeeRulesChange(serializeRules(next.length > 0 ? next : DEFAULT_RULES))
  }

  return (
    <div className='space-y-4'>
      <div className='rounded-lg border p-4'>
        <div className='mb-3 text-sm font-medium'>{t('Invoice types')}</div>
        <div className='flex flex-wrap gap-4'>
          {[
            { value: 'personal', label: t('Personal invoice') },
            { value: 'company', label: t('Company invoice') },
          ].map((item) => (
            <label
              key={item.value}
              className='flex items-center gap-2 text-sm'
            >
              <Checkbox
                checked={types.includes(item.value)}
                onCheckedChange={(checked) =>
                  toggleType(item.value, checked === true)
                }
              />
              {item.label}
            </label>
          ))}
        </div>
      </div>

      <div className='rounded-lg border'>
        <div className='flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <div className='text-sm font-medium'>
              {t('Invoice fee rules')}
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Rules match invoice amount in CNY. Leave max empty for no upper limit.')}
            </p>
          </div>
          <Button type='button' size='sm' onClick={addRule}>
            <Plus className='h-4 w-4 sm:mr-2' />
            {t('Add rule')}
          </Button>
        </div>

        <div className='hidden md:block'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Minimum')}</TableHead>
                <TableHead>{t('Maximum')}</TableHead>
                <TableHead>{t('Fee type')}</TableHead>
                <TableHead>{t('Value')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((rule, index) => (
                <TableRow key={`${rule.min}-${index}`}>
                  <TableCell>
                    <Input
                      type='number'
                      min={0}
                      value={rule.min}
                      onChange={(event) =>
                        patchRule(index, { min: Number(event.target.value) })
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <Input
                      type='number'
                      min={0}
                      value={rule.max ?? ''}
                      placeholder={t('No limit')}
                      onChange={(event) =>
                        patchRule(index, { maxText: event.target.value })
                      }
                    />
                  </TableCell>
                  <TableCell>
                    <Select
                      value={rule.type}
                      onValueChange={(value) =>
                        patchRule(index, {
                          type: value === 'percent' ? 'percent' : 'fixed',
                        })
                      }
                    >
                      <SelectTrigger className='w-32'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='fixed'>
                            {t('Fixed amount')}
                          </SelectItem>
                          <SelectItem value='percent'>
                            {t('Percentage')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell>
                    <Input
                      type='number'
                      min={0}
                      value={rule.value}
                      onChange={(event) =>
                        patchRule(index, { value: Number(event.target.value) })
                      }
                    />
                  </TableCell>
                  <TableCell className='text-right'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => deleteRule(index)}
                    >
                      <Trash2 className='h-4 w-4' />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        <div className='divide-y md:hidden'>
          {rules.map((rule, index) => (
            <div key={`${rule.min}-${index}`} className='space-y-3 p-4'>
              <div className='grid grid-cols-2 gap-3'>
                <label className='space-y-1 text-xs'>
                  <span className='text-muted-foreground'>{t('Minimum')}</span>
                  <Input
                    type='number'
                    min={0}
                    value={rule.min}
                    onChange={(event) =>
                      patchRule(index, { min: Number(event.target.value) })
                    }
                  />
                </label>
                <label className='space-y-1 text-xs'>
                  <span className='text-muted-foreground'>{t('Maximum')}</span>
                  <Input
                    type='number'
                    min={0}
                    value={rule.max ?? ''}
                    placeholder={t('No limit')}
                    onChange={(event) =>
                      patchRule(index, { maxText: event.target.value })
                    }
                  />
                </label>
              </div>
              <div className='grid grid-cols-2 gap-3'>
                <label className='space-y-1 text-xs'>
                  <span className='text-muted-foreground'>{t('Fee type')}</span>
                  <Select
                    value={rule.type}
                    onValueChange={(value) =>
                      patchRule(index, {
                        type: value === 'percent' ? 'percent' : 'fixed',
                      })
                    }
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='fixed'>
                          {t('Fixed amount')}
                        </SelectItem>
                        <SelectItem value='percent'>
                          {t('Percentage')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </label>
                <label className='space-y-1 text-xs'>
                  <span className='text-muted-foreground'>{t('Value')}</span>
                  <Input
                    type='number'
                    min={0}
                    value={rule.value}
                    onChange={(event) =>
                      patchRule(index, { value: Number(event.target.value) })
                    }
                  />
                </label>
              </div>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className={cn('w-full justify-center')}
                onClick={() => deleteRule(index)}
              >
                <Trash2 className='mr-2 h-4 w-4' />
                {t('Delete')}
              </Button>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
