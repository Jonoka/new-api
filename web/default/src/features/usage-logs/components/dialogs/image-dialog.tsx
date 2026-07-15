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
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'

interface ImageDialogProps {
  imageUrl?: string
  imageUrls?: string[]
  taskId?: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

function PreviewImage({
  src,
  alt,
  errorText,
}: {
  src: string
  alt: string
  errorText: string
}) {
  const [isLoading, setIsLoading] = useState(true)
  const [hasError, setHasError] = useState(false)

  useEffect(() => {
    setIsLoading(true)
    setHasError(false)
  }, [src])

  return (
    <div className='bg-muted/50 relative flex min-h-[300px] items-center justify-center rounded-lg border'>
      {(isLoading || hasError) && (
        <Skeleton className='absolute inset-0 size-full rounded-lg' />
      )}
      <img
        src={src}
        alt={alt}
        className={cn(
          'max-h-[550px] w-full rounded-lg object-contain transition-opacity',
          isLoading || hasError ? 'opacity-0' : 'opacity-100'
        )}
        onLoad={() => {
          setIsLoading(false)
          setHasError(false)
        }}
        onError={() => {
          setIsLoading(false)
          setHasError(true)
        }}
        loading='lazy'
      />
      {hasError && (
        <div className='absolute inset-0 flex items-center justify-center'>
          <p className='text-muted-foreground text-sm'>{errorText}</p>
        </div>
      )}
    </div>
  )
}

export function ImageDialog({
  imageUrl,
  imageUrls,
  taskId,
  open,
  onOpenChange,
}: ImageDialogProps) {
  const { t } = useTranslation()
  const urls = useMemo(() => {
    const candidates = imageUrls ?? (imageUrl ? [imageUrl] : [])
    return candidates.filter((url) => typeof url === 'string' && url.trim())
  }, [imageUrl, imageUrls])
  const [resolvedUrls, setResolvedUrls] = useState<string[]>([])
  const [isResolving, setIsResolving] = useState(false)
  const [resolveFailed, setResolveFailed] = useState(false)

  useEffect(() => {
    const objectUrls: string[] = []
    const abortController = new AbortController()
    let active = true

    if (!open || urls.length === 0) {
      setResolvedUrls([])
      setIsResolving(false)
      setResolveFailed(false)
      return () => abortController.abort()
    }

    setIsResolving(true)
    setResolveFailed(false)

    const resolveImages = async () => {
      const nextUrls = await Promise.all(
        urls.map(async (url) => {
          if (!url.startsWith('/api/task/')) return url
          try {
            const response = await api.get<Blob>(url, {
              responseType: 'blob',
              disableDuplicate: true,
              skipErrorHandler: true,
              signal: abortController.signal,
            })
            if (!active) return null
            const objectUrl = URL.createObjectURL(response.data)
            objectUrls.push(objectUrl)
            return objectUrl
          } catch {
            return null
          }
        })
      )
      if (!active) return
      setResolvedUrls(nextUrls.filter((url): url is string => url !== null))
      setResolveFailed(nextUrls.some((url) => url === null))
      setIsResolving(false)
    }

    void resolveImages()
    return () => {
      active = false
      abortController.abort()
      objectUrls.forEach((url) => URL.revokeObjectURL(url))
    }
  }, [open, urls])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('Image Preview')}</DialogTitle>
          <DialogDescription>
            {taskId
              ? `${t('Task ID:')} ${taskId}`
              : t('View the generated image')}
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className='max-h-[600px]'>
          <div className='flex flex-col gap-4 py-4'>
            {isResolving && (
              <Skeleton className='min-h-[300px] w-full rounded-lg' />
            )}
            <div
              className={cn(
                'grid gap-4',
                resolvedUrls.length > 1 && 'sm:grid-cols-2'
              )}
            >
              {resolvedUrls.map((url, index) => (
                <PreviewImage
                  key={`${url}-${index}`}
                  src={url}
                  alt={t('Generated image')}
                  errorText={t('Failed to load image')}
                />
              ))}
            </div>
            {resolveFailed && resolvedUrls.length === 0 && (
              <div className='text-muted-foreground flex min-h-[300px] items-center justify-center rounded-lg border text-sm'>
                {t('Failed to load image')}
              </div>
            )}

            {/* Image URL */}
            {imageUrl && (
              <div className='bg-muted rounded-md p-3'>
                <p className='text-muted-foreground font-mono text-xs break-all'>
                  {imageUrl}
                </p>
              </div>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
