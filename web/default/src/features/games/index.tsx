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
import { Link } from '@tanstack/react-router'
import { ArrowDownUp, Coins, Trophy } from 'lucide-react'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { SectionPageLayout } from '@/components/layout'
import {
  exchangeQuotaToToken,
  exchangeTokenToQuota,
  gameQueryKeys,
  getGamePredictions,
  getGameTransactions,
  getGameWallet,
} from './api'
import type { GamePrediction } from './types'

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

function PredictionStatusBadge({ prediction }: { prediction: GamePrediction }) {
  const { t } = useTranslation()
  const variant =
    prediction.status === 'settled'
      ? 'secondary'
      : prediction.status === 'open'
        ? 'default'
        : 'outline'
  return <Badge variant={variant}>{t(prediction.status)}</Badge>
}

export function GameCenter() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [quotaAmount, setQuotaAmount] = useState(1000)
  const [tokenAmount, setTokenAmount] = useState(1000)

  const walletQuery = useQuery({
    queryKey: gameQueryKeys.wallet,
    queryFn: getGameWallet,
  })
  const predictionsQuery = useQuery({
    queryKey: gameQueryKeys.predictions,
    queryFn: getGamePredictions,
  })
  const transactionsQuery = useQuery({
    queryKey: gameQueryKeys.transactions,
    queryFn: getGameTransactions,
  })

  const refreshWallet = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: gameQueryKeys.wallet }),
      queryClient.invalidateQueries({ queryKey: gameQueryKeys.transactions }),
    ])
  }

  const quotaToToken = useMutation({
    mutationFn: exchangeQuotaToToken,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Exchange succeeded'))
        await refreshWallet()
      }
    },
  })

  const tokenToQuota = useMutation({
    mutationFn: exchangeTokenToQuota,
    onSuccess: async (res) => {
      if (res.success) {
        toast.success(t('Exchange succeeded'))
        await refreshWallet()
      }
    },
  })

  const wallet = walletQuery.data?.data
  const predictions = predictionsQuery.data?.data?.items ?? []
  const transactions = transactionsQuery.data?.data?.items ?? []

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Game Center')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Trophy className='size-4' />
          {t('Prediction Game')}
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4 lg:grid-cols-[1fr_1.2fr]'>
          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <Coins className='size-4' />
                {t('Game Token Wallet')}
              </CardTitle>
              <CardDescription>
                {t('Game Tokens are independent from account balance')}
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('Available Game Tokens')}
                </div>
                <div className='text-3xl font-semibold tabular-nums'>
                  {walletQuery.isLoading ? '-' : (wallet?.balance ?? 0)}
                </div>
              </div>

              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-2'>
                  <label className='text-sm font-medium'>
                    {t('Balance quota to spend')}
                  </label>
                  <Input
                    type='number'
                    min={1}
                    value={quotaAmount}
                    onChange={(event) =>
                      setQuotaAmount(readPositiveInteger(event.target.value))
                    }
                  />
                  <Button
                    className='w-full'
                    disabled={quotaToToken.isPending || quotaAmount <= 0}
                    onClick={() => quotaToToken.mutate(quotaAmount)}
                  >
                    <ArrowDownUp className='size-4' />
                    {t('Exchange to Game Tokens')}
                  </Button>
                </div>
                <div className='space-y-2'>
                  <label className='text-sm font-medium'>
                    {t('Game Tokens to redeem')}
                  </label>
                  <Input
                    type='number'
                    min={1}
                    value={tokenAmount}
                    onChange={(event) =>
                      setTokenAmount(readPositiveInteger(event.target.value))
                    }
                  />
                  <Button
                    className='w-full'
                    variant='secondary'
                    disabled={tokenToQuota.isPending || tokenAmount <= 0}
                    onClick={() => tokenToQuota.mutate(tokenAmount)}
                  >
                    <ArrowDownUp className='size-4' />
                    {t('Redeem to Balance')}
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('Prediction Game')}</CardTitle>
              <CardDescription>
                {t(
                  'Choose one side before closing time and share the pool if correct'
                )}
              </CardDescription>
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
                      <TableHead className='text-right'>
                        {t('Action')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {predictions.map((prediction) => (
                      <TableRow key={prediction.id}>
                        <TableCell className='max-w-[320px] whitespace-normal'>
                          {prediction.title}
                        </TableCell>
                        <TableCell>{prediction.total_pool}</TableCell>
                        <TableCell>
                          <PredictionStatusBadge prediction={prediction} />
                        </TableCell>
                        <TableCell>
                          {formatTime(prediction.close_time)}
                        </TableCell>
                        <TableCell className='text-right'>
                          <Button
                            size='sm'
                            variant='outline'
                            render={
                              <Link
                                to='/game-center/prediction/$predictionId'
                                params={{ predictionId: String(prediction.id) }}
                              />
                            }
                          >
                            {t('Open')}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                    {predictions.length === 0 && (
                      <TableRow>
                        <TableCell
                          colSpan={5}
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

        <Card className='mt-4'>
          <CardHeader>
            <CardTitle>{t('Game Wallet Transactions')}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className='overflow-x-auto'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Time')}</TableHead>
                    <TableHead>{t('Type')}</TableHead>
                    <TableHead>{t('Token Amount')}</TableHead>
                    <TableHead>{t('Fee')}</TableHead>
                    <TableHead>{t('Balance After')}</TableHead>
                    <TableHead>{t('Content')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {transactions.slice(0, 10).map((tx) => (
                    <TableRow key={tx.id}>
                      <TableCell>{formatTime(tx.created_at)}</TableCell>
                      <TableCell>{t(tx.type)}</TableCell>
                      <TableCell>{tx.token_amount}</TableCell>
                      <TableCell>{tx.fee_amount}</TableCell>
                      <TableCell>{tx.balance_after}</TableCell>
                      <TableCell className='max-w-[360px] whitespace-normal'>
                        {tx.content}
                      </TableCell>
                    </TableRow>
                  ))}
                  {transactions.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={6} className='text-muted-foreground'>
                        {t('No wallet transactions')}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
