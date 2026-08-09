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
import { useMemo, useState } from 'react'
import { Check, GripVertical, Plus, X } from 'lucide-react'
import { Reorder, useDragControls } from 'motion/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  type ApiKeyGroupOption,
  formatGroupRatio,
  getRatioBadgeClassName,
} from './api-key-group-combobox'

// ============================================================================
// Reorder Group Card
// ============================================================================

function ReorderGroupCard({
  option,
  onRemove,
  ratioLabel,
}: {
  option: ApiKeyGroupOption
  onRemove: () => void
  ratioLabel: string
}) {
  const { t } = useTranslation()
  const controls = useDragControls()
  const ratioText = formatGroupRatio(option.ratio, ratioLabel)

  return (
    <Reorder.Item
      value={option.value}
      dragListener={false}
      dragControls={controls}
      className='border-border/60 bg-background flex items-center gap-2 rounded-lg border px-2.5 py-2 shadow-xs sm:gap-3 sm:px-3 sm:py-2.5'
      whileDrag={{ boxShadow: '0 4px 16px rgba(0,0,0,0.12)', zIndex: 50 }}
    >
      <button
        type='button'
        className='text-muted-foreground/60 hover:text-muted-foreground -ml-0.5 shrink-0 cursor-grab touch-none active:cursor-grabbing'
        onPointerDown={(e) => controls.start(e)}
      >
        <GripVertical className='size-4' />
      </button>

      <div className='min-w-0 flex-1'>
        <div className='truncate text-sm font-medium'>{option.label}</div>
        {option.desc && (
          <div className='text-muted-foreground truncate text-[11px] sm:text-xs'>
            {option.desc}
          </div>
        )}
      </div>

      {ratioText && (
        <Badge
          variant='outline'
          className={cn(
            'hidden shrink-0 text-[10px] sm:inline-flex sm:text-xs',
            getRatioBadgeClassName(option.ratio)
          )}
        >
          {ratioText}
        </Badge>
      )}

      {option.exclusive && (
        <Badge variant='secondary' className='shrink-0 text-[10px] sm:text-xs'>
          {t('Independent')}
        </Badge>
      )}

      <button
        type='button'
        onClick={onRemove}
        className='text-muted-foreground/60 hover:text-destructive shrink-0 transition-colors'
      >
        <X className='size-3.5' />
      </button>
    </Reorder.Item>
  )
}

// ============================================================================
// Multi-Select Group Component
// ============================================================================

type ApiKeyGroupMultiSelectProps = {
  options: ApiKeyGroupOption[]
  value: string[]
  onValueChange: (value: string[]) => void
  placeholder?: string
  disabled?: boolean
}

