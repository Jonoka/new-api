import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

test('分组表保留只读 ID、展示可编辑名称并在内部生成临时引用', () => {
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
    'exclusive',
    'description',
  ]);
  assert.match(tableSource, /title:\s*t\('ID'\)/);
  assert.match(tableSource, /\{record\.id \|\| '-'\}/);
  assert.match(tableSource, /title:\s*t\('分组名称'\)/);
  assert.match(tableSource, /createTemporaryGroupCode/);
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
    2,
  );
  assert.match(
    settingsSource,
    /groupOptions=\{groupOptions\.filter\(\(group\) => !group\.exclusive\)\}/,
  );
  assert.match(autoSource, /value=\{item\.code \|\| undefined\}/);
  assert.match(autoSource, /optionList=\{groupOptions\}/);
  assert.match(ratioRulesSource, /<Text strong>\{groupLabel\}<\/Text>/);
  assert.match(usableRulesSource, /<Text strong>\{groupLabel\}<\/Text>/);
  assert.match(usableRulesSource, /optionList=\{groupOptions\}/);
});

test('分组和高级配置通过单次原子请求保存且不使用 staging', () => {
  const settingsSource = readSource(
    'pages/Setting/Ratio/GroupRatioSettings.jsx',
  );

  assert.equal(
    [...settingsSource.matchAll(/API\.put\('\/api\/group\/details'/g)].length,
    1,
  );
  assert.match(settingsSource, /option_updates:\s*optionUpdates/);
  assert.match(
    settingsSource,
    /groupsChanged\s*\?\s*groupPayload\s*:\s*\{\s*groups:\s*\[\],\s*deleted_ids:\s*\[\]\s*\}/,
  );
  assert.doesNotMatch(settingsSource, /API\.put\('\/api\/option\/'/);
  assert.doesNotMatch(settingsSource, /prepareGroupNamesForInitialSave/);
  assert.doesNotMatch(settingsSource, /__group_prepare_/);
  assert.match(settingsSource, /'TopupGroupRatio'/);
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
  const performanceSource = readSource(
    'components/table/model-pricing/performance/ModelPerformancePanel.jsx',
  );
  const sideSheetSource = readSource(
    'components/table/model-pricing/modal/ModelDetailSideSheet.jsx',
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
  assert.match(
    performanceSource,
    /getGroupDisplayName\(row\.group, groupNames\)/,
  );
  assert.match(
    sideSheetSource,
    /<ModelPerformancePanel[\s\S]*?groupNames=\{groupNames\}/,
  );
});

test('用户编辑器显示当前分组名称但继续提交内部 code', () => {
  const source = readSource('components/table/users/modals/EditUserModal.jsx');

  assert.match(source, /API\.get\('\/api\/group\/details'\)/);
  assert.match(source, /extractGroupDetailsResponse\(res\?\.data\)/);
  assert.match(source, /setGroupOptions\(createGroupOptions\(groups\)\)/);
  assert.doesNotMatch(source, /API\.get\(`\/api\/group\/`\)/);
});

test('用户筛选和订阅套餐统一显示名称并提交内部 code', () => {
  const usersSource = readSource('hooks/users/useUsersData.jsx');
  const subscriptionsSource = readSource(
    'components/table/subscriptions/modals/AddEditSubscriptionModal.jsx',
  );

  for (const source of [usersSource, subscriptionsSource]) {
    assert.match(source, /API\.get\('\/api\/group\/details'\)/);
    assert.match(source, /extractGroupDetailsResponse\(res(?:\?|)\.data\)/);
    assert.match(source, /createGroupOptions\(groups \|\| \[\]\)/);
  }
  assert.match(subscriptionsSource, /value=\{group\.value\}/);
  assert.match(subscriptionsSource, /\{group\.label\}/);
  assert.doesNotMatch(usersSource, /API\.get\(`\/api\/group\/`\)/);
  assert.doesNotMatch(subscriptionsSource, /API\.get\('\/api\/group'\)/);
});
