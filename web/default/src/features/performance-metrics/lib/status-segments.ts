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
export const STATUS_SEGMENT_COUNT = 4
export const STATUS_WINDOW_HOURS = 24

const STATUS_WINDOW_SECONDS = STATUS_WINDOW_HOURS * 60 * 60
const STATUS_SEGMENT_SECONDS = STATUS_WINDOW_SECONDS / STATUS_SEGMENT_COUNT

export type StatusSeriesPoint = {
  ts: number
  success_rate: number
}

export type StatusSegment = {
  startTs: number
  endTs: number
  successRate: number | null
  sampleCount: number
}

type SegmentAccumulator = {
  totalRate: number
  sampleCount: number
}

/**
 * 将最近 24 小时的指标桶压缩为四个固定 6 小时时段。
 * 无样本时保留空段，避免把缺失流量误判为成功或失败。
 */
export function buildStatusSegments(
  series: StatusSeriesPoint[],
  endTs: number
): StatusSegment[] {
  const normalizedEndTs = Number.isFinite(endTs)
    ? Math.trunc(endTs)
    : Math.trunc(Date.now() / 1000)
  const startTs = normalizedEndTs - STATUS_WINDOW_SECONDS
  const accumulators: SegmentAccumulator[] = Array.from(
    { length: STATUS_SEGMENT_COUNT },
    () => ({ totalRate: 0, sampleCount: 0 })
  )

  for (const point of series) {
    if (
      !Number.isFinite(point.ts) ||
      !Number.isFinite(point.success_rate) ||
      point.success_rate < 0 ||
      point.success_rate > 100 ||
      point.ts < startTs ||
      point.ts > normalizedEndTs
    ) {
      continue
    }

    const rawIndex = Math.floor((point.ts - startTs) / STATUS_SEGMENT_SECONDS)
    const segmentIndex = Math.min(rawIndex, STATUS_SEGMENT_COUNT - 1)
    const accumulator = accumulators[segmentIndex]
    accumulator.totalRate += point.success_rate
    accumulator.sampleCount += 1
  }

  return accumulators.map((accumulator, index) => {
    const segmentStartTs = startTs + index * STATUS_SEGMENT_SECONDS
    const successRate =
      accumulator.sampleCount > 0
        ? Math.round((accumulator.totalRate / accumulator.sampleCount) * 100) /
          100
        : null

    return {
      startTs: segmentStartTs,
      endTs: segmentStartTs + STATUS_SEGMENT_SECONDS,
      successRate,
      sampleCount: accumulator.sampleCount,
    }
  })
}
