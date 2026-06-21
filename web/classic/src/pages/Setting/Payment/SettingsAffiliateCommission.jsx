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
  Checkbox,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import {
  API,
  compareObjects,
  renderQuota,
  renderQuotaWithAmount,
  showError,
  showSuccess,
  showWarning,
  timestamp2string,
} from '../../../helpers';
import { ITEMS_PER_PAGE } from '../../../constants';

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
  'affiliate_setting.filter_redemption_topup_enabled': false,
  'affiliate_setting.payout_methods': 'usdt,alipay,wechat',
  'affiliate_setting.usdt_chain': 'TRC20',
  'affiliate_setting.promotion_template': '邀请链接：{invite_link}',
  'affiliate_setting.review_enabled': false,
  'affiliate_setting.auto_approve_after_days': 0,
  'affiliate_setting.agreement_enabled': false,
  'affiliate_setting.agreement_text': '',
  'affiliate_setting.inviter_min_account_age_days': 0,
  'affiliate_setting.inviter_min_recharge_amount': 0,
  'affiliate_setting.invitee_min_account_age_days': 0,
  'affiliate_setting.invitee_min_recharge_amount': 0,
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
    available: t('已结算'),
  };
  return map[status] || status;
}

function statusColor(status) {
  if (status === 'paid' || status === 'available') return 'green';
  if (status === 'rejected') return 'red';
  if (status === 'approved') return 'blue';
  return 'orange';
}

function sourceTypeText(t, sourceType) {
  if (sourceType === 'topup') return t('余额充值');
  if (sourceType === 'subscription') return t('订阅购买');
  if (sourceType === 'redemption') return t('兑换码兑换');
  return sourceType || '-';
}

function renderAdminUser(userId, username, displayName, email) {
  const name = displayName || username || '-';
  return (
    <div className='min-w-0'>
      <Text strong ellipsis={{ showTooltip: true }}>
        #{userId} {name}
      </Text>
      <br />
      <Text type='secondary' ellipsis={{ showTooltip: true }}>
        {email || username || '-'}
      </Text>
    </div>
  );
}

function renderAdminRecordDetail(record) {
  const detail = record.detail || {};
  const title = detail.title || `${record.source_type} #${record.source_id}`;
  const parts = [record.source_id];
  if (detail.paid_amount > 0) {
    parts.push(renderQuotaWithAmount(detail.paid_amount));
  }
  if (detail.payment_method) {
    parts.push(detail.payment_method);
  }
  return (
    <div className='min-w-0'>
      <Text strong ellipsis={{ showTooltip: true }}>
        {title}
      </Text>
      <br />
      <Text type='secondary' ellipsis={{ showTooltip: true }}>
        {parts.filter(Boolean).join(' · ') || '-'}
      </Text>
    </div>
  );
}

