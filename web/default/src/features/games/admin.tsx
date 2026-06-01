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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import {
  adminCreatePrediction,
  adminGetGamePredictions,
  adminSetPredictionAnswer,
  adminSettlePrediction,
  gameQueryKeys,
} from './api'

function toUnixLocal(value: string) {
  if (!value) return 0
  const ms = new Date(value).getTime()
  if (Number.isNaN(ms)) return 0
  return Math.floor(ms / 1000)
}

function formatTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

export function GameManagement() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [optionA, setOptionA] = useState('')
  const [optionB, setOptionB] = useState('')
  const [closeTime, setCloseTime] = useState('')
  const [settleTime, setSettleTime] = useState('')
  const [judgeMode, setJudgeMode] = useState<'manual' | 'auto'>('manual')

  const predictionsQuery = useQuery({
    queryKey: gameQueryKeys.adminPredictions,
    queryFn: adminGetGamePredictions,
  })

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: gameQueryKeys.adminPredictions,
      }),
      queryClient.invalidateQueries({ queryKey: gameQueryKeys.predictions }),
    ])
  }

  const createMutation = useMutation({
    mutationFn: adminCreatePrediction,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Prediction created successfully'))
        setTitle('')
        setDescription('')
        setOptionA('')
        setOptionB('')
        setCloseTime('')
        setSettleTime('')
        setJudgeMode('manual')
        await refresh()
      }
    },
  })

  const answerMutation = useMutation({
    mutationFn: ({
      predictionId,
      answerIndex,
    }: {
      predictionId: number
      answerIndex: number
    }) => adminSetPredictionAnswer(predictionId, answerIndex),
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Answer saved successfully'))
        await refresh()
      }
    },
  })

  const settleMutation = useMutation({
    mutationFn: adminSettlePrediction,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Prediction settled successfully'))
        await refresh()
      }
    },
  })

  const predictions = predictionsQuery.data?.data?.items ?? []

  const handleCreate = () => {
    const trimmedTitle = title.trim()
    const trimmedDescription = description.trim()
    const trimmedOptionA = optionA.trim()
    const trimmedOptionB = optionB.trim()
    const closeUnix = toUnixLocal(closeTime)
    const settleUnix = toUnixLocal(settleTime)

    if (!trimmedTitle || !trimmedOptionA || !trimmedOptionB || !closeUnix) {
      toast.error(t('Please complete required fields'))
      return
    }
    if (closeUnix <= Math.floor(Date.now() / 1000)) {
      toast.error(t('Close time must be in the future'))
      return
    }
    if (settleUnix > 0 && settleUnix < closeUnix) {
      toast.error(t('Settle time cannot be earlier than close time'))
      return
    }

    createMutation.mutate({
      title: trimmedTitle,
      description: trimmedDescription,
      options: [trimmedOptionA, trimmedOptionB],
      close_time: closeUnix,
      settle_time: settleUnix,
      judge_mode: judgeMode,
    })
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Game Management')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          disabled={predictionsQuery.isFetching}
          onClick={() => refresh()}
        >
          <RefreshCw className='size-4' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4 lg:grid-cols-[0.8fr_1.4fr]'>
          <Card>
            <CardHeader>
              <CardTitle>{t('Create Prediction')}</CardTitle>
              <CardDescription>
                {t(
                  'Create a two-option prediction for users to bet Game Tokens'
                )}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-3'>
              <Input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
                placeholder={t('Question')}
              />
              <Textarea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder={t('Description')}
              />
              <div className='grid gap-3 sm:grid-cols-2'>
                <Input
                  value={optionA}
                  onChange={(event) => setOptionA(event.target.value)}
                  placeholder={t('Option 1')}
                />
                <Input
                  value={optionB}
                  onChange={(event) => setOptionB(event.target.value)}
                  placeholder={t('Option 2')}
                />
              </div>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-1'>
                  <label className='text-sm font-medium'>
                    {t('Close Time')}
                  </label>
                  <Input
                    type='datetime-local'
                    value={closeTime}
                    onChange={(event) => setCloseTime(event.target.value)}
                  />
                </div>
                <div className='space-y-1'>
                  <label className='text-sm font-medium'>
                    {t('Settle Time')}
                  </label>
                  <Input
                    type='datetime-local'
                    value={settleTime}
                    onChange={(event) => setSettleTime(event.target.value)}
                  />
                </div>
              </div>
              <div className='space-y-1'>
                <label className='text-sm font-medium'>{t('Judge mode')}</label>
                <Select
                  items={[
                    { value: 'manual', label: t('Manual judge') },
                    { value: 'auto', label: t('Automatic judge') },
                  ]}
                  value={judgeMode}
                  onValueChange={(value) =>
                    setJudgeMode(value as 'manual' | 'auto')
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='manual'>
                        {t('Manual judge')}
                      </SelectItem>
                      <SelectItem value='auto'>
                        {t('Automatic judge')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <Button
                className='w-full'
                disabled={
                  createMutation.isPending ||
                  !title ||
                  !optionA ||
                  !optionB ||
                  !closeTime
                }
                onClick={handleCreate}
              >
                <Plus className='size-4' />
                {t('Create Prediction')}
              </Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Prediction Rounds')}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className='overflow-x-auto'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Question')}</TableHead>
                      <TableHead>{t('Pool')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Close Time')}</TableHead>
                      <TableHead>{t('Answer')}</TableHead>
                      <TableHead className='text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {predictions.map((prediction) => (
                      <TableRow key={prediction.id}>
                        <TableCell className='max-w-[260px] whitespace-normal'>
                          {prediction.title}
                        </TableCell>
                        <TableCell>{prediction.total_pool}</TableCell>
                        <TableCell>
                          <Badge variant='outline'>
                            {t(prediction.status)}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          {formatTime(prediction.close_time)}
                        </TableCell>
                        <TableCell>
                          {prediction.answer_option_id
                            ? (prediction.options.find(
                                (item) =>
                                  item.id === prediction.answer_option_id
                              )?.title ?? '-')
                            : '-'}
                        </TableCell>
                        <TableCell>
                          <div className='flex min-w-max justify-end gap-2'>
                            <Button
                              size='sm'
                              variant='outline'
                              disabled={answerMutation.isPending}
                              onClick={() =>
                                answerMutation.mutate({
                                  predictionId: prediction.id,
                                  answerIndex: 1,
                                })
                              }
                            >
                              {t('Answer 1')}
                            </Button>
                            <Button
                              size='sm'
                              variant='outline'
                              disabled={answerMutation.isPending}
                              onClick={() =>
                                answerMutation.mutate({
                                  predictionId: prediction.id,
                                  answerIndex: 2,
                                })
                              }
                            >
                              {t('Answer 2')}
                            </Button>
                            <Button
                              size='sm'
                              disabled={
                                settleMutation.isPending ||
                                prediction.status === 'settled' ||
                                !prediction.answer_option_id
                              }
                              onClick={() =>
                                settleMutation.mutate(prediction.id)
                              }
                            >
                              {t('Settle')}
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                    {predictions.length === 0 && (
                      <TableRow>
                        <TableCell
                          colSpan={6}
                          className='text-muted-foreground'
                        >
                          {t('No predictions available')}
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
