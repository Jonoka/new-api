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

import React, { useEffect, useState, useContext, useRef } from 'react';
import {
  API,
  showError,
  showSuccess,
  timestamp2string,
  getCurrencyConfig,
  getModelCategories,
  selectFilter,
  buildGroupSelectionPayload,
  createUserGroupOptions,
  includeSelectedGroupOptions,
  resolveGroupCodes,
} from '../../../../helpers';
import {
  quotaToDisplayAmount,
  displayAmountToQuota,
} from '../../../../helpers/quota';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import {
  Button,
  SideSheet,
  Space,
  Spin,
  Typography,
  Card,
  Tag,
  TagGroup,
  Avatar,
  Form,
  Col,
  Row,
  InputNumber,
  Popover,
  Input,
  Empty,
} from '@douyinfe/semi-ui';
import {
  IconCreditCard,
  IconLink,
  IconSave,
  IconClose,
  IconKey,
  IconPlus,
  IconDelete,
  IconSearch,
} from '@douyinfe/semi-icons';
import { GripVertical } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { StatusContext } from '../../../../context/Status';

const { Text, Title } = Typography;

// ============================================================================
// GroupMultiPicker — 多分组选择 + 排序组件（Semi Design 风格）
// ============================================================================

const GroupMultiPicker = ({
  groups,
  selectedGroups,
  onChange,
  groupRatioLimits,
  onGroupRatioLimitChange,
  t,
}) => {
  const [popVisible, setPopVisible] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [draggedGroup, setDraggedGroup] = useState(null);
  const [dragOverGroup, setDragOverGroup] = useState(null);

  const isAutoSelected = selectedGroups.includes('auto');
  const isExclusiveSelected = groups.some(
    (group) => group.exclusive === true && selectedGroups.includes(group.value),
  );

  const availableGroups = groups.filter((g) => {
    if (isExclusiveSelected) return false;
    if (selectedGroups.includes(g.value)) return false;
    if (isAutoSelected && g.value !== 'auto') return false;
    if (g.value === 'auto' && selectedGroups.length > 0 && !isAutoSelected)
      return false;
    if (g.exclusive === true && selectedGroups.length > 0) return false;
    if (searchText) {
      const q = searchText.toLowerCase();
      return (
        g.value.toLowerCase().includes(q) ||
        (typeof g.label === 'string' && g.label.toLowerCase().includes(q))
      );
    }
    return true;
  });

  const handleAdd = (value) => {
    const selectedOption = groups.find((group) => group.value === value);
    if (selectedOption?.exclusive === true) {
      onChange([value]);
    } else if (value === 'auto') {
      onChange(['auto']);
    } else {
      onChange([...selectedGroups.filter((v) => v !== 'auto'), value]);
    }
    setSearchText('');
  };

  const handleRemove = (value) => {
    onChange(selectedGroups.filter((v) => v !== value));
  };

  const handleMove = (index, direction) => {
    const newGroups = [...selectedGroups];
    const targetIndex = index + direction;
    if (targetIndex < 0 || targetIndex >= newGroups.length) return;
    [newGroups[index], newGroups[targetIndex]] = [
      newGroups[targetIndex],
      newGroups[index],
    ];
    onChange(newGroups);
  };

  const resetDragState = () => {
    setDraggedGroup(null);
    setDragOverGroup(null);
  };

  const handleDragStart = (event, value) => {
    setDraggedGroup(value);
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', value);
  };

  const handleDragOver = (event, value) => {
    event.preventDefault();
    if (!draggedGroup || draggedGroup === value) return;
    event.dataTransfer.dropEffect = 'move';
    setDragOverGroup(value);
  };

  const handleDrop = (event, targetValue) => {
    event.preventDefault();
    const sourceValue =
      draggedGroup || event.dataTransfer.getData('text/plain');
    if (!sourceValue || sourceValue === targetValue) {
      resetDragState();
      return;
    }

    const sourceIndex = selectedGroups.indexOf(sourceValue);
    const targetIndex = selectedGroups.indexOf(targetValue);
    if (sourceIndex < 0 || targetIndex < 0) {
      resetDragState();
      return;
    }

    const reorderedGroups = [...selectedGroups];
    const [movedGroup] = reorderedGroups.splice(sourceIndex, 1);
    reorderedGroups.splice(targetIndex, 0, movedGroup);
    onChange(reorderedGroups);
    resetDragState();
  };

  const groupMap = {};
  groups.forEach((g) => {
    groupMap[g.value] = g;
  });

  const renderRatioBadge = (ratio) => {
    if (ratio === undefined || ratio === null || ratio === '') return null;
    return (
      <Tag size='small' color='green' shape='circle' style={{ marginLeft: 4 }}>
        {ratio}x
      </Tag>
    );
  };

  return (
    <div>
      {/* Selected groups list */}
      {selectedGroups.length > 0 && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 6,
            marginBottom: 8,
          }}
        >
          {selectedGroups.map((value, index) => {
            const info = groupMap[value];
            const displayName = info?.label || value;
            return (
              <div
                key={value}
                onDragOver={(event) => handleDragOver(event, value)}
                onDrop={(event) => handleDrop(event, value)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '6px 10px',
                  borderRadius: 8,
                  border:
                    dragOverGroup === value
                      ? '1px solid var(--semi-color-primary)'
                      : '1px solid var(--semi-color-border)',
                  backgroundColor: 'var(--semi-color-bg-2)',
                  opacity: draggedGroup === value ? 0.55 : 1,
                  transition: 'border-color 0.15s, opacity 0.15s',
                }}
              >
                {/* 拖拽手柄；键盘上下键同时提供无障碍排序能力 */}
                {selectedGroups.length > 1 && (
                  <span
                    draggable
                    role='button'
                    tabIndex={0}
                    aria-label={t('排序')}
                    title={t('排序')}
                    onDragStart={(event) => handleDragStart(event, value)}
                    onDragEnd={resetDragState}
                    onKeyDown={(event) => {
                      if (event.key === 'ArrowUp') {
                        event.preventDefault();
                        handleMove(index, -1);
                      } else if (event.key === 'ArrowDown') {
                        event.preventDefault();
                        handleMove(index, 1);
                      }
                    }}
                    style={{
                      display: 'inline-flex',
                      alignItems: 'center',
                      color: 'var(--semi-color-text-2)',
                      cursor: 'grab',
                      flexShrink: 0,
                    }}
                  >
                    <GripVertical size={16} />
                  </span>
                )}
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div
                    style={{ display: 'flex', alignItems: 'center', gap: 4 }}
                  >
                    <Text
                      strong
                      size='small'
                      ellipsis={{ showTooltip: true }}
                      style={{ maxWidth: 200 }}
                    >
                      {displayName}
                    </Text>
                    {info && renderRatioBadge(info.ratio)}
                    {info?.exclusive && (
                      <Tag size='small' color='purple' shape='circle'>
                        {t('独立')}
                      </Tag>
                    )}
                  </div>
                  {info?.description && (
                    <Text
                      type='tertiary'
                      size='small'
                      ellipsis={{ showTooltip: true }}
                      style={{ maxWidth: 300 }}
                    >
                      {info.description}
                    </Text>
                  )}
                </div>
                {!isAutoSelected && (
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                      flexShrink: 0,
                    }}
                  >
                    <Text type='tertiary' size='small'>
                      {t('倍率保护')}
                    </Text>
                    <InputNumber
                      size='small'
                      min={0}
                      step={0.1}
                      precision={4}
                      placeholder={t('不限制')}
                      value={groupRatioLimits?.[value]}
                      onNumberChange={(ratio) =>
                        onGroupRatioLimitChange?.(value, ratio)
                      }
                      style={{ width: 96 }}
                    />
                  </div>
                )}
                <Button
                  icon={<IconDelete size='small' />}
                  size='small'
                  theme='borderless'
                  type='danger'
                  onClick={() => handleRemove(value)}
                  style={{ flexShrink: 0 }}
                />
              </div>
            );
          })}
        </div>
      )}

      {/* Add button with popover */}
      <Popover
        visible={popVisible}
        onVisibleChange={setPopVisible}
        trigger='click'
        position='bottomLeft'
        showArrow
        content={
          <div
            style={{ width: 320, maxHeight: 360, overflow: 'auto', padding: 8 }}
          >
            <Input
              prefix={<IconSearch />}
              placeholder={t('搜索分组...')}
              value={searchText}
              onChange={setSearchText}
              showClear
              size='small'
              style={{ marginBottom: 8 }}
            />
            {availableGroups.length === 0 ? (
              <Empty description={t('没有可选分组')} style={{ padding: 16 }} />
            ) : (
              availableGroups.map((g) => (
                <div
                  key={g.value}
                  onClick={() => {
                    handleAdd(g.value);
                    setPopVisible(false);
                  }}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    padding: '8px 12px',
                    borderRadius: 6,
                    cursor: 'pointer',
                    transition: 'background 0.15s',
                  }}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.backgroundColor =
                      'var(--semi-color-fill-0)';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.backgroundColor = 'transparent';
                  }}
                >
                  <div>
                    <Text strong size='small'>
                      {g.label || g.value}
                    </Text>
                    {g.description && (
                      <div>
                        <Text type='tertiary' size='small'>
                          {g.description}
                        </Text>
                      </div>
                    )}
                  </div>
                  {renderRatioBadge(g.ratio)}
                  {g.exclusive && (
                    <Tag size='small' color='purple' shape='circle'>
                      {t('独立')}
                    </Tag>
                  )}
                </div>
              ))
            )}
          </div>
        }
      >
        <Button
          icon={<IconPlus />}
          theme='light'
          type='tertiary'
          size='small'
          disabled={isAutoSelected || isExclusiveSelected}
        >
          {selectedGroups.length === 0 ? t('选择分组') : t('添加分组')}
        </Button>
      </Popover>

      {/* Hint */}
      {selectedGroups.length > 1 && !isAutoSelected && !isExclusiveSelected && (
        <div style={{ marginTop: 6 }}>
          <Text type='tertiary' size='small'>
            {t('多个分组包含相同模型时，将按排列顺序依次尝试')}
          </Text>
        </div>
      )}
      {selectedGroups.length > 1 && isExclusiveSelected && (
        <div style={{ marginTop: 6 }}>
          <Text type='danger' size='small'>
            {t('独立分组必须单独选择')}
          </Text>
        </div>
      )}
    </div>
  );
};

