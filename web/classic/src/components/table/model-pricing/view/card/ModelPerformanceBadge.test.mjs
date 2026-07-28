import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

test('Classic 模型卡片把性能摘要放在右侧并隐藏时间与百分比', () => {
  const source = readFileSync(
    resolve(root, 'ModelPerformanceBadge.jsx'),
    'utf8',
  );
  const sparklineSource = readFileSync(
    resolve(root, '../../performance/SuccessRateSparkline.jsx'),
    'utf8',
  );

  assert.match(source, /ml-auto grid w-\[132px\]/);
  assert.match(source, /performance\.status_rate/);
  assert.match(source, /showOverall=\{false\}/);
  assert.match(source, /availabilityTone/);
  assert.match(source, /signalStyle/);
  assert.match(source, /aggregateWindow/);
  assert.match(source, /maxPoints=\{3\}/);
  assert.match(source, /replace\(' t\/s', 't'\)/);
  assert.match(sparklineSource, /const gap = signalStyle\s*\?\s*'gap-0\.5'/);
  assert.match(sparklineSource, /signalStyle \? 'rounded-full'/);
  assert.match(sparklineSource, /8 \+ Math\.min\(index, 2\) \* 2/);
  assert.doesNotMatch(source, /latestTimestamp=/);
});

test('Classic 性能详情保留 24 个原始历史点', () => {
  const panelSource = readFileSync(
    resolve(root, '../../performance/ModelPerformancePanel.jsx'),
    'utf8',
  );
  const sparklineSource = readFileSync(
    resolve(root, '../../performance/SuccessRateSparkline.jsx'),
    'utf8',
  );

  assert.match(panelSource, /maxPoints=\{24\}/);
  assert.match(sparklineSource, /aggregateWindow = false/);
  assert.match(sparklineSource, /normalizePerformanceSeries\(series\)\.slice/);
});
