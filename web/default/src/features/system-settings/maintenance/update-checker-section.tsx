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
import { useState } from 'react'
import { DownloadIcon, ExternalLinkIcon, RefreshCcwIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { formatTimestamp, formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Markdown } from '@/components/ui/markdown'
import { Spinner } from '@/components/ui/spinner'
import { SettingsSection } from '../components/settings-section'

type ReleaseInfo = {
  tag_name: string
  name?: string
  body?: string
  html_url?: string
  published_at?: string
  asset_name?: string
  checksum_asset_name?: string
  update_available?: boolean
  self_update_supported?: boolean
  self_update_disabled_reason?: string
}

type ApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

type SelfUpdateResult = {
  target_version?: string
  restart_scheduled?: boolean
  exit_delay_seconds?: number
  message?: string
  already_up_to_date?: boolean
}

type UpdateCheckerSectionProps = {
  currentVersion?: string | null
  startTime?: number | null
}

export function UpdateCheckerSection({
  currentVersion,
  startTime,
}: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const [checking, setChecking] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [release, setRelease] = useState<ReleaseInfo | null>(null)

  const uptime = startTime ? formatTimestamp(startTime) : t('Unknown')
  const version = currentVersion || t('Unknown')

  const handleCheckUpdates = async () => {
    setChecking(true)
    try {
      const response = await api.get<ApiResponse<ReleaseInfo>>(
        '/api/status/github-latest-release',
        { disableDuplicate: true, skipBusinessError: true }
      )
      if (!response.data.success || !response.data.data) {
        throw new Error(
          response.data.message || t('Failed to check for updates')
        )
      }

      const data = response.data.data
      if (!data?.tag_name) {
        throw new Error(t('Unexpected release payload'))
      }

      if (currentVersion && data.tag_name === currentVersion) {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: data.tag_name,
          })
        )
        return
      }

      setRelease(data)
      setDialogOpen(true)
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to check for updates')
      toast.error(message)
    } finally {
      setChecking(false)
    }
  }

  const handleSelfUpdate = async () => {
    if (!release?.tag_name) return
    setUpdating(true)
    try {
      const response = await api.post<ApiResponse<SelfUpdateResult>>(
        '/api/status/self-update',
        { tag_name: release.tag_name },
        {
          skipBusinessError: true,
          skipErrorHandler: true,
          timeout: 10 * 60 * 1000,
        }
      )
      if (!response.data.success || !response.data.data) {
        throw new Error(response.data.message || t('Self update failed'))
      }

      const result = response.data.data
      toast.success(
        result.message ||
          t(
            'Update installed. The Docker container will restart automatically.'
          )
      )
      setDialogOpen(false)
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Self update failed')
      toast.error(message)
    } finally {
      setUpdating(false)
    }
  }

  const goToRelease = () => {
    if (release?.html_url) {
      window.open(release.html_url, '_blank', 'noopener,noreferrer')
    }
  }

  return (
    <>
      <SettingsSection title={t('System maintenance')}>
        <div className='space-y-6'>
          <div className='grid gap-4 md:grid-cols-2'>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Current version')}
              </div>
              <div className='text-lg font-semibold'>{version}</div>
            </div>
            <div className='rounded-lg border p-4'>
              <div className='text-muted-foreground text-sm'>
                {t('Uptime since')}
              </div>
              <div className='text-lg font-semibold'>{uptime}</div>
            </div>
          </div>

          <Button onClick={handleCheckUpdates} disabled={checking}>
            {checking ? (
              t('Checking updates...')
            ) : (
              <>
                <RefreshCcwIcon className='me-2 h-4 w-4' />
                {t('Check for updates')}
              </>
            )}
          </Button>
        </div>
      </SettingsSection>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className='max-h-[80vh] overflow-y-auto'>
          <DialogHeader>
            <DialogTitle>
              {release?.tag_name
                ? t('New version available: {{version}}', {
                    version: release.tag_name,
                  })
                : t('Release details')}
            </DialogTitle>
            {release?.published_at && (
              <DialogDescription>
                {t('Published')}{' '}
                {formatTimestampToDate(
                  new Date(release.published_at).getTime(),
                  'milliseconds'
                )}
              </DialogDescription>
            )}
          </DialogHeader>

          <div className='space-y-4'>
            <Alert>
              <AlertDescription>
                {release?.self_update_supported
                  ? t(
                      'One-click update downloads the release binary, verifies checksum, replaces the running Docker binary, and exits for Docker to restart it.'
                    )
                  : release?.self_update_disabled_reason ||
                    t('One-click update is unavailable in this environment.')}
              </AlertDescription>
            </Alert>

            {release?.body ? (
              <Markdown>{release.body}</Markdown>
            ) : (
              <p className='text-muted-foreground text-sm'>
                {t('No release notes provided.')}
              </p>
            )}
          </div>

          <DialogFooter>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setDialogOpen(false)}
            >
              {t('Close')}
            </Button>
            {release?.html_url && (
              <Button type='button' variant='secondary' onClick={goToRelease}>
                <ExternalLinkIcon className='me-2 h-4 w-4' />
                {t('Open release')}
              </Button>
            )}
            <Button
              type='button'
              onClick={handleSelfUpdate}
              disabled={
                updating ||
                !release?.tag_name ||
                !release?.self_update_supported
              }
            >
              {updating ? (
                <Spinner className='me-2 h-4 w-4' />
              ) : (
                <DownloadIcon className='me-2 h-4 w-4' />
              )}
              {updating ? t('Updating...') : t('One-click update')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
