import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildGroupDetailsPayload,
  buildGroupSelectionPayload,
  createGroupOptions,
  createPlaygroundGroupOptions,
  createUniqueGroupCode,
  createUserGroupOptions,
  extractAutoGroupConfigResponse,
  extractGroupDetailsResponse,
  formatGroupLabel,
  getGroupDisplayName,
  getDeletedGroupIds,
  groupDetailsToLegacyOptions,
  normalizeAutoGroupConfig,
  reorderAutoGroupItems,
  resolveGroupCodes,
} from './groupDetails.js';

test('新增分组生成未占用的内部 code 并随保存 payload 发送', () => {
  const code = createUniqueGroupCode(new Set(['group_1', 'group_3']));
  const payload = buildGroupDetailsPayload(
    [{ code, name: '新分组', ratio: 1, status: 1 }],
    [],
  );

  assert.equal(code, 'group_2');
  assert.equal(payload.groups[0].code, 'group_2');
  assert.equal(payload.groups[0].name, '新分组');
});

test('显示名称映射不会改变内部 code', () => {
  const groupNames = { group_1: '高级分组' };

  assert.equal(getGroupDisplayName('group_1', groupNames), '高级分组');
  assert.equal(getGroupDisplayName('legacy', groupNames), 'legacy');
  assert.equal(getGroupDisplayName('group_1', null), 'group_1');
});

test('分组保存以 ID 关联并保留旧客户端兼容字段', () => {
  const original = {
    id: 42,
    code: 'codex-pro',
    name: 'Codex 专线',
    ratio: 0.5,
    user_selectable: true,
    description: '稳定线路',
    status: 'active',
  };
  const renamed = { ...original, name: 'Codex 高速专线' };
  const payload = buildGroupDetailsPayload([renamed], []);

  assert.equal(formatGroupLabel(renamed), 'Codex 高速专线');
  assert.equal(payload.groups[0].id, 42);
  assert.equal(payload.groups[0].code, 'codex-pro');
  assert.equal(payload.groups[0].name, 'Codex 高速专线');
});

test('渠道和令牌兼容 payload 同时包含有序 IDs 与旧 CSV', () => {
  const payload = buildGroupSelectionPayload(
    ['vip', 'default'],
    [
      { id: 9, code: 'default' },
      { id: 12, code: 'vip' },
    ],
  );

  assert.deepEqual(payload, {
    group: 'vip,default',
    group_ids: [12, 9],
    group_mode: 'explicit',
  });
});

test('存在无法映射的 code 时保留完整旧字符串并回退旧接口语义', () => {
  const payload = buildGroupSelectionPayload(
    ['vip', 'legacy-only'],
    [{ id: 12, code: 'vip' }],
  );

  assert.equal(payload.group, 'vip,legacy-only');
  assert.equal(payload.group_ids, undefined);
  assert.equal(payload.group_mode, 'explicit');
});

test('选择器显示名称但始终以 code 作为选中值', () => {
  const [adminOption] = createGroupOptions([
    { id: 12, code: 'vip', name: '尊贵用户' },
  ]);
  const [userOption] = createUserGroupOptions({
    'legacy-vip': { id: 12, code: 'vip', name: '尊贵用户', desc: '专属线路' },
  });

  assert.equal(adminOption.label, '尊贵用户');
  assert.equal(adminOption.value, 'vip');
  assert.equal(userOption.label, '尊贵用户');
  assert.equal(userOption.value, 'vip');
  assert.equal(userOption.legacy_code, 'legacy-vip');
});

test('虚拟 auto 配置独立于实体分组并投影到旧可选分组', () => {
  const autoGroup = extractAutoGroupConfigResponse({
    success: true,
    data: [],
    auto_group: {
      user_selectable: true,
      description: '按线路健康度自动选择',
    },
  });
  assert.deepEqual(autoGroup, {
    user_selectable: true,
    description: '按线路健康度自动选择',
  });

  const projection = groupDetailsToLegacyOptions(
    [{ id: 1, code: 'default', name: '默认', user_selectable: true }],
    autoGroup,
  );
  assert.deepEqual(JSON.parse(projection.UserUsableGroups), {
    default: '',
    auto: '按线路健康度自动选择',
  });
  assert.equal(
    normalizeAutoGroupConfig({ user_selectable: false, description: '' })
      .description,
    '自动选择最佳可用分组，失败时按配置顺序切换',
  );
});

