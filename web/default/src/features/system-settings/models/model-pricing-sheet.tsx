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
import { useEffect, useMemo, useState } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { AlertTriangle, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  MODEL_PRICE_UNITS,
  normalizeModelPriceUnit,
  type ModelPriceUnit,
} from '@/lib/model-price-unit'
import {
  getModelPriceVariantCombinationKey,
  isGrokImagineVideoModel,
  normalizeModelPriceVariantQuality,
  normalizeModelPriceVariantResolution,
  type ModelPriceVariantConfig,
} from '@/lib/model-price-variants'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
} from '@/components/drawer-layout'
import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'
import {
  SettingsControlGroup,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { formatPricingNumber } from './pricing-format'
import { TieredPricingEditor } from './tiered-pricing-editor'

const createModelPricingSchema = (t: (key: string) => string) =>
  z.object({
    name: z.string().min(1, t('Model name is required')),
    price: z.string().optional(),
    ratio: z.string().optional(),
    cacheRatio: z.string().optional(),
    createCacheRatio: z.string().optional(),
    completionRatio: z.string().optional(),
    imageRatio: z.string().optional(),
    audioRatio: z.string().optional(),
    audioCompletionRatio: z.string().optional(),
  })

type ModelPricingFormValues = z.infer<
  ReturnType<typeof createModelPricingSchema>
>

type PricingMode = 'per-token' | 'per-request' | 'tiered_expr'
type LaneKey =
  | 'completion'
  | 'cache'
  | 'createCache'
  | 'image'
  | 'audioInput'
  | 'audioOutput'

export type ModelRatioData = {
  name: string
  price?: string
  priceUnit?: ModelPriceUnit
  /** null 表示删除显式覆盖并恢复后端内置继承配置。 */
  priceVariant?: ModelPriceVariantConfig | null
  ratio?: string
  cacheRatio?: string
  createCacheRatio?: string
  completionRatio?: string
  imageRatio?: string
  audioRatio?: string
  audioCompletionRatio?: string
  billingMode?: PricingMode
  billingExpr?: string
  requestRuleExpr?: string
}

type ModelPricingSheetProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: ModelRatioData) => void
  onCancel?: () => void
  editData?: ModelRatioData | null
  selectedTargetCount?: number
}

type ModelPricingEditorPanelProps = Omit<
  ModelPricingSheetProps,
  'open' | 'onOpenChange'
> & {
  className?: string
}

type PreviewRow = {
  key: string
  label: string
  value: string
  multiline?: boolean
}

type PriceVariantRuleDraft = {
  id: string
  resolution: string
  quality: string
  price: string
}

type PriceVariantDraft = {
  configured: boolean
  resolutionEnabled: boolean
  qualityEnabled: boolean
  rules: PriceVariantRuleDraft[]
  inherited: boolean
  restoreInherited: boolean
}

type PriceVariantRuleErrors = Partial<
  Record<'resolution' | 'quality' | 'price' | 'duplicate', string>
>

type PriceVariantValidation = {
  sectionError?: string
  ruleErrors: Record<string, PriceVariantRuleErrors>
  valid: boolean
}

const numericDraftRegex = /^(\d+(\.\d*)?|\.\d*)?$/
let priceVariantRuleSequence = 0

function createPriceVariantRuleId(): string {
  priceVariantRuleSequence += 1
  return `model-price-variant-rule-${priceVariantRuleSequence}`
}

const EMPTY_LANE_PRICES: Record<LaneKey, string> = {
  completion: '',
  cache: '',
  createCache: '',
  image: '',
  audioInput: '',
  audioOutput: '',
}

const EMPTY_LANE_ENABLED: Record<LaneKey, boolean> = {
  completion: false,
  cache: false,
  createCache: false,
  image: false,
  audioInput: false,
  audioOutput: false,
}

const ratioFieldByLane: Record<LaneKey, keyof ModelPricingFormValues> = {
  completion: 'completionRatio',
  cache: 'cacheRatio',
  createCache: 'createCacheRatio',
  image: 'imageRatio',
  audioInput: 'audioRatio',
  audioOutput: 'audioCompletionRatio',
}

const laneConfigs: Array<{
  key: LaneKey
  titleKey: string
  descriptionKey: string
  placeholder: string
}> = [
  {
    key: 'completion',
    titleKey: 'Completion price',
    descriptionKey: 'Output token price for generated tokens.',
    placeholder: '15',
  },
  {
    key: 'cache',
    titleKey: 'Cache read price',
    descriptionKey: 'Token price for cache reads.',
    placeholder: '0.3',
  },
  {
    key: 'createCache',
    titleKey: 'Cache write price',
    descriptionKey: 'Token price for creating cache entries.',
    placeholder: '3.75',
  },
  {
    key: 'image',
    titleKey: 'Image input price',
    descriptionKey: 'Token price for image input.',
    placeholder: '2.5',
  },
  {
    key: 'audioInput',
    titleKey: 'Audio input price',
    descriptionKey: 'Token price for audio input.',
    placeholder: '3.81',
  },
  {
    key: 'audioOutput',
    titleKey: 'Audio output price',
    descriptionKey: 'Token price for audio output.',
    placeholder: '15.11',
  },
]

function createInitialPriceVariantDraft(
  data?: ModelRatioData | null
): PriceVariantDraft {
  const config = data?.priceVariant
  if (config === null) {
    return {
      configured: false,
      resolutionEnabled: false,
      qualityEnabled: false,
      rules: [],
      inherited: false,
      restoreInherited: true,
    }
  }

  return {
    configured: config !== undefined,
    resolutionEnabled: config?.resolution_enabled ?? false,
    qualityEnabled: config?.quality_enabled ?? false,
    rules: (config?.rules ?? []).map((rule) => ({
      id: createPriceVariantRuleId(),
      resolution: rule.resolution ?? '',
      quality: rule.quality ?? '',
      price: String(rule.price),
    })),
    inherited: config?.inherited === true,
    restoreInherited: false,
  }
}

