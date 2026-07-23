/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { api } from '@/lib/api'
import type {
  NotificationApiResponse,
  NotificationBot,
  NotificationBotInput,
  NotificationDelivery,
  NotificationEventType,
  NotificationTask,
  NotificationTaskInput,
} from './types'

type ListPayload<T> = T[] | { items?: T[] }

function normalizeList<T>(payload?: ListPayload<T>): T[] {
  if (Array.isArray(payload)) return payload
  return payload?.items ?? []
}

function requireSuccessfulResponse<T>(response: NotificationApiResponse<T>) {
  if (response.success === false) {
    throw new Error(response.message || 'Request failed')
  }
  return response
}

export async function getNotificationBots(): Promise<NotificationBot[]> {
  const response = await api.get<
    NotificationApiResponse<ListPayload<NotificationBot>>
  >('/api/notification/bots')
  return normalizeList(requireSuccessfulResponse(response.data).data)
}

export async function createNotificationBot(
  input: NotificationBotInput
): Promise<NotificationApiResponse<NotificationBot>> {
  const response = await api.post('/api/notification/bots', input)
  return response.data
}

export async function updateNotificationBot(
  id: number,
  input: NotificationBotInput
): Promise<NotificationApiResponse<NotificationBot>> {
  const response = await api.put(`/api/notification/bots/${id}`, input)
  return response.data
}

export async function deleteNotificationBot(
  id: number
): Promise<NotificationApiResponse> {
  const response = await api.delete(`/api/notification/bots/${id}`)
  return response.data
}

export async function testNotificationBot(
  id: number,
  chatId: string
): Promise<NotificationApiResponse> {
  const response = await api.post(`/api/notification/bots/${id}/test`, {
    chat_id: chatId,
  })
  return response.data
}

export async function getNotificationTasks(): Promise<NotificationTask[]> {
  const response = await api.get<
    NotificationApiResponse<ListPayload<NotificationTask>>
  >('/api/notification/tasks')
  return normalizeList(requireSuccessfulResponse(response.data).data)
}

export async function createNotificationTask(
  input: NotificationTaskInput
): Promise<NotificationApiResponse<number>> {
  const response = await api.post('/api/notification/tasks', input)
  return response.data
}

export async function updateNotificationTask(
  id: number,
  input: NotificationTaskInput
): Promise<NotificationApiResponse<null>> {
  const response = await api.put(`/api/notification/tasks/${id}`, input)
  return response.data
}

export async function deleteNotificationTask(
  id: number
): Promise<NotificationApiResponse> {
  const response = await api.delete(`/api/notification/tasks/${id}`)
  return response.data
}

export async function getNotificationEventTypes(): Promise<
  NotificationEventType[]
> {
  const response = await api.get<
    NotificationApiResponse<ListPayload<NotificationEventType>>
  >('/api/notification/event-types')
  return normalizeList(requireSuccessfulResponse(response.data).data)
}

export async function getNotificationDeliveries(): Promise<
  NotificationDelivery[]
> {
  const response = await api.get<
    NotificationApiResponse<ListPayload<NotificationDelivery>>
  >('/api/notification/deliveries')
  return normalizeList(requireSuccessfulResponse(response.data).data).slice(
    0,
    5
  )
}
