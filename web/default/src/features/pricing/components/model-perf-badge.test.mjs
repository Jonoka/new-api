import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))

test('Default 模型卡片把性能摘要放在右侧并隐藏整体百分比', () => {
  const source = readFileSync(resolve(root, 'model-perf-badge.tsx'), 'utf8')
  const statusSegmentsSource = readFileSync(
    resolve(root, '../../performance-metrics/components/status-segments.tsx'),
    'utf8'
  )

  assert.match(source, /min-\[460px\]:w-\[132px\]/)
  assert.match(source, /props\.perf\.status_rate/)
  assert.match(source, /showOverall=\{false\}/)
  assert.match(source, /tone='availability'/)
  assert.match(source, /shape='signal'/)
  assert.match(source, /segmentCount=\{3\}/)
  assert.match(source, /replace\(' t\/s', 't'\)/)
  assert.match(statusSegmentsSource, /'h-4 items-end gap-0\.5'/)
  assert.match(statusSegmentsSource, /'w-1 rounded-full'/)
  assert.match(statusSegmentsSource, /\['h-2', 'h-2\.5', 'h-3'\]/)
})