function getPageTotal(pageData) {
  return pageData?.total || pageData?.Total || 0;
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

function ApplicationsPanel() {
  const { t } = useTranslation();
  const [applications, setApplications] = useState([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [statusFilter, setStatusFilter] = useState('');

  const loadApplications = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/affiliate/admin/applications', {
        params: {
          p: page,
          page_size: pageSize,
          status: statusFilter || undefined,
        },
      });
      if (res.data.success) {
        const pageData = res.data.data || {};
        setApplications(pageData.items || []);
        setTotal(getPageTotal(pageData));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取申请列表失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadApplications();
  }, [page, pageSize, statusFilter]);

  const handleApprove = (id) => {
    Modal.confirm({
      title: t('确认通过'),
      content: t('确认通过该用户的返佣申请？'),
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/affiliate/admin/applications/${id}/approve`,
            { remark: '' },
          );
          if (res.data.success) {
            showSuccess(t('已通过'));
            await loadApplications();
          } else {
            showError(res.data.message);
          }
        } catch (error) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const handleReject = (id) => {
    let reason = '';
    Modal.confirm({
      title: t('驳回申请'),
      content: (
        <Input
          placeholder={t('驳回原因（可选）')}
          onChange={(value) => {
            reason = value;
          }}
        />
      ),
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/affiliate/admin/applications/${id}/reject`,
            { reason },
          );
          if (res.data.success) {
            showSuccess(t('已驳回'));
            await loadApplications();
          } else {
            showError(res.data.message);
          }
        } catch (error) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const handleRevoke = (id, status) => {
    Modal.confirm({
      title: status === 'approved' ? t('撤销申请') : t('重置申请'),
      content:
        status === 'approved'
          ? t('撤销后该用户将失去邀请权限，并可重新提交返佣申请。')
          : t('重置后该用户可重新提交返佣申请。'),
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/affiliate/admin/applications/${id}/revoke`,
          );
          if (res.data.success) {
            showSuccess(t('已撤销，用户可重新提交申请'));
            await loadApplications();
          } else {
            showError(res.data.message);
          }
        } catch (error) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const applicationStatusText = (status) => {
    const map = {
      pending: t('待审核'),
      approved: t('已通过'),
      rejected: t('已驳回'),
    };
    return map[status] || status;
  };

  const applicationStatusColor = (status) => {
    if (status === 'approved') return 'green';
    if (status === 'rejected') return 'red';
    return 'orange';
  };

  const applicationColumns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: t('用户 ID'), dataIndex: 'user_id', width: 100 },
    {
      title: t('用户名'),
      dataIndex: 'username',
      width: 160,
      render: (value) => value || '-',
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value) => (
        <Tag color={applicationStatusColor(value)}>
          {applicationStatusText(value)}
        </Tag>
      ),
    },
    {
      title: t('申请时间'),
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
                onClick={() => handleApprove(record.id)}
              >
                {t('通过')}
              </Button>
              <Button
                size='small'
                type='danger'
                onClick={() => handleReject(record.id)}
              >
                {t('驳回')}
              </Button>
            </>
          )}
          {record.status !== 'pending' && (
            <Button
              size='small'
              type='warning'
              onClick={() => handleRevoke(record.id, record.status)}
            >
              {record.status === 'approved' ? t('撤销') : t('重置')}
            </Button>
          )}
        </Space>
      ),
    },
  ];

  return (
    <Card>
      <Space vertical align='start' style={{ width: '100%' }}>
        <SectionHeader
          title={t('返佣申请审核')}
          description={t('审核用户的返佣参与申请')}
        />
        <Space wrap>
          <Select
            value={statusFilter}
            onChange={(value) => {
              setPage(1);
              setStatusFilter(value);
            }}
            style={{ width: 140 }}
          >
            <Select.Option value=''>{t('全部状态')}</Select.Option>
            <Select.Option value='pending'>{t('待审核')}</Select.Option>
            <Select.Option value='approved'>{t('已通过')}</Select.Option>
            <Select.Option value='rejected'>{t('已驳回')}</Select.Option>
          </Select>
          <Button theme='outline' onClick={loadApplications}>
            {t('刷新')}
          </Button>
        </Space>
        <Table
          rowKey='id'
          loading={loading}
          columns={applicationColumns}
          dataSource={applications}
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: total,
            pageSizeOpts: [10, 20, 50, 100],
            showSizeChanger: true,
            onPageChange: setPage,
            onPageSizeChange: (size) => {
              setPageSize(size);
              setPage(1);
            },
          }}
          empty={<Empty description={t('暂无申请记录')} />}
          size='small'
          scroll={{ x: 830 }}
          className='w-full'
        />
      </Space>
    </Card>
  );
}

function FraudAlertsPanel({ onQueryInviter }) {
  const { t } = useTranslation();
  const [alerts, setAlerts] = useState([]);
  const [loading, setLoading] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [statusFilter, setStatusFilter] = useState('');
  const [keyword, setKeyword] = useState('');
  const [keywordSearch, setKeywordSearch] = useState('');
  const [ipKeyword, setIpKeyword] = useState('');
  const [ipSearch, setIpSearch] = useState('');
  const [scanDays, setScanDays] = useState(30);

  const loadAlerts = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/affiliate/admin/fraud-alerts', {
        params: {
          p: page,
          page_size: pageSize,
          status: statusFilter || undefined,
          keyword: keywordSearch || undefined,
          ip: ipSearch || undefined,
        },
      });
      if (res.data.success) {
        const pageData = res.data.data || {};
        setAlerts(pageData.items || []);
        setTotal(getPageTotal(pageData));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取异常检测列表失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAlerts();
  }, [page, pageSize, statusFilter, keywordSearch, ipSearch]);

  const handleScan = async () => {
    setScanning(true);
    try {
      const res = await API.post(
        '/api/affiliate/admin/fraud-alerts/scan',
        null,
        {
          params: { days: Number(scanDays) || 0 },
        },
      );
      if (res.data.success) {
        const newAlerts = res.data.data?.new_alerts || 0;
        showSuccess(
          t('检测完成') + `，${t('新增')} ${newAlerts} ${t('条警报')}`,
        );
        await loadAlerts();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('检测失败'));
    } finally {
      setScanning(false);
    }
  };

  const handleScanDeep = async () => {
    setScanning(true);
    try {
      const res = await API.post(
        '/api/affiliate/admin/fraud-alerts/scan-deep',
        null,
        {
          params: { days: Number(scanDays) || 0 },
        },
      );
      if (res.data.success) {
        const newAlerts = res.data.data?.new_alerts || 0;
        showSuccess(
          t('深度检测完成') + `，${t('新增')} ${newAlerts} ${t('条警报')}`,
        );
        await loadAlerts();
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('深度检测失败'));
    } finally {
      setScanning(false);
    }
  };

  const handleAction = (id, action) => {
    const actionText = {
      unbind: t('解绑邀请关系'),
      clawback: t('追回返佣'),
      dismiss: t('忽略'),
      delete: t('删除'),
    }[action];

    if (action === 'delete') {
      Modal.confirm({
        title: t('删除警报'),
        content: t('确认删除该异常警报？'),
        onOk: async () => {
          try {
            const res = await API.delete(
              `/api/affiliate/admin/fraud-alerts/${id}`,
            );
            if (res.data.success) {
              showSuccess(t('已删除'));
              await loadAlerts();
            } else {
              showError(res.data.message);
            }
          } catch (error) {
            showError(t('删除失败'));
          }
        },
      });
      return;
    }

    if (action === 'dismiss') {
      let remark = '';
      Modal.confirm({
        title: t('忽略警报'),
        content: (
          <Input
            placeholder={t('备注（可选）')}
            onChange={(value) => {
              remark = value;
            }}
          />
        ),
        onOk: async () => {
          try {
            const res = await API.post(
              `/api/affiliate/admin/fraud-alerts/${id}/dismiss`,
              { remark },
            );
            if (res.data.success) {
              showSuccess(t('已忽略'));
              await loadAlerts();
            } else {
              showError(res.data.message);
            }
          } catch (error) {
            showError(t('操作失败'));
          }
        },
      });
      return;
    }

    Modal.confirm({
      title: t('确认操作'),
      content: `${t('确认执行')}：${actionText}？`,
      onOk: async () => {
        try {
          const res = await API.post(
            `/api/affiliate/admin/fraud-alerts/${id}/${action}`,
          );
          if (res.data.success) {
            showSuccess(t('操作成功'));
            await loadAlerts();
          } else {
            showError(res.data.message);
          }
        } catch (error) {
          showError(t('操作失败'));
        }
      },
    });
  };

  const alertStatusText = (status) => {
    const map = {
      detected: t('待处理'),
      resolved: t('已处理'),
      dismissed: t('已忽略'),
    };
    return map[status] || status;
  };

  const alertStatusColor = (status) => {
    if (status === 'dismissed') return 'grey';
    if (status === 'resolved') return 'green';
    return 'orange';
  };

  const parseSharedIps = (value) => {
    if (Array.isArray(value)) return value;
    try {
      return typeof value === 'string' ? JSON.parse(value) : value || [];
    } catch {
      return [];
    }
  };

  const getChildAlerts = (record) => {
    if (Array.isArray(record?.alerts) && record.alerts.length > 0) {
      return record.alerts;
    }
    return record?.id ? [record] : [];
  };

  const jumpToInviterData = (record) => {
    onQueryInviter?.(record);
    showSuccess(t('已切换到该邀请人的邀请数据'));
  };

  const renderAlertActions = (alert) => (
    <Space wrap>
      {alert.status === 'detected' && (
        <>
          <Button
            size='small'
            type='warning'
            onClick={() => handleAction(alert.id, 'unbind')}
          >
            {t('解绑')}
          </Button>
          <Button
            size='small'
            type='danger'
            onClick={() => handleAction(alert.id, 'clawback')}
          >
            {t('追回')}
          </Button>
          <Button
            size='small'
            theme='outline'
            onClick={() => handleAction(alert.id, 'dismiss')}
          >
            {t('忽略')}
          </Button>
        </>
      )}
      <Button
        size='small'
        theme='borderless'
        type='danger'
        onClick={() => handleAction(alert.id, 'delete')}
      >
        {t('删除')}
      </Button>
    </Space>
  );

  const alertColumns = [
    {
      title: t('邀请人'),
      dataIndex: 'inviter_id',
      width: 220,
      render: (_, record) =>
        renderAdminUser(
          record.inviter_id,
          record.inviter_username,
          record.inviter_name,
          record.inviter_email,
        ),
    },
    {
      title: t('可疑被邀请人'),
      dataIndex: 'alerts',
      width: 280,
      render: (_, record) => {
        const childAlerts = getChildAlerts(record);
        return (
          <Space vertical align='start' spacing={4}>
            {childAlerts.map((alert) => (
              <div key={alert.id}>
                {renderAdminUser(
                  alert.invitee_id,
                  alert.invitee_username,
                  alert.invitee_name,
                  alert.invitee_email,
                )}
              </div>
            ))}
            {childAlerts.length === 0 && '-'}
          </Space>
        );
      },
    },
    {
      title: t('共享 IP'),
      dataIndex: 'shared_ips',
      width: 260,
      render: (value, record) => {
        const childIps = getChildAlerts(record).flatMap((alert) =>
          parseSharedIps(alert.shared_ips),
        );
        const ips = Array.from(
          new Set([...parseSharedIps(value), ...childIps]),
        );
        return (
          <Space wrap>
            {ips.slice(0, 5).map((ip) => (
              <Tag key={ip} size='small'>
                {ip}
              </Tag>
            ))}
            {ips.length > 5 && <Tag size='small'>+{ips.length - 5}</Tag>}
            {ips.length === 0 && '-'}
          </Space>
        );
      },
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value) => (
        <Tag color={alertStatusColor(value)}>{alertStatusText(value)}</Tag>
      ),
    },
    {
      title: t('操作'),
      key: 'action',
      width: 360,
      render: (_, record) => {
        const childAlerts = getChildAlerts(record);
        return (
          <Space vertical align='start' spacing={6}>
            <Button
              size='small'
              theme='outline'
              onClick={() => jumpToInviterData(record)}
            >
              {t('查询邀请人数据')}
            </Button>
            {childAlerts.map((alert) => (
              <Space key={alert.id} wrap>
                <Text type='secondary'>#{alert.invitee_id}</Text>
                {renderAlertActions(alert)}
              </Space>
            ))}
          </Space>
        );
      },
    },
  ];

  return (
    <Card>
      <Space vertical align='start' style={{ width: '100%' }}>
        <SectionHeader
          title={t('异常检测')}
          description={t('检测可疑的邀请关系（如共享 IP）')}
        />
        <Space wrap>
          <Button type='primary' loading={scanning} onClick={handleScan}>
            {t('一键检测')}
          </Button>
          <Button type='warning' loading={scanning} onClick={handleScanDeep}>
            {t('深度检测（含历史日志）')}
          </Button>
          <Input
            value={keyword}
            placeholder={t('搜索邀请人或被邀请人')}
            style={{ width: 200 }}
            onChange={setKeyword}
            onEnterPress={() => {
              setPage(1);
              setKeywordSearch(keyword.trim());
            }}
          />
          <Input
            value={ipKeyword}
            placeholder={t('搜索共享 IP')}
            style={{ width: 160 }}
            onChange={setIpKeyword}
            onEnterPress={() => {
              setPage(1);
              setIpSearch(ipKeyword.trim());
            }}
          />
          <Select
            value={statusFilter}
            onChange={(value) => {
              setPage(1);
              setStatusFilter(value);
            }}
            style={{ width: 140 }}
          >
            <Select.Option value=''>{t('全部状态')}</Select.Option>
            <Select.Option value='detected'>{t('待处理')}</Select.Option>
            <Select.Option value='resolved'>{t('已处理')}</Select.Option>
            <Select.Option value='dismissed'>{t('已忽略')}</Select.Option>
          </Select>
          <InputNumber
            min={0}
            value={scanDays}
            onChange={setScanDays}
            style={{ width: 120 }}
            placeholder={t('检测天数')}
          />
          <Button
            theme='outline'
            onClick={() => {
              setPage(1);
              setKeywordSearch(keyword.trim());
              setIpSearch(ipKeyword.trim());
            }}
          >
            {t('搜索')}
          </Button>
          <Button theme='outline' onClick={loadAlerts}>
            {t('刷新')}
          </Button>
        </Space>
        <Table
          rowKey={(record) =>
            `${record.inviter_id || 0}-${record.id || record.latest_detected_at || record.alert_count || 0}`
          }
          loading={loading}
          columns={alertColumns}
          dataSource={alerts}
          expandedRowRender={(record) => (
            <Table
              rowKey='id'
              size='small'
              pagination={false}
              dataSource={getChildAlerts(record)}
              columns={[
                {
                  title: t('被邀请人'),
                  width: 220,
                  render: (_, alert) =>
                    renderAdminUser(
                      alert.invitee_id,
                      alert.invitee_username,
                      alert.invitee_name,
                      alert.invitee_email,
                    ),
                },
                {
                  title: t('共享 IP'),
                  width: 260,
                  render: (_, alert) => {
                    const ips = parseSharedIps(alert.shared_ips);
                    return (
                      <Space wrap>
                        {ips.map((ip) => (
                          <Tag key={ip} size='small'>
                            {ip}
                          </Tag>
                        ))}
                        {ips.length === 0 && '-'}
                      </Space>
                    );
                  },
                },
                {
                  title: t('状态'),
                  width: 100,
                  render: (_, alert) => (
                    <Tag color={alertStatusColor(alert.status)}>
                      {alertStatusText(alert.status)}
                    </Tag>
                  ),
                },
                {
                  title: t('操作'),
                  width: 260,
                  render: (_, alert) => renderAlertActions(alert),
                },
              ]}
            />
          )}
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: total,
            pageSizeOpts: [10, 20, 50, 100],
            showSizeChanger: true,
            onPageChange: setPage,
            onPageSizeChange: (size) => {
              setPageSize(size);
              setPage(1);
            },
          }}
          empty={<Empty description={t('暂无异常警报')} />}
          size='small'
          scroll={{ x: 1040 }}
          className='w-full'
        />
      </Space>
    </Card>
  );
}

export default function SettingsAffiliateCommission(props) {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState(DEFAULT_INPUTS);
  const [originInputs, setOriginInputs] = useState(DEFAULT_INPUTS);
  const [saving, setSaving] = useState(false);
  const [invitations, setInvitations] = useState([]);
  const [invitationKeyword, setInvitationKeyword] = useState('');
  const [invitationSearch, setInvitationSearch] = useState('');
  const [invitationsLoading, setInvitationsLoading] = useState(false);
  const [invitationPage, setInvitationPage] = useState(1);
  const [invitationPageSize, setInvitationPageSize] = useState(ITEMS_PER_PAGE);
  const [invitationTotal, setInvitationTotal] = useState(0);
  const [records, setRecords] = useState([]);
  const [recordSourceType, setRecordSourceType] = useState('topup');
  const [recordStatus, setRecordStatus] = useState('');
  const [recordsLoading, setRecordsLoading] = useState(false);
  const [recordPage, setRecordPage] = useState(1);
  const [recordPageSize, setRecordPageSize] = useState(ITEMS_PER_PAGE);
  const [recordTotal, setRecordTotal] = useState(0);
  const [withdrawals, setWithdrawals] = useState([]);
  const [withdrawalStatus, setWithdrawalStatus] = useState('');
  const [withdrawalsLoading, setWithdrawalsLoading] = useState(false);
  const [withdrawalPage, setWithdrawalPage] = useState(1);
  const [withdrawalPageSize, setWithdrawalPageSize] = useState(ITEMS_PER_PAGE);
  const [withdrawalTotal, setWithdrawalTotal] = useState(0);
  const [bindUserKeyword, setBindUserKeyword] = useState('');
  const [bindUserCandidates, setBindUserCandidates] = useState([]);
  const [selectedBindUser, setSelectedBindUser] = useState(null);
  const [bindUserSearching, setBindUserSearching] = useState(false);
  const [bindAffCode, setBindAffCode] = useState('');
  const [bindForce, setBindForce] = useState(false);
  const [bindLoading, setBindLoading] = useState(false);
  const [bindResult, setBindResult] = useState(null);
  const [activeTab, setActiveTab] = useState('rules');
  const formApiRef = useRef(null);
  const antifraudFormApiRef = useRef(null);

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

  const loadInvitations = async () => {
    setInvitationsLoading(true);
    try {
      const res = await API.get('/api/affiliate/admin/invitations', {
        params: {
          p: invitationPage,
          page_size: invitationPageSize,
          keyword: invitationSearch || undefined,
        },
      });
      if (res.data.success) {
        const pageData = res.data.data || {};
        setInvitations(pageData.items || []);
        setInvitationPage(pageData.page || invitationPage);
        setInvitationPageSize(pageData.page_size || invitationPageSize);
        setInvitationTotal(getPageTotal(pageData));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取邀请记录失败'));
    } finally {
      setInvitationsLoading(false);
    }
  };

  const queryInviterInvitations = (record) => {
    const nextKeyword = `#${record.inviter_id}`;
    setInvitationKeyword(nextKeyword);
    setInvitationSearch(nextKeyword);
    setInvitationPage(1);
    setActiveTab('invitations');
  };

  const loadRecords = async () => {
    setRecordsLoading(true);
    try {
      const res = await API.get('/api/affiliate/admin/records', {
        params: {
          p: recordPage,
          page_size: recordPageSize,
          source_type: recordSourceType || undefined,
          status: recordStatus || undefined,
        },
      });
      if (res.data.success) {
        const pageData = res.data.data || {};
        setRecords(pageData.items || []);
        setRecordPage(pageData.page || recordPage);
        setRecordPageSize(pageData.page_size || recordPageSize);
        setRecordTotal(getPageTotal(pageData));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('获取返佣记录失败'));
    } finally {
      setRecordsLoading(false);
    }
  };

  const loadWithdrawals = async () => {
    setWithdrawalsLoading(true);
    try {
      const res = await API.get('/api/affiliate/admin/withdrawals', {
        params: {
          p: withdrawalPage,
          page_size: withdrawalPageSize,
          status: withdrawalStatus || undefined,
        },
      });
      if (res.data.success) {
        const pageData = res.data.data || {};
        setWithdrawals(pageData.items || []);
        setWithdrawalPage(pageData.page || withdrawalPage);
        setWithdrawalPageSize(pageData.page_size || withdrawalPageSize);
        setWithdrawalTotal(getPageTotal(pageData));
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
    loadInvitations();
  }, [invitationPage, invitationPageSize, invitationSearch]);

  useEffect(() => {
    loadRecords();
  }, [recordPage, recordPageSize, recordSourceType, recordStatus]);

  useEffect(() => {
    loadWithdrawals();
  }, [withdrawalPage, withdrawalPageSize, withdrawalStatus]);

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
    antifraudFormApiRef.current?.setValues(nextInputs);
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

  const searchBindUsers = async () => {
    const keyword = bindUserKeyword.trim();
    if (!keyword) {
      showWarning(t('请先输入用户关键词'));
      return;
    }
    setBindUserSearching(true);
    setSelectedBindUser(null);
    try {
      const res = await API.get('/api/user/search', {
        params: { keyword, p: 1, page_size: 10 },
      });
      if (res.data.success) {
        setBindUserCandidates(res.data.data?.items || []);
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('搜索用户失败'));
    } finally {
      setBindUserSearching(false);
    }
  };

  const bindInviter = async () => {
    const affCode = bindAffCode.trim();
    if (!selectedBindUser || !affCode) {
      showWarning(t('请先搜索并选择被绑定用户，同时填写邀请代码'));
      return;
    }
    setBindLoading(true);
    try {
      const res = await API.post('/api/affiliate/admin/bind-inviter', {
        user_id: selectedBindUser.id,
        aff_code: affCode,
        force: bindForce,
      });
      if (res.data.success) {
        setBindResult(res.data.data);
        showSuccess(t('邀请关系已绑定'));
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError(t('绑定邀请关系失败'));
    } finally {
      setBindLoading(false);
    }
  };

  const bindUserColumns = [
    { title: 'ID', dataIndex: 'id', width: 80, render: (value) => `#${value}` },
    { title: t('用户名'), dataIndex: 'username', width: 140 },
    {
      title: t('显示名'),
      dataIndex: 'display_name',
      width: 160,
      render: (value) => value || '-',
    },
    {
      title: t('邮箱'),
      dataIndex: 'email',
      width: 220,
      render: (value) => value || '-',
    },
    {
      title: t('用户组'),
      dataIndex: 'group',
      width: 120,
      render: (value) => value || '-',
    },
    {
      title: t('操作'),
      key: 'action',
      width: 100,
      render: (_, record) => {
        const selected = selectedBindUser?.id === record.id;
        return (
          <Button
            size='small'
            type={selected ? 'primary' : 'tertiary'}
            theme={selected ? 'solid' : 'outline'}
            onClick={() => setSelectedBindUser(record)}
          >
            {selected ? t('已选择') : t('选择')}
          </Button>
        );
      },
    },
  ];

  const invitationColumns = [
    {
      title: t('邀请人'),
      dataIndex: 'inviter_id',
      width: 220,
      render: (_, record) =>
        renderAdminUser(
          record.inviter_id,
          record.inviter_username,
          record.inviter_name,
          record.inviter_email,
        ),
    },
    {
      title: t('下级用户'),
      dataIndex: 'invitee_id',
      width: 220,
      render: (_, record) =>
        renderAdminUser(
          record.invitee_id,
          record.invitee_username,
          record.invitee_name,
          record.invitee_email,
        ),
    },
    {
      title: t('邀请代码'),
      dataIndex: 'inviter_aff_code',
      width: 120,
      render: (value) => value || '-',
    },
    { title: t('充值次数'), dataIndex: 'topup_count', width: 100 },
    {
      title: t('充值额度'),
      dataIndex: 'topup_quota',
      width: 140,
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('返佣额度'),
      dataIndex: 'commission_quota',
      width: 140,
      render: (value) => renderQuota(value || 0),
    },
    {
      title: t('邀请时间'),
      dataIndex: 'invitee_created_at',
      width: 170,
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
    {
      title: t('最近充值'),
      dataIndex: 'last_topup_time',
      width: 170,
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
    {
      title: t('操作'),
      dataIndex: 'action',
      width: 100,
      fixed: 'right',
      render: (_, record) => (
        <Button
          size='small'
          type='danger'
          theme='borderless'
          onClick={async () => {
            if (!window.confirm(t('确认解除该邀请关系？解除后不可恢复。')))
              return;
            try {
              const res = await API.post(
                '/api/affiliate/admin/unbind-inviter',
                {
                  user_id: record.invitee_id,
                },
              );
              if (res.data.success) {
                showSuccess(t('已解除邀请关系'));
                loadInvitations();
              } else {
                showError(res.data.message || t('解除失败'));
              }
            } catch {
              showError(t('解除失败'));
            }
          }}
        >
          {t('解除绑定')}
        </Button>
      ),
    },
  ];

  const recordColumns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    {
      title: t('邀请人'),
      dataIndex: 'user_id',
      width: 220,
      render: (_, record) =>
        renderAdminUser(
          record.inviter?.id,
          record.inviter?.username,
          record.inviter?.display_name,
          record.inviter?.email,
        ),
    },
    {
      title: t('下级用户'),
      dataIndex: 'invitee_id',
      width: 220,
      render: (_, record) =>
        renderAdminUser(
          record.invitee?.id,
          record.invitee?.username,
          record.invitee?.display_name,
          record.invitee?.email,
        ),
    },
    {
      title: t('来源'),
      dataIndex: 'source_type',
      width: 120,
      render: (value) => sourceTypeText(t, value),
    },
    {
      title: t('订单详情'),
      dataIndex: 'source_id',
      width: 260,
      render: (_, record) => renderAdminRecordDetail(record),
    },
    { title: t('层级'), dataIndex: 'level', width: 80 },
    {
      title: t('比例'),
      dataIndex: 'ratio',
      width: 80,
      render: (value) => `${value || 0}%`,
    },
    {
      title: t('返佣额度'),
      dataIndex: 'reward_quota',
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
      title: t('创建时间'),
      dataIndex: 'created_at',
      width: 170,
      render: (value) => (value ? timestamp2string(value) : '-'),
    },
  ];

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
      <Tabs type='line' activeKey={activeTab} onChange={setActiveTab}>
        <Tabs.TabPane tab={t('返佣规则')} itemKey='rules'>
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

                    <SwitchRow
                      title={t('过滤兑换码兑换返佣')}
                      description={t(
                        '开启后，用户兑换兑换码增加额度不会生成返佣',
                      )}
                    >
                      <Form.Switch
                        field='affiliate_setting.filter_redemption_topup_enabled'
                        noLabel
                        checkedText='｜'
                        uncheckedText='〇'
                        onChange={handleFieldChange(
                          'affiliate_setting.filter_redemption_topup_enabled',
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
                          const checked =
                            selectedPayoutMethods.includes(method);
                          return (
                            <Button
                              key={method}
                              type={checked ? 'primary' : 'tertiary'}
                              theme={checked ? 'solid' : 'outline'}
                              onClick={() =>
                                togglePayoutMethod(method, !checked)
                              }
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
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('手动绑定邀请')} itemKey='manual-bind'>
          <Card>
            <Space vertical align='start' style={{ width: '100%' }}>
              <SectionHeader
                title={t('手动绑定邀请关系')}
                description={t('给用户补绑注册时未捕获到的 ?aff=xxxx 邀请关系')}
              />
              <div className='grid grid-cols-1 gap-4 md:grid-cols-2 w-full'>
                <Space vertical align='start' style={COMPACT_INPUT_STYLE}>
                  <Text strong>{t('被绑定用户')}</Text>
                  <Space style={COMPACT_INPUT_STYLE}>
                    <Input
                      placeholder={t('用户 ID、用户名、邮箱或显示名')}
                      value={bindUserKeyword}
                      style={COMPACT_INPUT_STYLE}
                      onChange={(value) => {
                        setBindUserKeyword(value);
                        setSelectedBindUser(null);
                      }}
                      onEnterPress={searchBindUsers}
                    />
                    <Button
                      theme='outline'
                      loading={bindUserSearching}
                      onClick={searchBindUsers}
                    >
                      {t('搜索用户')}
                    </Button>
                  </Space>
                </Space>
                <Space vertical align='start' style={COMPACT_INPUT_STYLE}>
                  <Text strong>{t('邀请代码')}</Text>
                  <Input
                    placeholder={t('支持纯代码或包含 ?aff= 的链接')}
                    value={bindAffCode}
                    style={COMPACT_INPUT_STYLE}
                    onChange={setBindAffCode}
                  />
                </Space>
              </div>
              <Table
                rowKey='id'
                size='small'
                columns={bindUserColumns}
                dataSource={bindUserCandidates}
                pagination={false}
                empty={<Empty description={t('暂无匹配用户')} />}
                scroll={{ x: 820 }}
                className='w-full'
              />
              {selectedBindUser && (
                <div style={SOFT_PANEL_STYLE} className='w-full'>
                  <Text>
                    {t('已选择被绑定用户')}：#{selectedBindUser.id}{' '}
                    {selectedBindUser.display_name ||
                      selectedBindUser.username ||
                      ''}
                    {selectedBindUser.email
                      ? ` (${selectedBindUser.email})`
                      : ''}
                  </Text>
                </div>
              )}
              <Checkbox
                checked={bindForce}
                onChange={(event) => setBindForce(event.target.checked)}
              >
                {t('强制覆盖已有邀请人')}
              </Checkbox>
              <Text type='secondary'>
                {t(
                  '默认不会覆盖已有邀请人；强制覆盖会同步调整新旧邀请人的邀请人数。',
                )}
              </Text>
              <Space>
                <Button
                  type='primary'
                  loading={bindLoading}
                  onClick={bindInviter}
                >
                  {t('绑定邀请人')}
                </Button>
                <Button
                  type='danger'
                  loading={bindLoading}
                  disabled={!selectedBindUser}
                  onClick={async () => {
                    if (!selectedBindUser) {
                      showError(t('请先搜索并选择要解除绑定的用户'));
                      return;
                    }
                    if (!window.confirm(t('确认解除该用户的邀请关系？')))
                      return;
                    try {
                      setBindLoading(true);
                      const res = await API.post(
                        '/api/affiliate/admin/unbind-inviter',
                        {
                          user_id: selectedBindUser.id,
                        },
                      );
                      if (res.data.success) {
                        showSuccess(t('已解除邀请关系'));
                        setBindResult(res.data.data);
                      } else {
                        showError(res.data.message || t('解除失败'));
                      }
                    } catch {
                      showError(t('解除失败'));
                    } finally {
                      setBindLoading(false);
                    }
                  }}
                >
                  {t('解除绑定')}
                </Button>
              </Space>
              {bindResult && (
                <div style={SOFT_PANEL_STYLE} className='w-full'>
                  <Space vertical align='start'>
                    <Text strong>{t('绑定结果')}</Text>
                    <Text>
                      {t('被绑定用户')}：#{bindResult.user_id}{' '}
                      {bindResult.display_name || bindResult.username || ''}
                    </Text>
                    <Text>
                      {t('邀请人')}：#{bindResult.inviter_id}{' '}
                      {bindResult.inviter_username || ''} (
                      {bindResult.inviter_aff_code || '-'})
                    </Text>
                    <Text type='secondary'>
                      {t('原邀请人')}：
                      {bindResult.previous_inviter_id || t('无邀请人')}
                    </Text>
                  </Space>
                </div>
              )}
            </Space>
          </Card>
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('用户邀请')} itemKey='invitations'>
          <Card>
            <Space vertical align='start' style={{ width: '100%' }}>
              <SectionHeader
                title={t('用户邀请')}
                description={t('核查邀请关系和下级充值记录')}
              />
              <Space wrap>
                <Input
                  placeholder={t('搜索邀请人或下级用户')}
                  value={invitationKeyword}
                  style={{ width: 260 }}
                  onChange={setInvitationKeyword}
                  onEnterPress={() => {
                    setInvitationPage(1);
                    setInvitationSearch(invitationKeyword.trim());
                  }}
                />
                <Button
                  theme='outline'
                  loading={invitationsLoading}
                  onClick={() => {
                    setInvitationPage(1);
                    setInvitationSearch(invitationKeyword.trim());
                  }}
                >
                  {t('搜索')}
                </Button>
                <Button theme='outline' onClick={loadInvitations}>
                  {t('刷新')}
                </Button>
              </Space>
              <Table
                rowKey={(record) => `${record.inviter_id}-${record.invitee_id}`}
                loading={invitationsLoading}
                columns={invitationColumns}
                dataSource={invitations}
                pagination={{
                  currentPage: invitationPage,
                  pageSize: invitationPageSize,
                  total: invitationTotal,
                  pageSizeOpts: [10, 20, 50, 100],
                  showSizeChanger: true,
                  onPageChange: setInvitationPage,
                  onPageSizeChange: (pageSize) => {
                    setInvitationPageSize(pageSize);
                    setInvitationPage(1);
                  },
                }}
                empty={<Empty description={t('暂无邀请记录')} />}
                size='small'
                scroll={{ x: 1230 }}
                className='w-full'
              />
            </Space>
          </Card>
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('返佣记录')} itemKey='records'>
          <Card>
            <Space vertical align='start' style={{ width: '100%' }}>
              <SectionHeader
                title={t('返佣记录')}
                description={t('核查产生返佣的下级订单')}
              />
              <Space wrap>
                <Select
                  value={recordSourceType}
                  onChange={(value) => {
                    setRecordPage(1);
                    setRecordSourceType(value);
                  }}
                  style={{ width: 170 }}
                >
                  <Select.Option value=''>{t('全部来源')}</Select.Option>
                  <Select.Option value='topup'>{t('余额充值')}</Select.Option>
                  <Select.Option value='subscription'>
                    {t('订阅购买')}
                  </Select.Option>
                  <Select.Option value='redemption'>
                    {t('兑换码兑换')}
                  </Select.Option>
                </Select>
                <Select
                  value={recordStatus}
                  onChange={(value) => {
                    setRecordPage(1);
                    setRecordStatus(value);
                  }}
                  style={{ width: 140 }}
                >
                  <Select.Option value=''>{t('全部状态')}</Select.Option>
                  <Select.Option value='pending'>{t('待结算')}</Select.Option>
                  <Select.Option value='available'>{t('已结算')}</Select.Option>
                </Select>
                <Button theme='outline' onClick={loadRecords}>
                  {t('刷新')}
                </Button>
              </Space>
              <Table
                rowKey='id'
                loading={recordsLoading}
                columns={recordColumns}
                dataSource={records}
                pagination={{
                  currentPage: recordPage,
                  pageSize: recordPageSize,
                  total: recordTotal,
                  pageSizeOpts: [10, 20, 50, 100],
                  showSizeChanger: true,
                  onPageChange: setRecordPage,
                  onPageSizeChange: (pageSize) => {
                    setRecordPageSize(pageSize);
                    setRecordPage(1);
                  },
                }}
                empty={<Empty description={t('暂无返佣记录')} />}
                size='small'
                scroll={{ x: 1390 }}
                className='w-full'
              />
            </Space>
          </Card>
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('提现审核')} itemKey='withdrawals'>
          <Card
            title={
              <div className='flex items-center justify-between gap-3'>
                <span>{t('返佣提现审核')}</span>
                <Select
                  value={withdrawalStatus}
                  onChange={(value) => {
                    setWithdrawalPage(1);
                    setWithdrawalStatus(value);
                  }}
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
              pagination={{
                currentPage: withdrawalPage,
                pageSize: withdrawalPageSize,
                total: withdrawalTotal,
                pageSizeOpts: [10, 20, 50, 100],
                showSizeChanger: true,
                onPageChange: setWithdrawalPage,
                onPageSizeChange: (pageSize) => {
                  setWithdrawalPageSize(pageSize);
                  setWithdrawalPage(1);
                },
              }}
              empty={<Empty description={t('暂无提现申请')} />}
              size='small'
              scroll={{ x: 1170 }}
            />
          </Card>
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('反欺诈设置')} itemKey='anti-fraud'>
          <Card>
            <Form
              values={inputs}
              getFormApi={(api) => (antifraudFormApiRef.current = api)}
              style={{ marginBottom: 16 }}
            >
              <div className='space-y-4'>
                <SectionHeader
                  title={t('申请审核')}
                  description={t('开启后用户需申请才能参与返佣')}
                />
                <SwitchRow title={t('开启返佣申请审核')}>
                  <Form.Switch
                    field='affiliate_setting.review_enabled'
                    noLabel
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={handleFieldChange(
                      'affiliate_setting.review_enabled',
                    )}
                  />
                </SwitchRow>
                <Form.InputNumber
                  field='affiliate_setting.auto_approve_after_days'
                  label={t('自动通过天数（0=仅手动）')}
                  min={0}
                  precision={0}
                  style={COMPACT_INPUT_STYLE}
                  onChange={handleFieldChange(
                    'affiliate_setting.auto_approve_after_days',
                  )}
                />

                <SectionHeader
                  title={t('协议设置')}
                  description={t('用户申请前需确认协议内容')}
                />
                <SwitchRow title={t('启用协议')}>
                  <Form.Switch
                    field='affiliate_setting.agreement_enabled'
                    noLabel
                    checkedText='｜'
                    uncheckedText='〇'
                    onChange={handleFieldChange(
                      'affiliate_setting.agreement_enabled',
                    )}
                  />
                </SwitchRow>
                <Form.TextArea
                  field='affiliate_setting.agreement_text'
                  label={t('协议内容')}
                  autosize={{ minRows: 4, maxRows: 10 }}
                  style={WIDE_INPUT_STYLE}
                  onChange={handleFieldChange(
                    'affiliate_setting.agreement_text',
                  )}
                />

                <SectionHeader
                  title={t('资格门槛')}
                  description={t('设置邀请人和被邀请人的最低要求')}
                />
                <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
                  <Form.InputNumber
                    field='affiliate_setting.inviter_min_account_age_days'
                    label={t('邀请人最少注册天数')}
                    min={0}
                    precision={0}
                    style={COMPACT_INPUT_STYLE}
                    onChange={handleFieldChange(
                      'affiliate_setting.inviter_min_account_age_days',
                    )}
                  />
                  <Form.InputNumber
                    field='affiliate_setting.inviter_min_recharge_amount'
                    label={t('邀请人最少充值额度')}
                    min={0}
                    style={COMPACT_INPUT_STYLE}
                    onChange={handleFieldChange(
                      'affiliate_setting.inviter_min_recharge_amount',
                    )}
                  />
                  <Form.InputNumber
                    field='affiliate_setting.invitee_min_account_age_days'
                    label={t('被邀请人最少注册天数')}
                    min={0}
                    precision={0}
                    style={COMPACT_INPUT_STYLE}
                    onChange={handleFieldChange(
                      'affiliate_setting.invitee_min_account_age_days',
                    )}
                  />
                  <Form.InputNumber
                    field='affiliate_setting.invitee_min_recharge_amount'
                    label={t('被邀请人最少充值额度')}
                    min={0}
                    style={COMPACT_INPUT_STYLE}
                    onChange={handleFieldChange(
                      'affiliate_setting.invitee_min_recharge_amount',
                    )}
                  />
                </div>

                <div className='flex justify-end'>
                  <Button
                    type='primary'
                    loading={saving}
                    onClick={saveSettings}
                  >
                    {t('保存设置')}
                  </Button>
                </div>
              </div>
            </Form>
          </Card>
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('申请审核')} itemKey='applications'>
          <ApplicationsPanel />
        </Tabs.TabPane>

        <Tabs.TabPane tab={t('异常检测')} itemKey='fraud'>
          <FraudAlertsPanel onQueryInviter={queryInviterInvitations} />
        </Tabs.TabPane>
      </Tabs>
    </Spin>
  );
}