function validatePriceVariantDraft(
  draft: PriceVariantDraft,
  modelName: string,
  t: (key: string) => string
): PriceVariantValidation {
  const resolutionEnabled = draft.resolutionEnabled
  const qualityEnabled =
    !isGrokImagineVideoModel(modelName) && draft.qualityEnabled
  const ruleErrors: Record<string, PriceVariantRuleErrors> = {}

  if (draft.restoreInherited || (!resolutionEnabled && !qualityEnabled)) {
    return { ruleErrors, valid: true }
  }
  if (draft.rules.length === 0) {
    return {
      sectionError: t('Add at least one specification price rule.'),
      ruleErrors,
      valid: false,
    }
  }

  const combinations = new Map<string, string>()
  const setRuleError = (
    ruleId: string,
    field: keyof PriceVariantRuleErrors,
    message: string
  ) => {
    ruleErrors[ruleId] = { ...ruleErrors[ruleId], [field]: message }
  }

  draft.rules.forEach((rule) => {
    const resolution = rule.resolution.trim()
    const quality = rule.quality.trim()
    if (resolutionEnabled && !resolution) {
      setRuleError(rule.id, 'resolution', t('Resolution is required.'))
    }
    if (qualityEnabled && !quality) {
      setRuleError(rule.id, 'quality', t('Quality tier is required.'))
    }
    if (!rule.price.trim()) {
      setRuleError(rule.id, 'price', t('Price is required.'))
    } else {
      const price = Number(rule.price)
      if (!Number.isFinite(price) || price < 0) {
        setRuleError(rule.id, 'price', t('Enter a valid non-negative price.'))
      }
    }

    if ((!resolutionEnabled || resolution) && (!qualityEnabled || quality)) {
      const combination = getModelPriceVariantCombinationKey(
        resolutionEnabled ? resolution : '',
        qualityEnabled ? quality : ''
      )
      const existingRuleId = combinations.get(combination)
      if (existingRuleId) {
        const message = t('This specification combination is duplicated.')
        setRuleError(existingRuleId, 'duplicate', message)
        setRuleError(rule.id, 'duplicate', message)
      } else {
        combinations.set(combination, rule.id)
      }
    }
  })

  return {
    ruleErrors,
    valid: Object.keys(ruleErrors).length === 0,
  }
}

function buildPriceVariantConfig(
  draft: PriceVariantDraft,
  modelName: string
): ModelPriceVariantConfig | null | undefined {
  if (draft.restoreInherited) return null

  const qualityEnabled =
    !isGrokImagineVideoModel(modelName) && draft.qualityEnabled
  if (
    !draft.configured &&
    !draft.resolutionEnabled &&
    !qualityEnabled &&
    draft.rules.length === 0
  ) {
    return undefined
  }

  const config: ModelPriceVariantConfig = {
    resolution_enabled: draft.resolutionEnabled,
    quality_enabled: qualityEnabled,
    inherited: draft.inherited,
  }
  if (draft.resolutionEnabled || qualityEnabled) {
    config.rules = draft.rules.map((rule) => ({
      ...(draft.resolutionEnabled
        ? {
            resolution: draft.inherited
              ? rule.resolution
              : normalizeModelPriceVariantResolution(rule.resolution),
          }
        : {}),
      ...(qualityEnabled
        ? {
            quality: draft.inherited
              ? rule.quality
              : normalizeModelPriceVariantQuality(rule.quality),
          }
        : {}),
      price: Number(rule.price),
    }))
  }
  return config
}

function buildPriceVariantPreview(
  draft: PriceVariantDraft,
  modelName: string,
  t: (key: string) => string
): string | null {
  if (draft.restoreInherited) return t('Restore inherited defaults')

  const qualityEnabled =
    !isGrokImagineVideoModel(modelName) && draft.qualityEnabled
  if (
    !draft.configured &&
    !draft.resolutionEnabled &&
    !qualityEnabled &&
    draft.rules.length === 0
  ) {
    return null
  }

  const preview: Record<string, unknown> = {
    resolution_enabled: draft.resolutionEnabled,
    quality_enabled: qualityEnabled,
    inherited: draft.inherited,
  }
  if (draft.resolutionEnabled || qualityEnabled) {
    preview.rules = draft.rules.map((rule) => {
      const price = Number(rule.price)
      return {
        ...(draft.resolutionEnabled
          ? { resolution: rule.resolution || t('Empty') }
          : {}),
        ...(qualityEnabled ? { quality: rule.quality || t('Empty') } : {}),
        price: rule.price.trim() && Number.isFinite(price) ? price : t('Empty'),
      }
    })
  }

  return JSON.stringify(preview, null, 2)
}

function hasValue(value: unknown): boolean {
  return (
    value !== '' && value !== null && value !== undefined && value !== false
  )
}

