/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

// 保留旧入口作为兼容别名，正式页面仍由 channel-quality 扩展注册。
export const Route = createFileRoute('/_authenticated/channel-observability/')({
  beforeLoad: () => {
    throw redirect({
      to: '/extensions/$moduleId/$pageKey',
      params: { moduleId: 'channel-quality', pageKey: 'index' },
    })
  },
})
