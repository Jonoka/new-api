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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Button,
  Col,
  Collapsible,
  Form,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconChevronDown,
  IconChevronUp,
  IconDelete,
  IconPlus,
} from '@douyinfe/semi-icons';
import {
  compareObjects,
  API,
  showError,
  showSuccess,
  showWarning,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

function parseJSONSafe(str, fallback) {
  if (!str || !String(str).trim()) return fallback;
  try {
    return JSON.parse(str);
  } catch {
    return fallback;
  }
}

function normalizeLimitPair(value, fallback = [0, 1]) {
  if (!Array.isArray(value)) return fallback;
  const maxRequests = Number(value[0]);
  const maxSuccess = Number(value[1]);
  return [
    Number.isFinite(maxRequests) && maxRequests >= 0
      ? maxRequests
      : fallback[0],
    Number.isFinite(maxSuccess) && maxSuccess >= 1 ? maxSuccess : fallback[1],
  ];
}

function normalizeGroupRateLimit(value) {
  const raw = parseJSONSafe(value, {});
  const result = {};
  Object.entries(raw).forEach(([groupName, limits]) => {
    if (!groupName) return;
    result[groupName] = normalizeLimitPair(limits);
  });
  return result;
}

function serializeGroupRateLimit(config) {
  const result = {};
  Object.entries(config).forEach(([groupName, limits]) => {
    if (!groupName) return;
    result[groupName] = normalizeLimitPair(limits);
  });
  return Object.keys(result).length === 0
    ? ''
    : JSON.stringify(result, null, 2);
}

function GroupRateLimitRules({ value, onChange }) {
  const { t } = useTranslation();
  const [config, setConfig] = useState(() => normalizeGroupRateLimit(value));
  const [newGroupName, setNewGroupName] = useState('');

  useEffect(() => {
    setConfig(normalizeGroupRateLimit(value));
  }, [value]);

  const emitChange = useCallback(
    (nextConfig) => {
      setConfig(nextConfig);
      onChange?.(serializeGroupRateLimit(nextConfig));
    },
    [onChange],
  );

  const setLimit = useCallback(
    (groupName, limits) => {
      const nextConfig = { ...config };
      nextConfig[groupName] = normalizeLimitPair(limits);
      emitChange(nextConfig);
    },
    [config, emitChange],
  );

  const renameGroup = useCallback(
    (oldName, newName) => {
      const name = String(newName || '').trim();
      if (!name || name === oldName) return;
      if (config[name]) {
        showWarning(t('该分组已存在'));
        return;
      }
      const nextConfig = {};
      Object.entries(config).forEach(([k, v]) => {
        nextConfig[k === oldName ? name : k] = v;
      });
      emitChange(nextConfig);
    },
    [config, emitChange, t],
  );

  const removeGroup = useCallback(
    (groupName) => {
      const nextConfig = { ...config };
      delete nextConfig[groupName];
      emitChange(nextConfig);
    },
    [config, emitChange],
  );

  const addGroup = useCallback(() => {
    const name = String(newGroupName || '').trim();
    if (!name) {
      showWarning(t('请输入分组名称'));
      return;
    }
    if (config[name]) {
      showWarning(t('该分组已存在'));
      return;
    }
    const nextConfig = { ...config };
    nextConfig[name] = [0, 1];
    emitChange(nextConfig);
    setNewGroupName('');
  }, [config, emitChange, newGroupName, t]);

  const groupOptions = useMemo(
    () => Object.keys(config).map((name) => ({ value: name, label: name })),
    [config],
  );

  const entries = Object.entries(config);

  return (
    <div className='space-y-3'>
      {entries.length === 0 ? (
        <Text type='tertiary' className='block py-4 text-center'>
          {t('暂无分组规则，点击下方按钮添加')}
        </Text>
      ) : (
        entries.map(([groupName, limits]) => (
          <div
            key={groupName}
            className='flex flex-wrap items-center gap-2'
            style={{
              border: '1px solid var(--semi-color-border)',
              borderRadius: 6,
              padding: 8,
            }}
          >
            <Select
              size='small'
              filter
              allowCreate
              placeholder={t('分组名称')}
              value={groupName}
              optionList={groupOptions}
              style={{ minWidth: 180, flex: 1 }}
              position='bottomLeft'
              onChange={(nextName) => renameGroup(groupName, nextName)}
            />
            <InputNumber
              size='small'
              min={0}
              max={2147483647}
              value={limits[0]}
              suffix={t('次')}
              style={{ width: 150 }}
              onChange={(v) => setLimit(groupName, [Number(v) || 0, limits[1]])}
            />
            <InputNumber
              size='small'
              min={1}
              max={2147483647}
              value={limits[1]}
              suffix={t('成功次')}
              style={{ width: 150 }}
              onChange={(v) =>
                setLimit(groupName, [limits[0], Math.max(1, Number(v) || 1)])
              }
            />
            <Popconfirm
              title={t('确认删除该分组？')}
              onConfirm={() => removeGroup(groupName)}
              position='left'
            >
              <Button
                icon={<IconDelete />}
                type='danger'
                theme='borderless'
                size='small'
              />
            </Popconfirm>
          </div>
        ))
      )}
      <div className='mt-3 flex flex-wrap justify-center gap-2'>
        <Select
          size='small'
          filter
          allowCreate
          placeholder={t('输入分组名称')}
          value={newGroupName || undefined}
          optionList={groupOptions}
          onChange={setNewGroupName}
          style={{ width: 220 }}
          position='bottomLeft'
        />
        <Button icon={<IconPlus />} theme='outline' onClick={addGroup}>
          {t('添加分组')}
        </Button>
      </div>
      <div
        style={{
          border: '1px solid var(--semi-color-border)',
          borderRadius: 6,
          padding: 12,
          background: 'var(--semi-color-fill-0)',
        }}
      >
        <Text strong>{t('JSON 预览')}</Text>
        <pre style={{ marginTop: 8, whiteSpace: 'pre-wrap' }}>
          {serializeGroupRateLimit(config) || '{}'}
        </pre>
      </div>
    </div>
  );
}

function normalizeUserGroupRateLimit(value) {
  const raw = parseJSONSafe(value, {});
  const result = {};
  Object.entries(raw).forEach(([userGroup, config]) => {
    if (!userGroup || typeof config !== 'object' || config === null) return;
    const nextConfig = { groups: {} };
    if (Array.isArray(config.global)) {
      nextConfig.global = normalizeLimitPair(config.global);
    }
    if (config.groups && typeof config.groups === 'object') {
      Object.entries(config.groups).forEach(([groupName, limits]) => {
        if (!groupName) return;
        nextConfig.groups[groupName] = normalizeLimitPair(limits);
      });
    }
    result[userGroup] = nextConfig;
  });
  return result;
}

function serializeUserGroupRateLimit(config) {
  const result = {};
  Object.entries(config).forEach(([userGroup, item]) => {
    if (!userGroup) return;
    const nextItem = {};
    if (item.global) {
      nextItem.global = normalizeLimitPair(item.global);
    }
    const groups = {};
    Object.entries(item.groups || {}).forEach(([groupName, limits]) => {
      if (!groupName) return;
      groups[groupName] = normalizeLimitPair(limits);
    });
    if (Object.keys(groups).length > 0) {
      nextItem.groups = groups;
    }
    if (nextItem.global || nextItem.groups) {
      result[userGroup] = nextItem;
    }
  });
  return Object.keys(result).length === 0
    ? ''
    : JSON.stringify(result, null, 2);
}

function UserGroupRateLimitSection({
  userGroup,
  config,
  groupOptions,
  onSetGlobal,
  onClearGlobal,
  onSetGroupLimit,
  onRemoveGroupLimit,
  onAddGroupLimit,
  onRemoveUserGroup,
  t,
}) {
  const [open, setOpen] = useState(false);
  const groupEntries = Object.entries(config.groups || {});
  const ruleCount = groupEntries.length + (config.global ? 1 : 0);

  return (
    <div
      style={{
        border: '1px solid var(--semi-color-border)',
        borderRadius: 8,
        overflow: 'hidden',
      }}
    >
      <div
        className='flex cursor-pointer items-center justify-between'
        style={{ padding: '8px 12px', background: 'var(--semi-color-fill-0)' }}
        onClick={() => setOpen(!open)}
      >
        <div className='flex items-center gap-2'>
          {open ? (
            <IconChevronUp size='small' />
          ) : (
            <IconChevronDown size='small' />
          )}
          <Text strong>{userGroup}</Text>
          <Tag size='small' color='blue'>
            {ruleCount} {t('条规则')}
          </Tag>
        </div>
        <div
          className='flex items-center gap-1'
          onClick={(e) => e.stopPropagation()}
        >
          <Button
            icon={<IconPlus />}
            size='small'
            theme='borderless'
            onClick={() => onAddGroupLimit(userGroup)}
          />
          <Popconfirm
            title={t('确认删除该用户组的特殊请求次数？')}
            onConfirm={() => onRemoveUserGroup(userGroup)}
            position='left'
          >
            <Button
              icon={<IconDelete />}
              size='small'
              type='danger'
              theme='borderless'
            />
          </Popconfirm>
        </div>
      </div>
      <Collapsible isOpen={open} keepDOM>
        <div className='space-y-3' style={{ padding: '12px' }}>
          <div
            className='flex flex-wrap items-end gap-2'
            style={{
              border: '1px solid var(--semi-color-border)',
              borderRadius: 6,
              padding: 12,
            }}
          >
            <div style={{ minWidth: 120 }}>
              <Text strong>{t('用户组全局')}</Text>
              <div>
                <Text type='tertiary' size='small'>
                  {t('不区分请求分组时生效')}
                </Text>
              </div>
            </div>
            <InputNumber
              size='small'
              min={0}
              max={2147483647}
              value={config.global?.[0] ?? 0}
              suffix={t('次')}
              style={{ width: 150 }}
              onChange={(value) =>
                onSetGlobal(userGroup, [
                  Number(value) || 0,
                  config.global?.[1] ?? 1,
                ])
              }
            />
            <InputNumber
              size='small'
              min={1}
              max={2147483647}
              value={config.global?.[1] ?? 1}
              suffix={t('成功次')}
              style={{ width: 150 }}
              onChange={(value) =>
                onSetGlobal(userGroup, [
                  config.global?.[0] ?? 0,
                  Math.max(1, Number(value) || 1),
                ])
              }
            />
            {config.global ? (
              <Button
                size='small'
                type='danger'
                theme='borderless'
                onClick={() => onClearGlobal(userGroup)}
              >
                {t('清除全局')}
              </Button>
            ) : (
              <Button
                size='small'
                theme='outline'
                onClick={() => onSetGlobal(userGroup, [0, 1])}
              >
                {t('启用全局')}
              </Button>
            )}
          </div>

          {groupEntries.length === 0 ? (
            <Text type='tertiary' className='block py-2 text-center'>
              {t('暂无请求分组规则')}
            </Text>
          ) : (
            groupEntries.map(([groupName, limits]) => (
              <div
                key={groupName}
                className='flex flex-wrap items-center gap-2'
              >
                <Select
                  size='small'
                  filter
                  allowCreate
                  placeholder={t('请求分组')}
                  value={groupName}
                  optionList={groupOptions}
                  style={{ minWidth: 180, flex: 1 }}
                  position='bottomLeft'
                  onChange={(nextName) =>
                    onSetGroupLimit(
                      userGroup,
                      groupName,
                      nextName,
                      normalizeLimitPair(limits),
                    )
                  }
                />
                <InputNumber
                  size='small'
                  min={0}
                  max={2147483647}
                  value={limits[0]}
                  suffix={t('次')}
                  style={{ width: 150 }}
                  onChange={(value) =>
                    onSetGroupLimit(userGroup, groupName, groupName, [
                      Number(value) || 0,
                      limits[1],
                    ])
                  }
                />
                <InputNumber
                  size='small'
                  min={1}
                  max={2147483647}
                  value={limits[1]}
                  suffix={t('成功次')}
                  style={{ width: 150 }}
                  onChange={(value) =>
                    onSetGroupLimit(userGroup, groupName, groupName, [
                      limits[0],
                      Math.max(1, Number(value) || 1),
                    ])
                  }
                />
                <Popconfirm
                  title={t('确认删除该规则？')}
                  onConfirm={() => onRemoveGroupLimit(userGroup, groupName)}
                  position='left'
                >
                  <Button
                    icon={<IconDelete />}
                    type='danger'
                    theme='borderless'
                    size='small'
                  />
                </Popconfirm>
              </div>
            ))
          )}
        </div>
      </Collapsible>
    </div>
  );
}

function UserGroupRateLimitRules({ value, groupNames = [], onChange }) {
  const { t } = useTranslation();
  const [config, setConfig] = useState(() =>
    normalizeUserGroupRateLimit(value),
  );
  const [newUserGroup, setNewUserGroup] = useState('');
  const [newRuleType, setNewRuleType] = useState('global');
  const [newRequestGroup, setNewRequestGroup] = useState('');

  useEffect(() => {
    setConfig(normalizeUserGroupRateLimit(value));
  }, [value]);

  const emitChange = useCallback(
    (nextConfig) => {
      setConfig(nextConfig);
      onChange?.(serializeUserGroupRateLimit(nextConfig));
    },
    [onChange],
  );

  const ensureUserGroup = useCallback((current, userGroup) => {
    const currentItem = current[userGroup] || { groups: {} };
    return {
      ...currentItem,
      groups: { ...(currentItem.groups || {}) },
    };
  }, []);

  const setGlobal = useCallback(
    (userGroup, limits) => {
      const nextConfig = { ...config };
      nextConfig[userGroup] = ensureUserGroup(nextConfig, userGroup);
      nextConfig[userGroup].global = normalizeLimitPair(limits);
      emitChange(nextConfig);
    },
    [config, emitChange, ensureUserGroup],
  );

  const clearGlobal = useCallback(
    (userGroup) => {
      const nextConfig = { ...config };
      nextConfig[userGroup] = ensureUserGroup(nextConfig, userGroup);
      delete nextConfig[userGroup].global;
      emitChange(nextConfig);
    },
    [config, emitChange, ensureUserGroup],
  );

  const setGroupLimit = useCallback(
    (userGroup, oldGroupName, nextGroupName, limits) => {
      const groupName = String(nextGroupName || '').trim();
      if (!groupName) return;
      const nextConfig = { ...config };
      nextConfig[userGroup] = ensureUserGroup(nextConfig, userGroup);
      if (oldGroupName !== groupName) {
        delete nextConfig[userGroup].groups[oldGroupName];
      }
      nextConfig[userGroup].groups[groupName] = normalizeLimitPair(limits);
      emitChange(nextConfig);
    },
    [config, emitChange, ensureUserGroup],
  );

  const removeGroupLimit = useCallback(
    (userGroup, groupName) => {
      const nextConfig = { ...config };
      nextConfig[userGroup] = ensureUserGroup(nextConfig, userGroup);
      delete nextConfig[userGroup].groups[groupName];
      emitChange(nextConfig);
    },
    [config, emitChange, ensureUserGroup],
  );

  const addGroupLimit = useCallback(
    (userGroup) => {
      const nextConfig = { ...config };
      nextConfig[userGroup] = ensureUserGroup(nextConfig, userGroup);
      let index = 1;
      let groupName = '';
      do {
        groupName = `group-${index}`;
        index += 1;
      } while (nextConfig[userGroup].groups[groupName]);
      nextConfig[userGroup].groups[groupName] = [0, 1];
      emitChange(nextConfig);
    },
    [config, emitChange, ensureUserGroup],
  );

  const removeUserGroup = useCallback(
    (userGroup) => {
      const nextConfig = { ...config };
      delete nextConfig[userGroup];
      emitChange(nextConfig);
    },
    [config, emitChange],
  );

  const addUserGroupRule = useCallback(() => {
    const userGroup = String(newUserGroup || '').trim();
    if (!userGroup) {
      showWarning(t('请先选择用户分组'));
      return;
    }
    const nextConfig = { ...config };
    nextConfig[userGroup] = ensureUserGroup(nextConfig, userGroup);
    if (newRuleType === 'global') {
      nextConfig[userGroup].global = [0, 1];
    } else {
      const requestGroup = String(newRequestGroup || '').trim();
      if (!requestGroup) {
        showWarning(t('请先选择渠道/请求分组'));
        return;
      }
      nextConfig[userGroup].groups[requestGroup] = [0, 1];
      setNewRequestGroup('');
    }
    emitChange(nextConfig);
    setNewUserGroup('');
  }, [
    config,
    emitChange,
    ensureUserGroup,
    newRequestGroup,
    newRuleType,
    newUserGroup,
    t,
  ]);

  const groupOptions = useMemo(
    () => groupNames.map((name) => ({ value: name, label: name })),
    [groupNames],
  );

  const userGroupOptions = useMemo(
    () =>
      Array.from(new Set([...groupNames, ...Object.keys(config)])).map(
        (name) => ({ value: name, label: name }),
      ),
    [config, groupNames],
  );

  const ruleTypeOptions = useMemo(
    () => [
      { value: 'global', label: t('用户组全局限制') },
      { value: 'group', label: t('指定渠道/请求分组') },
    ],
    [t],
  );

  const entries = Object.entries(config);

  return (
    <div className='space-y-3'>
      <Text type='tertiary' size='small' style={{ display: 'block' }}>
        {t('优先级：用户组请求分组 > 用户组全局 > 请求分组 > 全局默认值。')}
      </Text>
      {entries.length === 0 ? (
        <Text type='tertiary' className='block py-4 text-center'>
          {t('暂无规则，点击下方按钮添加')}
        </Text>
      ) : (
        entries.map(([userGroup, item]) => (
          <UserGroupRateLimitSection
            key={userGroup}
            userGroup={userGroup}
            config={item}
            groupOptions={groupOptions}
            onSetGlobal={setGlobal}
            onClearGlobal={clearGlobal}
            onSetGroupLimit={setGroupLimit}
            onRemoveGroupLimit={removeGroupLimit}
            onAddGroupLimit={addGroupLimit}
            onRemoveUserGroup={removeUserGroup}
            t={t}
          />
        ))
      )}
      <div className='mt-3 flex flex-wrap justify-center gap-2'>
        <Select
          size='small'
          filter
          allowCreate
          placeholder={t('选择用户分组')}
          optionList={userGroupOptions}
          value={newUserGroup || undefined}
          onChange={setNewUserGroup}
          style={{ width: 220 }}
          position='bottomLeft'
        />
        <Select
          size='small'
          value={newRuleType}
          optionList={ruleTypeOptions}
          onChange={setNewRuleType}
          style={{ width: 180 }}
          position='bottomLeft'
        />
        {newRuleType === 'group' && (
          <Select
            size='small'
            filter
            allowCreate
            placeholder={t('选择渠道/请求分组')}
            optionList={groupOptions}
            value={newRequestGroup || undefined}
            onChange={setNewRequestGroup}
            style={{ width: 220 }}
            position='bottomLeft'
          />
        )}
        <Button icon={<IconPlus />} theme='outline' onClick={addUserGroupRule}>
          {t('添加规则')}
        </Button>
      </div>
      <div
        style={{
          border: '1px solid var(--semi-color-border)',
          borderRadius: 6,
          padding: 12,
          background: 'var(--semi-color-fill-0)',
        }}
      >
        <Text strong>{t('JSON 预览')}</Text>
        <pre style={{ marginTop: 8, whiteSpace: 'pre-wrap' }}>
          {serializeUserGroupRateLimit(config) || '{}'}
        </pre>
      </div>
    </div>
  );
}

export default function RequestRateLimit(props) {
  const { t } = useTranslation();

  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    ModelRequestRateLimitEnabled: false,
    ModelRequestRateLimitCount: -1,
    ModelRequestRateLimitSuccessCount: 1000,
    ModelRequestRateLimitDurationMinutes: 1,
    ModelRequestRateLimitGroup: '',
    ModelRequestRateLimitUserGroup: '',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  const groupNames = useMemo(() => {
    const groupRateLimits = parseJSONSafe(
      inputs.ModelRequestRateLimitGroup,
      {},
    );
    const userGroupRateLimits = normalizeUserGroupRateLimit(
      inputs.ModelRequestRateLimitUserGroup,
    );
    return Array.from(
      new Set([
        ...Object.keys(groupRateLimits),
        ...Object.keys(userGroupRateLimits),
      ]),
    );
  }, [
    inputs.ModelRequestRateLimitGroup,
    inputs.ModelRequestRateLimitUserGroup,
  ]);

  const handleGroupRateLimitChange = useCallback((value) => {
    setInputs((current) => ({
      ...current,
      ModelRequestRateLimitGroup: value,
    }));
    refForm.current?.setValue('ModelRequestRateLimitGroup', value);
  }, []);

  const handleUserGroupRateLimitChange = useCallback((value) => {
    setInputs((current) => ({
      ...current,
      ModelRequestRateLimitUserGroup: value,
    }));
    refForm.current?.setValue('ModelRequestRateLimitUserGroup', value);
  }, []);

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof inputs[item.key] === 'boolean') {
        value = String(inputs[item.key]);
      } else {
        value = inputs[item.key];
      }
      return API.put('/api/option/', {
        key: item.key,
        value,
      });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (requestQueue.length === 1) {
          if (res.includes(undefined)) return;
        } else if (requestQueue.length > 1) {
          if (res.includes(undefined))
            return showError(t('部分保存失败，请重试'));
        }

        for (let i = 0; i < res.length; i++) {
          if (!res[i].data.success) {
            return showError(res[i].data.message);
          }
        }

        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => {
        showError(t('保存失败，请重试'));
      })
      .finally(() => {
        setLoading(false);
      });
  }

  useEffect(() => {
    const currentInputs = {};
    for (let key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs(currentInputs);
    setInputsRow(structuredClone(currentInputs));
    refForm.current?.setValues(currentInputs);
  }, [props.options]);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('模型请求速率限制')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'ModelRequestRateLimitEnabled'}
                  label={t('启用用户模型请求速率限制（可能会影响高并发性能）')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitEnabled: value,
                    });
                  }}
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('限制周期')}
                  step={1}
                  min={0}
                  suffix={t('分钟')}
                  extraText={t('频率限制的周期（分钟）')}
                  field={'ModelRequestRateLimitDurationMinutes'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitDurationMinutes: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求次数')}
                  step={1}
                  min={0}
                  max={100000000}
                  suffix={t('次')}
                  extraText={t('包括失败请求的次数，0代表不限制')}
                  field={'ModelRequestRateLimitCount'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitCount: String(value),
                    })
                  }
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.InputNumber
                  label={t('用户每周期最多请求完成次数')}
                  step={1}
                  min={1}
                  max={100000000}
                  suffix={t('次')}
                  extraText={t('只包括请求成功的次数')}
                  field={'ModelRequestRateLimitSuccessCount'}
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      ModelRequestRateLimitSuccessCount: String(value),
                    })
                  }
                />
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.Slot label={t('分组速率限制')}>
                  <GroupRateLimitRules
                    value={inputs.ModelRequestRateLimitGroup}
                    onChange={handleGroupRateLimitChange}
                  />
                  <Form.TextArea
                    field={'ModelRequestRateLimitGroup'}
                    noLabel
                    style={{ display: 'none' }}
                    trigger='blur'
                    stopValidateWithError
                    rules={[
                      {
                        validator: (rule, value) => !value || verifyJSON(value),
                        message: t('不是合法的 JSON 字符串'),
                      },
                    ]}
                  />
                  <div style={{ marginTop: 8 }}>
                    <Text type='tertiary' size='small'>
                      {t(
                        '分组速率配置优先级高于全局速率限制，限制周期统一使用上方配置的限制周期值。',
                      )}
                    </Text>
                  </div>
                </Form.Slot>
              </Col>
            </Row>
            <Row>
              <Col xs={24} sm={16}>
                <Form.Slot label={t('用户组特殊请求次数')}>
                  <UserGroupRateLimitRules
                    value={inputs.ModelRequestRateLimitUserGroup}
                    groupNames={groupNames}
                    onChange={handleUserGroupRateLimitChange}
                  />
                  <Form.TextArea
                    field={'ModelRequestRateLimitUserGroup'}
                    noLabel
                    style={{ display: 'none' }}
                    trigger='blur'
                    stopValidateWithError
                    rules={[
                      {
                        validator: (rule, value) => !value || verifyJSON(value),
                        message: t('不是合法的 JSON 字符串'),
                      },
                    ]}
                  />
                  <div style={{ marginTop: 8 }}>
                    <Text type='tertiary' size='small'>
                      {t(
                        '按用户组覆盖请求次数，global 可省略，groups 可省略。',
                      )}
                    </Text>
                  </div>
                </Form.Slot>
              </Col>
            </Row>
            <Row>
              <Button size='default' onClick={onSubmit}>
                {t('保存模型速率限制')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
