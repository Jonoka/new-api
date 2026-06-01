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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowLeft, Coins } from 'lucide-react'
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
import { Progress } from '@/components/ui/progress'
import { SectionPageLayout } from '@/components/layout'
import {
  gameQueryKeys,
  getGamePrediction,
  getGameWallet,
  placePredictionBet,
} from './api'

function formatTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

function readPositiveInteger(value: string) {
  if (value === '') return 0
  const next = Number(value)
  if (!Number.isFinite(next)) return 0
  return Math.max(0, Math.trunc(next))
}

export function PredictionDetail({ predictionId }: { predictionId: number }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [amount, setAmount] = useState(100)

  const predictionQuery = useQuery({
    queryKey: gameQueryKeys.prediction(predictionId),
    queryFn: () => getGamePrediction(predictionId),
    enabled: predictionId > 0,
  })
  const walletQuery = useQuery({
    queryKey: gameQueryKeys.wallet,
    queryFn: getGameWallet,
  })

  const prediction = predictionQuery.data?.data
  const totalPool = prediction?.total_pool ?? 0
  const now = Math.floor(Date.now() / 1000)
  const canBet =
    prediction?.status === 'open' &&
    (!prediction.close_time || prediction.close_time > now)

  const betMutation = useMutation({
    mutationFn: (optionId: number) =>
      placePredictionBet(predictionId, optionId, amount),
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Bet placed successfully'))
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: gameQueryKeys.prediction(predictionId),
          }),
          queryClient.invalidateQueries({
            queryKey: gameQueryKeys.predictions,
          }),
          queryClient.invalidateQueries({
            queryKey: gameQueryKeys.transactions,
          }),
          queryClient.invalidateQueries({ queryKey: gameQueryKeys.wallet }),
        ])
      }
    },
  })

  const sortedOptions = useMemo(
    () => [...(prediction?.options ?? [])].sort((a, b) => a.index - b.index),
    [prediction?.options]
  )

  if (predictionId <= 0) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Prediction Game')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <Card>
            <CardContent className='text-muted-foreground py-8 text-sm'>
              {t('Select a prediction from Game Center')}
            </CardContent>
          </Card>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {prediction?.title ?? t('Prediction Game')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button variant='outline' render={<Link to='/game-center' />}>
          <ArrowLeft className='size-4' />
          {t('Back')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4 lg:grid-cols-[1.4fr_0.8fr]'>
          <Card>
            <CardHeader>
              <CardTitle className='flex flex-wrap items-center gap-2'>
                {prediction?.title ?? t('Loading')}
                {prediction && (
                  <Badge variant='outline'>{t(prediction.status)}</Badge>
                )}
              </CardTitle>
              <CardDescription>
                {prediction?.description || t('No description')}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='grid gap-3 sm:grid-cols-3'>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Total Pool')}
                  </div>
                  <div className='font-medium tabular-nums'>{totalPool}</div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Close Time')}
                  </div>
                  <div className='font-medium'>
                    {formatTime(prediction?.close_time ?? 0)}
                  </div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Settle Time')}
                  </div>
                  <div className='font-medium'>
                    {formatTime(prediction?.settle_time ?? 0)}
                  </div>
                </div>
              </div>

              <div className='grid gap-3 sm:grid-cols-2'>
                {sortedOptions.map((option) => {
                  const ratio =
                    totalPool > 0
                      ? Math.round((option.pool_amount / totalPool) * 100)
                      : 0
                  const isAnswer = prediction?.answer_option_id === option.id
                  return (
                    <Card key={option.id} size='sm'>
                      <CardHeader>
                        <CardTitle className='flex items-center justify-between gap-2'>
                          <span>{option.title}</span>
                          {isAnswer && <Badge>{t('Correct Answer')}</Badge>}
                        </CardTitle>
                        <CardDescription>
                          {t('Pool share')}: {ratio}%
                        </CardDescription>
                      </CardHeader>
                      <CardContent className='space-y-3'>
                        <Progress value={ratio} />
                        <div className='flex items-center justify-between text-sm'>
                          <span>{t('Option Pool')}</span>
                          <span className='font-medium tabular-nums'>
                            {option.pool_amount}
                          </span>
                        </div>
                        <Button
                          className='w-full'
                          disabled={!canBet || betMutation.isPending || amount <= 0}
                          onClick={() => betMutation.mutate(option.id)}
                        >
                          {t('Bet on this option')}
                        </Button>
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <Coins className='size-4' />
                {t('Bet Amount')}
              </CardTitle>
              <CardDescription>
                {t('Available Game Tokens')}:{' '}
                {walletQuery.data?.data?.balance ?? 0}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-3'>
              <Input
                type='number'
                min={1}
                value={amount}
                onChange={(event) =>
                  setAmount(readPositiveInteger(event.target.value))
                }
              />
              <div className='text-muted-foreground text-sm'>
                {t(
                  'A smaller winning side receives a larger share of the pool'
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
