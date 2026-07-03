/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import {
  Activity,
  BookOpen,
  Bot,
  Box,
  Brush,
  ChartNoAxesColumn,
  CircleHelp,
  CreditCard,
  ExternalLink,
  FileText,
  FlaskConical,
  Gamepad2,
  Globe,
  HandCoins,
  Home,
  Key,
  LayoutDashboard,
  Link as LinkIcon,
  ListTodo,
  MessageSquare,
  Puzzle,
  Radio,
  Settings,
  Sparkles,
  Ticket,
  User,
  Users,
  Wallet,
} from 'lucide-react';

const CUSTOM_NAV_ICONS = {
  Activity,
  BookOpen,
  Bot,
  Box,
  Brush,
  ChartNoAxesColumn,
  CircleHelp,
  CreditCard,
  ExternalLink,
  FileText,
  FlaskConical,
  Gamepad2,
  Globe,
  HandCoins,
  Home,
  Key,
  LayoutDashboard,
  Link: LinkIcon,
  ListTodo,
  MessageSquare,
  Puzzle,
  Radio,
  Settings,
  Sparkles,
  Ticket,
  User,
  Users,
  Wallet,
};

export const CUSTOM_NAV_ICON_OPTIONS = Object.keys(CUSTOM_NAV_ICONS);

export const CUSTOM_NAV_SECTION_OPTIONS = [
  { value: 'chat', label: '聊天区域' },
  { value: 'console', label: '控制台区域' },
  { value: 'personal', label: '个人中心区域' },
  { value: 'admin', label: '管理员区域' },
];

const SIDEBAR_SECTIONS = new Set(
  CUSTOM_NAV_SECTION_OPTIONS.map((option) => option.value),
);

export function getCustomNavIconComponent(name) {
  if (typeof name !== 'string') return undefined;
  return CUSTOM_NAV_ICONS[name];
}

export function isCustomNavIconName(name) {
  return Boolean(getCustomNavIconComponent(name));
}

export function renderCustomNavIcon(name, selected = false) {
  const Icon = getCustomNavIconComponent(name) || ExternalLink;
  const SELECTED_COLOR = 'var(--semi-color-primary)';
  const iconColor = selected ? SELECTED_COLOR : 'currentColor';

  return (
    <Icon
      size={16}
      strokeWidth={2}
      color={iconColor}
      className={`transition-colors duration-200 ${selected ? 'transition-transform duration-200 scale-105' : ''}`}
    />
  );
}

export const getCustomNavIcon = renderCustomNavIcon;

function normalizeId(value, fallback) {
  const raw = typeof value === 'string' ? value.trim() : '';
  const normalized = raw
    .replace(/\s+/g, '-')
    .replace(/[^\p{L}\p{N}:_.-]+/gu, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '');

  return normalized || fallback;
}

function normalizeUrl(value) {
  if (typeof value !== 'string') return null;
  const raw = value.trim();
  if (!raw) return null;

  if (raw.startsWith('/')) {
    if (raw.startsWith('//')) return null;
    return { url: raw, external: false };
  }

  try {
    const url = new URL(raw);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null;
    return { url: url.toString(), external: true };
  } catch {
    return null;
  }
}

function toBoolean(value, fallback) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value === 1;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (normalized === 'true' || normalized === '1') return true;
    if (normalized === 'false' || normalized === '0') return false;
  }
  return fallback;
}

function toOrder(value, fallback) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

export function getSidebarCustomModuleKey(id) {
  return `custom:${normalizeId(id, 'item')}`;
}

export function parseCustomNavItems(value, options = {}) {
  const { includeDisabled = false } = options;
  if (!Array.isArray(value)) return [];

  return value
    .map((raw, index) => {
      if (!raw || typeof raw !== 'object') return null;
      const normalizedUrl = normalizeUrl(raw.url ?? raw.href);
      if (!normalizedUrl) return null;

      const title = typeof raw.title === 'string' ? raw.title.trim() : '';
      if (!title) return null;

      const section =
        typeof raw.section === 'string' && SIDEBAR_SECTIONS.has(raw.section)
          ? raw.section
          : undefined;
      const openInNewTab =
        raw.target === 'blank'
          ? true
          : toBoolean(raw.openInNewTab, normalizedUrl.external);

      return {
        id: normalizeId(raw.id, `custom-${index + 1}`),
        title,
        url: normalizedUrl.url,
        enabled: toBoolean(raw.enabled, true),
        icon: isCustomNavIconName(raw.icon) ? raw.icon : undefined,
        order: toOrder(raw.order, index),
        requireAuth: toBoolean(raw.requireAuth, false),
        openInNewTab,
        section,
        external: normalizedUrl.external || openInNewTab,
      };
    })
    .filter((item) => item && (includeDisabled || item.enabled))
    .sort((a, b) => a.order - b.order || a.title.localeCompare(b.title));
}

export function createCustomNavItem(section = 'chat') {
  const id = `custom-${Date.now().toString(36)}`;
  return {
    id,
    title: '',
    url: '',
    enabled: true,
    icon: 'ExternalLink',
    order: 0,
    requireAuth: false,
    openInNewTab: false,
    section,
  };
}