test('操练场显示当前名称但提交接口对象键', () => {
  const [option] = createPlaygroundGroupOptions({
    'Codex-Team': {
      name: 'Codex-福利组',
      desc: '福利线路',
      ratio: 0.05,
    },
  });

  assert.deepEqual(option, {
    label: 'Codex-福利组',
    value: 'Codex-Team',
    ratio: 0.05,
    fullLabel: '福利线路',
  });

  const [duplicateDescription] = createPlaygroundGroupOptions({
    'Codex-Team': {
      name: 'Codex-福利组',
      desc: 'Codex-福利组',
      ratio: 0.05,
    },
  });
  assert.equal(duplicateDescription.fullLabel, '');
});

test('编辑记录优先按 group_ids 回填，旧记录才回退 group 字符串', () => {
  const options = [
    { id: 9, code: 'default', value: 'default' },
    { id: 12, code: 'vip', value: 'vip', legacy_code: 'legacy-vip' },
  ];

  assert.deepEqual(
    resolveGroupCodes({ group_ids: [12, 9], group: 'default' }, options),
    ['vip', 'default'],
  );
  assert.deepEqual(resolveGroupCodes({ group: 'legacy-vip' }, options), [
    'vip',
  ]);
});

test('部分 ID 无法解析时保留服务端的完整分组引用', () => {
  assert.deepEqual(
    resolveGroupCodes(
      {
        group: 'vip,legacy',
        group_ids: [12, 99],
        group_details: [
          { id: 12, code: 'vip', name: 'VIP' },
          { id: 99, code: 'legacy', name: '旧分组' },
        ],
      },
      [{ id: 12, code: 'vip', name: 'VIP' }],
    ),
    ['vip', 'legacy'],
  );
  assert.deepEqual(
    resolveGroupCodes({ group: 'vip,legacy', group_ids: [12, 99] }, [
      { id: 12, code: 'vip', name: 'VIP' },
    ]),
    ['vip', 'legacy'],
  );
});

test('令牌分组模式显式兼容 auto 和 inherit', () => {
  assert.deepEqual(resolveGroupCodes({ group_mode: 'auto', group: '' }, []), [
    'auto',
  ]);
  assert.deepEqual(
    resolveGroupCodes({ group_mode: 'inherit', group: 'stale' }, []),
    [],
  );
});

test('auto 和空选择保持选择器语义，不伪造分组 ID', () => {
  assert.deepEqual(buildGroupSelectionPayload(['auto'], []), {
    group: 'auto',
    group_ids: [],
    group_mode: 'auto',
  });
  assert.deepEqual(buildGroupSelectionPayload([], []), {
    group: '',
    group_ids: [],
    group_mode: 'inherit',
  });
});

test('兼容标准响应与直接 groups 响应', () => {
  const group = { id: 7, code: 'vip', name: 'VIP', ratio: 0.8 };

  assert.equal(extractGroupDetailsResponse([group])[0].id, 7);
  assert.equal(extractGroupDetailsResponse({ data: [group] })[0].code, 'vip');
  assert.equal(
    extractGroupDetailsResponse({ data: { groups: [group] } })[0].name,
    'VIP',
  );
  assert.equal(extractGroupDetailsResponse({ success: true }), null);
});

test('删除列表只包含服务端已经分配的稳定 ID', () => {
  const original = [
    { id: 5, code: 'default' },
    { id: 8, code: 'vip' },
    { id: null, code: 'new-group' },
  ];
  const current = [{ id: 5, code: 'default' }];

  assert.deepEqual(getDeletedGroupIds(original, current), [8]);
});

test('禁用状态以数字原样提交，兼容后端整数字段', () => {
  const payload = buildGroupDetailsPayload(
    [{ id: 3, code: 'disabled', name: '停用分组', status: 0 }],
    [],
  );

  assert.equal(payload.groups[0].status, 0);
});

test('自动分组拖拽按落点前后精确重排', () => {
  const items = [
    { _id: 'a', code: 'group-a' },
    { _id: 'b', code: 'group-b' },
    { _id: 'c', code: 'group-c' },
  ];

  assert.deepEqual(
    reorderAutoGroupItems(items, 'a', 'c', 'after').map((item) => item._id),
    ['b', 'c', 'a'],
  );
  assert.deepEqual(
    reorderAutoGroupItems(items, 'c', 'a', 'before').map((item) => item._id),
    ['c', 'a', 'b'],
  );
});

test('自动分组无效或原位拖拽保持原引用和顺序', () => {
  const items = [
    { _id: 'a', code: 'group-a' },
    { _id: 'b', code: 'group-b' },
  ];

  assert.equal(reorderAutoGroupItems(items, 'a', 'a'), items);
  assert.equal(reorderAutoGroupItems(items, 'missing', 'b'), items);
  assert.equal(reorderAutoGroupItems(items, 'a', 'b', 'before'), items);
});
