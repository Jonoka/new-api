import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('分组表保留只读 ID、展示可编辑名称并在内部生成 code', () => {
  const tableSource = readSource(
    'pages/Setting/Ratio/components/GroupTable.jsx',
  );
  const settingsSource = readSource(
    'pages/Setting/Ratio/GroupRatioSettings.jsx',
  );

  const visibleDataColumns = [
    ...tableSource.matchAll(/dataIndex:\s*'([^']+)'/g),
  ].map((match) => match[1]);

  assert.deepEqual(visibleDataColumns, [
    'id',
    'name',
    'ratio',
    'user_selectable',
    'description',
  ]);
  assert.match(tableSource, /title:\s*t\('ID'\)/);
  assert.match(tableSource, /\{record\.id \|\| '-'\}/);
  assert.match(tableSource, /title:\s*t\('分组名称'\)/);
  assert.match(tableSource, /createUniqueGroupCode/);
  assert.match(tableSource, /code,\s*\n\s*name:\s*''/);
  assert.doesNotMatch(tableSource, /dataIndex:\s*'code'/);
  assert.doesNotMatch(tableSource, /t\('稳定标识'\)/);
  assert.doesNotMatch(settingsSource, /稳定标识/);
  assert.match(settingsSource, /showError\(t\('请输入分组名称'\)\)/);
});

test('Auto 与规则编辑器统一使用名称 label 和内部 code value', () => {
  const settingsSource = readSource(
    'pages/Setting/Ratio/GroupRatioSettings.jsx',
  );
  const autoSource = readSource(
    'pages/Setting/Ratio/components/AutoGroupList.jsx',
  );
  const ratioRulesSource = readSource(
    'pages/Setting/Ratio/components/GroupGroupRatioRules.jsx',
  );
  const usableRulesSource = readSource(
    'pages/Setting/Ratio/components/GroupSpecialUsableRules.jsx',
  );

  assert.equal(
    [...settingsSource.matchAll(/groupOptions=\{groupOptions\}/g)].length,
    3,
  );
  assert.match(autoSource, /value=\{item\.code \|\| undefined\}/);
  assert.match(autoSource, /optionList=\{groupOptions\}/);
  assert.match(ratioRulesSource, /<Text strong>\{groupLabel\}<\/Text>/);
  assert.match(usableRulesSource, /<Text strong>\{groupLabel\}<\/Text>/);
  assert.match(usableRulesSource, /optionList=\{groupOptions\}/);
});

test('模型广场消费 group_names 但筛选和计价仍使用内部 code', () => {
  const hookSource = readSource('hooks/model-pricing/useModelPricingData.jsx');
  const filterSource = readSource(
    'components/table/model-pricing/filter/PricingGroups.jsx',
  );
  const detailSource = readSource(
    'components/table/model-pricing/modal/components/ModelPricingTable.jsx',
  );
  const billingSource = readSource(
    'components/table/model-pricing/billing/BillingGuide.jsx',
  );

  assert.match(hookSource, /group_names/);
  assert.match(hookSource, /setGroupNames/);
  assert.match(hookSource, /getGroupDisplayName\(group, groupNames\)/);
  assert.match(filterSource, /getGroupDisplayName\(g, groupNames\)/);
  assert.match(filterSource, /m\.enable_groups\.includes\(g\)/);
  assert.match(detailSource, /getGroupDisplayName\(group, groupNames\)/);
  assert.match(billingSource, /value:\s*group\.value/);
  assert.match(
    billingSource,
    /getGroupDisplayName\(group\.value, groupNames\)/,
  );
});
