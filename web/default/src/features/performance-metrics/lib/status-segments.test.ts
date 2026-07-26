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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildStatusSegments, STATUS_SEGMENT_COUNT } from './status-segments.ts'

const END_TS = 24 * 60 * 60

describe('模型状态四段压缩', () => {
  test('把最近 24 小时等分为四个 6 小时时段', () => {
    const segments = buildStatusSegments(
      [
        { ts: 1 * 60 * 60, success_rate: 100 },
        { ts: 7 * 60 * 60, success_rate: 99.9 },
        { ts: 13 * 60 * 60, success_rate: 99 },
        { ts: 19 * 60 * 60, success_rate: 90 },
      ],
      END_TS
    )

    assert.equal(segments.length, STATUS_SEGMENT_COUNT)
    assert.deepEqual(
      segments.map((segment) => segment.successRate),
      [100, 99.9, 99, 90]
    )
    assert.deepEqual(
      segments.map((segment) => segment.sampleCount),
      [1, 1, 1, 1]
    )
  })

  test('同段取桶成功率平均值，并保留无数据时段', () => {
    const segments = buildStatusSegments(
      [
        { ts: 7 * 60 * 60, success_rate: 100 },
        { ts: 8 * 60 * 60, success_rate: 98 },
      ],
      END_TS
    )

    assert.deepEqual(
      segments.map((segment) => segment.successRate),
      [null, 99, null, null]
    )
    assert.equal(segments[1].sampleCount, 2)
  })

  test('边界点进入后一段，窗口终点进入最后一段', () => {
    const segments = buildStatusSegments(
      [
        { ts: 6 * 60 * 60, success_rate: 100 },
        { ts: END_TS, success_rate: 80 },
      ],
      END_TS
    )

    assert.deepEqual(
      segments.map((segment) => segment.successRate),
      [null, 100, null, 80]
    )
  })

  test('忽略窗口外和非法成功率样本', () => {
    const segments = buildStatusSegments(
      [
        { ts: -1, success_rate: 100 },
        { ts: 2 * 60 * 60, success_rate: -1 },
        { ts: 8 * 60 * 60, success_rate: 101 },
        { ts: 14 * 60 * 60, success_rate: Number.NaN },
      ],
      END_TS
    )

    assert.deepEqual(
      segments.map((segment) => segment.successRate),
      [null, null, null, null]
    )
  })
})