export function ApiKeyGroupMultiSelect({
  options,
  value,
  onValueChange,
  placeholder,
  disabled,
}: ApiKeyGroupMultiSelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [searchValue, setSearchValue] = useState('')

  const selectedSet = useMemo(() => new Set(value), [value])
  const isAutoSelected = selectedSet.has('auto')
  const isExclusiveSelected = options.some(
    (option) => option.exclusive && selectedSet.has(option.value)
  )

  // Options available for selection (not yet selected)
  const availableOptions = useMemo(() => {
    const search = searchValue.trim().toLowerCase()
    return options.filter((option) => {
      if (isExclusiveSelected) return false
      if (isAutoSelected && option.value !== 'auto') return false
      if (option.value === 'auto' && value.length > 0 && !isAutoSelected)
        return false
      if (option.exclusive && value.length > 0) return false
      if (selectedSet.has(option.value)) return false
      if (!search) return true
      return (
        option.value.toLowerCase().includes(search) ||
        option.label.toLowerCase().includes(search) ||
        option.desc?.toLowerCase().includes(search)
      )
    })
  }, [
    options,
    searchValue,
    selectedSet,
    isAutoSelected,
    isExclusiveSelected,
    value.length,
  ])

  // Map value to option for rendering selected cards
  const optionMap = useMemo(() => {
    const map = new Map<string, ApiKeyGroupOption>()
    for (const opt of options) {
      map.set(opt.value, opt)
    }
    return map
  }, [options])

  const handleSelect = (selectedValue: string) => {
    const selectedOption = options.find(
      (option) => option.value === selectedValue
    )
    if (selectedOption?.exclusive) {
      onValueChange([selectedValue])
    } else if (selectedValue === 'auto') {
      onValueChange(['auto'])
    } else {
      const filtered = value.filter((v) => v !== 'auto')
      onValueChange([...filtered, selectedValue])
    }
    setSearchValue('')
  }

  const handleRemove = (removeValue: string) => {
    onValueChange(value.filter((v) => v !== removeValue))
  }

  const canAddMore =
    !disabled &&
    !isAutoSelected &&
    !isExclusiveSelected &&
    options.some(
      (option) =>
        !selectedSet.has(option.value) &&
        option.value !== 'auto' &&
        (!option.exclusive || value.length === 0)
    )
  const canSelectAuto = !disabled && value.length === 0
  const showAddButton = canAddMore || canSelectAuto || value.length === 0

  return (
    <div className='flex flex-col gap-2'>
      {/* Selected groups — draggable list */}
      {value.length > 0 && (
        <Reorder.Group
          axis='y'
          values={value}
          onReorder={onValueChange}
          className='flex flex-col gap-1.5'
          layoutScroll
        >
          {value.map((v) => {
            const opt = optionMap.get(v)
            if (!opt) return null
            return (
              <ReorderGroupCard
                key={v}
                option={opt}
                onRemove={() => handleRemove(v)}
                ratioLabel={t('Ratio')}
              />
            )
          })}
        </Reorder.Group>
      )}

      {/* Add group button with popover */}
      {showAddButton && (
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger
            render={
              <Button
                type='button'
                variant='outline'
                disabled={disabled}
                className={cn(
                  'text-muted-foreground hover:text-foreground h-auto gap-1.5 border-dashed px-3 py-2 text-sm',
                  value.length === 0 &&
                    'border-input bg-muted/40 hover:bg-muted/55 min-h-14 justify-start border-solid sm:min-h-16'
                )}
              />
            }
          >
            <Plus className='size-3.5' />
            {value.length === 0
              ? placeholder || t('Select groups')
              : t('Add group')}
          </PopoverTrigger>
          <PopoverContent
            className='data-closed:zoom-out-100 data-open:zoom-in-100 data-[side=bottom]:slide-in-from-top-0 data-[side=left]:slide-in-from-right-0 data-[side=right]:slide-in-from-left-0 data-[side=top]:slide-in-from-bottom-0 w-[var(--anchor-width)] overflow-hidden rounded-xl p-0 shadow-lg data-closed:duration-75 data-open:duration-100'
            onWheel={(event) => event.stopPropagation()}
            onTouchMove={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
          >
            <Command shouldFilter={false}>
              <CommandInput
                placeholder={t('Search...')}
                value={searchValue}
                onValueChange={setSearchValue}
              />
              <CommandList className='max-h-[360px]'>
                <CommandEmpty>{t('No group found.')}</CommandEmpty>
                <CommandGroup>
                  {availableOptions.map((option) => (
                    <CommandItem
                      key={option.value}
                      value={option.value}
                      onSelect={() => handleSelect(option.value)}
                      className='data-[selected=true]:bg-muted items-start gap-3 rounded-lg px-3 py-3 transition-colors'
                    >
                      <Check
                        className={cn(
                          'mt-0.5 h-4 w-4',
                          selectedSet.has(option.value)
                            ? 'opacity-100'
                            : 'opacity-0'
                        )}
                      />
                      <span className='min-w-0 flex-1'>
                        <span className='block truncate font-medium'>
                          {option.label}
                        </span>
                        {option.desc && (
                          <span className='text-muted-foreground block truncate text-xs'>
                            {option.desc}
                          </span>
                        )}
                      </span>
                      {option.ratio !== undefined && (
                        <Badge
                          variant='outline'
                          className={cn(
                            'max-w-24 shrink-0 truncate text-[10px] sm:max-w-none sm:text-xs',
                            getRatioBadgeClassName(option.ratio)
                          )}
                        >
                          {formatGroupRatio(option.ratio, t('Ratio'))}
                        </Badge>
                      )}
                      {option.exclusive && (
                        <Badge
                          variant='secondary'
                          className='shrink-0 text-[10px] sm:text-xs'
                        >
                          {t('Independent')}
                        </Badge>
                      )}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      )}

      {/* Hint text */}
      {value.length > 1 && !isAutoSelected && !isExclusiveSelected && (
        <p className='text-muted-foreground text-[11px] sm:text-xs'>
          {t(
            'Drag to reorder. When multiple groups have the same model, they will be tried in order.'
          )}
        </p>
      )}
      {value.length > 1 && isExclusiveSelected && (
        <p className='text-destructive text-[11px] sm:text-xs'>
          {t('Independent groups must be selected alone.')}
        </p>
      )}
    </div>
  )
}