// ============================================================================
// Main Component
// ============================================================================

const EditTokenModal = (props) => {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();
  const formApiRef = useRef(null);
  const [models, setModels] = useState([]);
  const [groups, setGroups] = useState([]);
  const [showQuotaInput, setShowQuotaInput] = useState(false);
  const isEdit = props.editingToken.id !== undefined;
  const defaultUseAutoGroup =
    statusState?.status?.default_use_auto_group === true;
  const [selectedGroups, setSelectedGroups] = useState(() =>
    defaultUseAutoGroup ? ['auto'] : [],
  );
  const [groupRatioLimits, setGroupRatioLimits] = useState({});

  const getInitValues = () => ({
    name: '',
    remain_quota: 0,
    remain_amount: 0,
    expired_time: -1,
    unlimited_quota: true,
    model_limits_enabled: false,
    model_limits: [],
    allow_ips: '',
    cross_group_retry: defaultUseAutoGroup,
    tokenCount: 1,
  });

  const handleCancel = () => {
    props.handleClose();
  };

  const parseGroupRatioLimits = (raw) => {
    if (!raw || typeof raw !== 'string') return {};
    try {
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return {};
      }
      return Object.entries(parsed).reduce((acc, [group, ratio]) => {
        const normalizedGroup = String(group).trim();
        const numericRatio = Number(ratio);
        if (
          normalizedGroup &&
          Number.isFinite(numericRatio) &&
          numericRatio > 0
        ) {
          acc[normalizedGroup] = numericRatio;
        }
        return acc;
      }, {});
    } catch (error) {
      return {};
    }
  };

  const cleanGroupRatioLimits = (limits, groupsForToken = selectedGroups) => {
    const selectedSet = new Set(groupsForToken);
    return Object.entries(limits || {}).reduce((acc, [group, ratio]) => {
      const numericRatio = Number(ratio);
      if (
        group &&
        selectedSet.has(group) &&
        group !== 'auto' &&
        Number.isFinite(numericRatio) &&
        numericRatio > 0
      ) {
        acc[group] = numericRatio;
      }
      return acc;
    }, {});
  };

  const onSelectedGroupsChange = (groupsForToken) => {
    setSelectedGroups(groupsForToken);
    setGroupRatioLimits((prev) => cleanGroupRatioLimits(prev, groupsForToken));
  };

  const handleGroupRatioLimitChange = (group, ratio) => {
    setGroupRatioLimits((prev) => {
      const next = { ...prev };
      const numericRatio = Number(ratio);
      if (!Number.isFinite(numericRatio) || numericRatio <= 0) {
        delete next[group];
      } else {
        next[group] = numericRatio;
      }
      return cleanGroupRatioLimits(next, selectedGroups);
    });
  };

  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (!formApiRef.current) return;
    if (seconds !== 0) {
      timestamp += seconds;
      formApiRef.current.setValue('expired_time', timestamp2string(timestamp));
    } else {
      formApiRef.current.setValue('expired_time', -1);
    }
  };

  const loadModels = async () => {
    let res = await API.get(`/api/user/models`);
    const { success, message, data } = res.data;
    if (success) {
      const categories = getModelCategories(t);
      let localModelOptions = data.map((model) => {
        let icon = null;
        for (const [key, category] of Object.entries(categories)) {
          if (key !== 'all' && category.filter({ model_name: model })) {
            icon = category.icon;
            break;
          }
        }
        return {
          label: (
            <span className='flex items-center gap-1'>
              {icon}
              {model}
            </span>
          ),
          value: model,
        };
      });
      setModels(localModelOptions);
    } else {
      showError(t(message));
    }
  };

  const loadGroups = async () => {
    try {
      const res = await API.get(`/api/user/self/groups`);
      if (!res?.data) return groups;
      const { success, message, data } = res.data;
      if (success) {
        const localGroupOptions = createUserGroupOptions(data);
        if (defaultUseAutoGroup) {
          if (localGroupOptions.some((group) => group.value === 'auto')) {
            localGroupOptions.sort((a, b) =>
              a.value === 'auto' ? -1 : b.value === 'auto' ? 1 : 0,
            );
          }
        }
        setGroups(localGroupOptions);
        return localGroupOptions;
      } else {
        showError(t(message));
        return groups;
      }
    } catch (error) {
      showError(error?.message || t('加载分组失败'));
      return groups;
    }
  };

  const loadToken = async () => {
    setLoading(true);
    try {
      const [availableGroups, res] = await Promise.all([
        loadGroups(),
        API.get(`/api/token/${props.editingToken.id}`),
      ]);
      if (!res?.data) return;
      const { success, message, data } = res.data;
      if (success) {
        if (data.expired_time !== -1) {
          data.expired_time = timestamp2string(data.expired_time);
        }
        if (data.model_limits !== '') {
          data.model_limits = data.model_limits.split(',');
        } else {
          data.model_limits = [];
        }
        data.remain_amount = Number(
          quotaToDisplayAmount(data.remain_quota || 0).toFixed(6),
        );
        const resolvedGroups = resolveGroupCodes(data, availableGroups);
        const mergedGroupOptions = includeSelectedGroupOptions(
          availableGroups,
          resolvedGroups,
          data.group_details,
        );
        setGroups(mergedGroupOptions);
        setSelectedGroups(resolvedGroups);
        setGroupRatioLimits(parseGroupRatioLimits(data.group_ratio_limits));
        // 分组由独立选择器维护，避免表单回传服务端旧值。
        const formData = { ...data };
        delete formData.group;
        delete formData.group_ids;
        delete formData.group_mode;
        delete formData.group_details;
        delete formData.group_ratio_limits;
        if (formApiRef.current) {
          formApiRef.current.setValues({ ...getInitValues(), ...formData });
        }
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error?.message || t('加载令牌失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (formApiRef.current) {
      if (!isEdit) {
        formApiRef.current.setValues(getInitValues());
        onSelectedGroupsChange(defaultUseAutoGroup ? ['auto'] : []);
        setGroupRatioLimits({});
      }
    }
    loadModels();
  }, [props.editingToken.id]);

  useEffect(() => {
    if (props.visiable) {
      if (isEdit) {
        loadToken();
      } else {
        loadGroups();
        formApiRef.current?.setValues(getInitValues());
        onSelectedGroupsChange(defaultUseAutoGroup ? ['auto'] : []);
        setGroupRatioLimits({});
      }
    } else {
      formApiRef.current?.reset();
      onSelectedGroupsChange(defaultUseAutoGroup ? ['auto'] : []);
      setGroupRatioLimits({});
    }
  }, [props.visiable, props.editingToken.id, defaultUseAutoGroup]);

  const generateRandomSuffix = () => {
    const characters =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let result = '';
    for (let i = 0; i < 6; i++) {
      result += characters.charAt(
        Math.floor(Math.random() * characters.length),
      );
    }
    return result;
  };

  const submit = async (values) => {
    setLoading(true);
    const groupSelection = buildGroupSelectionPayload(selectedGroups, groups);
    const isMultiGroup = selectedGroups.length > 1;
    const isAuto = selectedGroups.length === 1 && selectedGroups[0] === 'auto';
    const cleanedGroupRatioLimits = cleanGroupRatioLimits(groupRatioLimits);
    const groupRatioLimitsJSON =
      Object.keys(cleanedGroupRatioLimits).length > 0
        ? JSON.stringify(cleanedGroupRatioLimits)
        : '';

    if (isEdit) {
      let { tokenCount: _tc, ...localInputs } = values;
      Object.assign(localInputs, groupSelection);
      localInputs.group_ratio_limits = groupRatioLimitsJSON;
      localInputs.cross_group_retry = isMultiGroup
        ? true
        : isAuto
          ? !!localInputs.cross_group_retry
          : false;
      localInputs.remain_quota = localInputs.unlimited_quota
        ? 0
        : displayAmountToQuota(localInputs.remain_amount);
      if (!localInputs.unlimited_quota && localInputs.remain_quota <= 0) {
        showError(t('请输入金额'));
        setLoading(false);
        return;
      }
      if (localInputs.expired_time !== -1) {
        let time = Date.parse(localInputs.expired_time);
        if (isNaN(time)) {
          showError(t('过期时间格式错误！'));
          setLoading(false);
          return;
        }
        localInputs.expired_time = Math.ceil(time / 1000);
      }
      localInputs.model_limits = localInputs.model_limits.join(',');
      localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
      let res = await API.put(`/api/token/`, {
        ...localInputs,
        id: parseInt(props.editingToken.id),
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('令牌更新成功！'));
        props.refresh();
        props.handleClose();
      } else {
        showError(t(message));
      }
    } else {
      const count = parseInt(values.tokenCount, 10) || 1;
      let successCount = 0;
      for (let i = 0; i < count; i++) {
        let { tokenCount: _tc, ...localInputs } = values;
        Object.assign(localInputs, groupSelection);
        localInputs.group_ratio_limits = groupRatioLimitsJSON;
        localInputs.cross_group_retry = isMultiGroup
          ? true
          : isAuto
            ? !!localInputs.cross_group_retry
            : false;
        const baseName =
          values.name.trim() === '' ? 'default' : values.name.trim();
        if (i !== 0 || values.name.trim() === '') {
          localInputs.name = `${baseName}-${generateRandomSuffix()}`;
        } else {
          localInputs.name = baseName;
        }
        localInputs.remain_quota = localInputs.unlimited_quota
          ? 0
          : displayAmountToQuota(localInputs.remain_amount);
        if (!localInputs.unlimited_quota && localInputs.remain_quota <= 0) {
          showError(t('请输入金额'));
          setLoading(false);
          break;
        }

        if (localInputs.expired_time !== -1) {
          let time = Date.parse(localInputs.expired_time);
          if (isNaN(time)) {
            showError(t('过期时间格式错误！'));
            setLoading(false);
            break;
          }
          localInputs.expired_time = Math.ceil(time / 1000);
        }
        localInputs.model_limits = localInputs.model_limits.join(',');
        localInputs.model_limits_enabled = localInputs.model_limits.length > 0;
        let res = await API.post(`/api/token/`, localInputs);
        const { success, message } = res.data;
        if (success) {
          successCount++;
        } else {
          showError(t(message));
          break;
        }
      }
      if (successCount > 0) {
        showSuccess(t('令牌创建成功，请在列表页面点击复制获取令牌！'));
        props.refresh();
        props.handleClose();
      }
    }
    setLoading(false);
    formApiRef.current?.setValues(getInitValues());
    onSelectedGroupsChange(defaultUseAutoGroup ? ['auto'] : []);
    setGroupRatioLimits({});
  };

  return (
    <SideSheet
      placement={isEdit ? 'right' : 'left'}
      title={
        <Space>
          {isEdit ? (
            <Tag color='blue' shape='circle'>
              {t('更新')}
            </Tag>
          ) : (
            <Tag color='green' shape='circle'>
              {t('新建')}
            </Tag>
          )}
          <Title heading={4} className='m-0'>
            {isEdit ? t('更新令牌信息') : t('创建新的令牌')}
          </Title>
        </Space>
      }
      bodyStyle={{ padding: '0' }}
      visible={props.visiable}
      width={isMobile ? '100%' : 600}
      footer={
        <div className='flex justify-end bg-white'>
          <Space>
            <Button
              theme='solid'
              className='!rounded-lg'
              onClick={() => formApiRef.current?.submitForm()}
              icon={<IconSave />}
              loading={loading}
            >
              {t('提交')}
            </Button>
            <Button
              theme='light'
              className='!rounded-lg'
              type='primary'
              onClick={handleCancel}
              icon={<IconClose />}
            >
              {t('取消')}
            </Button>
          </Space>
        </div>
      }
      closeIcon={null}
      onCancel={() => handleCancel()}
    >
      <Spin spinning={loading}>
        <Form
          key={isEdit ? 'edit' : 'new'}
          initValues={getInitValues()}
          getFormApi={(api) => (formApiRef.current = api)}
          onSubmit={submit}
        >
          {({ values }) => (
            <div className='p-2'>
              {/* 基本信息 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='blue' className='mr-2 shadow-md'>
                    <IconKey size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('基本信息')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的基本信息')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Input
                      field='name'
                      label={t('名称')}
                      placeholder={t('请输入名称')}
                      rules={[{ required: true, message: t('请输入名称') }]}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    <Form.Slot label={t('令牌分组')}>
                      <GroupMultiPicker
                        groups={groups}
                        selectedGroups={selectedGroups}
                        onChange={onSelectedGroupsChange}
                        groupRatioLimits={groupRatioLimits}
                        onGroupRatioLimitChange={handleGroupRatioLimitChange}
                        t={t}
                      />
                    </Form.Slot>
                  </Col>
                  <Col
                    span={24}
                    style={{
                      display:
                        selectedGroups.length === 1 &&
                        selectedGroups[0] === 'auto'
                          ? 'block'
                          : 'none',
                    }}
                  >
                    <Form.Switch
                      field='cross_group_retry'
                      label={t('跨分组重试')}
                      size='default'
                      extraText={t(
                        '开启后，当前分组渠道失败时会按顺序尝试下一个分组的渠道',
                      )}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={10} xl={10}>
                    <Form.DatePicker
                      field='expired_time'
                      label={t('过期时间')}
                      type='dateTime'
                      placeholder={t('请选择过期时间')}
                      rules={[
                        { required: true, message: t('请选择过期时间') },
                        {
                          validator: (rule, value) => {
                            // 允许 -1 表示永不过期，也允许空值在必填校验时被拦截
                            if (value === -1 || !value)
                              return Promise.resolve();
                            const time = Date.parse(value);
                            if (isNaN(time)) {
                              return Promise.reject(t('过期时间格式错误！'));
                            }
                            if (time <= Date.now()) {
                              return Promise.reject(
                                t('过期时间不能早于当前时间！'),
                              );
                            }
                            return Promise.resolve();
                          },
                        },
                      ]}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col xs={24} sm={24} md={24} lg={14} xl={14}>
                    <Form.Slot label={t('过期时间快捷设置')}>
                      <Space wrap>
                        <Button
                          theme='light'
                          type='primary'
                          onClick={() => setExpiredTime(0, 0, 0, 0)}
                        >
                          {t('永不过期')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(1, 0, 0, 0)}
                        >
                          {t('一个月')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 1, 0, 0)}
                        >
                          {t('一天')}
                        </Button>
                        <Button
                          theme='light'
                          type='tertiary'
                          onClick={() => setExpiredTime(0, 0, 1, 0)}
                        >
                          {t('一小时')}
                        </Button>
                      </Space>
                    </Form.Slot>
                  </Col>
                  {!isEdit && (
                    <Col span={24}>
                      <Form.InputNumber
                        field='tokenCount'
                        label={t('新建数量')}
                        min={1}
                        extraText={t('批量创建时会在名称后自动添加随机后缀')}
                        rules={[
                          { required: true, message: t('请输入新建数量') },
                        ]}
                        style={{ width: '100%' }}
                      />
                    </Col>
                  )}
                </Row>
              </Card>

              {/* 额度设置 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar size='small' color='green' className='mr-2 shadow-md'>
                    <IconCreditCard size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('额度设置')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌可用额度和数量')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.InputNumber
                      field='remain_amount'
                      label={t('金额')}
                      prefix={getCurrencyConfig().symbol}
                      placeholder={t('输入金额')}
                      precision={6}
                      disabled={values.unlimited_quota}
                      min={0}
                      step={0.000001}
                      onChange={(val) => {
                        const amount = val === '' || val == null ? 0 : val;
                        formApiRef.current?.setValue('remain_amount', amount);
                        formApiRef.current?.setValue(
                          'remain_quota',
                          displayAmountToQuota(amount),
                        );
                      }}
                      style={{ width: '100%' }}
                      showClear
                    />
                  </Col>
                  <Col span={24}>
                    <div
                      className='text-xs cursor-pointer mt-1'
                      style={{ color: 'var(--semi-color-text-2)' }}
                      onClick={() => setShowQuotaInput((v) => !v)}
                    >
                      {showQuotaInput
                        ? `▾ ${t('收起原生额度输入')}`
                        : `▸ ${t('使用原生额度输入')}`}
                    </div>
                    <div
                      style={{ display: showQuotaInput ? 'block' : 'none' }}
                      className='mt-2'
                    >
                      <Form.InputNumber
                        field='remain_quota'
                        label={t('额度')}
                        placeholder={t('输入额度')}
                        disabled={values.unlimited_quota}
                        min={0}
                        step={500000}
                        rules={
                          values.unlimited_quota
                            ? []
                            : [{ required: true, message: t('请输入额度') }]
                        }
                        onChange={(val) => {
                          const quota = val === '' || val == null ? 0 : val;
                          formApiRef.current?.setValue('remain_quota', quota);
                          formApiRef.current?.setValue(
                            'remain_amount',
                            Number(quotaToDisplayAmount(quota).toFixed(6)),
                          );
                        }}
                        style={{ width: '100%' }}
                        showClear
                      />
                    </div>
                  </Col>
                  <Col span={24}>
                    <Form.Switch
                      field='unlimited_quota'
                      label={t('无限额度')}
                      size='default'
                      extraText={t(
                        '令牌的额度仅用于限制令牌本身的最大额度使用量，实际的使用受到账户的剩余额度限制',
                      )}
                    />
                  </Col>
                </Row>
              </Card>

              {/* 访问限制 */}
              <Card className='!rounded-2xl shadow-sm border-0'>
                <div className='flex items-center mb-2'>
                  <Avatar
                    size='small'
                    color='purple'
                    className='mr-2 shadow-md'
                  >
                    <IconLink size={16} />
                  </Avatar>
                  <div>
                    <Text className='text-lg font-medium'>{t('访问限制')}</Text>
                    <div className='text-xs text-gray-600'>
                      {t('设置令牌的访问限制')}
                    </div>
                  </div>
                </div>
                <Row gutter={12}>
                  <Col span={24}>
                    <Form.Select
                      field='model_limits'
                      label={t('模型限制列表')}
                      placeholder={t(
                        '请选择该令牌支持的模型，留空支持所有模型',
                      )}
                      multiple
                      optionList={models}
                      extraText={t('非必要，不建议启用模型限制')}
                      filter={selectFilter}
                      autoClearSearchValue={false}
                      searchPosition='dropdown'
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                  <Col span={24}>
                    <Form.TextArea
                      field='allow_ips'
                      label={t('IP白名单（支持CIDR表达式）')}
                      placeholder={t('允许的IP，一行一个，不填写则不限制')}
                      autosize
                      rows={1}
                      extraText={t(
                        '请勿过度信任此功能，IP可能被伪造，请配合nginx和cdn等网关使用',
                      )}
                      showClear
                      style={{ width: '100%' }}
                    />
                  </Col>
                </Row>
              </Card>
            </div>
          )}
        </Form>
      </Spin>
    </SideSheet>
  );
};

export default EditTokenModal;
