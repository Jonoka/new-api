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
import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ChangeEvent } from 'react'
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Search,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatQuota, formatTimestampToDate } from '@/lib/format'
import { getPageNumbers } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  bindAdminAffiliateInviter,
  unbindAdminAffiliateInviter,
  applyAdminAffiliateRisk,
  getAdminAffiliateInvitations,
  getAdminAffiliateRecords,
  getAdminAffiliateRiskPreview,
  getAdminAffiliateRiskUsers,
  getAdminAffiliateWithdrawals,
  removeAdminAffiliateRisk,
  updateAdminAffiliateWithdrawal,
  getAdminAffiliateApplications,
  updateAdminAffiliateApplication,
  getAdminFraudAlerts,
  adminScanFraud,
  adminScanFraudDeep,
  adminResolveFraudAlert,
  adminDeleteFraudAlert,
} from '@/features/affiliate/api'
import type {
  AdminBindAffiliateInviterResult,
  AdminUnbindAffiliateInviterResult,
  AffiliateAdminInvitation,
  AffiliateAdminRecord,
  AffiliateRiskPreview,
  AffiliateRiskUserWithDetail,
  AffiliateWithdrawal,
} from '@/features/affiliate/types'
import { searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const affiliateSchema = z.object({
  affiliate_setting: z.object({
    first_level_enabled: z.boolean(),
    first_level_ratio: z.coerce.number().min(0).max(100),
    second_level_enabled: z.boolean(),
    second_level_ratio: z.coerce.number().min(0).max(100),
    settlement_delay_seconds: z.coerce.number().min(0),
    min_withdrawal_amount: z.coerce.number().min(0),
    trigger_topup_enabled: z.boolean(),
    trigger_subscription_enabled: z.boolean(),
    filter_redemption_topup_enabled: z.boolean(),
    payout_methods: z.string().min(1),
    usdt_chain: z.string().min(1),
    promotion_template: z.string().min(1),
    review_enabled: z.boolean(),
    auto_approve_after_days: z.coerce.number().min(0),
    agreement_enabled: z.boolean(),
    agreement_text: z.string(),
    inviter_min_account_age_days: z.coerce.number().min(0),
    inviter_min_recharge_amount: z.coerce.number().min(0),
    invitee_min_account_age_days: z.coerce.number().min(0),
    invitee_min_recharge_amount: z.coerce.number().min(0),
  }),
})

type AffiliateFormValues = z.infer<typeof affiliateSchema>

type Props = {
  defaultValues: AffiliateFormValues
}

type RiskTargetUser = Pick<User, 'id'> &
  Partial<Pick<User, 'username' | 'display_name' | 'email' | 'aff_code'>>

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline'

const SETTLEMENT_DELAY_KEY = 'affiliate_setting.settlement_delay_seconds'
const PAYOUT_METHODS_KEY = 'affiliate_setting.payout_methods'
const PAYOUT_METHOD_OPTIONS = [
  ['usdt', 'USDT'],
  ['alipay', 'Alipay'],
  ['wechat', 'WeChat'],
] as const
type AffiliateRiskActionState = {
  freeze_assets: boolean
  block_invite_code: boolean
  detach_invitees: boolean
  clear_assets: boolean
}
const AFFILIATE_RISK_DEFAULT_ACTIONS: AffiliateRiskActionState = {
  freeze_assets: false,
  block_invite_code: false,
  detach_invitees: false,
  clear_assets: false,
}
const ADMIN_PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
const DEFAULT_ADMIN_PAGE_SIZE = 50

type AdminPaginationProps = {
  page: number
  pageSize: number
  total: number
  loading?: boolean
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

function secondsToMinutes(value: number) {
  return Math.max(0, Math.round((Number(value) || 0) / 60))
}

function minutesToSeconds(value: number | string | boolean) {
  return Math.max(0, Math.round(Number(value) || 0)) * 60
}

function withdrawalStatusVariant(status: string): BadgeVariant {
  if (status === 'paid') return 'default'
  if (status === 'rejected' || status === 'confiscated') return 'destructive'
  if (status === 'approved') return 'secondary'
  return 'outline'
}

function riskStatusVariant(status: string): BadgeVariant {
  if (status === 'active') return 'destructive'
  if (status === 'removed') return 'secondary'
  return 'outline'
}

function withdrawalMethodLabel(method: string) {
  if (method === 'usdt') return 'USDT'
  if (method === 'alipay') return 'Alipay'
  if (method === 'wechat') return 'WeChat'
  return method
}

function sourceTypeLabel(t: (key: string) => string, sourceType: string) {
  if (sourceType === 'topup') return t('Wallet top-up')
  if (sourceType === 'subscription') return t('Subscription purchase')
  if (sourceType === 'redemption') return t('Redemption code')
  return sourceType || '-'
}

function adminUserLine(
  userId: number,
  username?: string,
  displayName?: string,
  email?: string
) {
  const name = displayName || username || '-'
  return (
    <div className='min-w-0'>
      <div className='truncate font-medium'>
        #{userId} {name}
      </div>
      <div className='text-muted-foreground truncate text-xs'>
        {email || username || '-'}
      </div>
    </div>
  )
}

function adminRecordDetailLine(record: AffiliateAdminRecord) {
  const detail = record.detail
  const title = detail?.title || `${record.source_type} #${record.source_id}`
  const paidAmount =
    detail?.paid_amount && detail.paid_amount > 0
      ? `$${detail.paid_amount.toFixed(2)}`
      : ''
  return (
    <div className='min-w-0'>
      <div className='truncate font-medium'>{title}</div>
      <div className='text-muted-foreground truncate text-xs'>
        {[record.source_id, paidAmount, detail?.payment_method]
          .filter(Boolean)
          .join(' · ') || '-'}
      </div>
    </div>
  )
}

function AdminTablePagination({
  page,
  pageSize,
  total,
  loading,
  onPageChange,
  onPageSizeChange,
}: AdminPaginationProps) {
  const { t } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)))
  const currentPage = Math.min(Math.max(1, page), totalPages)
  const pageNumbers = getPageNumbers(currentPage, totalPages)
  const start = total === 0 ? 0 : (currentPage - 1) * pageSize + 1
  const end = Math.min(currentPage * pageSize, total)

  if (total <= 0) return null

  return (
    <div className='flex flex-col gap-3 border-t pt-3 sm:flex-row sm:items-center sm:justify-between'>
      <div className='text-muted-foreground text-sm'>
        {t('Showing')} {start}-{end} {t('of')} {total}
      </div>
      <div className='flex flex-wrap items-center gap-3'>
        <div className='flex items-center gap-2'>
          <NativeSelect
            size='sm'
            value={String(pageSize)}
            onChange={(event) => {
              onPageSizeChange(Number(event.target.value))
            }}
            disabled={loading}
            className='w-[72px]'
          >
            {ADMIN_PAGE_SIZE_OPTIONS.map((size) => (
              <NativeSelectOption key={size} value={size}>
                {size}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <span className='text-muted-foreground text-sm'>
            {t('Rows per page')}
          </span>
        </div>
        <div className='flex items-center gap-1'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8 min-w-8 px-2'
            onClick={() => onPageChange(1)}
            disabled={loading || currentPage <= 1}
            aria-label={t('Go to first page')}
          >
            <ChevronsLeft className='size-4' />
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8 min-w-8 px-2'
            onClick={() => onPageChange(currentPage - 1)}
            disabled={loading || currentPage <= 1}
            aria-label={t('Go to previous page')}
          >
            <ChevronLeft className='size-4' />
          </Button>
          <div className='hidden items-center gap-1 sm:flex'>
            {pageNumbers.map((pageNumber, index) =>
              pageNumber === '...' ? (
                <span
                  key={`${pageNumber}-${index}`}
                  className='text-muted-foreground px-1 text-sm'
                >
                  ...
                </span>
              ) : (
                <Button
                  key={pageNumber}
                  type='button'
                  variant={currentPage === pageNumber ? 'default' : 'outline'}
                  size='sm'
                  className='h-8 min-w-8 px-2'
                  onClick={() => onPageChange(pageNumber as number)}
                  disabled={loading}
                >
                  {pageNumber}
                </Button>
              )
            )}
          </div>
          <div className='text-muted-foreground px-2 text-sm sm:hidden'>
            {t('Page {{current}} of {{total}}', {
              current: currentPage,
              total: totalPages,
            })}
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8 min-w-8 px-2'
            onClick={() => onPageChange(currentPage + 1)}
            disabled={loading || currentPage >= totalPages}
            aria-label={t('Go to next page')}
          >
            <ChevronRight className='size-4' />
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='h-8 min-w-8 px-2'
            onClick={() => onPageChange(totalPages)}
            disabled={loading || currentPage >= totalPages}
            aria-label={t('Go to last page')}
          >
            <ChevronsRight className='size-4' />
          </Button>
        </div>
      </div>
    </div>
  )
}

export function AffiliateSettingsSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [invitations, setInvitations] = useState<AffiliateAdminInvitation[]>([])
  const [invitationKeyword, setInvitationKeyword] = useState('')
  const [invitationSearch, setInvitationSearch] = useState('')
  const [invitationsLoading, setInvitationsLoading] = useState(false)
  const [invitationPage, setInvitationPage] = useState(1)
  const [invitationPageSize, setInvitationPageSize] = useState(
    DEFAULT_ADMIN_PAGE_SIZE
  )
  const [invitationTotal, setInvitationTotal] = useState(0)
  const [records, setRecords] = useState<AffiliateAdminRecord[]>([])
  const [recordSourceType, setRecordSourceType] = useState('topup')
  const [recordStatus, setRecordStatus] = useState('')
  const [recordKeyword, setRecordKeyword] = useState('')
  const [recordSearch, setRecordSearch] = useState('')
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [recordPage, setRecordPage] = useState(1)
  const [recordPageSize, setRecordPageSize] = useState(DEFAULT_ADMIN_PAGE_SIZE)
  const [recordTotal, setRecordTotal] = useState(0)
  const [riskUsers, setRiskUsers] = useState<AffiliateRiskUserWithDetail[]>([])
  const [riskKeyword, setRiskKeyword] = useState('')
  const [riskSearch, setRiskSearch] = useState('')
  const [riskStatus, setRiskStatus] = useState('active')
  const [riskLoading, setRiskLoading] = useState(false)
  const [riskPage, setRiskPage] = useState(1)
  const [riskPageSize, setRiskPageSize] = useState(DEFAULT_ADMIN_PAGE_SIZE)
  const [riskTotal, setRiskTotal] = useState(0)
  const [riskUserKeyword, setRiskUserKeyword] = useState('')
  const [riskUserCandidates, setRiskUserCandidates] = useState<User[]>([])
  const [selectedRiskUser, setSelectedRiskUser] =
    useState<RiskTargetUser | null>(null)
  const [riskPreview, setRiskPreview] = useState<AffiliateRiskPreview | null>(
    null
  )
  const [riskUserSearching, setRiskUserSearching] = useState(false)
  const [riskPreviewLoading, setRiskPreviewLoading] = useState(false)
  const [riskActions, setRiskActions] = useState({
    ...AFFILIATE_RISK_DEFAULT_ACTIONS,
  })
  const [riskReason, setRiskReason] = useState('')
  const [riskApplying, setRiskApplying] = useState(false)
  const [riskRestoreDetached, setRiskRestoreDetached] = useState(false)
  const [riskRemoveRemark, setRiskRemoveRemark] = useState('')
  const [riskRemovingUserId, setRiskRemovingUserId] = useState<number | null>(
    null
  )
  const [withdrawals, setWithdrawals] = useState<AffiliateWithdrawal[]>([])
  const [withdrawalStatus, setWithdrawalStatus] = useState('')
  const [withdrawalsLoading, setWithdrawalsLoading] = useState(false)
  const [withdrawalPage, setWithdrawalPage] = useState(1)
  const [withdrawalPageSize, setWithdrawalPageSize] = useState(
    DEFAULT_ADMIN_PAGE_SIZE
  )
  const [withdrawalTotal, setWithdrawalTotal] = useState(0)
  const [actionLoadingId, setActionLoadingId] = useState<number | null>(null)
  const [bindUserKeyword, setBindUserKeyword] = useState('')
  const [bindUserCandidates, setBindUserCandidates] = useState<User[]>([])
  const [selectedBindUser, setSelectedBindUser] = useState<User | null>(null)
  const [bindUserSearching, setBindUserSearching] = useState(false)
  const [bindAffCode, setBindAffCode] = useState('')
  const [bindForce, setBindForce] = useState(false)
  const [bindLoading, setBindLoading] = useState(false)
  const [bindResult, setBindResult] =
    useState<AdminBindAffiliateInviterResult | null>(null)
  const [unbindLoading, setUnbindLoading] = useState(false)
  const [unbindResult, setUnbindResult] =
    useState<AdminUnbindAffiliateInviterResult | null>(null)
  const [activeTab, setActiveTab] = useState('rules')
  // Anti-fraud: Applications state
  const [applications, setApplications] = useState<any[]>([])
  const [appStatus, setAppStatus] = useState('')
  const [appsLoading, setAppsLoading] = useState(false)
  const [appPage, setAppPage] = useState(1)
  const [appPageSize, setAppPageSize] = useState(DEFAULT_ADMIN_PAGE_SIZE)
  const [appTotal, setAppTotal] = useState(0)
  const [appActionLoadingId, setAppActionLoadingId] = useState<number | null>(
    null
  )
  // Anti-fraud: Fraud alerts state
  const [fraudAlerts, setFraudAlerts] = useState<any[]>([])
  const [fraudStatus, setFraudStatus] = useState('')
  const [fraudKeyword, setFraudKeyword] = useState('')
  const [fraudKeywordSearch, setFraudKeywordSearch] = useState('')
  const [fraudIP, setFraudIP] = useState('')
  const [fraudIPSearch, setFraudIPSearch] = useState('')
  const [fraudLoading, setFraudLoading] = useState(false)
  const [fraudPage, setFraudPage] = useState(1)
  const [fraudPageSize, setFraudPageSize] = useState(DEFAULT_ADMIN_PAGE_SIZE)
  const [fraudTotal, setFraudTotal] = useState(0)
  const [fraudScanDays, setFraudScanDays] = useState('30')
  const [fraudScanning, setFraudScanning] = useState(false)
  const [fraudActionLoadingId, setFraudActionLoadingId] = useState<
    number | null
  >(null)
  const displayDefaultValues = useMemo<AffiliateFormValues>(
    () => ({
      affiliate_setting: {
        ...defaultValues.affiliate_setting,
        settlement_delay_seconds: secondsToMinutes(
          defaultValues.affiliate_setting.settlement_delay_seconds
        ),
      },
    }),
    [defaultValues]
  )
  const handleNumberChange =
    (onChange: (value: number | string) => void) =>
    (event: ChangeEvent<HTMLInputElement>) => {
      onChange(
        event.target.value === '' ? '' : event.currentTarget.valueAsNumber
      )
    }

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<AffiliateFormValues>({
      resolver: zodResolver(affiliateSchema) as Resolver<
        AffiliateFormValues,
        unknown,
        AffiliateFormValues
      >,
      defaultValues: displayDefaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key: key === SETTLEMENT_DELAY_KEY ? SETTLEMENT_DELAY_KEY : key,
            value:
              key === SETTLEMENT_DELAY_KEY
                ? minutesToSeconds(value as string | number | boolean)
                : (value as string | number | boolean),
          })
        }
      },
    })

  const selectedPayoutMethods = (
    form.watch('affiliate_setting.payout_methods') || ''
  )
    .split(',')
    .map((method) => method.trim())
    .filter(Boolean)

  const togglePayoutMethod = (method: string, checked: boolean) => {
    if (!checked && selectedPayoutMethods.length <= 1) {
      toast.error(t('At least one payout method must remain enabled'))
      return
    }
    const next = checked
      ? [...selectedPayoutMethods, method]
      : selectedPayoutMethods.filter((item) => item !== method)
    const unique = PAYOUT_METHOD_OPTIONS.map(([value]) => value).filter(
      (value) => next.includes(value)
    )
    form.setValue(PAYOUT_METHODS_KEY, unique.join(','), {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const loadInvitations = useCallback(async () => {
    try {
      setInvitationsLoading(true)
      const res = await getAdminAffiliateInvitations(
        invitationSearch,
        invitationPage,
        invitationPageSize
      )
      if (res.success) {
        setInvitations(res.data.items || [])
        setInvitationTotal(res.data.total || 0)
      }
    } finally {
      setInvitationsLoading(false)
    }
  }, [invitationPage, invitationPageSize, invitationSearch])

  const loadRecords = useCallback(async () => {
    try {
      setRecordsLoading(true)
      const res = await getAdminAffiliateRecords(
        recordSourceType,
        recordStatus,
        recordSearch,
        recordPage,
        recordPageSize
      )
      if (res.success) {
        setRecords(res.data.items || [])
        setRecordTotal(res.data.total || 0)
      }
    } finally {
      setRecordsLoading(false)
    }
  }, [recordPage, recordPageSize, recordSearch, recordSourceType, recordStatus])

  const loadRiskUsers = useCallback(async () => {
    try {
      setRiskLoading(true)
      const res = await getAdminAffiliateRiskUsers(
        riskSearch,
        riskStatus,
        riskPage,
        riskPageSize
      )
      if (res.success) {
        setRiskUsers(res.data.items || [])
        setRiskTotal(res.data.total || 0)
      }
    } finally {
      setRiskLoading(false)
    }
  }, [riskPage, riskPageSize, riskSearch, riskStatus])

  const loadRiskPreview = useCallback(
    async (userId: number) => {
      try {
        setRiskPreviewLoading(true)
        const res = await getAdminAffiliateRiskPreview(userId)
        if (res.success) {
          setRiskPreview(res.data)
        } else {
          toast.error(res.message || t('Operation failed'))
        }
      } catch {
        toast.error(t('Operation failed'))
      } finally {
        setRiskPreviewLoading(false)
      }
    },
    [t]
  )

  const loadWithdrawals = useCallback(async () => {
    try {
      setWithdrawalsLoading(true)
      const res = await getAdminAffiliateWithdrawals(
        withdrawalStatus,
        withdrawalPage,
        withdrawalPageSize
      )
      if (res.success) {
        setWithdrawals(res.data.items || [])
        setWithdrawalTotal(res.data.total || 0)
      }
    } finally {
      setWithdrawalsLoading(false)
    }
  }, [withdrawalPage, withdrawalPageSize, withdrawalStatus])

  useEffect(() => {
    loadInvitations()
  }, [loadInvitations])

  useEffect(() => {
    loadRecords()
  }, [loadRecords])

  useEffect(() => {
    loadWithdrawals()
  }, [loadWithdrawals])

  useEffect(() => {
    loadRiskUsers()
  }, [loadRiskUsers])

  // Applications data loading
  const loadApplications = useCallback(async () => {
    try {
      setAppsLoading(true)
      const res = await getAdminAffiliateApplications(
        appStatus,
        appPage,
        appPageSize
      )
      if (res.success) {
        setApplications(res.data?.items || [])
        setAppTotal(res.data?.total || 0)
      }
    } finally {
      setAppsLoading(false)
    }
  }, [appPage, appPageSize, appStatus])

  // Fraud alerts data loading
  const loadFraudAlerts = useCallback(async () => {
    try {
      setFraudLoading(true)
      const res = await getAdminFraudAlerts({
        status: fraudStatus,
        keyword: fraudKeywordSearch,
        ip: fraudIPSearch,
        page: fraudPage,
        pageSize: fraudPageSize,
      })
      if (res.success) {
        setFraudAlerts(res.data?.items || [])
        setFraudTotal(res.data?.total || 0)
      }
    } finally {
      setFraudLoading(false)
    }
  }, [fraudIPSearch, fraudKeywordSearch, fraudPage, fraudPageSize, fraudStatus])

  useEffect(() => {
    loadApplications()
  }, [loadApplications])

  useEffect(() => {
    loadFraudAlerts()
  }, [loadFraudAlerts])

  const handleAppAction = async (
    id: number,
    action: 'approve' | 'reject' | 'revoke'
  ) => {
    const msg =
      action === 'approve'
        ? t('Approve this application?')
        : action === 'reject'
          ? t('Reject this application?')
          : t(
              'Revoke this application? The user will lose inviter access and can apply again.'
            )
    const reason =
      action === 'reject'
        ? window.prompt(t('Rejection reason (optional)'))
        : undefined
    if (!window.confirm(msg)) return
    try {
      setAppActionLoadingId(id)
      const res = await updateAdminAffiliateApplication(id, action, {
        remark: '',
        reason: reason || '',
      })
      if (res.success) {
        toast.success(t('Operation successful'))
        loadApplications()
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setAppActionLoadingId(null)
    }
  }

  const handleFraudScan = async () => {
    const days = Math.max(0, Number(fraudScanDays) || 0)
    if (
      !window.confirm(
        t(
          'Scan affiliate relationships for IP overlap in the selected time window? This may take a while.'
        )
      )
    )
      return
    try {
      setFraudScanning(true)
      const res = await adminScanFraud(days)
      if (res.success) {
        toast.success(
          t('Scan completed, {{count}} new alerts found', {
            count: res.data?.new_alerts || 0,
          })
        )
        loadFraudAlerts()
      } else {
        toast.error(res.message || t('Scan failed'))
      }
    } catch {
      toast.error(t('Scan failed'))
    } finally {
      setFraudScanning(false)
    }
  }

  const handleFraudDeepScan = async () => {
    const days = Math.max(0, Number(fraudScanDays) || 0)
    if (
      !window.confirm(
        t(
          'Scan all affiliate relationships including historical logs? This may take a while.'
        )
      )
    )
      return
    try {
      setFraudScanning(true)
      const res = await adminScanFraudDeep(days)
      if (res.success) {
        toast.success(
          t('Scan completed, {{count}} new alerts found', {
            count: res.data?.new_alerts || 0,
          })
        )
        loadFraudAlerts()
      } else {
        toast.error(res.message || t('Scan failed'))
      }
    } catch {
      toast.error(t('Scan failed'))
    } finally {
      setFraudScanning(false)
    }
  }

  const applyFraudSearch = () => {
    setFraudKeywordSearch(fraudKeyword.trim())
    setFraudIPSearch(fraudIP.trim())
    setFraudPage(1)
  }

  const resetFraudSearch = () => {
    setFraudKeyword('')
    setFraudKeywordSearch('')
    setFraudIP('')
    setFraudIPSearch('')
    setFraudPage(1)
  }

  const queryFraudInviter = (alert: any) => {
    const keyword = `#${alert.inviter_id}`
    setInvitationKeyword(keyword)
    setInvitationSearch(keyword)
    setInvitationPage(1)
    setActiveTab('invitations')
    toast.success(t('Switched to inviter invitation data'))
  }

  const handleFraudAction = async (
    id: number,
    action: 'unbind' | 'clawback' | 'dismiss' | 'delete'
  ) => {
    const msgs: Record<string, string> = {
      unbind: t('Unbind this invitation relationship?'),
      clawback: t(
        'Unbind and clawback all affiliate earnings for this relationship?'
      ),
      dismiss: t('Dismiss this alert?'),
      delete: t('Delete this alert?'),
    }
    if (!window.confirm(msgs[action])) return
    try {
      setFraudActionLoadingId(id)
      const res =
        action === 'delete'
          ? await adminDeleteFraudAlert(id)
          : await adminResolveFraudAlert(id, action)
      if (res.success) {
        toast.success(t('Operation successful'))
        loadFraudAlerts()
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setFraudActionLoadingId(null)
    }
  }

  const applyRecordSearch = () => {
    setRecordSearch(recordKeyword.trim())
    setRecordPage(1)
  }

  const searchRiskUsers = async () => {
    const keyword = riskUserKeyword.trim()
    if (!keyword) {
      toast.error(t('Enter a user keyword first'))
      return
    }
    try {
      setRiskUserSearching(true)
      setSelectedRiskUser(null)
      setRiskPreview(null)
      const res = await searchUsers({ keyword, p: 1, page_size: 10 })
      if (res.success) {
        setRiskUserCandidates(res.data?.items || [])
      }
    } finally {
      setRiskUserSearching(false)
    }
  }

  const selectRiskUser = async (user: User) => {
    setSelectedRiskUser(user)
    setRiskRestoreDetached(false)
    await loadRiskPreview(user.id)
  }

  const setRiskPreset = (
    preset: Partial<typeof AFFILIATE_RISK_DEFAULT_ACTIONS>
  ) => {
    setRiskActions({ ...AFFILIATE_RISK_DEFAULT_ACTIONS, ...preset })
  }

  const updateRiskAction = (
    key: keyof typeof AFFILIATE_RISK_DEFAULT_ACTIONS,
    checked: boolean
  ) => {
    setRiskActions((current) => ({ ...current, [key]: checked }))
  }

  const applyRiskSearch = () => {
    setRiskSearch(riskKeyword.trim())
    setRiskPage(1)
  }

  const applyRiskAction = async () => {
    if (!selectedRiskUser) {
      toast.error(t('Search and select target user first'))
      return
    }
    if (!Object.values(riskActions).some(Boolean)) {
      toast.error(t('Select at least one risk action'))
      return
    }
    if (riskActions.clear_assets && !riskReason.trim()) {
      toast.error(t('Clearing assets requires a reason'))
      return
    }
    if (
      riskActions.detach_invitees &&
      !window.confirm(
        t(
          'Detach {{count}} direct invitees from this user? This can be restored only for invitees who still have no inviter.',
          { count: riskPreview?.direct_invitee_count || 0 }
        )
      )
    ) {
      return
    }
    if (
      riskActions.clear_assets &&
      !window.confirm(
        t(
          'Clear {{quota}} affiliate assets and reject unpaid withdrawals for this user?',
          { quota: formatQuota(riskPreview?.clearable_quota || 0) }
        )
      )
    ) {
      return
    }
    try {
      setRiskApplying(true)
      const res = await applyAdminAffiliateRisk(selectedRiskUser.id, {
        ...riskActions,
        reason: riskReason.trim(),
      })
      if (res.success) {
        toast.success(t('Risk action applied'))
        setRiskActions({ ...AFFILIATE_RISK_DEFAULT_ACTIONS })
        await Promise.all([
          loadRiskUsers(),
          loadRiskPreview(selectedRiskUser.id),
        ])
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setRiskApplying(false)
    }
  }

  const removeRiskAction = async (userId: number) => {
    if (
      riskRestoreDetached &&
      !window.confirm(
        t(
          'Restore detached invitees that still have no inviter? Existing new inviter relationships will not be overwritten.'
        )
      )
    ) {
      return
    }
    if (!window.confirm(t('Remove this user from affiliate risk control?'))) {
      return
    }
    try {
      setRiskRemovingUserId(userId)
      const res = await removeAdminAffiliateRisk(userId, {
        restore_detached_invitees: riskRestoreDetached,
        remark: riskRemoveRemark.trim(),
      })
      if (res.success) {
        toast.success(
          t('Risk control removed, {{count}} invitees restored', {
            count: res.data?.restored_invitees || 0,
          })
        )
        setRiskRestoreDetached(false)
        setRiskRemoveRemark('')
        await loadRiskUsers()
        if (selectedRiskUser?.id === userId) {
          await loadRiskPreview(userId)
        }
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setRiskRemovingUserId(null)
    }
  }

  const updateWithdrawal = async (
    id: number,
    action: 'approve' | 'reject' | 'paid'
  ) => {
    if (!window.confirm(t('Confirm withdrawal action?'))) return
    try {
      setActionLoadingId(id)
      const res = await updateAdminAffiliateWithdrawal(id, action)
      if (res.success) {
        toast.success(t('Withdrawal updated'))
        await loadWithdrawals()
      }
    } finally {
      setActionLoadingId(null)
    }
  }

  const searchBindUsers = async () => {
    const keyword = bindUserKeyword.trim()
    if (!keyword) {
      toast.error(t('Enter a user keyword first'))
      return
    }
    try {
      setBindUserSearching(true)
      setSelectedBindUser(null)
      setBindResult(null)
      setUnbindResult(null)
      const res = await searchUsers({ keyword, p: 1, page_size: 10 })
      if (res.success) {
        setBindUserCandidates(res.data?.items || [])
      }
    } finally {
      setBindUserSearching(false)
    }
  }

  const bindInviter = async () => {
    const affCode = bindAffCode.trim()
    if (!selectedBindUser || !affCode) {
      toast.error(t('Search and select target user first'))
      return
    }
    try {
      setBindLoading(true)
      const res = await bindAdminAffiliateInviter({
        user_id: selectedBindUser.id,
        aff_code: affCode,
        force: bindForce,
      })
      if (res.success) {
        setBindResult(res.data)
        setUnbindResult(null)
        toast.success(t('Referral binding saved'))
        setSelectedBindUser((user) =>
          user ? { ...user, inviter_id: res.data.inviter_id } : user
        )
        setBindUserCandidates((users) =>
          users.map((user) =>
            user.id === res.data.user_id
              ? { ...user, inviter_id: res.data.inviter_id }
              : user
          )
        )
      }
    } finally {
      setBindLoading(false)
    }
  }

  const unbindInviter = async () => {
    if (!selectedBindUser) {
      toast.error(t('Search and select target user first'))
      return
    }
    if (!selectedBindUser.inviter_id) {
      toast.error(t('Selected user has no inviter'))
      return
    }
    if (!window.confirm(t('Unbind selected user from current inviter?'))) return
    try {
      setUnbindLoading(true)
      const res = await unbindAdminAffiliateInviter({
        user_id: selectedBindUser.id,
      })
      if (res.success) {
        setUnbindResult(res.data)
        setBindResult(null)
        toast.success(t('Referral binding removed'))
        setSelectedBindUser((user) =>
          user ? { ...user, inviter_id: 0 } : user
        )
        setBindUserCandidates((users) =>
          users.map((user) =>
            user.id === res.data.user_id ? { ...user, inviter_id: 0 } : user
          )
        )
      } else {
        toast.error(res.message || t('Operation failed'))
      }
    } catch {
      toast.error(t('Operation failed'))
    } finally {
      setUnbindLoading(false)
    }
  }

  return (
    <SettingsSection title={t('Affiliate Commission')}>
      <Tabs
        value={activeTab}
        onValueChange={setActiveTab}
        className='space-y-6'
      >
        <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
          <TabsTrigger value='rules'>{t('Commission rules')}</TabsTrigger>
          <TabsTrigger value='manual-bind'>{t('Manual referral')}</TabsTrigger>
          <TabsTrigger value='invitations'>{t('User Invitations')}</TabsTrigger>
          <TabsTrigger value='records'>{t('Commission Records')}</TabsTrigger>
          <TabsTrigger value='risk-users'>
            {t('Affiliate Risk Control')}
          </TabsTrigger>
          <TabsTrigger value='withdrawals'>{t('Withdrawals')}</TabsTrigger>
          <TabsTrigger value='anti-fraud'>{t('Anti-Fraud')}</TabsTrigger>
          <TabsTrigger value='applications'>{t('Applications')}</TabsTrigger>
          <TabsTrigger value='fraud-detection'>
            {t('Fraud Detection')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value='rules'>
          <FormNavigationGuard when={isDirty} />
          <Form {...form}>
            <form onSubmit={handleSubmit} className='space-y-6'>
              <FormDirtyIndicator isDirty={isDirty} />

              <div className='grid gap-4 md:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='affiliate_setting.first_level_enabled'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('Enable level 1 commission')}</FormLabel>
                        <FormDescription>
                          {t('Reward the direct inviter after a paid order.')}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='affiliate_setting.second_level_enabled'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('Enable level 2 commission')}</FormLabel>
                        <FormDescription>
                          {t('Reward the inviter above the direct inviter.')}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>

              <div className='grid gap-4 md:grid-cols-2'>
                {[
                  ['affiliate_setting.first_level_ratio', 'Level 1 ratio (%)'],
                  ['affiliate_setting.second_level_ratio', 'Level 2 ratio (%)'],
                  [
                    'affiliate_setting.settlement_delay_seconds',
                    'Settlement delay minutes',
                  ],
                  [
                    'affiliate_setting.min_withdrawal_amount',
                    'Minimum withdrawal',
                  ],
                ].map(([name, label]) => (
                  <FormField
                    key={name}
                    control={form.control}
                    // eslint-disable-next-line @typescript-eslint/no-explicit-any
                    name={name as any}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t(label)}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            value={field.value ?? ''}
                            onChange={handleNumberChange(field.onChange)}
                            name={field.name}
                            onBlur={field.onBlur}
                            ref={field.ref}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ))}
              </div>

              <div className='grid gap-4 md:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='affiliate_setting.trigger_topup_enabled'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                      <div className='space-y-0.5'>
                        <FormLabel>
                          {t('Top-up orders trigger commission')}
                        </FormLabel>
                        <FormDescription>
                          {t('Generate commission after successful top-ups.')}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='affiliate_setting.trigger_subscription_enabled'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                      <div className='space-y-0.5'>
                        <FormLabel>
                          {t('Subscription orders trigger commission')}
                        </FormLabel>
                        <FormDescription>
                          {t(
                            'Generate commission after subscription purchases.'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='affiliate_setting.filter_redemption_topup_enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel>
                        {t('Filter redemption-code top-ups')}
                      </FormLabel>
                      <FormDescription>
                        {t(
                          'When enabled, quota added by redemption codes does not generate commission.'
                        )}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='affiliate_setting.payout_methods'
                render={() => (
                  <FormItem>
                    <FormLabel>{t('Supported payout methods')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Only enabled methods are shown on the affiliate page.'
                      )}
                    </FormDescription>
                    <div className='grid gap-3 md:grid-cols-3'>
                      {PAYOUT_METHOD_OPTIONS.map(([method, label]) => (
                        <label
                          key={method}
                          className='flex items-center justify-between rounded-lg border p-4 text-sm'
                        >
                          <span>{t(label)}</span>
                          <Switch
                            checked={selectedPayoutMethods.includes(method)}
                            onCheckedChange={(checked) =>
                              togglePayoutMethod(method, checked)
                            }
                          />
                        </label>
                      ))}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='affiliate_setting.usdt_chain'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('USDT payout chain')}</FormLabel>
                    <FormControl>
                      <Input placeholder='TRC20' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('Users see this chain when saving a USDT address.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='affiliate_setting.promotion_template'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Promotion copy template')}</FormLabel>
                    <FormControl>
                      <Textarea className='min-h-28' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Use {invite_link} where the referral link should appear.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <Button
                type='submit'
                disabled={isSubmitting || updateOption.isPending}
              >
                {isSubmitting || updateOption.isPending
                  ? t('Saving...')
                  : t('Save affiliate settings')}
              </Button>
            </form>
          </Form>
        </TabsContent>

        <TabsContent value='manual-bind' className='space-y-4'>
          <div className='space-y-1'>
            <h3 className='text-base font-semibold'>
              {t('Manual referral binding')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t('Bind a user to a missed ?aff=xxxx referral code.')}
            </p>
          </div>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='space-y-2'>
              <FormLabel>{t('Target user')}</FormLabel>
              <div className='flex gap-2'>
                <Input
                  value={bindUserKeyword}
                  onChange={(event) => {
                    setBindUserKeyword(event.target.value)
                    setSelectedBindUser(null)
                  }}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.preventDefault()
                      searchBindUsers()
                    }
                  }}
                  placeholder={t('User ID, username, email, or display name')}
                />
                <Button
                  type='button'
                  variant='outline'
                  onClick={searchBindUsers}
                  disabled={bindUserSearching}
                >
                  <Search className='size-4' />
                  {bindUserSearching ? t('Searching...') : t('Search users')}
                </Button>
              </div>
            </div>
            <div className='space-y-2'>
              <FormLabel>{t('Affiliate code')}</FormLabel>
              <Input
                value={bindAffCode}
                onChange={(event) => setBindAffCode(event.target.value)}
                placeholder={t('Code or URL containing ?aff=')}
              />
            </div>
          </div>
          <div className='rounded-lg border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Display name')}</TableHead>
                  <TableHead>{t('Email')}</TableHead>
                  <TableHead>{t('Group')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {bindUserCandidates.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className='h-20 text-center'>
                      {t('No matched users')}
                    </TableCell>
                  </TableRow>
                ) : (
                  bindUserCandidates.map((user) => {
                    const selected = selectedBindUser?.id === user.id
                    return (
                      <TableRow key={user.id}>
                        <TableCell>#{user.id}</TableCell>
                        <TableCell>{user.username || '-'}</TableCell>
                        <TableCell>{user.display_name || '-'}</TableCell>
                        <TableCell>{user.email || '-'}</TableCell>
                        <TableCell>{user.group || '-'}</TableCell>
                        <TableCell className='text-right'>
                          <Button
                            type='button'
                            size='sm'
                            variant={selected ? 'default' : 'outline'}
                            onClick={() => setSelectedBindUser(user)}
                          >
                            {selected ? t('Selected') : t('Select')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    )
                  })
                )}
              </TableBody>
            </Table>
          </div>
          {selectedBindUser && (
            <div className='bg-muted/40 rounded-lg border p-3 text-sm'>
              <span className='font-medium'>{t('Selected target user')}:</span>{' '}
              #{selectedBindUser.id}{' '}
              {selectedBindUser.display_name || selectedBindUser.username}
              {selectedBindUser.email ? ` (${selectedBindUser.email})` : ''}
            </div>
          )}
          <label className='flex items-start gap-3 rounded-lg border p-4 text-sm'>
            <Checkbox
              checked={bindForce}
              onCheckedChange={(checked) => setBindForce(Boolean(checked))}
            />
            <span className='space-y-1'>
              <span className='block font-medium'>
                {t('Force overwrite existing inviter')}
              </span>
              <span className='text-muted-foreground block'>
                {t(
                  'By default existing inviters are not overwritten; force mode also adjusts old and new invite counts.'
                )}
              </span>
            </span>
          </label>
          <div className='flex flex-wrap gap-2'>
            <Button onClick={bindInviter} disabled={bindLoading}>
              {bindLoading ? t('Binding...') : t('Bind inviter')}
            </Button>
            <Button
              type='button'
              variant='outline'
              onClick={unbindInviter}
              disabled={unbindLoading || !selectedBindUser?.inviter_id}
            >
              {unbindLoading ? t('Unbinding...') : t('Unbind inviter')}
            </Button>
          </div>
          {bindResult && (
            <div className='bg-muted/40 space-y-2 rounded-lg border p-4 text-sm'>
              <div className='font-medium'>{t('Binding result')}</div>
              <div>
                {t('Target user')}: #{bindResult.user_id}{' '}
                {bindResult.display_name || bindResult.username || ''}
              </div>
              <div>
                {t('Inviter')}: #{bindResult.inviter_id}{' '}
                {bindResult.inviter_username} ({bindResult.inviter_aff_code})
              </div>
              <div className='text-muted-foreground'>
                {t('Previous inviter')}:{' '}
                {bindResult.previous_inviter_id || t('No Inviter')}
              </div>
            </div>
          )}
          {unbindResult && (
            <div className='bg-muted/40 space-y-2 rounded-lg border p-4 text-sm'>
              <div className='font-medium'>{t('Unbind result')}</div>
              <div>
                {t('Target user')}: #{unbindResult.user_id}{' '}
                {unbindResult.display_name || unbindResult.username || ''}
              </div>
              <div className='text-muted-foreground'>
                {t('Previous inviter')}:{' '}
                {unbindResult.previous_inviter_id || t('No Inviter')}
              </div>
            </div>
          )}
        </TabsContent>

        <TabsContent value='invitations' className='space-y-3'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('User Invitations')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Review inviter relationships and downstream top-ups.')}
              </p>
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Input
                value={invitationKeyword}
                onChange={(event) => setInvitationKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    setInvitationPage(1)
                    setInvitationSearch(invitationKeyword.trim())
                  }
                }}
                placeholder={t('Search inviter or invitee')}
                className='sm:w-64'
              />
              <Button
                variant='outline'
                onClick={() => {
                  setInvitationPage(1)
                  setInvitationSearch(invitationKeyword.trim())
                }}
                disabled={invitationsLoading}
              >
                <Search className='size-4' />
                {invitationsLoading ? t('Searching...') : t('Search')}
              </Button>
              <Button variant='outline' onClick={loadInvitations}>
                {invitationsLoading ? t('Refreshing...') : t('Refresh')}
              </Button>
            </div>
          </div>

          <div className='overflow-x-auto rounded-lg border'>
            <Table className='min-w-[920px]'>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Inviter')}</TableHead>
                  <TableHead>{t('Invitee')}</TableHead>
                  <TableHead>{t('Affiliate code')}</TableHead>
                  <TableHead>{t('Top-up count')}</TableHead>
                  <TableHead>{t('Top-up quota')}</TableHead>
                  <TableHead>{t('Commission')}</TableHead>
                  <TableHead>{t('Invited At')}</TableHead>
                  <TableHead>{t('Last top-up')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {invitations.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={8} className='h-24 text-center'>
                      {invitationsLoading
                        ? t('Loading...')
                        : t('No invitation records')}
                    </TableCell>
                  </TableRow>
                ) : (
                  invitations.map((item) => (
                    <TableRow key={`${item.inviter_id}-${item.invitee_id}`}>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          item.inviter_id,
                          item.inviter_username,
                          item.inviter_name,
                          item.inviter_email
                        )}
                      </TableCell>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          item.invitee_id,
                          item.invitee_username,
                          item.invitee_name,
                          item.invitee_email
                        )}
                      </TableCell>
                      <TableCell className='font-mono text-xs'>
                        {item.inviter_aff_code || '-'}
                      </TableCell>
                      <TableCell>{item.topup_count}</TableCell>
                      <TableCell>{formatQuota(item.topup_quota)}</TableCell>
                      <TableCell>
                        {formatQuota(item.commission_quota)}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(item.invitee_created_at)}
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(item.last_topup_time)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          <AdminTablePagination
            page={invitationPage}
            pageSize={invitationPageSize}
            total={invitationTotal}
            loading={invitationsLoading}
            onPageChange={setInvitationPage}
            onPageSizeChange={(pageSize) => {
              setInvitationPageSize(pageSize)
              setInvitationPage(1)
            }}
          />
        </TabsContent>

        <TabsContent value='records' className='space-y-3'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('Commission Records')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Audit downstream orders that generated commission.')}
              </p>
            </div>
            <div className='flex flex-col gap-2 sm:flex-row'>
              <Input
                value={recordKeyword}
                onChange={(event) => setRecordKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    applyRecordSearch()
                  }
                }}
                placeholder={t('Search inviter or invitee')}
                className='sm:w-64'
              />
              <NativeSelect
                value={recordSourceType}
                onChange={(event) => {
                  setRecordPage(1)
                  setRecordSourceType(event.target.value)
                }}
                className='sm:w-44'
              >
                <NativeSelectOption value=''>
                  {t('All sources')}
                </NativeSelectOption>
                <NativeSelectOption value='topup'>
                  {t('Wallet top-up')}
                </NativeSelectOption>
                <NativeSelectOption value='subscription'>
                  {t('Subscription purchase')}
                </NativeSelectOption>
                <NativeSelectOption value='redemption'>
                  {t('Redemption code')}
                </NativeSelectOption>
              </NativeSelect>
              <NativeSelect
                value={recordStatus}
                onChange={(event) => {
                  setRecordPage(1)
                  setRecordStatus(event.target.value)
                }}
                className='sm:w-36'
              >
                <NativeSelectOption value=''>
                  {t('All statuses')}
                </NativeSelectOption>
                <NativeSelectOption value='pending'>
                  {t('pending')}
                </NativeSelectOption>
                <NativeSelectOption value='available'>
                  {t('available')}
                </NativeSelectOption>
                <NativeSelectOption value='confiscated'>
                  {t('confiscated')}
                </NativeSelectOption>
              </NativeSelect>
              <Button
                type='button'
                variant='outline'
                onClick={applyRecordSearch}
                disabled={recordsLoading}
              >
                <Search className='size-4' />
                {recordsLoading ? t('Searching...') : t('Search')}
              </Button>
              <Button variant='outline' onClick={loadRecords}>
                {recordsLoading ? t('Refreshing...') : t('Refresh')}
              </Button>
            </div>
          </div>

          <div className='overflow-x-auto rounded-lg border'>
            <Table className='min-w-[1040px]'>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>{t('Inviter')}</TableHead>
                  <TableHead>{t('Invitee')}</TableHead>
                  <TableHead>{t('Source')}</TableHead>
                  <TableHead>{t('Order details')}</TableHead>
                  <TableHead>{t('Level')}</TableHead>
                  <TableHead>{t('Ratio')}</TableHead>
                  <TableHead>{t('Commission')}</TableHead>
                  <TableHead>{t('Balance after')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Created At')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={11} className='h-24 text-center'>
                      {recordsLoading
                        ? t('Loading...')
                        : t('No commission records')}
                    </TableCell>
                  </TableRow>
                ) : (
                  records.map((record) => (
                    <TableRow key={record.id}>
                      <TableCell>{record.id}</TableCell>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          record.inviter.id,
                          record.inviter.username,
                          record.inviter.display_name,
                          record.inviter.email
                        )}
                      </TableCell>
                      <TableCell className='max-w-[190px]'>
                        {adminUserLine(
                          record.invitee.id,
                          record.invitee.username,
                          record.invitee.display_name,
                          record.invitee.email
                        )}
                      </TableCell>
                      <TableCell>
                        {sourceTypeLabel(t, record.source_type)}
                      </TableCell>
                      <TableCell className='max-w-[240px]'>
                        {adminRecordDetailLine(record)}
                      </TableCell>
                      <TableCell>{record.level}</TableCell>
                      <TableCell>{record.ratio}%</TableCell>
                      <TableCell>{formatQuota(record.reward_quota)}</TableCell>
                      <TableCell>
                        {formatQuota(record.balance_after_quota || 0)}
                      </TableCell>
                      <TableCell>
                        <Badge variant={withdrawalStatusVariant(record.status)}>
                          {t(record.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        {formatTimestampToDate(record.created_at)}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
          <AdminTablePagination
            page={recordPage}
            pageSize={recordPageSize}
            total={recordTotal}
            loading={recordsLoading}
            onPageChange={setRecordPage}
            onPageSizeChange={(pageSize) => {
              setRecordPageSize(pageSize)
              setRecordPage(1)
            }}
          />
        </TabsContent>

        <TabsContent value='risk-users' className='space-y-4'>
          <div className='space-y-1'>
            <h3 className='text-base font-semibold'>
              {t('Affiliate Risk Control')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Search a user and apply selected affiliate risk controls for cheating or abnormal referral activity.'
              )}
            </p>
          </div>

          <div className='grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(380px,520px)]'>
            <div className='space-y-4'>
              <div className='grid gap-4 md:grid-cols-[minmax(0,1fr)_auto]'>
                <div className='space-y-2'>
                  <FormLabel>{t('Target user')}</FormLabel>
                  <Input
                    value={riskUserKeyword}
                    onChange={(event) => {
                      setRiskUserKeyword(event.target.value)
                      setSelectedRiskUser(null)
                      setRiskPreview(null)
                    }}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        event.preventDefault()
                        searchRiskUsers()
                      }
                    }}
                    placeholder={t('User ID, username, email, or display name')}
                  />
                </div>
                <div className='flex items-end'>
                  <Button
                    type='button'
                    variant='outline'
                    onClick={searchRiskUsers}
                    disabled={riskUserSearching}
                    className='w-full md:w-auto'
                  >
                    <Search className='size-4' />
                    {riskUserSearching ? t('Searching...') : t('Search users')}
                  </Button>
                </div>
              </div>

              <div className='overflow-x-auto rounded-lg border'>
                <Table className='min-w-[720px]'>
                  <TableHeader>
                    <TableRow>
                      <TableHead>ID</TableHead>
                      <TableHead>{t('Username')}</TableHead>
                      <TableHead>{t('Display name')}</TableHead>
                      <TableHead>{t('Email')}</TableHead>
                      <TableHead>{t('Affiliate code')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {riskUserCandidates.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={6} className='h-20 text-center'>
                          {t('No matched users')}
                        </TableCell>
                      </TableRow>
                    ) : (
                      riskUserCandidates.map((user) => {
                        const selected = selectedRiskUser?.id === user.id
                        return (
                          <TableRow key={user.id}>
                            <TableCell>#{user.id}</TableCell>
                            <TableCell>{user.username || '-'}</TableCell>
                            <TableCell>{user.display_name || '-'}</TableCell>
                            <TableCell>{user.email || '-'}</TableCell>
                            <TableCell className='font-mono text-xs'>
                              {user.aff_code || '-'}
                            </TableCell>
                            <TableCell className='text-right'>
                              <Button
                                type='button'
                                size='sm'
                                variant={selected ? 'default' : 'outline'}
                                onClick={() => selectRiskUser(user)}
                              >
                                {selected ? t('Selected') : t('Select')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        )
                      })
                    )}
                  </TableBody>
                </Table>
              </div>

              {riskPreview && (
                <div className='space-y-4 rounded-lg border p-4'>
                  <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
                    <div>
                      <div className='font-medium'>
                        #{riskPreview.user.id}{' '}
                        {riskPreview.user.display_name ||
                          riskPreview.user.username ||
                          '-'}
                      </div>
                      <div className='text-muted-foreground text-sm'>
                        {riskPreview.user.email || riskPreview.user.username}
                      </div>
                    </div>
                    {riskPreview.active_risk ? (
                      <Badge variant='destructive'>{t('active')}</Badge>
                    ) : (
                      <Badge variant='outline'>{t('No active risk')}</Badge>
                    )}
                  </div>

                  <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Available')}
                      </div>
                      <div className='font-medium'>
                        {formatQuota(riskPreview.balance.available_quota)}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Pending')}
                      </div>
                      <div className='font-medium'>
                        {formatQuota(riskPreview.balance.pending_quota)}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Withdrawal frozen')}
                      </div>
                      <div className='font-medium'>
                        {formatQuota(riskPreview.balance.frozen_quota)}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Risk frozen')}
                      </div>
                      <div className='font-medium'>
                        {formatQuota(riskPreview.balance.risk_frozen_quota)}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Confiscated')}
                      </div>
                      <div className='font-medium'>
                        {formatQuota(riskPreview.balance.confiscated_quota)}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Direct invitees')}
                      </div>
                      <div className='font-medium'>
                        {riskPreview.direct_invitee_count}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Restorable invitees')}
                      </div>
                      <div className='font-medium'>
                        {riskPreview.restorable_invitee_count}
                      </div>
                    </div>
                    <div className='bg-muted/40 rounded-lg border p-3'>
                      <div className='text-muted-foreground text-xs'>
                        {t('Clearable assets')}
                      </div>
                      <div className='font-medium'>
                        {formatQuota(riskPreview.clearable_quota)}
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>

            <div className='space-y-4 rounded-lg border p-4'>
              <div>
                <h4 className='font-medium'>{t('Risk actions')}</h4>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Choose one or more actions to apply to the selected user.'
                  )}
                </p>
              </div>

              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => setRiskPreset({ freeze_assets: true })}
                >
                  {t('Temporary freeze')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() => setRiskPreset({ block_invite_code: true })}
                >
                  {t('Disable invite code')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    setRiskPreset({
                      freeze_assets: true,
                      block_invite_code: true,
                      detach_invitees: true,
                      clear_assets: true,
                    })
                  }
                >
                  {t('Severe cheating')}
                </Button>
              </div>

              <div className='space-y-3'>
                <label className='flex items-start gap-3 rounded-lg border p-3 text-sm'>
                  <Checkbox
                    checked={riskActions.freeze_assets}
                    onCheckedChange={(checked) =>
                      updateRiskAction('freeze_assets', Boolean(checked))
                    }
                  />
                  <span>
                    <span className='block font-medium'>
                      {t('Freeze affiliate assets')}
                    </span>
                    <span className='text-muted-foreground block'>
                      {t(
                        'Move available commission into risk-frozen balance and block withdrawal or transfer.'
                      )}
                    </span>
                  </span>
                </label>
                <label className='flex items-start gap-3 rounded-lg border p-3 text-sm'>
                  <Checkbox
                    checked={riskActions.block_invite_code}
                    onCheckedChange={(checked) =>
                      updateRiskAction('block_invite_code', Boolean(checked))
                    }
                  />
                  <span>
                    <span className='block font-medium'>
                      {t('Block invite code')}
                    </span>
                    <span className='text-muted-foreground block'>
                      {t('Prevent new users from binding through this code.')}
                    </span>
                  </span>
                </label>
                <label className='flex items-start gap-3 rounded-lg border p-3 text-sm'>
                  <Checkbox
                    checked={riskActions.detach_invitees}
                    onCheckedChange={(checked) =>
                      updateRiskAction('detach_invitees', Boolean(checked))
                    }
                  />
                  <span>
                    <span className='block font-medium'>
                      {t('Detach direct invitees')}
                    </span>
                    <span className='text-muted-foreground block'>
                      {t(
                        'Remove existing direct invitee relationships and keep snapshots for optional restore.'
                      )}
                    </span>
                  </span>
                </label>
                <label className='flex items-start gap-3 rounded-lg border p-3 text-sm'>
                  <Checkbox
                    checked={riskActions.clear_assets}
                    onCheckedChange={(checked) =>
                      updateRiskAction('clear_assets', Boolean(checked))
                    }
                  />
                  <span>
                    <span className='block font-medium'>
                      {t('Clear affiliate assets')}
                    </span>
                    <span className='text-muted-foreground block'>
                      {t(
                        'Clear pending, available, withdrawal-frozen, and risk-frozen affiliate assets.'
                      )}
                    </span>
                  </span>
                </label>
              </div>

              <div className='space-y-2'>
                <FormLabel>{t('Reason')}</FormLabel>
                <Textarea
                  value={riskReason}
                  onChange={(event) => setRiskReason(event.target.value)}
                  placeholder={t('Required when clearing assets')}
                  rows={3}
                />
              </div>

              <Button
                type='button'
                onClick={applyRiskAction}
                disabled={
                  riskApplying || riskPreviewLoading || !selectedRiskUser
                }
                className='w-full'
              >
                {riskApplying ? t('Applying...') : t('Apply risk actions')}
              </Button>

              {riskPreview?.active_risk && (
                <div className='space-y-3 border-t pt-4'>
                  <div>
                    <h4 className='font-medium'>{t('Remove risk control')}</h4>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Removing risk control restores frozen commission to available balance.'
                      )}
                    </p>
                  </div>
                  <label className='flex items-start gap-3 rounded-lg border p-3 text-sm'>
                    <Checkbox
                      checked={riskRestoreDetached}
                      onCheckedChange={(checked) =>
                        setRiskRestoreDetached(Boolean(checked))
                      }
                    />
                    <span>
                      <span className='block font-medium'>
                        {t('Restore detached invitees')}
                      </span>
                      <span className='text-muted-foreground block'>
                        {t(
                          'Only invitees who still have no inviter will be restored.'
                        )}
                      </span>
                    </span>
                  </label>
                  <Textarea
                    value={riskRemoveRemark}
                    onChange={(event) =>
                      setRiskRemoveRemark(event.target.value)
                    }
                    placeholder={t('Removal remark')}
                    rows={2}
                  />
                  <Button
                    type='button'
                    variant='outline'
                    onClick={() => removeRiskAction(riskPreview.user.id)}
                    disabled={riskRemovingUserId === riskPreview.user.id}
                    className='w-full'
                  >
                    {riskRemovingUserId === riskPreview.user.id
                      ? t('Removing...')
                      : t('Remove risk control')}
                  </Button>
                </div>
              )}
            </div>
          </div>

          <div className='space-y-3'>
            <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
              <div>
                <h4 className='font-medium'>{t('Risk user list')}</h4>
                <p className='text-muted-foreground text-sm'>
                  {t('Review active and removed affiliate risk controls.')}
                </p>
              </div>
              <div className='flex flex-col gap-2 sm:flex-row'>
                <Input
                  value={riskKeyword}
                  onChange={(event) => setRiskKeyword(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      event.preventDefault()
                      applyRiskSearch()
                    }
                  }}
                  placeholder={t('Search risk user')}
                  className='sm:w-64'
                />
                <NativeSelect
                  value={riskStatus}
                  onChange={(event) => {
                    setRiskPage(1)
                    setRiskStatus(event.target.value)
                  }}
                  className='sm:w-36'
                >
                  <NativeSelectOption value='active'>
                    {t('active')}
                  </NativeSelectOption>
                  <NativeSelectOption value='removed'>
                    {t('removed')}
                  </NativeSelectOption>
                  <NativeSelectOption value=''>
                    {t('All statuses')}
                  </NativeSelectOption>
                </NativeSelect>
                <Button
                  type='button'
                  variant='outline'
                  onClick={applyRiskSearch}
                  disabled={riskLoading}
                >
                  <Search className='size-4' />
                  {riskLoading ? t('Searching...') : t('Search')}
                </Button>
                <Button variant='outline' onClick={loadRiskUsers}>
                  {riskLoading ? t('Refreshing...') : t('Refresh')}
                </Button>
              </div>
            </div>

            <div className='overflow-x-auto rounded-lg border'>
              <Table className='min-w-[1180px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('User')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead>{t('Risk actions')}</TableHead>
                    <TableHead>{t('Available')}</TableHead>
                    <TableHead>{t('Pending')}</TableHead>
                    <TableHead>{t('Risk frozen')}</TableHead>
                    <TableHead>{t('Confiscated')}</TableHead>
                    <TableHead>{t('Direct invitees')}</TableHead>
                    <TableHead>{t('Restorable invitees')}</TableHead>
                    <TableHead>{t('Reason')}</TableHead>
                    <TableHead>{t('Created At')}</TableHead>
                    <TableHead className='text-right'>{t('Actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {riskUsers.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={12} className='h-24 text-center'>
                        {riskLoading ? t('Loading...') : t('No risk users')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    riskUsers.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell className='max-w-[190px]'>
                          {adminUserLine(
                            item.user.id,
                            item.user.username,
                            item.user.display_name,
                            item.user.email
                          )}
                        </TableCell>
                        <TableCell>
                          <Badge variant={riskStatusVariant(item.status)}>
                            {t(item.status)}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className='flex flex-wrap gap-1'>
                            {item.freeze_assets && (
                              <Badge variant='outline'>
                                {t('Freeze assets')}
                              </Badge>
                            )}
                            {item.block_invite_code && (
                              <Badge variant='outline'>
                                {t('Invite code blocked')}
                              </Badge>
                            )}
                            {item.detached_invitees && (
                              <Badge variant='outline'>
                                {t('Invitees detached')}
                              </Badge>
                            )}
                            {item.cleared_quota > 0 && (
                              <Badge variant='destructive'>
                                {t('Assets cleared')}
                              </Badge>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          {formatQuota(item.balance.available_quota)}
                        </TableCell>
                        <TableCell>
                          {formatQuota(item.balance.pending_quota)}
                        </TableCell>
                        <TableCell>
                          {formatQuota(item.balance.risk_frozen_quota)}
                        </TableCell>
                        <TableCell>
                          {formatQuota(item.balance.confiscated_quota)}
                        </TableCell>
                        <TableCell>{item.direct_invitee_count}</TableCell>
                        <TableCell>{item.restorable_invitee_count}</TableCell>
                        <TableCell className='max-w-[220px] truncate'>
                          {item.reason || '-'}
                        </TableCell>
                        <TableCell>
                          {formatTimestampToDate(item.created_at)}
                        </TableCell>
                        <TableCell className='text-right'>
                          <div className='flex justify-end gap-2'>
                            <Button
                              type='button'
                              size='sm'
                              variant='outline'
                              onClick={() => {
                                setRiskUserKeyword(`#${item.user.id}`)
                                setSelectedRiskUser({
                                  id: item.user.id,
                                  username: item.user.username,
                                  display_name: item.user.display_name,
                                  email: item.user.email,
                                  aff_code: item.user.aff_code,
                                })
                                setRiskUserCandidates([])
                                loadRiskPreview(item.user.id)
                              }}
                            >
                              {t('Preview')}
                            </Button>
                            {item.status === 'active' && (
                              <Button
                                type='button'
                                size='sm'
                                variant='outline'
                                disabled={riskRemovingUserId === item.user.id}
                                onClick={() => removeRiskAction(item.user.id)}
                              >
                                {riskRemovingUserId === item.user.id
                                  ? t('Removing...')
                                  : t('Remove')}
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            <AdminTablePagination
              page={riskPage}
              pageSize={riskPageSize}
              total={riskTotal}
              loading={riskLoading}
              onPageChange={setRiskPage}
              onPageSizeChange={(pageSize) => {
                setRiskPageSize(pageSize)
                setRiskPage(1)
              }}
            />
          </div>
        </TabsContent>

        <TabsContent value='withdrawals' className='space-y-3'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('Affiliate Withdrawals')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Review withdrawals and mark offline payouts as paid.')}
              </p>
            </div>
            <div className='flex gap-2'>
              <NativeSelect
                value={withdrawalStatus}
                onChange={(event) => {
                  setWithdrawalPage(1)
                  setWithdrawalStatus(event.target.value)
                }}
                className='w-36'
              >
                <NativeSelectOption value=''>
                  {t('All statuses')}
                </NativeSelectOption>
                <NativeSelectOption value='pending'>
                  {t('pending')}
                </NativeSelectOption>
                <NativeSelectOption value='approved'>
                  {t('approved')}
                </NativeSelectOption>
                <NativeSelectOption value='paid'>
                  {t('paid')}
                </NativeSelectOption>
                <NativeSelectOption value='rejected'>
                  {t('rejected')}
                </NativeSelectOption>
              </NativeSelect>
              <Button variant='outline' onClick={loadWithdrawals}>
                {withdrawalsLoading ? t('Refreshing...') : t('Refresh')}
              </Button>
            </div>
          </div>

          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Method')}</TableHead>
                <TableHead>{t('Amount')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Created At')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {withdrawals.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className='h-24 text-center'>
                    {withdrawalsLoading
                      ? t('Loading...')
                      : t('No withdrawal records')}
                  </TableCell>
                </TableRow>
              ) : (
                withdrawals.map((withdrawal) => (
                  <TableRow key={withdrawal.id}>
                    <TableCell>{withdrawal.id}</TableCell>
                    <TableCell>{withdrawal.user_id}</TableCell>
                    <TableCell>
                      {withdrawalMethodLabel(withdrawal.method)}
                    </TableCell>
                    <TableCell>{formatQuota(withdrawal.quota)}</TableCell>
                    <TableCell>
                      <Badge
                        variant={withdrawalStatusVariant(withdrawal.status)}
                      >
                        {t(withdrawal.status)}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {formatTimestampToDate(withdrawal.created_at)}
                    </TableCell>
                    <TableCell>
                      <div className='flex justify-end gap-2'>
                        {withdrawal.status === 'pending' && (
                          <>
                            <Button
                              size='sm'
                              variant='outline'
                              disabled={actionLoadingId === withdrawal.id}
                              onClick={() =>
                                updateWithdrawal(withdrawal.id, 'approve')
                              }
                            >
                              {t('Approve')}
                            </Button>
                            <Button
                              size='sm'
                              variant='destructive'
                              disabled={actionLoadingId === withdrawal.id}
                              onClick={() =>
                                updateWithdrawal(withdrawal.id, 'reject')
                              }
                            >
                              {t('Reject')}
                            </Button>
                          </>
                        )}
                        {(withdrawal.status === 'pending' ||
                          withdrawal.status === 'approved') && (
                          <Button
                            size='sm'
                            disabled={actionLoadingId === withdrawal.id}
                            onClick={() =>
                              updateWithdrawal(withdrawal.id, 'paid')
                            }
                          >
                            {t('Mark Paid')}
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
          <AdminTablePagination
            page={withdrawalPage}
            pageSize={withdrawalPageSize}
            total={withdrawalTotal}
            loading={withdrawalsLoading}
            onPageChange={setWithdrawalPage}
            onPageSizeChange={(pageSize) => {
              setWithdrawalPageSize(pageSize)
              setWithdrawalPage(1)
            }}
          />
        </TabsContent>

        <TabsContent value='anti-fraud' className='space-y-6'>
          <Form {...form}>
            <form onSubmit={handleSubmit} className='space-y-6'>
              <div className='space-y-1'>
                <h3 className='text-base font-semibold'>
                  {t('Inviter Review')}
                </h3>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Require inviters to apply and be approved before earning commissions'
                  )}
                </p>
              </div>
              <FormField
                control={form.control}
                name='affiliate_setting.review_enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center gap-3'>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormLabel>{t('Enable affiliate review')}</FormLabel>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='affiliate_setting.auto_approve_after_days'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Auto-approve after days')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        {...field}
                        onChange={handleNumberChange(field.onChange)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Set to 0 for manual approval only')}
                    </FormDescription>
                  </FormItem>
                )}
              />

              <div className='space-y-1 border-t pt-4'>
                <h3 className='text-base font-semibold'>{t('Agreement')}</h3>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Require inviters to accept anti-fraud agreement when applying'
                  )}
                </p>
              </div>
              <FormField
                control={form.control}
                name='affiliate_setting.agreement_enabled'
                render={({ field }) => (
                  <FormItem className='flex items-center gap-3'>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormLabel>{t('Enable agreement')}</FormLabel>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='affiliate_setting.agreement_text'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Agreement text')}</FormLabel>
                    <FormControl>
                      <Textarea rows={4} {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Displayed to users when applying for affiliate program'
                      )}
                    </FormDescription>
                  </FormItem>
                )}
              />

              <div className='space-y-1 border-t pt-4'>
                <h3 className='text-base font-semibold'>
                  {t('Inviter eligibility')}
                </h3>
              </div>
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='affiliate_setting.inviter_min_account_age_days'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Min account age (days)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          {...field}
                          onChange={handleNumberChange(field.onChange)}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='affiliate_setting.inviter_min_recharge_amount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Min recharge amount (quota)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          {...field}
                          onChange={handleNumberChange(field.onChange)}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>

              <div className='space-y-1 border-t pt-4'>
                <h3 className='text-base font-semibold'>
                  {t('Invitee eligibility')}
                </h3>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Invitee must meet these conditions for commissions to be generated'
                  )}
                </p>
              </div>
              <div className='grid gap-4 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='affiliate_setting.invitee_min_account_age_days'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Min account age (days)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          {...field}
                          onChange={handleNumberChange(field.onChange)}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='affiliate_setting.invitee_min_recharge_amount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Min recharge amount (quota)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          {...field}
                          onChange={handleNumberChange(field.onChange)}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>

              <Button type='submit' disabled={isSubmitting || !isDirty}>
                <FormDirtyIndicator isDirty={isDirty} />
                {isSubmitting ? t('Saving...') : t('Save settings')}
              </Button>
            </form>
          </Form>
        </TabsContent>

        <TabsContent value='applications' className='space-y-3'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('Affiliate Applications')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t('Review inviter applications for affiliate program')}
              </p>
            </div>
            <NativeSelect
              value={appStatus}
              onChange={(e) => {
                setAppStatus(e.target.value)
                setAppPage(1)
              }}
            >
              <NativeSelectOption value=''>
                {t('All statuses')}
              </NativeSelectOption>
              <NativeSelectOption value='pending'>
                {t('Pending')}
              </NativeSelectOption>
              <NativeSelectOption value='approved'>
                {t('Approved')}
              </NativeSelectOption>
              <NativeSelectOption value='rejected'>
                {t('Rejected')}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Applied at')}</TableHead>
                <TableHead>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {appsLoading ? (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    className='text-muted-foreground py-8 text-center'
                  >
                    {t('Loading...')}
                  </TableCell>
                </TableRow>
              ) : applications.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    className='text-muted-foreground py-8 text-center'
                  >
                    {t('No applications found')}
                  </TableCell>
                </TableRow>
              ) : (
                applications.map((app) => (
                  <TableRow key={app.id}>
                    <TableCell>
                      {adminUserLine(
                        app.user_id,
                        app.username,
                        app.display_name,
                        app.email
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          app.status === 'approved'
                            ? 'default'
                            : app.status === 'rejected'
                              ? 'destructive'
                              : 'outline'
                        }
                      >
                        {t(app.status)}
                      </Badge>
                    </TableCell>
                    <TableCell className='text-sm'>
                      {formatTimestampToDate(app.created_at)}
                    </TableCell>
                    <TableCell>
                      <div className='flex flex-col gap-1'>
                        {app.status === 'pending' && (
                          <div className='flex gap-1'>
                            <Button
                              size='sm'
                              variant='default'
                              disabled={appActionLoadingId === app.id}
                              onClick={() => handleAppAction(app.id, 'approve')}
                            >
                              {t('Approve')}
                            </Button>
                            <Button
                              size='sm'
                              variant='destructive'
                              disabled={appActionLoadingId === app.id}
                              onClick={() => handleAppAction(app.id, 'reject')}
                            >
                              {t('Reject')}
                            </Button>
                          </div>
                        )}
                        {app.status !== 'pending' && (
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={appActionLoadingId === app.id}
                            onClick={() => handleAppAction(app.id, 'revoke')}
                          >
                            {app.status === 'approved'
                              ? t('Revoke')
                              : t('Reset')}
                          </Button>
                        )}
                        {app.status === 'rejected' && app.rejected_reason && (
                          <span className='text-muted-foreground text-xs'>
                            {app.rejected_reason}
                          </span>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
          <AdminTablePagination
            page={appPage}
            pageSize={appPageSize}
            total={appTotal}
            loading={appsLoading}
            onPageChange={setAppPage}
            onPageSizeChange={(ps) => {
              setAppPageSize(ps)
              setAppPage(1)
            }}
          />
        </TabsContent>

        <TabsContent value='fraud-detection' className='space-y-3'>
          <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
            <div>
              <h3 className='text-base font-semibold'>
                {t('Fraud Detection')}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Detect and manage suspicious affiliate relationships based on shared IP addresses'
                )}
              </p>
            </div>
            <div className='flex flex-wrap gap-2'>
              <Input
                value={fraudKeyword}
                onChange={(event) => setFraudKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    applyFraudSearch()
                  }
                }}
                placeholder={t('Search users')}
                className='w-full sm:w-[180px]'
              />
              <Input
                value={fraudIP}
                onChange={(event) => setFraudIP(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    applyFraudSearch()
                  }
                }}
                placeholder={t('Search shared IP')}
                className='w-full sm:w-[180px]'
              />
              <NativeSelect
                value={fraudStatus}
                onChange={(e) => {
                  setFraudStatus(e.target.value)
                  setFraudPage(1)
                }}
              >
                <NativeSelectOption value=''>
                  {t('All statuses')}
                </NativeSelectOption>
                <NativeSelectOption value='detected'>
                  {t('Detected')}
                </NativeSelectOption>
                <NativeSelectOption value='resolved'>
                  {t('Resolved')}
                </NativeSelectOption>
                <NativeSelectOption value='dismissed'>
                  {t('Dismissed')}
                </NativeSelectOption>
              </NativeSelect>
              <Button
                variant='outline'
                disabled={fraudLoading}
                onClick={applyFraudSearch}
              >
                <Search className='size-4' />
                {t('Search')}
              </Button>
              {(fraudKeywordSearch || fraudIPSearch) && (
                <Button
                  variant='ghost'
                  disabled={fraudLoading}
                  onClick={resetFraudSearch}
                >
                  {t('Reset')}
                </Button>
              )}
              <Input
                type='number'
                min={0}
                step={1}
                value={fraudScanDays}
                onChange={(event) => setFraudScanDays(event.target.value)}
                placeholder={t('Detection days')}
                className='w-full sm:w-[132px]'
              />
              <Button
                variant='outline'
                disabled={fraudScanning}
                onClick={handleFraudScan}
              >
                {fraudScanning ? t('Scanning...') : t('Scan all')}
              </Button>
              <Button
                variant='outline'
                disabled={fraudScanning}
                onClick={handleFraudDeepScan}
              >
                {fraudScanning ? t('Scanning...') : t('Deep scan')}
              </Button>
            </div>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Inviter')}</TableHead>
                <TableHead>{t('Invitee')}</TableHead>
                <TableHead>{t('Shared IPs')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {fraudLoading ? (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className='text-muted-foreground py-8 text-center'
                  >
                    {t('Loading...')}
                  </TableCell>
                </TableRow>
              ) : fraudAlerts.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className='text-muted-foreground py-8 text-center'
                  >
                    {t('No fraud alerts found')}
                  </TableCell>
                </TableRow>
              ) : (
                fraudAlerts.map((alert) => {
                  const childAlerts = Array.isArray(alert.alerts)
                    ? alert.alerts
                    : [alert]
                  const parsedIps: string[] = Array.isArray(alert.shared_ips)
                    ? alert.shared_ips
                    : (() => {
                        try {
                          return JSON.parse(alert.shared_ips || '[]')
                        } catch {
                          return []
                        }
                      })()
                  return (
                    <TableRow key={alert.inviter_id || alert.id}>
                      <TableCell>
                        <span className='font-medium'>#{alert.inviter_id}</span>
                        {alert.inviter_username && (
                          <span className='text-muted-foreground ml-1'>
                            {alert.inviter_username}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className='space-y-2'>
                          {childAlerts.map((item: any) => (
                            <div key={item.id} className='min-w-0'>
                              <span className='font-medium'>
                                #{item.invitee_id}
                              </span>
                              {item.invitee_username && (
                                <span className='text-muted-foreground ml-1'>
                                  {item.invitee_username}
                                </span>
                              )}
                              {item.invitee_email && (
                                <div className='text-muted-foreground truncate text-xs'>
                                  {item.invitee_email}
                                </div>
                              )}
                            </div>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className='flex flex-wrap gap-1'>
                          {parsedIps.slice(0, 3).map((ip) => (
                            <Badge
                              key={ip}
                              variant='outline'
                              className='text-xs'
                            >
                              {ip}
                            </Badge>
                          ))}
                          {parsedIps.length > 3 && (
                            <Badge variant='outline' className='text-xs'>
                              +{parsedIps.length - 3}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant={
                            alert.status === 'detected'
                              ? 'destructive'
                              : alert.status === 'resolved'
                                ? 'default'
                                : 'secondary'
                          }
                        >
                          {t(alert.status)}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className='space-y-2'>
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => queryFraudInviter(alert)}
                          >
                            {t('View inviter data')}
                          </Button>
                          {childAlerts.map((item: any) => (
                            <div
                              key={item.id}
                              className='flex flex-wrap items-center gap-1'
                            >
                              <span className='text-muted-foreground mr-1 text-xs'>
                                #{item.invitee_id}
                              </span>
                              {item.status === 'detected' && (
                                <>
                                  <Button
                                    size='sm'
                                    variant='outline'
                                    disabled={fraudActionLoadingId === item.id}
                                    onClick={() =>
                                      handleFraudAction(item.id, 'unbind')
                                    }
                                  >
                                    {t('Unbind')}
                                  </Button>
                                  <Button
                                    size='sm'
                                    variant='destructive'
                                    disabled={fraudActionLoadingId === item.id}
                                    onClick={() =>
                                      handleFraudAction(item.id, 'clawback')
                                    }
                                  >
                                    {t('Clawback')}
                                  </Button>
                                  <Button
                                    size='sm'
                                    variant='secondary'
                                    disabled={fraudActionLoadingId === item.id}
                                    onClick={() =>
                                      handleFraudAction(item.id, 'dismiss')
                                    }
                                  >
                                    {t('Dismiss')}
                                  </Button>
                                </>
                              )}
                              <Button
                                size='sm'
                                variant='ghost'
                                disabled={fraudActionLoadingId === item.id}
                                onClick={() =>
                                  handleFraudAction(item.id, 'delete')
                                }
                              >
                                {t('Delete')}
                              </Button>
                              {item.status !== 'detected' &&
                                item.resolved_action && (
                                  <span className='text-muted-foreground text-xs'>
                                    {t(item.resolved_action)}
                                    {item.clawback_quota > 0 &&
                                      ` (${formatQuota(item.clawback_quota)})`}
                                  </span>
                                )}
                            </div>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })
              )}
            </TableBody>
          </Table>
          <AdminTablePagination
            page={fraudPage}
            pageSize={fraudPageSize}
            total={fraudTotal}
            loading={fraudLoading}
            onPageChange={setFraudPage}
            onPageSizeChange={(ps) => {
              setFraudPageSize(ps)
              setFraudPage(1)
            }}
          />
        </TabsContent>
      </Tabs>
    </SettingsSection>
  )
}
