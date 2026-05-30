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

import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Card,
  Empty,
  Form,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  compareObjects,
  renderQuota,
  showError,
  showSuccess,
  showWarning,
  timestamp2string,
} from '../../../helpers';

const { Text } = Typography;

const DEFAULT_INPUTS = {
  'affiliate_setting.first_level_enabled': false,
  'affiliate_setting.first_level_ratio': 0,
  'affiliate_setting.second_level_enabled': false,
  'affiliate_setting.second_level_ratio': 0,
  'affiliate_setting.settlement_delay_seconds': 0,
  'affiliate_setting.min_withdrawal_amount': 10,
  'affiliate_setting.trigger_topup_enabled': true,
  'affiliate_setting.trigger_subscription_enabled': false,
  'affiliate_setting.payout_methods': 'usdt,alipay,wechat',
  'affiliate_setting.usdt_chain': 'TRC20',
  'affiliate_setting.promotion_template': '邀请链接：{invite_link}',
};

const SETTLEMENT_DELAY_KEY = 'affiliate_setting.settlement_delay_seconds';
const PAYOUT_METHOD_OPTIONS = [
  ['usdt', 'USDT'],
  ['alipay', '支付宝'],
  ['wechat', '微信'],
];

const COMPACT_INPUT_STYLE = { width: '100%' };
const WIDE_INPUT_STYLE = { width: '100%' };
const PANEL_STYLE = {
  border: '1px solid var(--semi-color-border)',
  borderRadius: 8,
  padding: 20,
  background: 'var(--semi-color-bg-0)',
};
const SOFT_PANEL_STYLE = {
  border: '1px solid var(--semi-color-border)',
  borderRadius: 6,
  padding: 16,
  background: 'var(--semi-color-fill-0)',
};

function secondsToMinutes(value) {
  const seconds = Number(value) || 0;
  return Math.max(0, Math.round(seconds / 60));
}

function minutesToSeconds(value) {
  const minutes = Number(value) || 0;
  return Math.max(0, Math.round(minutes)) * 60;
}

function methodText(t, method) {
  if (method === 'usdt') return 'USDT';
  if (method === 'alipay') return t('支付宝');
  if (method === 'wechat') return t('微信');
  return method;
}

function statusText(t, status) {
  const map = {
    pending: t('待审核'),
    approved: t('已通过'),
    paid: t('已打款'),
    rejected: t('已驳回'),
  };
  return map[status] || status;
}

function statusColor(status) {
  if (status === 'paid') return 'green';
  if (status === 'rejected') return 'red';
  if (status === 'approved') return 'blue';
  return 'orange';
}

function SectionHeader({ title, description }) {
  return (
    <div className='mb-4 flex flex-col gap-1'>
      <Text strong>{title}</Text>
      {description && <Text type='secondary'>{description}</Text>}
    </div>
  );
}

function SwitchRow({ title, description, children }) {
  return (
    <div
      className='flex items-center justify-between gap-4'
      style={SOFT_PANEL_STYLE}
    >
      <div className='min-w-0'>
        <Text strong>{title}</Text>
        {description && (
          <div>
            <Text type='secondary'>{description}</Text>
          </div>
        )}
      </div>
      {children}
    </div>
  );
}

