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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Col,
  Empty,
  Form,
  Input,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const ACTION_MASK = 'mask';
const ACTION_BLOCK = 'block';
const SCOPE_REQUEST = 'request';
const SCOPE_RESPONSE = 'response';
const SCOPE_BOTH = 'both';
const DEFAULT_REPLACEMENT = '[REDACTED]';
const SENSITIVE_WORD_GROUP_TYPE = 'sensitive_word';

function createLocalId() {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return `sensitive-rule-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function splitKeywords(value) {
  const seen = new Set();
  const keywords = [];

  String(value || '')
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase();
      if (seen.has(key)) return;
      seen.add(key);
      keywords.push(item);
    });

  return keywords;
}

function normalizeChannelIds(channelIds) {
  const seen = new Set();
  const normalized = [];

  (channelIds || []).forEach((item) => {
    const id =
      typeof item === 'number' ? item : Number.parseInt(String(item), 10);
    if (!Number.isInteger(id) || id <= 0 || seen.has(id)) return;
    seen.add(id);
    normalized.push(id);
  });

  return normalized.sort((a, b) => a - b);
}

function parseChannelIds(raw) {
  const trimmed = String(raw || '').trim();
  if (!trimmed) return [];

  try {
    const parsed = JSON.parse(trimmed);
    return Array.isArray(parsed) ? normalizeChannelIds(parsed) : [];
  } catch {
    return [];
  }
}

function serializeChannelIds(channelIds) {
  return JSON.stringify(normalizeChannelIds(channelIds));
}

function createRule() {
  return {
    id: createLocalId(),
    name: '',
    enabled: true,
    action: ACTION_MASK,
    scope: SCOPE_REQUEST,
    replacement: DEFAULT_REPLACEMENT,
    keywordsText: '',
    groupRefs: [],
  };
}

function normalizeGroupRefs(groupRefs) {
  const seen = new Set();
  const normalized = [];

  (groupRefs || [])
    .map((item) => String(item).trim())
    .filter(Boolean)
    .forEach((item) => {
      const key = item.toLowerCase();
      if (seen.has(key)) return;
      seen.add(key);
      normalized.push(item);
    });

  return normalized;
}

function normalizeRule(rule) {
  const keywords = splitKeywords(rule.keywordsText);
  const groupRefs = normalizeGroupRefs(rule.groupRefs);
  if (keywords.length === 0 && groupRefs.length === 0) return null;

  const action = rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK;
  const scope =
    rule.scope === SCOPE_RESPONSE || rule.scope === SCOPE_BOTH
      ? rule.scope
      : SCOPE_REQUEST;
  const fallbackName = keywords[0] || groupRefs[0] || '';

  return {
    id: rule.id || fallbackName.toLowerCase() || createLocalId(),
    name: String(rule.name || '').trim() || fallbackName,
    enabled: rule.enabled !== false,
    action,
    scope,
    replacement:
      action === ACTION_MASK
        ? String(rule.replacement || '').trim() || DEFAULT_REPLACEMENT
        : undefined,
    keywords,
    group_refs: groupRefs.length > 0 ? groupRefs : undefined,
  };
}

function rulesToDrafts(rules) {
  return (rules || []).map((rule) => ({
    id: rule.id || createLocalId(),
    name: rule.name || '',
    enabled: rule.enabled !== false,
    action: rule.action === ACTION_BLOCK ? ACTION_BLOCK : ACTION_MASK,
    scope:
      rule.scope === SCOPE_RESPONSE || rule.scope === SCOPE_BOTH
        ? rule.scope
        : SCOPE_REQUEST,
    replacement: rule.replacement || DEFAULT_REPLACEMENT,
    keywordsText: (rule.keywords || []).join('\n'),
    groupRefs: normalizeGroupRefs(rule.group_refs),
  }));
}

function serializeRules(rules) {
  return JSON.stringify(
    {
      rules: (rules || [])
        .map((rule) => normalizeRule(rule))
        .filter((rule) => rule !== null),
    },
    null,
    2,
  );
}

function parseRulesConfig(raw, legacyWords) {
  const trimmed = String(raw || '').trim();
  if (trimmed) {
    try {
      const parsed = JSON.parse(trimmed);
      if (Array.isArray(parsed.rules) && parsed.rules.length > 0) {
        return rulesToDrafts(parsed.rules);
      }
    } catch {
      return [];
    }
  }

  const legacyKeywords = splitKeywords(legacyWords);
  if (legacyKeywords.length === 0) return [];

  return rulesToDrafts([
    {
      id: 'legacy-sensitive-words',
      name: 'Legacy sensitive words',
      enabled: true,
      action: ACTION_BLOCK,
      keywords: legacyKeywords,
    },
  ]);
}

function getChannelLabel(channel) {
  return channel?.name?.trim() || `#${channel?.id}`;
}

