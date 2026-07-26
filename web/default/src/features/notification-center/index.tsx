/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BellRing,
  Bot,
  CheckCircle2,
  Clock3,
  Loader2,
  Pencil,
  Plus,
  PowerOff,
  RefreshCw,
  Send,
  XCircle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import { TagInput } from '@/components/tag-input'
import {
  createNotificationBot,
  createNotificationTask,
  deleteNotificationBot,
  deleteNotificationTask,
  getNotificationBots,
  getNotificationDeliveries,
  getNotificationEventTypes,
  getNotificationTasks,
  testNotificationBot,
  updateNotificationBot,
  updateNotificationTask,
} from './api'
import type {
  NotificationBot,
  NotificationBotInput,
  NotificationDelivery,
  NotificationEventType,
  NotificationTarget,
  NotificationTask,
  NotificationTaskInput,
} from './types'

const FALLBACK_EVENT: NotificationEventType = {
  value: 'invoice_pending',
  label: 'New pending invoice order',
  description: 'Triggered when a new invoice request is ready to issue.',
  variables: [
    'mention',
    'invoice_id',
    'source_type',
    'source_id',
    'user_id',
    'title',
    'total_amount',
    'create_time',
  ],
  default_template:
    '{{mention}} 来新的发票订单啦~\n订单：{{invoice_id}}\n金额：{{total_amount}}',
}

const EMPTY_BOT_FORM: NotificationBotInput = {
  name: '',
  token: '',
  enabled: true,
}

const EMPTY_TARGET: NotificationTarget = {
  chat_id: '',
  mention_user_id: '',
  mention_name: '',
}

function formatTime(value?: number) {
  return value ? formatTimestampToDate(value) : '-'
}

function getEventLabel(
  event: NotificationEventType,
  t: (key: string) => string
) {
  if (event.value === FALLBACK_EVENT.value) return t(FALLBACK_EVENT.label)
  return t(event.label)
}

function LoadingList() {
  return (
    <div className='space-y-3'>
      {Array.from({ length: 3 }).map((_, index) => (
        <Skeleton key={index} className='h-28 w-full' />
      ))}
    </div>
  )
}

function EmptyState(props: { icon: ReactNode; title: string; text: string }) {
  return (
    <Card>
      <CardContent className='flex min-h-56 flex-col items-center justify-center text-center'>
        {props.icon}
        <p className='mt-3 text-sm font-medium'>{props.title}</p>
        <p className='text-muted-foreground mt-1 max-w-md text-xs'>
          {props.text}
        </p>
      </CardContent>
    </Card>
  )
}