export default function SettingsAffiliateCommission(props) {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState(DEFAULT_INPUTS);
  const [originInputs, setOriginInputs] = useState(DEFAULT_INPUTS);
  const [saving, setSaving] = useState(false);
  const [withdrawals, setWithdrawals] = useState([]);
  const [withdrawalStatus, setWithdrawalStatus] = useState('');
  const [withdrawalsLoading, setWithdrawalsLoading] = useState(false);
  const formApiRef = useRef(null);

  const handleFieldChange = (fieldName) => (value) => {
    setInputs((current) => ({ ...current, [fieldName]: value }));
  };

  const selectedPayoutMethods = String(
    inputs['affiliate_setting.payout_methods'] || '',
  )
    .split(',')
    .map((method) => method.trim())
    .filter(Boolean);

  const togglePayoutMethod = (method, checked) => {
    if (!checked && selectedPayoutMethods.length <= 1) {
      showWarning(t('至少需要保留一个提现渠道'));
      return;
    }
    const next = checked
      ? [...selectedPayoutMethods, method]
      : selectedPayoutMethods.filter((item) => item !== method);
    const ordered = PAYOUT_METHOD_OPTIONS.map(([value]) => value).filter(
      (value) => next.includes(value),
    );
    handleFieldChange('affiliate_setting.payout_methods')(ordered.join(','));
    formApiRef.current?.setValue(
      'affiliate_setting.payout_methods',
      ordered.join(','),
    );
  };

  const loadWithdrawals = async () => {
    setWithdrawalsLoading(true);
    try {
      const res = await API.get('/api/affiliate/admin/withdrawals', {
        params: { p: 1, page_size: 50, status: withdrawalStatus || undefined },
      });
      if (res.data.success) {
        setWithdrawals(res.data.data?.items || []);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取提现申请失败'));
    } finally {
      setWithdrawalsLoading(false);
    }
  };

  useEffect(() => {
    loadWithdrawals();
  }, [withdrawalStatus]);

  useEffect(() => {
    if (!props.options) return;
    const nextInputs = { ...DEFAULT_INPUTS };
    Object.keys(nextInputs).forEach((key) => {
      if (props.options[key] !== undefined) {
        nextInputs[key] =
          key === SETTLEMENT_DELAY_KEY
            ? secondsToMinutes(props.options[key])
            : props.options[key];
      }
    });
    setInputs(nextInputs);
    setOriginInputs(nextInputs);
    formApiRef.current?.setValues(nextInputs);
  }, [props.options]);

  const saveSettings = async () => {
    const updateArray = compareObjects(originInputs, inputs);
    if (!updateArray.length) {
      showWarning(t('你似乎并没有修改什么'));
      return;
    }
    setSaving(true);
    try {
      const results = await Promise.all(
        updateArray.map((item) =>
          API.put('/api/option/', {
            key: item.key,
            value: String(
              item.key === SETTLEMENT_DELAY_KEY
                ? minutesToSeconds(inputs[item.key])
                : inputs[item.key],
            ),
          }),
        ),
      );
      const failed = results.filter((res) => !res.data.success);
      if (failed.length > 0) {
        failed.forEach((res) => showError(res.data.message));
        return;
      }
      showSuccess(t('保存成功'));
      setOriginInputs({ ...inputs });
      formApiRef.current?.setValues(inputs);
      await props.refresh?.();
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setSaving(false);
    }
  };

  const updateWithdrawal = async (id, action) => {
    const actionText = {
      approve: t('通过'),
      reject: t('驳回'),
      paid: t('标记打款'),
    }[action];
    Modal.confirm({
      title: t('确认操作'),
      content: t('确认要执行该提现操作吗？') + ` ${actionText}`,
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/affiliate/admin/withdrawals/${id}/${action}`,
            { remark: '' },
          );
          if (res.data.success) {
            showSuccess(t('操作成功'));
            await loadWithdrawals();
          } else {
            showError(res.data.message);
          }
        } catch (error) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: t('用户 ID'), dataIndex: 'user_id', width: 100 },
    {
      title: t('提现方式'),
      dataIndex: 'method',
      width: 120,
      render: (value) => methodText(t, value),
    },
    {
      title: t('提现额度'),
      dataIndex: 'quota',
      width: 140,
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value) => (
        <Tag color={statusColor(value)}>{statusText(t, value)}</Tag>
      ),
    },
    {
      title: t('收款快照'),
      dataIndex: 'payout_snapshot',
      width: 240,
      render: (value) => (
        <Text ellipsis={{ showTooltip: true }} style={{ maxWidth: 220 }}>
          {value || '-'}
        </Text>
      ),
    },
    {
      title: t('提交时间'),
      dataIndex: 'created_at',
      width: 170,
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 220,
      render: (_, record) => (
        <Space>
          {record.status === 'pending' && (
            <>
              <Button
                size='small'
                type='primary'
                onClick={() => updateWithdrawal(record.id, 'approve')}
              >
                {t('通过')}
              </Button>
              <Button
                size='small'
                type='danger'
                onClick={() => updateWithdrawal(record.id, 'reject')}
              >
                {t('驳回')}
              </Button>
            </>
          )}
          {(record.status === 'pending' || record.status === 'approved') && (
            <Button
              size='small'
              onClick={() => updateWithdrawal(record.id, 'paid')}
            >
              {t('标记打款')}
            </Button>
          )}
        </Space>
      ),
    },
  ];

  return (
    <Spin spinning={saving}>
      <Form
        values={inputs}
        getFormApi={(api) => (formApiRef.current = api)}
        style={{ marginBottom: 16 }}
      >
        <div className='space-y-4'>
          <Text type='secondary'>
            {t('设置付费后返佣比例、延迟到账、提现门槛和推广文案')}
          </Text>

          <div style={PANEL_STYLE}>
            <SectionHeader
              title={t('返佣规则')}
              description={t('设置一级和二级返佣的开关与比例')}
            />
            <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
              <div style={SOFT_PANEL_STYLE}>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <Text strong>{t('启用一级返佣')}</Text>
                  <Form.Switch
                    field='affiliate_setting.first_level_enabled'
                    noLabel
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={handleFieldChange(
                      'affiliate_setting.first_level_enabled',
                    )}
                  />
                </div>
                <Form.InputNumber
                  field='affiliate_setting.first_level_ratio'
                  label={t('一级返佣比例（%）')}
                  min={0}
                  max={100}
                  style={COMPACT_INPUT_STYLE}
                  onChange={handleFieldChange(
                    'affiliate_setting.first_level_ratio',
                  )}
                />
              </div>

              <div style={SOFT_PANEL_STYLE}>
                <div className='mb-3 flex items-center justify-between gap-3'>
                  <Text strong>{t('启用二级返佣')}</Text>
                  <Form.Switch
                    field='affiliate_setting.second_level_enabled'
                    noLabel
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={handleFieldChange(
                      'affiliate_setting.second_level_enabled',
                    )}
                  />
                </div>
                <Form.InputNumber
                  field='affiliate_setting.second_level_ratio'
                  label={t('二级返佣比例（%）')}
                  min={0}
                  max={100}
                  style={COMPACT_INPUT_STYLE}
                  onChange={handleFieldChange(
                    'affiliate_setting.second_level_ratio',
                  )}
                />
              </div>
            </div>
          </div>

          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <div style={PANEL_STYLE}>
              <SectionHeader
                title={t('触发与提现')}
                description={t('配置返佣触发范围和用户可见的提现渠道')}
              />
              <div className='space-y-4'>
                <SwitchRow
                  title={t('充值触发返佣')}
                  description={t('充值成功后按规则生成返佣')}
                >
                  <Form.Switch
                    field='affiliate_setting.trigger_topup_enabled'
                    noLabel
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={handleFieldChange(
                      'affiliate_setting.trigger_topup_enabled',
                    )}
                  />
                </SwitchRow>

                <SwitchRow
                  title={t('订阅触发返佣')}
                  description={t('订阅付费后按规则生成返佣')}
                >
                  <Form.Switch
                    field='affiliate_setting.trigger_subscription_enabled'
                    noLabel
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={handleFieldChange(
                      'affiliate_setting.trigger_subscription_enabled',
                    )}
                  />
                </SwitchRow>

                <Form.InputNumber
                  field='affiliate_setting.settlement_delay_seconds'
                  label={t('延迟到账分钟数')}
                  min={0}
                  precision={0}
                  style={COMPACT_INPUT_STYLE}
                  onChange={handleFieldChange(
                    'affiliate_setting.settlement_delay_seconds',
                  )}
                />
              </div>
            </div>

            <div style={PANEL_STYLE}>
              <SectionHeader
                title={t('支持的提现渠道')}
                description={t('未开放的渠道不会在用户返佣页展示')}
              />
              <div className='space-y-4'>
                <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
                  <Form.InputNumber
                    field='affiliate_setting.min_withdrawal_amount'
                    label={t('最低提现额度')}
                    min={0}
                    style={COMPACT_INPUT_STYLE}
                    onChange={handleFieldChange(
                      'affiliate_setting.min_withdrawal_amount',
                    )}
                  />
                  <Form.Input
                    field='affiliate_setting.usdt_chain'
                    label={t('USDT 提现链')}
                    placeholder='TRC20'
                    style={COMPACT_INPUT_STYLE}
                    onChange={handleFieldChange(
                      'affiliate_setting.usdt_chain',
                    )}
                  />
                </div>

                <Space vertical align='start' style={WIDE_INPUT_STYLE}>
                  <Space wrap>
                    {PAYOUT_METHOD_OPTIONS.map(([method, label]) => {
                      const checked = selectedPayoutMethods.includes(method);
                      return (
                        <Button
                          key={method}
                          type={checked ? 'primary' : 'tertiary'}
                          theme={checked ? 'solid' : 'outline'}
                          onClick={() => togglePayoutMethod(method, !checked)}
                        >
                          {t(label)}
                        </Button>
                      );
                    })}
                  </Space>
                  <Form.Input
                    field='affiliate_setting.payout_methods'
                    noLabel
                    style={{ display: 'none' }}
                  />
                </Space>
              </div>
            </div>
          </div>

          <div style={PANEL_STYLE}>
            <SectionHeader
              title={t('推广文案模板')}
              description={t('使用 {invite_link} 表示邀请链接变量')}
            />
            <Form.TextArea
              field='affiliate_setting.promotion_template'
              noLabel
              autosize={{ minRows: 4, maxRows: 8 }}
              style={WIDE_INPUT_STYLE}
              onChange={handleFieldChange(
                'affiliate_setting.promotion_template',
              )}
            />
          </div>

          <div className='flex justify-end'>
            <Button type='primary' loading={saving} onClick={saveSettings}>
              {t('保存设置')}
            </Button>
          </div>
        </div>
      </Form>

      <Card
        title={
          <div className='flex items-center justify-between gap-3'>
            <span>{t('返佣提现审核')}</span>
            <Select
              value={withdrawalStatus}
              onChange={setWithdrawalStatus}
              style={{ width: 140 }}
            >
              <Select.Option value=''>{t('全部状态')}</Select.Option>
              <Select.Option value='pending'>{t('待审核')}</Select.Option>
              <Select.Option value='approved'>{t('已通过')}</Select.Option>
              <Select.Option value='paid'>{t('已打款')}</Select.Option>
              <Select.Option value='rejected'>{t('已驳回')}</Select.Option>
            </Select>
          </div>
        }
      >
        <Table
          rowKey='id'
          loading={withdrawalsLoading}
          columns={columns}
          dataSource={withdrawals}
          pagination={false}
          empty={<Empty description={t('暂无提现申请')} />}
          size='small'
          scroll={{ x: 1170 }}
        />
      </Card>
    </Spin>
  );
}
