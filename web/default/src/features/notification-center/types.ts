/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

export interface NotificationBot {
  id: number
  name: string
  enabled: boolean
  token_configured: boolean
  created_at?: number
  updated_at?: number
}

export interface NotificationTarget {
  id?: number
  chat_id: string
  mention_user_id?: string
  mention_name?: string
  enabled?: boolean
}

export interface NotificationTask {
  id: number
  name: string
  event_type: string
  event_name?: string
  bot_id: number
  bot_name?: string
  targets: NotificationTarget[]
  template: string
  enabled: boolean
  last_triggered_at?: number
  created_at?: number
  updated_at?: number
}

export interface NotificationEventType {
  value: string
  label: string
  description?: string
  variables?: string[]
  default_template?: string
}

export type NotificationDeliveryStatus = 'success' | 'dead' | 'canceled'

export interface NotificationDelivery {
  id: number
  task_id: number
  task_name?: string
  event_type: string
  source_id?: string
  chat_id?: string
  status: NotificationDeliveryStatus
  last_error?: string
  sent_at?: number
  created_at: number
}

export interface NotificationBotInput {
  name: string
  token?: string
  enabled: boolean
}

export interface NotificationTaskInput {
  name: string
  event_type: string
  bot_id: number
  targets: NotificationTarget[]
  template: string
  enabled: boolean
}

export interface NotificationApiResponse<T = unknown> {
  success?: boolean
  message?: string
  data?: T
}