function ErrorState(props: { message: string; onRetry: () => void }) {
  const { t } = useTranslation()
  return (
    <Alert variant='destructive'>
      <XCircle className='h-4 w-4' />
      <AlertDescription className='flex items-center justify-between gap-3'>
        <span>{props.message}</span>
        <Button variant='outline' size='sm' onClick={props.onRetry}>
          <RefreshCw className='mr-2 h-4 w-4' />
          {t('Retry')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}

function BotSheet(props: {
  open: boolean
  bot: NotificationBot | null
  saving: boolean
  onOpenChange: (open: boolean) => void
  onSave: (input: NotificationBotInput) => void
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState<NotificationBotInput>(EMPTY_BOT_FORM)

  useEffect(() => {
    setForm(
      props.bot
        ? { name: props.bot.name, token: '', enabled: props.bot.enabled }
        : EMPTY_BOT_FORM
    )
  }, [props.bot, props.open])

  const submit = () => {
    if (!form.name.trim()) {
      toast.error(t('Please enter a bot name'))
      return
    }
    if (!props.bot && !form.token?.trim()) {
      toast.error(t('Please enter a bot token'))
      return
    }
    props.onSave({
      name: form.name.trim(),
      enabled: form.enabled,
      ...(form.token?.trim() ? { token: form.token.trim() } : {}),
    })
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='flex w-full flex-col sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle>
            {props.bot ? t('Edit Telegram Bot') : t('Add Telegram Bot')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'Bot tokens are stored only on the server and never displayed after saving.'
            )}
          </SheetDescription>
        </SheetHeader>
        <div className='flex-1 space-y-5 overflow-y-auto px-4 py-2'>
          <div className='grid gap-2'>
            <Label htmlFor='notification-bot-name'>{t('Bot name')}</Label>
            <Input
              id='notification-bot-name'
              value={form.name}
              onChange={(event) =>
                setForm((current) => ({ ...current, name: event.target.value }))
              }
              placeholder={t('Invoice notification bot')}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='notification-bot-token'>{t('Bot token')}</Label>
            <Input
              id='notification-bot-token'
              type='password'
              autoComplete='new-password'
              value={form.token ?? ''}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  token: event.target.value,
                }))
              }
              placeholder={
                props.bot
                  ? t('Leave blank to keep the existing token')
                  : t('Enter the token issued by BotFather')
              }
            />
          </div>
          <div className='flex items-center justify-between gap-4 rounded-md border p-3'>
            <div>
              <Label htmlFor='notification-bot-enabled'>{t('Enabled')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('Disabled bots cannot deliver notification tasks.')}
              </p>
            </div>
            <Switch
              id='notification-bot-enabled'
              checked={form.enabled}
              onCheckedChange={(enabled) =>
                setForm((current) => ({ ...current, enabled }))
              }
            />
          </div>
        </div>
        <SheetFooter>
          <SheetClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </SheetClose>
          <Button onClick={submit} disabled={props.saving}>
            {props.saving && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Save')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function TargetEditor(props: {
  targets: NotificationTarget[]
  onChange: (targets: NotificationTarget[]) => void
}) {
  const { t } = useTranslation()
  const chatIds = props.targets.map((target) => target.chat_id).filter(Boolean)

  const updateChatIds = (ids: string[]) => {
    props.onChange(
      ids.map(
        (chatId) =>
          props.targets.find((target) => target.chat_id === chatId) ?? {
            ...EMPTY_TARGET,
            chat_id: chatId,
          }
      )
    )
  }

  const updateTarget = (
    chatId: string,
    field: 'mention_user_id' | 'mention_name',
    value: string
  ) => {
    props.onChange(
      props.targets.map((target) =>
        target.chat_id === chatId ? { ...target, [field]: value } : target
      )
    )
  }

  return (
    <div className='grid gap-3'>
      <div className='grid gap-2'>
        <Label>{t('Recipient Chat IDs')}</Label>
        <TagInput
          value={chatIds}
          onChange={updateChatIds}
          placeholder={t('Enter a Chat ID and press Enter')}
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Personal chats require the user to start the bot first. Group and channel IDs may be negative.'
          )}
        </p>
      </div>
      {props.targets.map((target) => (
        <div
          key={target.chat_id}
          className='grid gap-3 rounded-md border p-3 sm:grid-cols-2'
        >
          <div className='sm:col-span-2'>
            <Badge variant='outline'>{target.chat_id}</Badge>
          </div>
          <div className='grid gap-2'>
            <Label>{t('Mention user ID')}</Label>
            <Input
              value={target.mention_user_id ?? ''}
              onChange={(event) =>
                updateTarget(
                  target.chat_id,
                  'mention_user_id',
                  event.target.value
                )
              }
              placeholder={t('Optional Telegram user ID')}
            />
          </div>
          <div className='grid gap-2'>
            <Label>{t('Mention name')}</Label>
            <Input
              value={target.mention_name ?? ''}
              onChange={(event) =>
                updateTarget(target.chat_id, 'mention_name', event.target.value)
              }
              placeholder={t('Optional display name')}
            />
          </div>
        </div>
      ))}
    </div>
  )
}