function toNumberOrNull(value: unknown): number | null {
  if (!hasValue(value) && value !== 0) return null
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

function ratioToBasePrice(ratio: unknown): string {
  const num = toNumberOrNull(ratio)
  if (num === null) return ''
  return formatPricingNumber(num * 2)
}

function deriveLanePrice(
  ratio: unknown,
  denominator: unknown,
  fallback = ''
): string {
  const ratioNumber = toNumberOrNull(ratio)
  const denominatorNumber = toNumberOrNull(denominator)
  if (ratioNumber === null || denominatorNumber === null) return fallback
  return formatPricingNumber(ratioNumber * denominatorNumber)
}

function createInitialLaneState(data?: ModelRatioData | null) {
  if (!data) {
    return {
      promptPrice: '',
      prices: { ...EMPTY_LANE_PRICES },
      enabled: { ...EMPTY_LANE_ENABLED },
    }
  }

  const promptPrice = ratioToBasePrice(data.ratio)
  const audioInputPrice = deriveLanePrice(data.audioRatio, promptPrice)
  const prices: Record<LaneKey, string> = {
    completion: deriveLanePrice(data.completionRatio, promptPrice),
    cache: deriveLanePrice(data.cacheRatio, promptPrice),
    createCache: deriveLanePrice(data.createCacheRatio, promptPrice),
    image: deriveLanePrice(data.imageRatio, promptPrice),
    audioInput: audioInputPrice,
    audioOutput: deriveLanePrice(data.audioCompletionRatio, audioInputPrice),
  }

  return {
    promptPrice,
    prices,
    enabled: {
      completion: hasValue(data.completionRatio),
      cache: hasValue(data.cacheRatio),
      createCache: hasValue(data.createCacheRatio),
      image: hasValue(data.imageRatio),
      audioInput: hasValue(data.audioRatio),
      audioOutput: hasValue(data.audioCompletionRatio),
    },
  }
}

function getModeLabel(mode: PricingMode) {
  if (mode === 'per-request') return 'Per-request'
  if (mode === 'tiered_expr') return 'Expression'
  return 'Per-token'
}

function getModeBadgeVariant(
  mode: PricingMode
): 'default' | 'secondary' | 'outline' {
  if (mode === 'per-request') return 'secondary'
  if (mode === 'tiered_expr') return 'default'
  return 'outline'
}

function buildPreviewRows(
  values: ModelPricingFormValues,
  mode: PricingMode,
  billingExpr: string,
  requestRuleExpr: string,
  promptPrice: string,
  lanePrices: Record<LaneKey, string>,
  laneEnabled: Record<LaneKey, boolean>,
  priceUnit: ModelPriceUnit,
  priceVariantPreview: string | null,
  t: (key: string) => string
): PreviewRow[] {
  if (mode === 'tiered_expr') {
    const effectiveExpr = combineBillingExpr(billingExpr, requestRuleExpr)
    return [
      { key: 'mode', label: 'BillingMode', value: 'tiered_expr' },
      {
        key: 'expr',
        label: t('Expression'),
        value: effectiveExpr || t('Empty'),
        multiline: true,
      },
    ]
  }

  if (mode === 'per-request') {
    const rows: PreviewRow[] = [
      {
        key: 'price',
        label: 'ModelPrice',
        value: values.price || t('Empty'),
      },
      {
        key: 'priceUnit',
        label: 'ModelPriceUnit',
        value: priceUnit,
      },
    ]
    if (priceVariantPreview) {
      rows.push({
        key: 'priceVariants',
        label: 'ModelPriceVariants',
        value: priceVariantPreview,
        multiline: true,
      })
    }
    return rows
  }

  return [
    {
      key: 'inputPrice',
      label: t('Input price'),
      value: promptPrice ? `$${promptPrice}` : t('Empty'),
    },
    {
      key: 'completion',
      label: t('Completion price'),
      value:
        laneEnabled.completion && lanePrices.completion
          ? `$${lanePrices.completion}`
          : t('Empty'),
    },
    {
      key: 'cache',
      label: t('Cache read price'),
      value:
        laneEnabled.cache && lanePrices.cache
          ? `$${lanePrices.cache}`
          : t('Empty'),
    },
    {
      key: 'createCache',
      label: t('Cache write price'),
      value:
        laneEnabled.createCache && lanePrices.createCache
          ? `$${lanePrices.createCache}`
          : t('Empty'),
    },
    {
      key: 'image',
      label: t('Image input price'),
      value:
        laneEnabled.image && lanePrices.image
          ? `$${lanePrices.image}`
          : t('Empty'),
    },
    {
      key: 'audio',
      label: t('Audio input price'),
      value:
        laneEnabled.audioInput && lanePrices.audioInput
          ? `$${lanePrices.audioInput}`
          : t('Empty'),
    },
    {
      key: 'audioCompletion',
      label: t('Audio output price'),
      value:
        laneEnabled.audioOutput && lanePrices.audioOutput
          ? `$${lanePrices.audioOutput}`
          : t('Empty'),
    },
  ]
}

export function ModelPricingSheet({
  open,
  onOpenChange,
  onSave,
  onCancel,
  editData,
  selectedTargetCount = 0,
}: ModelPricingSheetProps) {
  const { t } = useTranslation()
  const title = editData ? t('Edit model pricing') : t('Add model pricing')
  const description = editData?.name || t('New model')

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className={sideDrawerContentClassName('sm:max-w-2xl')}
      >
        <SheetHeader className='sr-only'>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <ModelPricingEditorPanel
          onSave={onSave}
          editData={editData}
          selectedTargetCount={selectedTargetCount}
          onCancel={() => {
            onCancel?.()
            onOpenChange(false)
          }}
          className='h-full rounded-none border-0'
        />
      </SheetContent>
    </Sheet>
  )
}