function getPrefillGroupLabel(group) {
  return group?.name?.trim() || `#${group?.id}`;
}

export default function SettingsSensitiveWords(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [channelsLoading, setChannelsLoading] = useState(false);
  const [channels, setChannels] = useState([]);
  const [sensitiveGroupsLoading, setSensitiveGroupsLoading] = useState(false);
  const [sensitiveGroups, setSensitiveGroups] = useState([]);
  const [inputs, setInputs] = useState({
    CheckSensitiveEnabled: false,
    CheckSensitiveOnPromptEnabled: false,
    SensitiveWords: '',
    SensitiveRules: '{"rules":[]}',
    SensitiveRuleChannelIds: '[]',
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);
  const [rules, setRules] = useState([]);
  const [selectedChannelIds, setSelectedChannelIds] = useState([]);

  const currentRulesValue = useMemo(() => serializeRules(rules), [rules]);
  const currentChannelIdsValue = useMemo(
    () => serializeChannelIds(selectedChannelIds),
    [selectedChannelIds],
  );
  const selectedChannelSummary =
    selectedChannelIds.length === 0
      ? t('不应用任何渠道')
      : selectedChannelIds.length === 1
        ? getChannelLabel(
            channels.find(
              (channel) => channel.id === selectedChannelIds[0],
            ) || {
              id: selectedChannelIds[0],
            },
          )
        : t('已选择 {{count}} 个渠道', { count: selectedChannelIds.length });

  const fetchChannels = async () => {
    setChannelsLoading(true);
    try {
      const res = await API.get('/api/ratio_sync/channels');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      const sortedChannels = [...(data || [])].sort((a, b) => {
        const nameCompare = getChannelLabel(a).localeCompare(
          getChannelLabel(b),
        );
        return nameCompare === 0 ? a.id - b.id : nameCompare;
      });
      setChannels(sortedChannels);
    } catch {
      showError(t('获取渠道列表失败'));
    } finally {
      setChannelsLoading(false);
    }
  };

  const fetchSensitiveGroups = async () => {
    setSensitiveGroupsLoading(true);
    try {
      const res = await API.get(
        `/api/prefill_group?type=${SENSITIVE_WORD_GROUP_TYPE}`,
      );
      const { success, message, data } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      const sortedGroups = [...(data || [])].sort((a, b) =>
        getPrefillGroupLabel(a).localeCompare(getPrefillGroupLabel(b)),
      );
      setSensitiveGroups(sortedGroups);
    } catch {
      showError(t('获取屏蔽词组失败'));
    } finally {
      setSensitiveGroupsLoading(false);
    }
  };

  const updateRule = (id, patch) => {
    setRules((prev) =>
      prev.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule)),
    );
  };

  const resetFormState = (values) => {
    const nextInputs = {
      CheckSensitiveEnabled: false,
      CheckSensitiveOnPromptEnabled: false,
      SensitiveWords: '',
      SensitiveRules: '{"rules":[]}',
      SensitiveRuleChannelIds: '[]',
      ...values,
    };

    setInputs(nextInputs);
    setInputsRow({ ...nextInputs });
    setRules(
      parseRulesConfig(nextInputs.SensitiveRules, nextInputs.SensitiveWords),
    );
    setSelectedChannelIds(parseChannelIds(nextInputs.SensitiveRuleChannelIds));
    refForm.current?.setValues(nextInputs);
  };

  function onSubmit() {
    const submitInputs = {
      ...inputs,
      SensitiveWords: '',
      SensitiveRules: currentRulesValue,
      SensitiveRuleChannelIds: currentChannelIdsValue,
    };
    const updateArray = Object.keys(submitInputs)
      .filter((key) => submitInputs[key] !== inputsRow[key])
      .map((key) => ({ key, value: submitInputs[key] }));

    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      let value = '';
      if (typeof submitInputs[item.key] === 'boolean') {
        value = String(submitInputs[item.key]);
      } else {
        value = submitInputs[item.key];
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
        showSuccess(t('保存成功'));
        setInputs(submitInputs);
        setInputsRow({ ...submitInputs });
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
    resetFormState(currentInputs);
  }, [props.options]);

  useEffect(() => {
    fetchChannels();
    fetchSensitiveGroups();
  }, []);

  return (
    <>
      <Spin spinning={loading}>
        <Form
          values={inputs}
          getFormApi={(formAPI) => (refForm.current = formAPI)}
          style={{ marginBottom: 15 }}
        >
          <Form.Section text={t('屏蔽词过滤设置')}>
            <Row gutter={16}>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'CheckSensitiveEnabled'}
                  label={t('启用屏蔽词过滤功能')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) => {
                    setInputs({
                      ...inputs,
                      CheckSensitiveEnabled: value,
                    });
                  }}
                />
              </Col>
              <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                <Form.Switch
                  field={'CheckSensitiveOnPromptEnabled'}
                  label={t('启用 Prompt 检查')}
                  size='default'
                  checkedText='｜'
                  uncheckedText='〇'
                  onChange={(value) =>
                    setInputs({
                      ...inputs,
                      CheckSensitiveOnPromptEnabled: value,
                    })
                  }
                />
              </Col>
            </Row>
            <Row gutter={16}>
              <Col xs={24} sm={24} md={16} lg={14} xl={12}>
                <div style={{ marginBottom: 16 }}>
                  <Typography.Text strong>{t('应用渠道')}</Typography.Text>
                  <Select
                    multiple
                    filter
                    loading={channelsLoading}
                    value={selectedChannelIds}
                    placeholder={t('不应用任何渠道')}
                    emptyContent={t('暂无渠道')}
                    style={{ width: '100%', marginTop: 8 }}
                    onChange={(value) => {
                      setSelectedChannelIds(
                        normalizeChannelIds(Array.isArray(value) ? value : []),
                      );
                    }}
                  >
                    {channels.map((channel) => (
                      <Select.Option key={channel.id} value={channel.id}>
                        {getChannelLabel(channel)} #{channel.id}
                      </Select.Option>
                    ))}
                  </Select>
                  <div
                    style={{ marginTop: 6, color: 'var(--semi-color-text-2)' }}
                  >
                    {selectedChannelSummary}，{t('空选择表示不应用任何渠道')}
                  </div>
                </div>
              </Col>
            </Row>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 12,
              }}
            >
              <div>
                <Typography.Title heading={6} style={{ margin: 0 }}>
                  {t('过滤规则')}
                </Typography.Title>
                <Typography.Text type='tertiary'>
                  {t(
                    '每条规则可独立选择请求、返回或全部范围，并执行脱敏或拦截',
                  )}
                </Typography.Text>
              </div>
              <Button
                icon={<IconPlus />}
                onClick={() => setRules((prev) => [...prev, createRule()])}
              >
                {t('新增规则')}
              </Button>
            </div>

            {rules.length === 0 ? (
              <Empty
                title={t('暂无屏蔽词规则')}
                description={t('新增规则后可配置关键词、脱敏或拦截')}
                style={{ padding: 24 }}
              />
            ) : (
              <Space
                vertical
                align='start'
                spacing='medium'
                style={{ width: '100%' }}
              >
                {rules.map((rule, index) => (
                  <div
                    key={rule.id}
                    style={{
                      width: '100%',
                      border: '1px solid var(--semi-color-border)',
                      borderRadius: 8,
                      padding: 16,
                    }}
                  >
                    <Row gutter={12} type='flex' align='middle'>
                      <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                        <Space>
                          <Tag color={rule.enabled ? 'green' : 'grey'}>
                            {rule.enabled ? t('已启用') : t('已禁用')}
                          </Tag>
                          <Typography.Text type='tertiary'>
                            {t('规则 {{number}}', { number: index + 1 })}
                          </Typography.Text>
                        </Space>
                      </Col>
                      <Col xs={24} sm={12} md={16} lg={18} xl={18}>
                        <Space style={{ float: 'right' }}>
                          <Switch
                            checked={rule.enabled}
                            checkedText='｜'
                            uncheckedText='〇'
                            onChange={(enabled) =>
                              updateRule(rule.id, { enabled })
                            }
                          />
                          <Button
                            type='danger'
                            theme='borderless'
                            icon={<IconDelete />}
                            onClick={() =>
                              setRules((prev) =>
                                prev.filter((item) => item.id !== rule.id),
                              )
                            }
                          >
                            {t('删除')}
                          </Button>
                        </Space>
                      </Col>
                    </Row>

                    <Row gutter={16} style={{ marginTop: 12 }}>
                      <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                        <Typography.Text strong>
                          {t('规则名称')}
                        </Typography.Text>
                        <Input
                          value={rule.name}
                          placeholder={t('规则名称')}
                          style={{ marginTop: 8 }}
                          onChange={(value) =>
                            updateRule(rule.id, { name: value })
                          }
                        />
                      </Col>
                      <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                        <Typography.Text strong>
                          {t('处理动作')}
                        </Typography.Text>
                        <Select
                          value={rule.action}
                          style={{ width: '100%', marginTop: 8 }}
                          onChange={(value) =>
                            updateRule(rule.id, {
                              action:
                                value === ACTION_BLOCK
                                  ? ACTION_BLOCK
                                  : ACTION_MASK,
                              replacement:
                                value === ACTION_MASK
                                  ? rule.replacement || DEFAULT_REPLACEMENT
                                  : rule.replacement,
                            })
                          }
                        >
                          <Select.Option value={ACTION_MASK}>
                            {t('脱敏')}
                          </Select.Option>
                          <Select.Option value={ACTION_BLOCK}>
                            {t('拦截')}
                          </Select.Option>
                        </Select>
                      </Col>
                      <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                        <Typography.Text strong>
                          {t('应用范围')}
                        </Typography.Text>
                        <Select
                          value={rule.scope}
                          style={{ width: '100%', marginTop: 8 }}
                          onChange={(value) =>
                            updateRule(rule.id, {
                              scope:
                                value === SCOPE_RESPONSE || value === SCOPE_BOTH
                                  ? value
                                  : SCOPE_REQUEST,
                            })
                          }
                        >
                          <Select.Option value={SCOPE_REQUEST}>
                            {t('请求')}
                          </Select.Option>
                          <Select.Option value={SCOPE_RESPONSE}>
                            {t('返回')}
                          </Select.Option>
                          <Select.Option value={SCOPE_BOTH}>
                            {t('全部')}
                          </Select.Option>
                        </Select>
                      </Col>
                      {rule.action === ACTION_MASK ? (
                        <Col xs={24} sm={12} md={8} lg={6} xl={6}>
                          <Typography.Text strong>
                            {t('替换文本')}
                          </Typography.Text>
                          <Input
                            value={rule.replacement}
                            placeholder={DEFAULT_REPLACEMENT}
                            style={{ marginTop: 8 }}
                            onChange={(value) =>
                              updateRule(rule.id, { replacement: value })
                            }
                          />
                        </Col>
                      ) : null}
                    </Row>

                    <Row style={{ marginTop: 12 }}>
                      <Col xs={24} sm={24} md={16} lg={14} xl={12}>
                        <Typography.Text strong>{t('关键词')}</Typography.Text>
                        <TextArea
                          value={rule.keywordsText}
                          placeholder={t('一行一个关键词')}
                          autosize={{ minRows: 4, maxRows: 8 }}
                          style={{
                            marginTop: 8,
                            fontFamily: 'JetBrains Mono, Consolas',
                          }}
                          onChange={(value) =>
                            updateRule(rule.id, { keywordsText: value })
                          }
                        />
                        <div
                          style={{
                            marginTop: 6,
                            color: 'var(--semi-color-text-2)',
                          }}
                        >
                          {t('空行和重复关键词会被自动忽略')}
                        </div>
                      </Col>
                    </Row>

                    <Row style={{ marginTop: 12 }}>
                      <Col xs={24} sm={24} md={16} lg={14} xl={12}>
                        <Typography.Text strong>
                          {t('分组引用')}
                        </Typography.Text>
                        <Select
                          multiple
                          filter
                          loading={sensitiveGroupsLoading}
                          value={rule.groupRefs || []}
                          placeholder={t('选择屏蔽词组')}
                          emptyContent={t('暂无屏蔽词组')}
                          style={{ width: '100%', marginTop: 8 }}
                          onChange={(value) =>
                            updateRule(rule.id, {
                              groupRefs: normalizeGroupRefs(
                                Array.isArray(value) ? value : [],
                              ),
                            })
                          }
                        >
                          {sensitiveGroups.map((group) => (
                            <Select.Option
                              key={group.id}
                              value={String(group.id)}
                            >
                              {getPrefillGroupLabel(group)} #{group.id}
                            </Select.Option>
                          ))}
                        </Select>
                        <div
                          style={{
                            marginTop: 6,
                            color: 'var(--semi-color-text-2)',
                          }}
                        >
                          {t('引用的分组会和上方手动关键词一起生效')}
                        </div>
                      </Col>
                    </Row>
                  </div>
                ))}
              </Space>
            )}
            <Row>
              <Button
                size='default'
                onClick={onSubmit}
                style={{ marginTop: 16 }}
              >
                {t('保存屏蔽词过滤设置')}
              </Button>
            </Row>
          </Form.Section>
        </Form>
      </Spin>
    </>
  );
}