function TaskSheet(props: {
  open: boolean
  task: NotificationTask | null
  bots: NotificationBot[]
  events: NotificationEventType[]
  saving: boolean
  onOpenChange: (open: boolean) => void
  onSave: (input: NotificationTaskInput) => void
}) {
  const { t } = useTranslation()
  const eventOptions = props.events.length > 0 ? props.events : [FALLBACK_EVENT]
  const [form, setForm] = useState<NotificationTaskInput>({
    name: '',
    event_type: FALLBACK_EVENT.value,
    bot_id: 0,
    targets: [],
    template: FALLBACK_EVENT.default_template ?? '',
    enabled: true,
  })

  useEffect(() => {
    const defaultEvent = eventOptions[0] ?? FALLBACK_EVENT
    setForm(
      props.task
        ? {
            name: props.task.name,
            event_type: props.task.event_type,
            bot_id: props.task.bot_id,
            targets: props.task.targets ?? [],
            template: props.task.template,
            enabled: props.task.enabled,
          }
        : {
            name: '',
            event_type: defaultEvent.value,
            bot_id: props.bots[0]?.id ?? 0,
            targets: [],
            template: defaultEvent.default_template ?? '',
            enabled: true,
          }
    )
  }, [eventOptions, props.bots, props.open, props.task])

  const selectedEvent =
    eventOptions.find((event) => event.value === form.event_type) ??
    FALLBACK_EVENT

  const submit = () => {
    if (!form.name.trim() || !form.event_type || !form.bot_id) {
      toast.error(t('Please complete the required task fields'))
      return
    }
    if (
      form.targets.length === 0 ||
      form.targets.some((target) => !target.chat_id.trim())
    ) {
      toast.error(t('Please add at least one recipient Chat ID'))
      return
    }
    if (!form.template.trim()) {
      toast.error(t('Please enter a notification template'))
      return
    }
    props.onSave({
      ...form,
      name: form.name.trim(),
      template: form.template.trim(),
      targets: form.targets.map((target) => ({
        ...(target.id ? { id: target.id } : {}),
        chat_id: target.chat_id.trim(),
        ...(target.mention_user_id?.trim()
          ? { mention_user_id: target.mention_user_id.trim() }
          : {}),
        ...(target.mention_name?.trim()
          ? { mention_name: target.mention_name.trim() }
          : {}),
        enabled: target.enabled !== false,
      })),
    })
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className='flex w-full flex-col sm:max-w-2xl'>
        <SheetHeader>
          <SheetTitle>
            {props.task
              ? t('Edit notification task')
              : t('Create notification task')}
          </SheetTitle>
          <SheetDescription>
            {t(
              'Choose an event, Telegram Bot, recipients, and a custom message template.'
            )}
          </SheetDescription>
        </SheetHeader>
        <div className='flex-1 space-y-5 overflow-y-auto px-4 py-2'>
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='grid gap-2'>
              <Label htmlFor='notification-task-name'>{t('Task name')}</Label>
              <Input
                id='notification-task-name'
                value={form.name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
                placeholder={t('Pending invoice reminder')}
              />
            </div>
            <div className='grid gap-2'>
              <Label>{t('Telegram Bot')}</Label>
              <Select
                items={props.bots.map((bot) => ({
                  value: String(bot.id),
                  label: bot.name,
                }))}
                value={form.bot_id ? String(form.bot_id) : null}
                onValueChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    bot_id: Number(value ?? 0),
                  }))
                }
              >
                <SelectTrigger className='h-9 w-full'>
                  <SelectValue placeholder={t('Select a bot')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {props.bots.map((bot) => (
                      <SelectItem key={bot.id} value={String(bot.id)}>
                        {bot.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className='grid gap-2'>
            <Label>{t('Event type')}</Label>
            <Select
              items={eventOptions.map((event) => ({
                value: event.value,
                label: getEventLabel(event, t),
              }))}
              value={form.event_type}
              onValueChange={(value) => {
                if (!value) return
                const nextEvent = eventOptions.find(
                  (event) => event.value === value
                )
                setForm((current) => ({
                  ...current,
                  event_type: value,
                  template:
                    current.template ||
                    nextEvent?.default_template ||
                    current.template,
                }))
              }}
            >
              <SelectTrigger className='h-9 w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {eventOptions.map((event) => (
                    <SelectItem key={event.value} value={event.value}>
                      {getEventLabel(event, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            {selectedEvent.description && (
              <p className='text-muted-foreground text-xs'>
                {t(selectedEvent.description)}
              </p>
            )}
          </div>
          <TargetEditor
            targets={form.targets}
            onChange={(targets) =>
              setForm((current) => ({ ...current, targets }))
            }
          />
          <div className='grid gap-2'>
            <Label htmlFor='notification-template'>
              {t('Message template')}
            </Label>
            <Textarea
              id='notification-template'
              className='min-h-40 font-mono'
              value={form.template}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  template: event.target.value,
                }))
              }
            />
            <div className='flex flex-wrap gap-1.5'>
              {(selectedEvent.variables ?? []).map((variable) => (
                <Badge key={variable} variant='secondary'>
                  {'{{'}
                  {variable}
                  {'}}'}
                </Badge>
              ))}
            </div>
          </div>
          <div className='flex items-center justify-between gap-4 rounded-md border p-3'>
            <div>
              <Label htmlFor='notification-task-enabled'>{t('Enabled')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('Only enabled tasks receive new events.')}
              </p>
            </div>
            <Switch
              id='notification-task-enabled'
              checked={form.enabled}
              onCheckedChange={(enabled) =>
                setForm((current) => ({ ...current, enabled }))
              }
            />
          </div>
        </div>
        <SheetFooter>
          <SheetClose render={<Button variant='outline' />}>
            {t('Cancel')}
          </SheetClose>
          <Button
            onClick={submit}
            disabled={props.saving || props.bots.length === 0}
          >
            {props.saving && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Save')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function NotificationTasksPanel(props: {
  tasks: NotificationTask[]
  loading: boolean
  error: boolean
  onRetry: () => void
  onCreate: () => void
  onEdit: (task: NotificationTask) => void
  onDelete: (task: NotificationTask) => void
}) {
  const { t } = useTranslation()
  if (props.loading) return <LoadingList />
  if (props.error) {
    return (
      <ErrorState
        message={t('Failed to load notification tasks')}
        onRetry={props.onRetry}
      />
    )
  }
  if (props.tasks.length === 0) {
    return (
      <EmptyState
        icon={<BellRing className='text-muted-foreground h-9 w-9' />}
        title={t('No notification tasks')}
        text={t(
          'Create a task to send Telegram messages when a supported event occurs.'
        )}
      />
    )
  }
  return (
    <div className='space-y-3'>
      {props.tasks.map((task) => (
        <Card key={task.id} size='sm'>
          <CardHeader>
            <CardTitle className='flex flex-wrap items-center gap-2'>
              {task.name}
              <Badge variant={task.enabled ? 'default' : 'secondary'}>
                {task.enabled ? t('Enabled') : t('Disabled')}
              </Badge>
            </CardTitle>
            <CardDescription>
              {task.event_name || task.event_type}
            </CardDescription>
            <CardAction className='flex gap-1'>
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onEdit(task)}
                aria-label={t('Edit')}
              >
                <Pencil className='h-4 w-4' />
              </Button>
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onDelete(task)}
                aria-label={t('Disable')}
              >
                <PowerOff className='text-destructive h-4 w-4' />
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className='grid gap-3 text-sm sm:grid-cols-3'>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Telegram Bot')}
              </span>
              <p>{task.bot_name || `#${task.bot_id}`}</p>
            </div>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Recipients')}
              </span>
              <p>{task.targets?.length ?? 0}</p>
            </div>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Last triggered')}
              </span>
              <p>{formatTime(task.last_triggered_at)}</p>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function BotsPanel(props: {
  bots: NotificationBot[]
  loading: boolean
  error: boolean
  testingId: number | null
  onRetry: () => void
  onEdit: (bot: NotificationBot) => void
  onDelete: (bot: NotificationBot) => void
  onTest: (bot: NotificationBot) => void
}) {
  const { t } = useTranslation()
  if (props.loading) return <LoadingList />
  if (props.error) {
    return (
      <ErrorState
        message={t('Failed to load Telegram Bots')}
        onRetry={props.onRetry}
      />
    )
  }
  if (props.bots.length === 0) {
    return (
      <EmptyState
        icon={<Bot className='text-muted-foreground h-9 w-9' />}
        title={t('No Telegram Bots')}
        text={t('Add a Telegram Bot before creating a notification task.')}
      />
    )
  }
  return (
    <div className='space-y-3'>
      {props.bots.map((bot) => (
        <Card key={bot.id} size='sm'>
          <CardHeader>
            <CardTitle className='flex flex-wrap items-center gap-2'>
              {bot.name}
              <Badge variant={bot.enabled ? 'default' : 'secondary'}>
                {bot.enabled ? t('Enabled') : t('Disabled')}
              </Badge>
              {bot.token_configured && (
                <Badge variant='outline'>{t('Token configured')}</Badge>
              )}
            </CardTitle>
            <CardDescription>
              {bot.username ? `@${bot.username}` : t('Telegram Bot')}
            </CardDescription>
            <CardAction className='flex gap-1'>
              <Button
                variant='outline'
                size='sm'
                onClick={() => props.onTest(bot)}
                disabled={props.testingId === bot.id}
              >
                {props.testingId === bot.id ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <Send className='mr-2 h-4 w-4' />
                )}
                {t('Test')}
              </Button>
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onEdit(bot)}
                aria-label={t('Edit')}
              >
                <Pencil className='h-4 w-4' />
              </Button>
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onDelete(bot)}
                aria-label={t('Disable')}
              >
                <PowerOff className='text-destructive h-4 w-4' />
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent className='grid gap-3 text-sm sm:grid-cols-2'>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Last test')}
              </span>
              <p>{formatTime(bot.last_test_at)}</p>
            </div>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Test result')}
              </span>
              <p className={bot.last_test_error ? 'text-destructive' : ''}>
                {bot.last_test_error || (bot.last_test_at ? t('Success') : '-')}
              </p>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function DeliveriesPanel(props: {
  deliveries: NotificationDelivery[]
  loading: boolean
  error: boolean
  onRetry: () => void
}) {
  const { t } = useTranslation()
  if (props.loading) return <LoadingList />
  if (props.error) {
    return (
      <ErrorState
        message={t('Failed to load recent notifications')}
        onRetry={props.onRetry}
      />
    )
  }
  if (props.deliveries.length === 0) {
    return (
      <EmptyState
        icon={<Clock3 className='text-muted-foreground h-9 w-9' />}
        title={t('No recent notifications')}
        text={t('The latest five delivery results will appear here.')}
      />
    )
  }
  return (
    <div className='space-y-3'>
      {props.deliveries.map((delivery) => (
        <Card key={delivery.id} size='sm'>
          <CardHeader>
            <CardTitle className='flex items-center gap-2'>
              {delivery.status === 'success' ? (
                <CheckCircle2 className='h-4 w-4 text-green-600' />
              ) : (
                <XCircle className='text-destructive h-4 w-4' />
              )}
              {delivery.task_name || `#${delivery.task_id}`}
            </CardTitle>
            <CardDescription>
              {delivery.event_type} · {formatTime(delivery.created_at)}
            </CardDescription>
          </CardHeader>
          <CardContent className='grid gap-3 text-sm sm:grid-cols-3'>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Source ID')}
              </span>
              <p className='break-all'>{delivery.source_id || '-'}</p>
            </div>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Recipients')}
              </span>
              <p>{delivery.chat_id || '-'}</p>
            </div>
            <div>
              <span className='text-muted-foreground text-xs'>
                {t('Result')}
              </span>
              <p className={delivery.last_error ? 'text-destructive' : ''}>
                {delivery.last_error || t('Success')}
              </p>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

export function NotificationCenter() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [tab, setTab] = useState('tasks')
  const [botSheetOpen, setBotSheetOpen] = useState(false)
  const [taskSheetOpen, setTaskSheetOpen] = useState(false)
  const [editingBot, setEditingBot] = useState<NotificationBot | null>(null)
  const [editingTask, setEditingTask] = useState<NotificationTask | null>(null)
  const [testingId, setTestingId] = useState<number | null>(null)

  const botsQuery = useQuery({
    queryKey: ['notification-bots'],
    queryFn: getNotificationBots,
  })
  const tasksQuery = useQuery({
    queryKey: ['notification-tasks'],
    queryFn: getNotificationTasks,
  })
  const eventsQuery = useQuery({
    queryKey: ['notification-event-types'],
    queryFn: getNotificationEventTypes,
  })
  const deliveriesQuery = useQuery({
    queryKey: ['notification-deliveries'],
    queryFn: getNotificationDeliveries,
  })

  const invalidate = async (key: string) => {
    await queryClient.invalidateQueries({ queryKey: [key] })
  }

  const saveBot = useMutation({
    mutationFn: async (input: NotificationBotInput) => {
      const response = editingBot
        ? updateNotificationBot(editingBot.id, input)
        : createNotificationBot(input)
      const result = await response
      if (result.success === false) throw new Error(result.message)
      return result
    },
    onSuccess: async () => {
      toast.success(t('Telegram Bot saved'))
      setBotSheetOpen(false)
      setEditingBot(null)
      await invalidate('notification-bots')
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : t('Save failed')),
  })

  const saveTask = useMutation({
    mutationFn: async (input: NotificationTaskInput) => {
      const response = editingTask
        ? updateNotificationTask(editingTask.id, input)
        : createNotificationTask(input)
      const result = await response
      if (result.success === false) throw new Error(result.message)
      return result
    },
    onSuccess: async () => {
      toast.success(t('Notification task saved'))
      setTaskSheetOpen(false)
      setEditingTask(null)
      await invalidate('notification-tasks')
    },
    onError: (error) =>
      toast.error(error instanceof Error ? error.message : t('Save failed')),
  })

  const removeBot = async (bot: NotificationBot) => {
    if (!window.confirm(t('Disable this Telegram Bot?'))) return
    try {
      const response = await deleteNotificationBot(bot.id)
      if (response.success === false) {
        toast.error(response.message || t('Operation failed'))
        return
      }
      toast.success(t('Disabled successfully'))
      await invalidate('notification-bots')
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  const removeTask = async (task: NotificationTask) => {
    if (!window.confirm(t('Disable this notification task?'))) return
    try {
      const response = await deleteNotificationTask(task.id)
      if (response.success === false) {
        toast.error(response.message || t('Operation failed'))
        return
      }
      toast.success(t('Disabled successfully'))
      await invalidate('notification-tasks')
    } catch {
      toast.error(t('Operation failed'))
    }
  }

  const testBot = async (bot: NotificationBot) => {
    const chatId = window
      .prompt(t('Enter a Chat ID for the test message'))
      ?.trim()
    if (!chatId) return
    setTestingId(bot.id)
    try {
      const response = await testNotificationBot(bot.id, chatId)
      if (response.success === false) {
        toast.error(response.message || t('Bot test failed'))
      } else {
        toast.success(t('Bot test succeeded'))
      }
      await invalidate('notification-bots')
    } catch {
      toast.error(t('Bot test failed'))
    } finally {
      setTestingId(null)
    }
  }

  const actions = useMemo(() => {
    if (tab === 'bots') {
      return (
        <Button
          size='sm'
          onClick={() => {
            setEditingBot(null)
            setBotSheetOpen(true)
          }}
        >
          <Plus className='mr-2 h-4 w-4' />
          {t('Add Telegram Bot')}
        </Button>
      )
    }
    if (tab === 'tasks') {
      return (
        <Button
          size='sm'
          onClick={() => {
            setEditingTask(null)
            setTaskSheetOpen(true)
          }}
          disabled={(botsQuery.data?.length ?? 0) === 0}
        >
          <Plus className='mr-2 h-4 w-4' />
          {t('Create notification task')}
        </Button>
      )
    }
    return null
  }, [botsQuery.data?.length, t, tab])

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Notification Center')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>{actions}</SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto w-full max-w-6xl'>
            <Tabs
              value={tab}
              onValueChange={(value) => value && setTab(value)}
              className='gap-4'
            >
              <TabsList>
                <TabsTrigger value='tasks'>
                  {t('Notification tasks')}
                </TabsTrigger>
                <TabsTrigger value='bots'>{t('Telegram Bots')}</TabsTrigger>
                <TabsTrigger value='deliveries'>
                  {t('Recent notifications')}
                </TabsTrigger>
              </TabsList>
              <TabsContent value='tasks'>
                {(botsQuery.data?.length ?? 0) === 0 &&
                  !botsQuery.isLoading && (
                    <Alert className='mb-4'>
                      <Bot className='h-4 w-4' />
                      <AlertDescription>
                        {t(
                          'Add and test a Telegram Bot before creating a notification task.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                <NotificationTasksPanel
                  tasks={tasksQuery.data ?? []}
                  loading={tasksQuery.isLoading}
                  error={tasksQuery.isError}
                  onRetry={() => void tasksQuery.refetch()}
                  onCreate={() => setTaskSheetOpen(true)}
                  onEdit={(task) => {
                    setEditingTask(task)
                    setTaskSheetOpen(true)
                  }}
                  onDelete={(task) => void removeTask(task)}
                />
              </TabsContent>
              <TabsContent value='bots'>
                <BotsPanel
                  bots={botsQuery.data ?? []}
                  loading={botsQuery.isLoading}
                  error={botsQuery.isError}
                  testingId={testingId}
                  onRetry={() => void botsQuery.refetch()}
                  onEdit={(bot) => {
                    setEditingBot(bot)
                    setBotSheetOpen(true)
                  }}
                  onDelete={(bot) => void removeBot(bot)}
                  onTest={(bot) => void testBot(bot)}
                />
              </TabsContent>
              <TabsContent value='deliveries'>
                <DeliveriesPanel
                  deliveries={deliveriesQuery.data ?? []}
                  loading={deliveriesQuery.isLoading}
                  error={deliveriesQuery.isError}
                  onRetry={() => void deliveriesQuery.refetch()}
                />
              </TabsContent>
            </Tabs>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <BotSheet
        open={botSheetOpen}
        bot={editingBot}
        saving={saveBot.isPending}
        onOpenChange={(open) => {
          setBotSheetOpen(open)
          if (!open) setEditingBot(null)
        }}
        onSave={(input) => saveBot.mutate(input)}
      />
      <TaskSheet
        open={taskSheetOpen}
        task={editingTask}
        bots={botsQuery.data ?? []}
        events={eventsQuery.data ?? []}
        saving={saveTask.isPending}
        onOpenChange={(open) => {
          setTaskSheetOpen(open)
          if (!open) setEditingTask(null)
        }}
        onSave={(input) => saveTask.mutate(input)}
      />
    </>
  )
}