export function ModelPricingEditorPanel({
  onSave,
  editData,
  selectedTargetCount = 0,
  onCancel,
  className,
}: ModelPricingEditorPanelProps) {
  const { t } = useTranslation()
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [promptPrice, setPromptPrice] = useState('')
  const [lanePrices, setLanePrices] = useState<Record<LaneKey, string>>({
    ...EMPTY_LANE_PRICES,
  })
  const [laneEnabled, setLaneEnabled] = useState<Record<LaneKey, boolean>>({
    ...EMPTY_LANE_ENABLED,
  })
  const [billingExpr, setBillingExpr] = useState('')
  const [requestRuleExpr, setRequestRuleExpr] = useState('')
  const [priceUnit, setPriceUnit] = useState<ModelPriceUnit>(
    MODEL_PRICE_UNITS.REQUEST
  )
  const [priceVariantDraft, setPriceVariantDraft] = useState<PriceVariantDraft>(
    () => createInitialPriceVariantDraft()
  )
  const [showPriceVariantErrors, setShowPriceVariantErrors] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(true)
  const isEditMode = !!editData

  const form = useForm<ModelPricingFormValues>({
    resolver: zodResolver(createModelPricingSchema(t)),
    defaultValues: {
      name: '',
      price: '',
      ratio: '',
      cacheRatio: '',
      createCacheRatio: '',
      completionRatio: '',
      imageRatio: '',
      audioRatio: '',
      audioCompletionRatio: '',
    },
  })

  useEffect(() => {
    const nextLaneState = createInitialLaneState(editData)

    if (editData) {
      form.reset({
        name: editData.name,
        price: editData.price || '',
        ratio: editData.ratio || '',
        cacheRatio: editData.cacheRatio || '',
        createCacheRatio: editData.createCacheRatio || '',
        completionRatio: editData.completionRatio || '',
        imageRatio: editData.imageRatio || '',
        audioRatio: editData.audioRatio || '',
        audioCompletionRatio: editData.audioCompletionRatio || '',
      })
      setPricingMode(
        editData.billingMode === 'tiered_expr'
          ? 'tiered_expr'
          : editData.billingMode === 'per-request'
            ? 'per-request'
            : 'per-token'
      )
      setBillingExpr(editData.billingExpr || '')
      setRequestRuleExpr(editData.requestRuleExpr || '')
      setPriceUnit(normalizeModelPriceUnit(editData.priceUnit))
    } else {
      form.reset({
        name: '',
        price: '',
        ratio: '',
        cacheRatio: '',
        createCacheRatio: '',
        completionRatio: '',
        imageRatio: '',
        audioRatio: '',
        audioCompletionRatio: '',
      })
      setPricingMode('per-token')
      setBillingExpr('')
      setRequestRuleExpr('')
      setPriceUnit(MODEL_PRICE_UNITS.REQUEST)
    }

    setPromptPrice(nextLaneState.promptPrice)
    setLanePrices(nextLaneState.prices)
    setLaneEnabled(nextLaneState.enabled)
    setPriceVariantDraft(createInitialPriceVariantDraft(editData))
    setShowPriceVariantErrors(false)
    setPreviewOpen(true)
  }, [editData, form])

  const setFormValue = (field: keyof ModelPricingFormValues, value: string) => {
    form.setValue(field, value, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const deriveLaneRatio = (
    lane: LaneKey,
    price: string,
    nextPromptPrice = promptPrice,
    nextLanePrices = lanePrices
  ) => {
    const priceNumber = toNumberOrNull(price)
    if (priceNumber === null) return ''

    if (lane === 'audioOutput') {
      const audioInputPrice = toNumberOrNull(nextLanePrices.audioInput)
      if (audioInputPrice === null || audioInputPrice === 0) return ''
      return formatPricingNumber(priceNumber / audioInputPrice)
    }

    const inputPrice = toNumberOrNull(nextPromptPrice)
    if (inputPrice === null || inputPrice === 0) return ''
    return formatPricingNumber(priceNumber / inputPrice)
  }

  const syncLaneRatios = (
    nextPromptPrice = promptPrice,
    nextLanePrices = lanePrices,
    nextLaneEnabled = laneEnabled
  ) => {
    const inputPrice = toNumberOrNull(nextPromptPrice)
    setFormValue(
      'ratio',
      inputPrice !== null ? formatPricingNumber(inputPrice / 2) : ''
    )

    laneConfigs.forEach(({ key }) => {
      const ratioField = ratioFieldByLane[key]
      if (!nextLaneEnabled[key]) {
        setFormValue(ratioField, '')
        return
      }
      setFormValue(
        ratioField,
        deriveLaneRatio(
          key,
          nextLanePrices[key],
          nextPromptPrice,
          nextLanePrices
        )
      )
    })
  }

  const handlePromptPriceChange = (value: string) => {
    if (!numericDraftRegex.test(value)) return
    setPromptPrice(value)
    syncLaneRatios(value, lanePrices, laneEnabled)
  }

  const handleLanePriceChange = (lane: LaneKey, value: string) => {
    if (!numericDraftRegex.test(value)) return
    const nextLanePrices = { ...lanePrices, [lane]: value }
    setLanePrices(nextLanePrices)

    if (laneEnabled[lane]) {
      setFormValue(
        ratioFieldByLane[lane],
        deriveLaneRatio(lane, value, promptPrice, nextLanePrices)
      )
    }

    if (lane === 'audioInput' && laneEnabled.audioOutput) {
      setFormValue(
        'audioCompletionRatio',
        deriveLaneRatio(
          'audioOutput',
          nextLanePrices.audioOutput,
          promptPrice,
          nextLanePrices
        )
      )
    }
  }

  const handleLaneToggle = (lane: LaneKey, checked: boolean) => {
    const nextEnabled = { ...laneEnabled, [lane]: checked }
    let nextPrices = lanePrices

    if (!checked) {
      nextPrices = { ...nextPrices, [lane]: '' }
      setFormValue(ratioFieldByLane[lane], '')
      if (lane === 'audioInput') {
        nextEnabled.audioOutput = false
        nextPrices.audioOutput = ''
        setFormValue('audioCompletionRatio', '')
      }
    }

    setLaneEnabled(nextEnabled)
    setLanePrices(nextPrices)

    if (checked) {
      setFormValue(
        ratioFieldByLane[lane],
        deriveLaneRatio(lane, nextPrices[lane], promptPrice, nextPrices)
      )
    }
  }

  const updatePriceVariantDraft = (
    updater: (current: PriceVariantDraft) => PriceVariantDraft
  ) => {
    setPriceVariantDraft((current) => ({
      ...updater(current),
      configured: true,
      inherited: false,
      restoreInherited: false,
    }))
  }

  const handlePriceVariantSwitchChange = (
    field: 'resolutionEnabled' | 'qualityEnabled',
    checked: boolean
  ) => {
    updatePriceVariantDraft((current) => ({ ...current, [field]: checked }))
  }

  const handleAddPriceVariantRule = () => {
    updatePriceVariantDraft((current) => ({
      ...current,
      rules: [
        ...current.rules,
        {
          id: createPriceVariantRuleId(),
          resolution: '',
          quality: '',
          price: '',
        },
      ],
    }))
  }

  const handleRemovePriceVariantRule = (ruleId: string) => {
    updatePriceVariantDraft((current) => ({
      ...current,
      rules: current.rules.filter((rule) => rule.id !== ruleId),
    }))
  }

  const handlePriceVariantRuleChange = (
    ruleId: string,
    field: 'resolution' | 'quality' | 'price',
    value: string
  ) => {
    if (field === 'price' && !numericDraftRegex.test(value)) return
    updatePriceVariantDraft((current) => ({
      ...current,
      rules: current.rules.map((rule) =>
        rule.id === ruleId ? { ...rule, [field]: value } : rule
      ),
    }))
  }

  const handleRestoreInheritedPriceVariants = () => {
    setPriceVariantDraft({
      configured: false,
      resolutionEnabled: false,
      qualityEnabled: false,
      rules: [],
      inherited: false,
      restoreInherited: true,
    })
    setShowPriceVariantErrors(false)
  }

  const handleModeChange = (value: string) => {
    const nextMode = value as PricingMode
    setPricingMode(nextMode)
    if (nextMode === 'per-token') {
      setPriceUnit(MODEL_PRICE_UNITS.REQUEST)
    }
    if (nextMode === 'tiered_expr' && !billingExpr) {
      setBillingExpr('tier("base", p * 0 + c * 0)')
    }
  }

  const watchedValues = form.watch()
  const activeModelName = watchedValues.name || editData?.name || ''
  const priceVariantValidation = useMemo(
    () => validatePriceVariantDraft(priceVariantDraft, activeModelName, t),
    [activeModelName, priceVariantDraft, t]
  )
  const priceVariantPreview = useMemo(
    () => buildPriceVariantPreview(priceVariantDraft, activeModelName, t),
    [activeModelName, priceVariantDraft, t]
  )
  const previewRows = useMemo(
    () =>
      buildPreviewRows(
        watchedValues,
        pricingMode,
        billingExpr,
        requestRuleExpr,
        promptPrice,
        lanePrices,
        laneEnabled,
        priceUnit,
        priceVariantPreview,
        t
      ),
    [
      billingExpr,
      laneEnabled,
      lanePrices,
      pricingMode,
      priceUnit,
      priceVariantPreview,
      promptPrice,
      requestRuleExpr,
      t,
      watchedValues,
    ]
  )

  const warnings = useMemo(() => {
    const nextWarnings: string[] = []
    const hasConflict =
      !!editData?.price &&
      [
        editData.ratio,
        editData.completionRatio,
        editData.cacheRatio,
        editData.createCacheRatio,
        editData.imageRatio,
        editData.audioRatio,
        editData.audioCompletionRatio,
      ].some(hasValue)

    if (hasConflict) {
      nextWarnings.push(
        t(
          'This model has both fixed-price and token-price settings. Saving the current mode will rewrite the conflicting fields.'
        )
      )
    }

    if (
      pricingMode === 'per-token' &&
      toNumberOrNull(promptPrice) === null &&
      laneConfigs.some(
        ({ key }) => laneEnabled[key] && hasValue(lanePrices[key])
      )
    ) {
      nextWarnings.push(
        t('Input price is required before saving dependent prices.')
      )
    }

    if (
      pricingMode === 'per-token' &&
      laneEnabled.audioOutput &&
      !hasValue(lanePrices.audioInput)
    ) {
      nextWarnings.push(t('Audio output price requires an audio input price.'))
    }

    return nextWarnings
  }, [editData, laneEnabled, lanePrices, pricingMode, promptPrice, t])

  const handleSubmit = (values: ModelPricingFormValues) => {
    if (pricingMode === 'per-request') {
      const price = values.price?.trim() ?? ''
      const parsedPrice = Number(price)
      if (!price) {
        form.setError('price', { message: t('Price is required.') })
        return
      }
      if (!Number.isFinite(parsedPrice) || parsedPrice < 0) {
        form.setError('price', {
          message: t('Enter a valid non-negative price.'),
        })
        return
      }
      form.clearErrors('price')
      if (!priceVariantValidation.valid) {
        setShowPriceVariantErrors(true)
        return
      }
    }

    if (
      pricingMode === 'per-token' &&
      toNumberOrNull(promptPrice) === null &&
      laneConfigs.some(
        ({ key }) => laneEnabled[key] && hasValue(lanePrices[key])
      )
    ) {
      form.setError('ratio', {
        message: t('Input price is required before saving dependent prices.'),
      })
      return
    }

    if (
      pricingMode === 'per-token' &&
      laneEnabled.audioOutput &&
      !hasValue(lanePrices.audioInput)
    ) {
      form.setError('audioRatio', {
        message: t('Audio output price requires an audio input price.'),
      })
      return
    }

    const data: ModelRatioData = {
      name: values.name.trim(),
      billingMode: pricingMode,
      price: values.price?.trim() || '',
      priceUnit:
        pricingMode === 'per-request' ||
        (pricingMode === 'tiered_expr' && hasValue(values.price))
          ? priceUnit
          : undefined,
      ratio: values.ratio || '',
      cacheRatio: values.cacheRatio || '',
      createCacheRatio: values.createCacheRatio || '',
      completionRatio: values.completionRatio || '',
      imageRatio: values.imageRatio || '',
      audioRatio: values.audioRatio || '',
      audioCompletionRatio: values.audioCompletionRatio || '',
      priceVariant:
        pricingMode === 'per-request'
          ? buildPriceVariantConfig(priceVariantDraft, values.name)
          : undefined,
    }

    if (pricingMode === 'tiered_expr') {
      data.billingExpr = billingExpr
      data.requestRuleExpr = requestRuleExpr
    }

    onSave(data)
    form.reset()
    onCancel?.()
  }

  const activeName = activeModelName || t('New model')

  return (
    <div
      className={cn(
        'bg-background flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border',
        className
      )}
    >
      <div className='border-b p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0'>
            <h3 className='truncate text-base font-medium'>
              {isEditMode ? t('Edit model pricing') : t('Add model pricing')}
            </h3>
            <p className='text-muted-foreground truncate text-sm'>
              {activeName}
            </p>
          </div>
          <Badge variant={getModeBadgeVariant(pricingMode)}>
            {t(getModeLabel(pricingMode))}
          </Badge>
        </div>
      </div>

      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(handleSubmit)}
          className='flex min-h-0 flex-1 flex-col'
          autoComplete='off'
        >
          <div className='min-h-0 flex-1 overflow-y-auto p-4'>
            <FieldGroup>
              {warnings.length > 0 && (
                <Alert variant='destructive'>
                  <AlertTriangle data-icon='inline-start' />
                  <AlertDescription>
                    <div className='flex flex-col gap-1'>
                      {warnings.map((warning) => (
                        <span key={warning}>{warning}</span>
                      ))}
                    </div>
                  </AlertDescription>
                </Alert>
              )}

              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('gpt-4')}
                        {...field}
                        disabled={isEditMode}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The exact model identifier as used in API requests.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <Tabs value={pricingMode} onValueChange={handleModeChange}>
                <TabsList className='grid w-full grid-cols-3'>
                  <TabsTrigger value='per-token'>{t('Per-token')}</TabsTrigger>
                  <TabsTrigger value='per-request'>
                    {t('Per-request')}
                  </TabsTrigger>
                  <TabsTrigger value='tiered_expr'>
                    {t('Expression')}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value='per-token' className='flex flex-col gap-5'>
                  <FieldGroup>
                    <Field>
                      <FieldLabel>{t('Input price')}</FieldLabel>
                      <PriceInput
                        value={promptPrice}
                        placeholder='3'
                        onChange={handlePromptPriceChange}
                      />
                      <FieldDescription>
                        {t('USD price per 1M input tokens.')}
                      </FieldDescription>
                    </Field>

                    <div className='grid gap-3 sm:grid-cols-2'>
                      {laneConfigs.map((lane) => {
                        const disabled =
                          lane.key === 'audioOutput' &&
                          (!laneEnabled.audioInput ||
                            !hasValue(lanePrices.audioInput))
                        return (
                          <PriceLane
                            key={lane.key}
                            title={t(lane.titleKey)}
                            description={t(lane.descriptionKey)}
                            placeholder={lane.placeholder}
                            value={lanePrices[lane.key]}
                            enabled={laneEnabled[lane.key]}
                            disabled={disabled}
                            onEnabledChange={(checked) =>
                              handleLaneToggle(lane.key, checked)
                            }
                            onChange={(value) =>
                              handleLanePriceChange(lane.key, value)
                            }
                          />
                        )
                      })}
                    </div>
                  </FieldGroup>
                </TabsContent>

                <TabsContent
                  value='per-request'
                  className='flex flex-col gap-5'
                >
                  <FormItem>
                    <FormLabel htmlFor='model-fixed-price-unit'>
                      {t('Price unit')}
                    </FormLabel>
                    <Select
                      items={[
                        {
                          value: MODEL_PRICE_UNITS.REQUEST,
                          label: t('Per request'),
                        },
                        {
                          value: MODEL_PRICE_UNITS.SECOND,
                          label: t('Per second'),
                        },
                      ]}
                      value={priceUnit}
                      onValueChange={(value) => {
                        if (value !== null) {
                          setPriceUnit(normalizeModelPriceUnit(value))
                        }
                      }}
                    >
                      <SelectTrigger
                        id='model-fixed-price-unit'
                        className='w-full'
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value={MODEL_PRICE_UNITS.REQUEST}>
                            {t('Per request')}
                          </SelectItem>
                          <SelectItem value={MODEL_PRICE_UNITS.SECOND}>
                            {t('Per second')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </FormItem>

                  <FormField
                    control={form.control}
                    name='price'
                    render={({ field, fieldState }) => (
                      <FormItem>
                        <FormLabel>{t('Fallback price')}</FormLabel>
                        <FormControl>
                          <InputGroup>
                            <InputGroupAddon>$</InputGroupAddon>
                            <InputGroupInput
                              inputMode='decimal'
                              placeholder='0.01'
                              aria-invalid={fieldState.invalid}
                              {...field}
                              onChange={(event) => {
                                const value = event.target.value
                                if (numericDraftRegex.test(value)) {
                                  field.onChange(value)
                                }
                              }}
                            />
                            <InputGroupAddon align='inline-end'>
                              {priceUnit === MODEL_PRICE_UNITS.SECOND
                                ? t('Per second')
                                : t('Per request')}
                            </InputGroupAddon>
                          </InputGroup>
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Used only when no specification rule matches; it is not added to a rule price.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <SpecificationPricingEditor
                    modelName={activeModelName}
                    draft={priceVariantDraft}
                    priceUnit={priceUnit}
                    validation={priceVariantValidation}
                    showErrors={showPriceVariantErrors}
                    onSwitchChange={handlePriceVariantSwitchChange}
                    onAddRule={handleAddPriceVariantRule}
                    onRemoveRule={handleRemovePriceVariantRule}
                    onRuleChange={handlePriceVariantRuleChange}
                    onRestoreInherited={handleRestoreInheritedPriceVariants}
                  />
                </TabsContent>

                <TabsContent
                  value='tiered_expr'
                  className='flex flex-col gap-5'
                >
                  <TieredPricingEditor
                    modelName={watchedValues.name}
                    billingExpr={billingExpr}
                    requestRuleExpr={requestRuleExpr}
                    onBillingExprChange={setBillingExpr}
                    onRequestRuleExprChange={setRequestRuleExpr}
                  />
                </TabsContent>
              </Tabs>

              <Collapsible open={previewOpen} onOpenChange={setPreviewOpen}>
                <CollapsibleTrigger
                  render={
                    <Button
                      type='button'
                      variant='outline'
                      className='flex w-full justify-between'
                    />
                  }
                >
                  <span>{t('Save preview')}</span>
                  <ChevronDown
                    className={cn(
                      'transition-transform',
                      previewOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent className='pt-3'>
                  <div className='rounded-lg border'>
                    {previewRows.map((row) => (
                      <div
                        key={row.key}
                        className='grid grid-cols-[140px_1fr] gap-3 border-b px-3 py-2 text-sm last:border-b-0'
                      >
                        <span className='text-muted-foreground text-xs'>
                          {row.label}
                        </span>
                        <span
                          className={cn(
                            'min-w-0',
                            row.multiline
                              ? 'font-mono text-xs leading-5 break-words whitespace-pre-wrap'
                              : 'truncate'
                          )}
                        >
                          {row.value}
                        </span>
                      </div>
                    ))}
                  </div>
                </CollapsibleContent>
              </Collapsible>
            </FieldGroup>
          </div>

          <SheetFooter
            className={sideDrawerFooterClassName(
              'grid-cols-1 sm:items-center sm:justify-between'
            )}
          >
            <div className='text-muted-foreground text-xs'>
              {selectedTargetCount > 0
                ? t('{{count}} selected targets available for bulk copy.', {
                    count: selectedTargetCount,
                  })
                : t('Changes are written to the settings draft on save.')}
            </div>
            <div className='flex justify-end gap-2'>
              <Button type='button' variant='outline' onClick={onCancel}>
                {t('Cancel')}
              </Button>
              <Button type='submit'>
                {isEditMode ? t('Update') : t('Add')}
              </Button>
            </div>
          </SheetFooter>
        </form>
      </Form>
    </div>
  )
}

function SpecificationPricingEditor(props: {
  modelName: string
  draft: PriceVariantDraft
  priceUnit: ModelPriceUnit
  validation: PriceVariantValidation
  showErrors: boolean
  onSwitchChange: (
    field: 'resolutionEnabled' | 'qualityEnabled',
    checked: boolean
  ) => void
  onAddRule: () => void
  onRemoveRule: (ruleId: string) => void
  onRuleChange: (
    ruleId: string,
    field: 'resolution' | 'quality' | 'price',
    value: string
  ) => void
  onRestoreInherited: () => void
}) {
  const { t } = useTranslation()
  const hideQuality = isGrokImagineVideoModel(props.modelName)
  const qualityEnabled = !hideQuality && props.draft.qualityEnabled
  const hasActiveDimension = props.draft.resolutionEnabled || qualityEnabled
  const canRestore =
    hideQuality &&
    !props.draft.inherited &&
    (props.draft.configured ||
      props.draft.restoreInherited ||
      props.draft.rules.length > 0)
  const unitLabel =
    props.priceUnit === MODEL_PRICE_UNITS.SECOND
      ? t('Per second')
      : t('Per request')

  return (
    <FieldSet>
      <FieldLegend>{t('Specification pricing')}</FieldLegend>
      <FieldDescription>
        {t(
          'Each matching rule sets the final price for that specification and uses the selected fixed-price unit.'
        )}
      </FieldDescription>

      {(props.draft.inherited || canRestore) && (
        <div className='flex flex-wrap items-center gap-2'>
          {props.draft.inherited && (
            <Badge variant='outline'>{t('Inherited defaults')}</Badge>
          )}
          {canRestore && (
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={props.onRestoreInherited}
            >
              {t('Restore inherited defaults')}
            </Button>
          )}
        </div>
      )}

      {props.draft.restoreInherited && (
        <Alert>
          <AlertDescription>
            {t('Inherited defaults will be restored when this draft is saved.')}
          </AlertDescription>
        </Alert>
      )}

      <FieldGroup className='gap-3'>
        <Field orientation='horizontal'>
          <FieldContent>
            <FieldLabel htmlFor='variant-resolution-enabled'>
              {t('Price by resolution')}
            </FieldLabel>
            <FieldDescription>
              {t('Match rules using the request resolution.')}
            </FieldDescription>
          </FieldContent>
          <Switch
            id='variant-resolution-enabled'
            checked={props.draft.resolutionEnabled}
            onCheckedChange={(checked) =>
              props.onSwitchChange('resolutionEnabled', checked)
            }
            aria-label={t('Price by resolution')}
          />
        </Field>

        {!hideQuality && (
          <Field orientation='horizontal'>
            <FieldContent>
              <FieldLabel htmlFor='variant-quality-enabled'>
                {t('Price by quality')}
              </FieldLabel>
              <FieldDescription>
                {t('Match rules using the request quality tier.')}
              </FieldDescription>
            </FieldContent>
            <Switch
              id='variant-quality-enabled'
              checked={props.draft.qualityEnabled}
              onCheckedChange={(checked) =>
                props.onSwitchChange('qualityEnabled', checked)
              }
              aria-label={t('Price by quality')}
            />
          </Field>
        )}
      </FieldGroup>

      {hasActiveDimension ? (
        <FieldGroup className='gap-3'>
          {props.showErrors && props.validation.sectionError && (
            <FieldError>{props.validation.sectionError}</FieldError>
          )}

          {props.draft.rules.map((rule, index) => {
            const errors = props.showErrors
              ? props.validation.ruleErrors[rule.id]
              : undefined
            const resolutionId = `${rule.id}-resolution`
            const qualityId = `${rule.id}-quality`
            const priceId = `${rule.id}-price`
            return (
              <FieldSet
                key={rule.id}
                className='rounded-lg border p-3'
                data-invalid={errors ? true : undefined}
              >
                <FieldLegend className='sr-only'>
                  {t('Specification rule {{index}}', { index: index + 1 })}
                </FieldLegend>
                <div
                  className={cn(
                    'grid gap-3',
                    props.draft.resolutionEnabled && qualityEnabled
                      ? 'sm:grid-cols-3'
                      : 'sm:grid-cols-2'
                  )}
                >
                  {props.draft.resolutionEnabled && (
                    <Field data-invalid={Boolean(errors?.resolution)}>
                      <FieldLabel htmlFor={resolutionId}>
                        {t('Resolution')}
                      </FieldLabel>
                      <Input
                        id={resolutionId}
                        value={rule.resolution}
                        placeholder='720p'
                        aria-invalid={Boolean(errors?.resolution)}
                        onChange={(event) =>
                          props.onRuleChange(
                            rule.id,
                            'resolution',
                            event.target.value
                          )
                        }
                      />
                      <FieldError>{errors?.resolution}</FieldError>
                    </Field>
                  )}

                  {qualityEnabled && (
                    <Field data-invalid={Boolean(errors?.quality)}>
                      <FieldLabel htmlFor={qualityId}>
                        {t('Quality tier')}
                      </FieldLabel>
                      <Input
                        id={qualityId}
                        value={rule.quality}
                        placeholder={t('high')}
                        aria-invalid={Boolean(errors?.quality)}
                        onChange={(event) =>
                          props.onRuleChange(
                            rule.id,
                            'quality',
                            event.target.value
                          )
                        }
                      />
                      <FieldError>{errors?.quality}</FieldError>
                    </Field>
                  )}

                  <Field data-invalid={Boolean(errors?.price)}>
                    <FieldLabel htmlFor={priceId}>
                      {t('Final price')}
                    </FieldLabel>
                    <InputGroup>
                      <InputGroupAddon>$</InputGroupAddon>
                      <InputGroupInput
                        id={priceId}
                        inputMode='decimal'
                        value={rule.price}
                        placeholder='0.01'
                        aria-invalid={Boolean(errors?.price)}
                        onChange={(event) =>
                          props.onRuleChange(
                            rule.id,
                            'price',
                            event.target.value
                          )
                        }
                      />
                      <InputGroupAddon align='inline-end'>
                        {unitLabel}
                      </InputGroupAddon>
                    </InputGroup>
                    <FieldError>{errors?.price}</FieldError>
                  </Field>
                </div>

                <div className='flex items-center justify-between gap-3'>
                  <FieldError>{errors?.duplicate}</FieldError>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    className='ml-auto'
                    onClick={() => props.onRemoveRule(rule.id)}
                    aria-label={t('Delete specification rule {{index}}', {
                      index: index + 1,
                    })}
                  >
                    {t('Delete rule')}
                  </Button>
                </div>
              </FieldSet>
            )
          })}

          <Button type='button' variant='outline' onClick={props.onAddRule}>
            {t('Add specification rule')}
          </Button>
        </FieldGroup>
      ) : (
        <FieldDescription>
          {t('Enable a dimension to add specification price rules.')}
        </FieldDescription>
      )}
    </FieldSet>
  )
}

function PriceInput(props: {
  value: string
  placeholder?: string
  disabled?: boolean
  onChange: (value: string) => void
}) {
  return (
    <InputGroup>
      <InputGroupAddon>$</InputGroupAddon>
      <InputGroupInput
        inputMode='decimal'
        value={props.value}
        placeholder={props.placeholder}
        disabled={props.disabled}
        onChange={(event) => props.onChange(event.target.value)}
      />
      <InputGroupAddon align='inline-end'>$/1M</InputGroupAddon>
    </InputGroup>
  )
}

function PriceLane(props: {
  title: string
  description: string
  placeholder: string
  value: string
  enabled: boolean
  disabled?: boolean
  onEnabledChange: (checked: boolean) => void
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const effectiveDisabled = props.disabled || !props.enabled

  return (
    <SettingsControlGroup
      className={cn('space-y-3', effectiveDisabled && 'opacity-75')}
      data-disabled={effectiveDisabled || undefined}
    >
      <SettingsSwitchField
        checked={props.enabled}
        disabled={props.disabled}
        onCheckedChange={props.onEnabledChange}
        label={props.title}
        description={props.description}
        aria-label={props.title}
      />
      <PriceInput
        value={props.value}
        placeholder={props.placeholder}
        disabled={effectiveDisabled}
        onChange={props.onChange}
      />
      <p className='text-muted-foreground text-xs'>
        {props.enabled
          ? t('USD price per 1M tokens.')
          : t('Disabled lanes are omitted on save.')}
      </p>
    </SettingsControlGroup>
  )
}
